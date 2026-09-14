// Package policy is AgentPlane's identity-based authorization layer. It is
// deliberately separate from internal/guardrail: guardrails evaluate request
// and response *content*, while policies evaluate *who* is making a request
// and whether they are permitted to perform a given action.
//
// The package is intentionally generic. A Policy answers a single question —
// "is this principal allowed this permission?" — and an EnforcementPoint
// runs a chain of policies, denying if any policy denies. RBAC
// (internal/policy/rbac) is the first implementation; rate limits, quotas,
// and attribute-based rules can be added as additional Policy values without
// changing callers.
package policy

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Decision is the outcome of evaluating a policy.
type Decision int

const (
	// DecisionAllow means the request may proceed.
	DecisionAllow Decision = iota
	// DecisionDeny means the request must be rejected.
	DecisionDeny
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	default:
		return "unknown"
	}
}

// Request describes the access being attempted. Permission is an opaque
// string of the form "<resource>:<action>" (e.g. "chat:invoke") or
// "<resource>:<name>" (e.g. "model:gpt-4o").
type Request struct {
	// UserID is the authenticated principal.
	UserID uuid.UUID

	// Permission is the capability being requested.
	Permission string
}

// Policy decides whether a Request is allowed. Evaluate must be safe for
// concurrent use. An error means the policy could not reach a decision; the
// EnforcementPoint treats that as a denial (fail-closed).
type Policy interface {
	Name() string
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

// Match reports whether a granted permission satisfies a requested one.
//
// Supported forms:
//
//	"*"            matches everything (superuser)
//	"chat:invoke"  matches only "chat:invoke" (exact)
//	"model:*"      matches "model:gpt-4o", "model:gpt-4o-mini", ...
//
// A trailing ":*" is a prefix wildcard: the granted permission matches any
// requested permission sharing the text before the "*".
func Match(granted, requested string) bool {
	if granted == "" || requested == "" {
		return false
	}
	if granted == "*" || granted == requested {
		return true
	}
	if strings.HasSuffix(granted, ":*") {
		prefix := strings.TrimSuffix(granted, "*")
		return strings.HasPrefix(requested, prefix)
	}
	return false
}

// MatchAny reports whether any of the granted permissions satisfies the
// requested permission.
func MatchAny(granted []string, requested string) bool {
	for _, g := range granted {
		if Match(g, requested) {
			return true
		}
	}
	return false
}
