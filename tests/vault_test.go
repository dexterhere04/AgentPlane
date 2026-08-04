package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

func TestVaultStore_GetSecret_KV1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Vault-Token") != "test-token" {
			t.Errorf("expected X-Vault-Token 'test-token', got %q", r.Header.Get("X-Vault-Token"))
		}
		if r.URL.Path != "/v1/secret/my-api-key" {
			t.Errorf("expected path /v1/secret/my-api-key, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"value": "kv1-secret-value"},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL,
		Token:     "test-token",
		MountPath: "secret",
		KVVersion: 1,
	}

	val, err := store.GetSecret("my-api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "kv1-secret-value" {
		t.Errorf("expected 'kv1-secret-value', got %q", val)
	}
}

func TestVaultStore_GetSecret_KV1_NumericValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"value": 42},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL,
		Token:     "t",
		MountPath: "kv",
		KVVersion: 1,
	}

	val, err := store.GetSecret("key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "42" {
		t.Errorf("expected '42', got %q", val)
	}
}

func TestVaultStore_GetSecret_KV2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mount/data/api-key" {
			t.Errorf("expected path /v1/mount/data/api-key, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{"value": "kv2-secret-value"},
			},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL,
		Token:     "t",
		MountPath: "mount",
		KVVersion: 2,
	}

	val, err := store.GetSecret("api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "kv2-secret-value" {
		t.Errorf("expected 'kv2-secret-value', got %q", val)
	}
}

func TestVaultStore_GetSecret_KV2_NumericValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{"value": 100},
			},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL, Token: "t", MountPath: "m", KVVersion: 2,
	}

	val, err := store.GetSecret("k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "100" {
		t.Errorf("expected '100', got %q", val)
	}
}

func TestVaultStore_GetSecret_KV2_MissingValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{"other": "not-value"},
			},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL, Token: "t", MountPath: "m", KVVersion: 2,
	}

	_, err := store.GetSecret("k")
	if err == nil {
		t.Fatal("expected error for missing value key, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestVaultStore_GetSecret_KV2_NilValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{"value": nil},
			},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL, Token: "t", MountPath: "m", KVVersion: 2,
	}

	_, err := store.GetSecret("k")
	if err == nil {
		t.Fatal("expected error for nil value, got nil")
	}
}

func TestVaultStore_GetSecret_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("permission denied"))
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL, Token: "t", MountPath: "m", KVVersion: 1,
	}

	_, err := store.GetSecret("k")
	if err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected '403' in error, got: %v", err)
	}
}

func TestVaultStore_GetSecret_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL, Token: "t", MountPath: "m", KVVersion: 1,
	}

	_, err := store.GetSecret("k")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestVaultStore_GetSecret_ServerUnreachable(t *testing.T) {
	store := secrets.VaultStore{
		Addr:      "http://127.0.0.1:1",
		Token:     "t",
		MountPath: "m",
		KVVersion: 1,
	}

	_, err := store.GetSecret("k")
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

func TestVaultStore_GetSecret_TrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"value": "good"},
		})
	}))
	defer server.Close()

	store := secrets.VaultStore{
		Addr:      server.URL + "/",
		Token:     "t",
		MountPath: "m",
		KVVersion: 1,
	}

	val, err := store.GetSecret("k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "good" {
		t.Errorf("expected 'good', got %q", val)
	}
}

func TestVaultAppRoleLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/auth/approle/login" {
			t.Errorf("expected path /v1/auth/approle/login, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		if body["role_id"] != "rid" {
			t.Errorf("expected role_id 'rid', got %q", body["role_id"])
		}
		if body["secret_id"] != "sid" {
			t.Errorf("expected secret_id 'sid', got %q", body["secret_id"])
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"auth": map[string]interface{}{
				"client_token": "test-client-token",
			},
		})
	}))
	defer server.Close()

	token, err := secrets.VaultAppRoleLogin(server.URL, "rid", "sid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-client-token" {
		t.Errorf("expected 'test-client-token', got %q", token)
	}
}

func TestVaultAppRoleLogin_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad credentials"))
	}))
	defer server.Close()

	_, err := secrets.VaultAppRoleLogin(server.URL, "rid", "sid")
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected '401' in error, got: %v", err)
	}
}

func TestVaultAppRoleLogin_MissingToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"auth": map[string]interface{}{
				"other": "no-client-token",
			},
		})
	}))
	defer server.Close()

	_, err := secrets.VaultAppRoleLogin(server.URL, "rid", "sid")
	if err == nil {
		t.Fatal("expected error for missing client_token, got nil")
	}
	if !strings.Contains(err.Error(), "no client_token") {
		t.Errorf("expected 'no client_token' in error, got: %v", err)
	}
}

func TestVaultAppRoleLogin_NilAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"auth": nil,
		})
	}))
	defer server.Close()

	_, err := secrets.VaultAppRoleLogin(server.URL, "rid", "sid")
	if err == nil {
		t.Fatal("expected error for nil auth, got nil")
	}
}

func TestVaultAppRoleLogin_Unreachable(t *testing.T) {
	_, err := secrets.VaultAppRoleLogin("http://127.0.0.1:1", "rid", "sid")
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}
