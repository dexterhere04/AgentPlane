package secrets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type VaultStore struct {
	Addr      string
	Token     string
	MountPath string
	KVVersion int
}

type vaultResponse struct {
	Data json.RawMessage `json:"data"`
	Auth *vaultAuth      `json:"auth"`
}

type vaultAuth struct {
	ClientToken string `json:"client_token"`
}

func (v VaultStore) GetSecret(key string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	base := strings.TrimRight(v.Addr, "/")
	var url string
	if v.KVVersion == 1 {
		url = fmt.Sprintf("%s/v1/%s/%s", base, v.MountPath, key)
	} else {
		url = fmt.Sprintf("%s/v1/%s/data/%s", base, v.MountPath, key)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("vault: building request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.Token)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vault: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault: HTTP %d: %s", resp.StatusCode, string(body))
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
