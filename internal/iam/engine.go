package iam

import (
	"context"

	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/google/uuid"
)

// PolicyLoader resolves the policy documents that apply to a principal. It is
// satisfied by *Store, and exists so the engine can be tested without a
// database.
type PolicyLoader interface {
	// LoadPrincipalPolicies returns every policy document that applies to the
	// user: directly attached policies, policies attached to the user's
	// groups, and policies attached to roles the user has been granted.
	LoadPrincipalPolicies(ctx context.Context, userID uuid.UUID) ([]*PolicyDocument, error)

	// LoadPermissionsBoundary returns the user's permission boundary, or
	// (nil, nil) when none is set.
	LoadPermissionsBoundary(ctx context.Context, userID uuid.UUID) (*PolicyDocument, error)
}

// Engine is the IAM implementation of policy.Policy. It loads a user's
// effective policies and applies the deny-wins evaluation algorithm, subject to
// the user's permission boundary.
type Engine struct {
	loader PolicyLoader
}

// New builds an IAM engine backed by the given policy loader.
func New(loader PolicyLoader) *Engine {
	return &Engine{loader: loader}
}

// Name implements policy.Policy.
func (e *Engine) Name() string { return "iam" }

// Evaluate implements policy.Policy. A nil user or empty action denies; a
// loader error is surfaced so the caller fails closed (503) rather than
// mistaking an outage for a policy decision.
func (e *Engine) Evaluate(ctx context.Context, req policy.Request) (policy.Decision, error) {
	if req.UserID == uuid.Nil || req.Action == "" {
		return policy.DecisionDeny, nil
	}

	policies, err := e.loader.LoadPrincipalPolicies(ctx, req.UserID)
	if err != nil {
		return policy.DecisionDeny, err
	}

	boundary, err := e.loader.LoadPermissionsBoundary(ctx, req.UserID)
	if err != nil {
		return policy.DecisionDeny, err
	}

	decision := EvaluateWithBoundary(policies, boundary, Request{
		Action:   req.Action,
		Resource: req.Resource,
		Context:  req.Context,
	})

	if decision == DecisionAllow {
		return policy.DecisionAllow, nil
	}
	return policy.DecisionDeny, nil
}

// Authorize is a convenience for callers that hold a user ID and an
// action/resource pair directly, bypassing the generic policy.Request. It
// returns allow/deny and any loader error.
func (e *Engine) Authorize(ctx context.Context, userID uuid.UUID, action, resource string) (Decision, error) {
	if userID == uuid.Nil || action == "" {
		return DecisionDeny, nil
	}

	policies, err := e.loader.LoadPrincipalPolicies(ctx, userID)
	if err != nil {
		return DecisionDeny, err
	}

	boundary, err := e.loader.LoadPermissionsBoundary(ctx, userID)
	if err != nil {
		return DecisionDeny, err
	}

	return EvaluateWithBoundary(policies, boundary, Request{Action: action, Resource: resource}), nil
}
