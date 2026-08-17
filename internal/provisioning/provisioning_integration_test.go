//go:build integration

package provisioning

import (
	"context"
	"fmt"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
)

// TestProvisionUserWithAPIKey_Integration runs against a real PostgreSQL
// database (DATABASE_URL) using the real pgx/v5 driver, and requires
// API_KEY_PEPPER to be set (same as any real call to api.GenerateAPIKey).
//
// Run with:
//
//	DATABASE_URL=postgres://... API_KEY_PEPPER=... go test -tags=integration ./internal/provisioning/...
//
// It verifies:
//   - CreateUser + GenerateAPIKey + CreateAPIKey all run and succeed
//   - the persisted api_keys row's user_id matches the created user's ID
//   - secret_hash is present, is a 64-char hex SHA-256 digest, and is NOT
//     the plaintext key or secret (i.e. nothing plaintext was stored)
//   - the row can be independently re-queried directly from the database,
//     not just read back through the Go types
func TestProvisionUserWithAPIKey_Integration(t *testing.T) {
	ctx := context.Background()

	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	pepper, err := config.KeyPepper()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	usersStore := users.NewStore(pool)
	apiKeyStore := api.NewStore(pool)
	provisioner := NewProvisioner(usersStore, apiKeyStore, pepper)

	// Unique per run so repeated test runs don't collide on the
	// users.username / users.email UNIQUE constraints.
	suffix := uuid.NewString()
	username := fmt.Sprintf("itest_provisioning_%s", suffix)
	email := fmt.Sprintf("itest_provisioning_%s@example.com", suffix)

	result, err := provisioner.ProvisionUserWithAPIKey(ctx, users.CreateUserParams{
		Username: username,
		Email:    &email,
	}, "integration test key")
	if err != nil {
		t.Fatalf("ProvisionUserWithAPIKey failed: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup so reruns don't accumulate test data.
		// api_keys first, since it has a FK to users.
		if result.APIKeyRecord != nil {
			_, _ = pool.Exec(ctx, `DELETE FROM api_keys WHERE key_id = $1`, result.APIKeyRecord.KeyID)
		}
		if result.User != nil {
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, result.User.ID)
		}
	})

	if result.User == nil {
		t.Fatal("expected a non-nil User")
	}
	if result.APIKeyRecord == nil {
		t.Fatal("expected a non-nil APIKeyRecord")
	}
	if result.FullKey == "" {
		t.Fatal("expected a non-empty FullKey")
	}

	// The mapping: does the stored key actually belong to the user we just created?
	if result.APIKeyRecord.UserID != result.User.ID {
		t.Errorf("APIKeyRecord.UserID (%s) does not match created User.ID (%s)", result.APIKeyRecord.UserID, result.User.ID)
	}

	// Re-query api_keys directly (bypassing the Go layer we're testing) to
	// confirm what's actually sitting in the database.
	var storedUserID uuid.UUID
	var storedKeyID, storedSecretHash string
	err = pool.QueryRow(ctx,
		`SELECT user_id, key_id, secret_hash FROM api_keys WHERE key_id = $1`,
		result.APIKeyRecord.KeyID,
	).Scan(&storedUserID, &storedKeyID, &storedSecretHash)
	if err != nil {
		t.Fatalf("failed to query the stored api_keys row: %v", err)
	}

	if storedUserID != result.User.ID {
		t.Errorf("stored api_keys.user_id (%s) does not match created user (%s)", storedUserID, result.User.ID)
	}
	if storedKeyID != result.APIKeyRecord.KeyID {
		t.Errorf("stored api_keys.key_id (%s) does not match returned KeyID (%s)", storedKeyID, result.APIKeyRecord.KeyID)
	}

	// Verify only a hash was stored, never plaintext.
	if storedSecretHash == "" {
		t.Fatal("secret_hash was not stored")
	}
	if len(storedSecretHash) != 64 {
		t.Errorf("expected secret_hash to be a 64-char hex SHA-256 digest, got %d chars: %q", len(storedSecretHash), storedSecretHash)
	}
	if storedSecretHash == result.FullKey {
		t.Error("stored secret_hash equals the plaintext FullKey — plaintext was stored")
	}

	// Also confirm the row's user actually exists with the expected
	// username/email, i.e. the user side of the flow really persisted too.
	var storedUsername string
	var storedEmail *string
	err = pool.QueryRow(ctx,
		`SELECT username, email FROM users WHERE id = $1`,
		result.User.ID,
	).Scan(&storedUsername, &storedEmail)
	if err != nil {
		t.Fatalf("failed to query the stored users row: %v", err)
	}
	if storedUsername != username {
		t.Errorf("stored users.username (%q) does not match requested (%q)", storedUsername, username)
	}
	if storedEmail == nil || *storedEmail != email {
		t.Errorf("stored users.email does not match requested (%q)", email)
	}
}
