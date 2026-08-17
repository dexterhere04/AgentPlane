//go:build integration

package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthenticate_UpdatesLastUsedAt_Integration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	pepper := os.Getenv("API_KEY_PEPPER")
	if pepper == "" {
		t.Skip("API_KEY_PEPPER is not set")
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
	`, userID, "last-used-test-user", "last-used-test@example.com", "active")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM api_keys WHERE user_id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	}()

	apiStore := api.NewStore(pool)
	userStore := users.NewStore(pool)

	generated, err := api.GenerateAPIKey(pepper)
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}

	_, err = apiStore.CreateAPIKey(ctx, api.CreateAPIKeyParams{
		UserID:     userID,
		Name:       "last-used integration test",
		KeyID:      generated.KeyID,
		SecretHash: generated.SecretHash,
		Status:     api.StatusActive,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	var before *time.Time

	err = pool.QueryRow(ctx, `
		SELECT last_used_at
		FROM api_keys
		WHERE key_id = $1
	`, generated.KeyID).Scan(&before)
	if err != nil {
		t.Fatalf("failed to read initial last_used_at: %v", err)
	}

	if before != nil {
		t.Fatalf("expected last_used_at to be NULL initially, got %v", before)
	}

	authStore := NewStore(pool)
	authenticator := NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	user, err := authenticator.Authenticate(ctx, generated.FullKey)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if user.ID != userID {
		t.Fatalf("expected authenticated user %s, got %s", userID, user.ID)
	}

	var firstUsedAt *time.Time

	err = pool.QueryRow(ctx, `
		SELECT last_used_at
		FROM api_keys
		WHERE key_id = $1
	`, generated.KeyID).Scan(&firstUsedAt)
	if err != nil {
		t.Fatalf("failed to read updated last_used_at: %v", err)
	}

	if firstUsedAt == nil {
		t.Fatal("expected last_used_at to be set after successful authentication")
	}

	// Ensure enough time passes that the second authentication produces
	// a distinguishable timestamp.
	time.Sleep(10 * time.Millisecond)

	if _, err := authenticator.Authenticate(ctx, generated.FullKey); err != nil {
		t.Fatalf("second Authenticate failed: %v", err)
	}

	var secondUsedAt *time.Time

	err = pool.QueryRow(ctx, `
		SELECT last_used_at
		FROM api_keys
		WHERE key_id = $1
	`, generated.KeyID).Scan(&secondUsedAt)
	if err != nil {
		t.Fatalf("failed to read second last_used_at: %v", err)
	}

	if secondUsedAt == nil {
		t.Fatal("expected last_used_at after second authentication")
	}

	if !secondUsedAt.After(*firstUsedAt) {
		t.Fatalf(
			"expected last_used_at to advance: first=%v second=%v",
			*firstUsedAt,
			*secondUsedAt,
		)
	}
}
