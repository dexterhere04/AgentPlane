package policy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
)

func testUser() *users.User {
	return &users.User{ID: uuid.New(), Username: "alice", Status: users.StatusActive}
}

func TestRequire_NoUserInContext(t *testing.T) {
	ep := NewEnforcementPoint(fakePolicy{name: "a", decision: DecisionAllow})

	called := false
	handler := ep.Require("chat:invoke")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/chat", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler should not be called")
	}
}

func TestRequire_Allow(t *testing.T) {
	ep := NewEnforcementPoint(fakePolicy{name: "a", decision: DecisionAllow})

	called := false
	handler := ep.Require("chat:invoke")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/chat", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), testUser()))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Fatal("next handler should be called")
	}
}

func TestRequire_DenyReturns403(t *testing.T) {
	ep := NewEnforcementPoint(fakePolicy{name: "a", decision: DecisionDeny})

	called := false
	handler := ep.Require("chat:invoke")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/chat", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), testUser()))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler should not be called")
	}
}

func TestRequire_ErrorReturns503(t *testing.T) {
	ep := NewEnforcementPoint(fakePolicy{name: "a", err: errors.New("store down")})

	called := false
	handler := ep.Require("chat:invoke")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/chat", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), testUser()))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler should not be called")
	}
}
