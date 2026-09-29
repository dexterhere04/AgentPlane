package policy

import (
	"context"
	"errors"
	"testing"
)

type fakePolicy struct {
	name     string
	decision Decision
	err      error
}

func (f fakePolicy) Name() string { return f.name }

func (f fakePolicy) Evaluate(context.Context, Request) (Decision, error) {
	return f.decision, f.err
}

func TestEnforcementPoint_AllAllow(t *testing.T) {
	ep := NewEnforcementPoint(
		fakePolicy{name: "a", decision: DecisionAllow},
		fakePolicy{name: "b", decision: DecisionAllow},
	)

	decision, err := ep.Authorize(context.Background(), Request{Permission: "chat:invoke"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != DecisionAllow {
		t.Fatalf("expected allow, got %s", decision)
	}
}

func TestEnforcementPoint_DenyWins(t *testing.T) {
	ep := NewEnforcementPoint(
		fakePolicy{name: "a", decision: DecisionAllow},
		fakePolicy{name: "b", decision: DecisionDeny},
		fakePolicy{name: "c", decision: DecisionAllow},
	)

	decision, err := ep.Authorize(context.Background(), Request{Permission: "chat:invoke"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != DecisionDeny {
		t.Fatalf("expected deny, got %s", decision)
	}
}

func TestEnforcementPoint_ErrorFailsClosed(t *testing.T) {
	sentinel := errors.New("store down")
	ep := NewEnforcementPoint(
		fakePolicy{name: "a", decision: DecisionAllow},
		fakePolicy{name: "b", err: sentinel},
	)

	decision, err := ep.Authorize(context.Background(), Request{Permission: "chat:invoke"})
	if decision != DecisionDeny {
		t.Fatalf("expected deny on error, got %s", decision)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestEnforcementPoint_EmptyChainAllows(t *testing.T) {
	ep := NewEnforcementPoint()

	decision, err := ep.Authorize(context.Background(), Request{Permission: "chat:invoke"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != DecisionAllow {
		t.Fatalf("expected allow for empty chain, got %s", decision)
	}
}
