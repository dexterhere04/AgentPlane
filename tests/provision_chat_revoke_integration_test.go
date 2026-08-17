//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/users"
)

func TestProvisionChatRevokeLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	pepper := os.Getenv("API_KEY_PEPPER")
	if pepper == "" {
		t.Skip("API_KEY_PEPPER is not set")
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	defer pool.Close()

	userStore := users.NewStore(pool)
	apiStore := api.NewStore(pool)

	provisioner := provisioning.NewProvisioner(
		userStore,
		apiStore,
		pepper,
	)

	authStore := auth.NewStore(pool)

	authenticator := auth.NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	// Generate unique test data so repeated test runs do not collide.
	runID := time.Now().UnixNano()

	username := fmt.Sprintf("http-integration-test-user-%d", runID)
	email := fmt.Sprintf("http-integration-test-%d@example.com", runID)
	keyName := fmt.Sprintf("http-integration-test-key-%d", runID)

	// 1. Provision a real user + API key.
	result, err := provisioner.ProvisionUserWithAPIKey(
		ctx,
		users.CreateUserParams{
			Username: username,
			Email:    stringPtr(email),
		},
		keyName,
	)
	if err != nil {
		t.Fatalf("provisioning failed: %v", err)
	}

	if result.User == nil {
		t.Fatal("expected provisioned user")
	}

	if result.APIKeyRecord == nil {
		t.Fatal("expected API key record")
	}

	if result.FullKey == "" {
		t.Fatal("expected plaintext API key")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			ctx,
			`DELETE FROM api_keys WHERE id = $1`,
			result.APIKeyRecord.ID,
		)

		_, _ = pool.Exec(
			ctx,
			`DELETE FROM users WHERE id = $1`,
			result.User.ID,
		)
	})

	// 2. Build the protected /chat handler.
	protectedChat := authenticator.Middleware(
		http.HandlerFunc(handlers.Chat),
	)

	// We deliberately use a fake downstream provider here. The purpose of
	// this test is authentication/revocation, not OpenAI connectivity.
	server := httptest.NewServer(protectedChat)
	defer server.Close()

	// 3. Valid key must pass authentication.
	req, err := http.NewRequest(
		http.MethodPost,
		server.URL,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+result.FullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request with valid API key failed: %v", err)
	}
	resp.Body.Close()

	// The request may reach the provider and fail with 502 because this test
	// does not configure OpenAI. What matters here is that authentication
	// did NOT return 401.
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("valid API key was rejected with 401")
	}

	// 4. Revoke the key.
	if err := apiStore.RevokeAPIKey(ctx, result.APIKeyRecord.KeyID); err != nil {
		t.Fatalf("failed to revoke API key: %v", err)
	}

	// 5. The exact same key must now be rejected.
	req, err = http.NewRequest(
		http.MethodPost,
		server.URL,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to create revoked-key request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+result.FullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("request with revoked API key failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf(
			"expected revoked API key to return 401, got %d",
			resp.StatusCode,
		)
	}
}

func stringPtr(s string) *string {
	return &s
}

// Keep json imported if your handlers or future assertions are expanded.
var _ = json.Valid
