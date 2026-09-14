package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/google/uuid"
)

type fakeLoader struct {
	permissions []string
	err         error
}

func (f fakeLoader) LoadPermissions(context.Context, uuid.UUID) ([]string, error) {
	return f.permissions, f.err
}

func TestEngine_AllowsMatchingPermission(t *testing.T) {
	engine := New(fakeLoader{permissions: []string{"chat:invoke", "model:gpt-4o"}})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		UserID:     uuid.New(),
		Permission: "model:gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != policy.DecisionAllow {
		t.Fatalf("expected allow, got %s", decision)
	}
}

func TestEngine_DeniesUnmatchedPermission(t *testing.T) {
	engine := New(fakeLoader{permissions: []string{"chat:invoke", "model:gpt-4o"}})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		UserID:     uuid.New(),
		Permission: "model:gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny, got %s", decision)
	}
}

func TestEngine_NoRolesDeniesEverything(t *testing.T) {
	engine := New(fakeLoader{permissions: nil})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		UserID:     uuid.New(),
		Permission: "chat:invoke",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny for user with no roles, got %s", decision)
	}
}

func TestEngine_WildcardAllows(t *testing.T) {
	engine := New(fakeLoader{permissions: []string{"*"}})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		UserID:     uuid.New(),
		Permission: "admin:roles:manage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != policy.DecisionAllow {
		t.Fatalf("expected allow, got %s", decision)
	}
}

func TestEngine_NilUserDenies(t *testing.T) {
	engine := New(fakeLoader{permissions: []string{"*"}})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		Permission: "chat:invoke",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny for nil user, got %s", decision)
	}
}

func TestEngine_LoaderErrorPropagates(t *testing.T) {
	sentinel := errors.New("db down")
	engine := New(fakeLoader{err: sentinel})

	decision, err := engine.Evaluate(context.Background(), policy.Request{
		UserID:     uuid.New(),
		Permission: "chat:invoke",
	})
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny on loader error, got %s", decision)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
