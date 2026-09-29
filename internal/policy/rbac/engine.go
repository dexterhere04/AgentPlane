package rbac

import (
	"context"

	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/google/uuid"
)

// PermissionLoader resolves the effective permission set for a user. It is
// satisfied by *Store and exists so the engine can be unit tested without a
// database.
type PermissionLoader interface {
	LoadPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// Engine is the RBAC implementation of policy.Policy. It grants a request
// when any of the user's effective permissions matches the requested
// permission (see policy.Match). A user with no roles has no permissions
// and is therefore denied everything.
type Engine struct {
	loader PermissionLoader
}

// New builds an RBAC engine backed by the given permission loader.
func New(loader PermissionLoader) *Engine {
	return &Engine{loader: loader}
}

// Name implements policy.Policy.
func (e *Engine) Name() string { return "rbac" }

// Evaluate implements policy.Policy.
func (e *Engine) Evaluate(ctx context.Context, req policy.Request) (policy.Decision, error) {
	if req.UserID == uuid.Nil || req.Permission == "" {
		return policy.DecisionDeny, nil
	}

	permissions, err := e.loader.LoadPermissions(ctx, req.UserID)
	if err != nil {
		return policy.DecisionDeny, err
	}

	if policy.MatchAny(permissions, req.Permission) {
		return policy.DecisionAllow, nil
	}
	return policy.DecisionDeny, nil
}
