package api

import (
	"context"
	"os"
	"strings"
	"testing"

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

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email, status)
		VALUES ($1, $2, $3, $4)
	`, userID, "integration-test-user", "integration-test@example.com", "active")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	store := NewStore(pool)

	plaintextSecret := "this-secret-must-never-be-stored"
	secretHash := "test-secret-hash"

	record, err := store.CreateAPIKey(ctx, CreateAPIKeyParams{
		UserID:     userID,
		Name:       "integration test key",
		KeyID:      "test-key-id",
		SecretHash: secretHash,
		Status:     StatusActive,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if record.KeyID != "test-key-id" {
		t.Errorf("expected key ID %q, got %q", "test-key-id", record.KeyID)
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
	`, "test-key-id").Scan(&storedKeyID, &storedHash)
	if err != nil {
		t.Fatalf("failed to read inserted API key: %v", err)
	}

	if storedKeyID != "test-key-id" {
		t.Errorf("expected stored key ID %q, got %q", "test-key-id", storedKeyID)
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

	// Clean up the test data.
	_, err = pool.Exec(ctx, `DELETE FROM api_keys WHERE key_id = $1`, "test-key-id")
	if err != nil {
		t.Errorf("failed to clean up API key: %v", err)
	}

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		t.Errorf("failed to clean up test user: %v", err)
	}
}