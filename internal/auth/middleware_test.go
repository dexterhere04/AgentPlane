package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
)

func TestUserFromContext(t *testing.T) {
	user := &users.User{
		ID:       uuid.New(),
		Username: "alice",
		Status:   users.StatusActive,
	}

	ctx := context.WithValue(
		context.Background(),
		authenticatedUserKey,
		user,
	)

	got, ok := UserFromContext(ctx)

	if !ok {
		t.Fatal("expected user to be present in context")
	}

	if got.ID != user.ID {
		t.Fatalf("expected user ID %s, got %s", user.ID, got.ID)
	}
}

func TestUserFromContext_Missing(t *testing.T) {
	_, ok := UserFromContext(context.Background())

	if ok {
		t.Fatal("expected no authenticated user in empty context")
	}
}

func TestMiddleware_MissingAuthorization(t *testing.T) {
	authenticator := &Authenticator{}

	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	if nextCalled {
		t.Fatal("next handler should not have been called")
	}
}

func TestMiddleware_InvalidAuthorizationScheme(t *testing.T) {
	authenticator := &Authenticator{}

	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc123")

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	if nextCalled {
		t.Fatal("next handler should not have been called")
	}
}

func TestMiddleware_EmptyBearerToken(t *testing.T) {
	authenticator := &Authenticator{}

	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := authenticator.Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}

	if nextCalled {
		t.Fatal("next handler should not have been called")
	}
}
