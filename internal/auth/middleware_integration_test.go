//go:build integration

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMiddleware_ValidAPIKey_ResolvesCorrectUser(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	pepper := os.Getenv("API_KEY_PEPPER")

	if databaseURL == "" {
		t.Skip("DATABASE_URL not configured")
	}

	if pepper == "" {
		t.Skip("API_KEY_PEPPER not configured")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}

	userStore := users.NewStore(pool)
	apiStore := api.NewStore(pool)
	authStore := NewStore(pool)

	username := "auth_test_" + uuid.NewString()
	email := username + "@example.com"

	user, err := userStore.CreateUser(ctx, users.CreateUserParams{
		Username: username,
		Email:    &email,
	})
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM api_keys WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	generated, err := api.GenerateAPIKey(pepper)
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}

	_, err = apiStore.CreateAPIKey(ctx, api.CreateAPIKeyParams{
		UserID:     user.ID,
		Name:       "middleware integration test",
		KeyID:      generated.KeyID,
		SecretHash: generated.SecretHash,
		Status:     api.StatusActive,
	})
	if err != nil {
		t.Fatalf("failed to store API key: %v", err)
	}

	authenticator := NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticatedUser, ok := UserFromContext(r.Context())

		if !ok {
			t.Fatal("authenticated user not found in request context")
		}

		if authenticatedUser.ID != user.ID {
			t.Fatalf(
				"wrong user in context: expected %s, got %s",
				user.ID,
				authenticatedUser.ID,
			)
		}

		if authenticatedUser.Username != user.Username {
			t.Fatalf(
				"wrong username in context: expected %s, got %s",
				user.Username,
				authenticatedUser.Username,
			)
		}

		w.WriteHeader(http.StatusOK)
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+generated.FullKey)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_InvalidAPIKey_Returns401(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	pepper := os.Getenv("API_KEY_PEPPER")

	if databaseURL == "" {
		t.Skip("DATABASE_URL not configured")
	}

	if pepper == "" {
		t.Skip("API_KEY_PEPPER not configured")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	authStore := NewStore(pool)
	userStore := users.NewStore(pool)

	authenticator := NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)

	// Correct format, but completely invalid key.
	req.Header.Set(
		"Authorization",
		"Bearer ap_live_invalid_invalid",
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_RevokedAPIKey_Returns401(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	pepper := os.Getenv("API_KEY_PEPPER")

	if databaseURL == "" {
		t.Skip("DATABASE_URL not configured")
	}

	if pepper == "" {
		t.Skip("API_KEY_PEPPER not configured")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}

	userStore := users.NewStore(pool)
	apiStore := api.NewStore(pool)
	authStore := NewStore(pool)

	username := "auth_revoke_" + uuid.NewString()
	email := username + "@example.com"

	user, err := userStore.CreateUser(ctx, users.CreateUserParams{
		Username: username,
		Email:    &email,
	})
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM api_keys WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", user.ID)
	})

	generated, err := api.GenerateAPIKey(pepper)
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}

	_, err = apiStore.CreateAPIKey(ctx, api.CreateAPIKeyParams{
		UserID:     user.ID,
		Name:       "revocation integration test",
		KeyID:      generated.KeyID,
		SecretHash: generated.SecretHash,
		Status:     api.StatusActive,
	})
	if err != nil {
		t.Fatalf("failed to store API key: %v", err)
	}

	_, err = pool.Exec(
		ctx,
		"UPDATE api_keys SET revoked_at = $1 WHERE key_id = $2",
		time.Now(),
		generated.KeyID,
	)
	if err != nil {
		t.Fatalf("failed to revoke API key: %v", err)
	}

	authenticator := NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for revoked key")
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+generated.FullKey)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}
