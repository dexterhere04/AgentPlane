package secrets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// VaultStore talks to a HashiCorp Vault KV mount. It authenticates either with
// a static token (VAULT_TOKEN) or via AppRole (VAULT_ROLE_ID/VAULT_SECRET_ID).
//
// AppRole-issued tokens are short-lived (Vault defaults to a 1h TTL), so the
// store re-authenticates lazily whenever the cached token is missing or Vault
// rejects it with 401/403. Without this, a long-running gateway starts failing
// every read/write once its boot-time token expires.
type VaultStore struct {
	Addr      string
	Token     string
	MountPath string
	KVVersion int

	// AppRole credentials used to (re)issue tokens. Optional when Token is a
	// long-lived token that never expires.
	RoleID   string
	SecretID string

	HTTPClient *http.Client

	mu sync.Mutex
}

type vaultResponse struct {
	Data json.RawMessage `json:"data"`
	Auth *vaultAuth      `json:"auth"`
}

type vaultAuth struct {
	ClientToken string `json:"client_token"`
}

func (v *VaultStore) client() *http.Client {
	if v.HTTPClient != nil {
		return v.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (v *VaultStore) hasAppRole() bool {
	return v.RoleID != "" && v.SecretID != ""
}

// relogin exchanges the AppRole credentials for a fresh client token and caches
// it on the store.
func (v *VaultStore) relogin() error {
	if !v.hasAppRole() {
		return fmt.Errorf("vault: token missing or expired and no AppRole credentials (VAULT_ROLE_ID/VAULT_SECRET_ID) configured")
	}
	token, err := VaultAppRoleLogin(v.Addr, v.RoleID, v.SecretID)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.Token = token
	v.mu.Unlock()
	return nil
}

func (v *VaultStore) currentToken() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.Token
}

// send performs a Vault request. If the cached token is missing it logs in via
// AppRole first, and if Vault rejects the request with 401/403 it forces a
// re-authentication and retries exactly once. The returned body is always read
// and the response closed, so callers only inspect the status and body.
func (v *VaultStore) send(method, url string, payload []byte) (int, []byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token := v.currentToken()
		if token == "" && v.hasAppRole() {
			if err := v.relogin(); err != nil {
				return 0, nil, err
			}
			token = v.currentToken()
		}

		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return 0, nil, fmt.Errorf("vault: building request: %w", err)
		}
		req.Header.Set("X-Vault-Token", token)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := v.client().Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("vault: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return 0, nil, fmt.Errorf("vault: reading response: %w", readErr)
		}

		// A 401/403 usually means the token expired. Drop it, log in again and
		// retry once. Only do this when AppRole credentials are available so a
		// genuine permission error with a static token is surfaced as-is.
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
			attempt == 0 && v.hasAppRole() {
			v.mu.Lock()
			v.Token = ""
			v.mu.Unlock()
			if err := v.relogin(); err == nil {
				continue
			}
		}

		return resp.StatusCode, respBody, nil
	}
	return 0, nil, fmt.Errorf("vault: request failed after re-authentication")
}

func (v *VaultStore) GetSecret(key string) (string, error) {
	base := strings.TrimRight(v.Addr, "/")
	var url string
	if v.KVVersion == 1 {
		url = fmt.Sprintf("%s/v1/%s/%s", base, v.MountPath, key)
	} else {
		url = fmt.Sprintf("%s/v1/%s/data/%s", base, v.MountPath, key)
	}

	status, body, err := v.send(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("vault: HTTP %d: %s", status, string(body))
	}

	var vr vaultResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return "", fmt.Errorf("vault: parsing response: %w", err)
	}

	if v.KVVersion == 1 {
		return extractKV1Data(vr.Data)
	}
	return extractKV2Data(vr.Data)
}

// SetSecret writes a secret to the Vault KV mount under the given key.
// The value is stored under the "value" field to mirror the layout used by
// the docker-compose vault-init seeding (vault kv put agentplane/KEY value=...).
func (v *VaultStore) SetSecret(key, value string) error {
	base := strings.TrimRight(v.Addr, "/")
	var url string
	var payload []byte
	if v.KVVersion == 1 {
		url = fmt.Sprintf("%s/v1/%s/%s", base, v.MountPath, key)
		payload, _ = json.Marshal(map[string]string{"value": value})
	} else {
		url = fmt.Sprintf("%s/v1/%s/data/%s", base, v.MountPath, key)
		payload, _ = json.Marshal(map[string]any{
			"data": map[string]string{"value": value},
		})
	}

	status, body, err := v.send(http.MethodPost, url, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("vault: HTTP %d: %s", status, string(body))
	}
	return nil
}

func extractKV1Data(data json.RawMessage) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("vault: parsing kv1 data: %w", err)
	}
	val, ok := m["value"]
	if !ok || val == nil {
		return "", fmt.Errorf("vault: key 'value' not found in secret data")
	}
	return fmt.Sprintf("%v", val), nil
}

func extractKV2Data(data json.RawMessage) (string, error) {
	var wrapper struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("vault: parsing kv2 data: %w", err)
	}
	val, ok := wrapper.Data["value"]
	if !ok || val == nil {
		return "", fmt.Errorf("vault: key 'value' not found in secret data")
	}
	return fmt.Sprintf("%v", val), nil
}

func VaultAppRoleLogin(addr, roleID, secretID string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := strings.TrimRight(addr, "/") + "/v1/auth/approle/login"

	payload, err := json.Marshal(map[string]string{"role_id": roleID, "secret_id": secretID})
	if err != nil {
		return "", fmt.Errorf("vault approle: building request body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return "", fmt.Errorf("vault approle: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault approle: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vault approle: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault approle: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var vr vaultResponse
	if err := json.Unmarshal(respBody, &vr); err != nil {
		return "", fmt.Errorf("vault approle: parsing response: %w", err)
	}

	if vr.Auth == nil || vr.Auth.ClientToken == "" {
		return "", fmt.Errorf("vault approle: no client_token in response")
	}

	return vr.Auth.ClientToken, nil
}
