package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAPIKey_RealPostgres(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping PostgreSQL: %v", err)
	}

	userID := uuid.New()
	keyID := "integration-test-key-" + uuid.NewString()
	username := "integration-test-user-" + uuid.NewString()

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email, status)
		VALUES ($1, $2, $3, $4)
	`, userID, username, "integration-test@example.com", "active")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	store := NewStore(pool)

	plaintextSecret := "this-secret-must-never-be-stored"
	secretHash := "test-secret-hash"

	record, err := store.CreateAPIKey(ctx, CreateAPIKeyParams{
		UserID:     userID,
		Name:       "integration test key",
		KeyID:      keyID,
		SecretHash: secretHash,
		Status:     StatusActive,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if record.KeyID != keyID {
		t.Errorf("expected key ID %q, got %q", keyID, record.KeyID)
	}

	if record.UserID != userID {
		t.Errorf("expected user ID %q, got %q", userID, record.UserID)
	}

	if record.Status != StatusActive {
		t.Errorf("expected status %q, got %q", StatusActive, record.Status)
	}

	var storedHash string
	var storedKeyID string

	err = pool.QueryRow(ctx, `
		SELECT key_id, secret_hash
		FROM api_keys
		WHERE key_id = $1
	`, keyID).Scan(&storedKeyID, &storedHash)
	if err != nil {
		t.Fatalf("failed to read inserted API key: %v", err)
	}

	if storedKeyID != keyID {
		t.Errorf("expected stored key ID %q, got %q", keyID, storedKeyID)
	}

	if storedHash != secretHash {
		t.Errorf("expected stored secret hash %q, got %q", secretHash, storedHash)
	}

	if strings.Contains(storedHash, plaintextSecret) {
		t.Errorf("plaintext secret was stored in secret_hash")
	}

	var plaintextMatches int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM api_keys
		WHERE secret_hash = $1
	`, plaintextSecret).Scan(&plaintextMatches)
	if err != nil {
		t.Fatalf("failed to check for plaintext secret: %v", err)
	}

	if plaintextMatches != 0 {
		t.Fatalf("plaintext secret was stored in api_keys")
	}

	_, err = pool.Exec(ctx, `DELETE FROM api_keys WHERE key_id = $1`, keyID)
	if err != nil {
		t.Errorf("failed to clean up API key: %v", err)
	}

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		t.Errorf("failed to clean up test user: %v", err)
	}
}

func TestRevokeAPIKey_RealPostgres(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to ping PostgreSQL: %v", err)
	}

	userID := uuid.New()
	keyID := "integration-revoke-key-" + uuid.NewString()
	username := "integration-revoke-user-" + uuid.NewString()

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email, status)
		VALUES ($1, $2, $3, $4)
	`, userID, username, "integration-revoke@example.com", "active")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM api_keys WHERE key_id = $1`, keyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	store := NewStore(pool)

	record, err := store.CreateAPIKey(ctx, CreateAPIKeyParams{
		UserID:     userID,
		Name:       "integration revoke test",
		KeyID:      keyID,
		SecretHash: "integration-revoke-secret-hash",
		Status:     StatusActive,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if record.RevokedAt != nil {
		t.Fatalf("new API key unexpectedly has revoked_at set")
	}

	err = store.RevokeAPIKey(ctx, keyID)
	if err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	var revokedAt *time.Time

	err = pool.QueryRow(ctx, `
		SELECT revoked_at
		FROM api_keys
		WHERE key_id = $1
	`, keyID).Scan(&revokedAt)
	if err != nil {
		t.Fatalf("failed to read revoked API key: %v", err)
	}

	if revokedAt == nil {
		t.Fatal("expected revoked_at to be set after revocation")
	}
}
