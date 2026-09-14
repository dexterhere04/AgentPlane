package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/policy/rbac"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
)

type stubPermissions struct {
	permissions []string
}

func (s stubPermissions) LoadPermissions(context.Context, uuid.UUID) ([]string, error) {
	return s.permissions, nil
}

func chatWithPolicy(t *testing.T, permissions []string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	engine := rbac.New(stubPermissions{permissions: permissions})
	policyEP := policy.NewEnforcementPoint(engine)

	cfg := guardrail.Config{Strategies: make(map[string]guardrail.Strategy)}
	registry := guardrail.NewRegistry()
	enforcement := guardrail.NewEnforcementPoint(registry, cfg, observability.NewEventBus(16))

	provider := &stubProvider{NameStr: "test", Response: []byte(`{"choices":[{"message":{"content":"ok"}}]}`)}

	user := &users.User{ID: uuid.New(), Username: "alice", Status: users.StatusActive}

	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUser(req.Context(), user))

	rec := httptest.NewRecorder()
	handlers.Chat(rec, req, enforcement, policyEP, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)
	return rec
}

func TestChatPolicy_AllowedModel(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)

	rec := chatWithPolicy(t, []string{"chat:invoke", "model:gpt-4o"}, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatPolicy_DeniedModel(t *testing.T) {
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)

	rec := chatWithPolicy(t, []string{"chat:invoke", "model:gpt-4o"}, body)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["error"]["type"] != "policy_denied" {
		t.Fatalf("expected policy_denied, got %v", resp["error"]["type"])
	}
	if resp["error"]["permission"] != "model:gpt-4o-mini" {
		t.Fatalf("expected denied permission model:gpt-4o-mini, got %v", resp["error"]["permission"])
	}
}

func TestChatPolicy_NoPermissionsDenies(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)

	rec := chatWithPolicy(t, nil, body)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user with no roles, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatPolicy_WildcardModelAllows(t *testing.T) {
	body := []byte(`{"model":"some-future-model","messages":[{"role":"user","content":"hi"}]}`)

	rec := chatWithPolicy(t, []string{"chat:invoke", "model:*"}, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
