package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

func setupAWSEnv() func() {
	prevAccessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	prevSecretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	prevSessionToken := os.Getenv("AWS_SESSION_TOKEN")
	prevRegion := os.Getenv("AWS_REGION")

	os.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	os.Setenv("AWS_SESSION_TOKEN", "")
	os.Setenv("AWS_REGION", "us-east-1")

	return func() {
		restoreEnv("AWS_ACCESS_KEY_ID", prevAccessKey)
		restoreEnv("AWS_SECRET_ACCESS_KEY", prevSecretKey)
		restoreEnv("AWS_SESSION_TOKEN", prevSessionToken)
		restoreEnv("AWS_REGION", prevRegion)
	}
}

func restoreEnv(key, prev string) {
	if prev == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, prev)
	}
}

func newAWSHandler(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != "secretsmanager.GetSecretValue" {
			t.Errorf("expected X-Amz-Target secretsmanager.GetSecretValue, got %q", r.Header.Get("X-Amz-Target"))
		}
		handler(w, r)
	}))
}

func TestAWSSecretsManagerStore_GetSecret_PlainText(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": "my-aws-secret",
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "my-secret-id",
	}

	val, err := store.GetSecret("ignored-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-aws-secret" {
		t.Errorf("expected 'my-aws-secret', got %q", val)
	}
}

func TestAWSSecretsManagerStore_GetSecret_JSONKey(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": `{"api_key":"extracted-key","other":"ignored"}`,
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "json-secret",
		JSONKey:     "api_key",
	}

	val, err := store.GetSecret("ignored-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "extracted-key" {
		t.Errorf("expected 'extracted-key', got %q", val)
	}
}

func TestAWSSecretsManagerStore_GetSecret_JSONKey_NonString(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": `{"numeric_key":42}`,
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "s",
		JSONKey:     "numeric_key",
	}

	_, err := store.GetSecret("ignored-key")
	if err == nil {
		t.Fatal("expected error for non-string JSON key value, got nil")
	}
	if !strings.Contains(err.Error(), "not a string") {
		t.Errorf("expected 'not a string' in error, got: %v", err)
	}
}

func TestAWSSecretsManagerStore_GetSecret_JSONKey_MissingKey(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": `{"other":"value"}`,
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "s",
		JSONKey:     "missing_key",
	}

	_, err := store.GetSecret("ignored-key")
	if err == nil {
		t.Fatal("expected error for missing JSON key, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestAWSSecretsManagerStore_GetSecret_JSONKey_InvalidJSON(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": "not-valid-json",
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "s",
		JSONKey:     "some_key",
	}

	_, err := store.GetSecret("ignored-key")
	if err == nil {
		t.Fatal("expected error for invalid JSON secret, got nil")
	}
	if !strings.Contains(err.Error(), "parsing secret as JSON") {
		t.Errorf("expected 'parsing secret as JSON' in error, got: %v", err)
	}
}

func TestAWSSecretsManagerStore_GetSecret_NilSecretString(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ARN": "arn:aws:secretsmanager:us-east-1:123456789012:secret:s-abc",
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "binary-secret",
	}

	_, err := store.GetSecret("ignored-key")
	if err == nil {
		t.Fatal("expected error for nil SecretString, got nil")
	}
	if !strings.Contains(err.Error(), "no SecretString") {
		t.Errorf("expected 'no SecretString' in error, got: %v", err)
	}
}

func TestAWSSecretsManagerStore_GetSecret_ResourceNotFound(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"__type":  "ResourceNotFoundException",
			"Message": "Secrets Manager can't find the specified secret.",
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "nonexistent",
	}

	_, err := store.GetSecret("ignored-key")
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}
}

func TestAWSSecretsManagerStore_GetSecret_EmptySecretString(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := newAWSHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": "",
		})
	})
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "empty-secret",
	}

	val, err := store.GetSecret("ignored-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}
}

func TestAWSSecretsManagerStore_GetSecret_RequestVerification(t *testing.T) {
	cleanup := setupAWSEnv()
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != "secretsmanager.GetSecretValue" {
			t.Errorf("unexpected X-Amz-Target: %q", r.Header.Get("X-Amz-Target"))
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)
		if req["SecretId"] != "verify-id" {
			t.Errorf("expected SecretId 'verify-id', got %v", req["SecretId"])
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"SecretString": "verified",
		})
	}))
	defer server.Close()

	store := secrets.AWSSecretsManagerStore{
		Region:      "us-east-1",
		EndpointURL: server.URL,
		SecretID:    "verify-id",
	}

	val, err := store.GetSecret("ignored-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "verified" {
		t.Errorf("expected 'verified', got %q", val)
	}
}
