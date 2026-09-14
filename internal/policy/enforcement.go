package policy

import (
	"context"
	"fmt"
)

// EnforcementPoint runs a chain of policies for a single authorization
// request. The first policy to deny wins; if every policy allows, the
// request is allowed. A policy error is treated as a denial (fail-closed)
// and surfaced to the caller so it can respond with 503 rather than 403.
type EnforcementPoint struct {
	policies []Policy
}

// NewEnforcementPoint builds an EnforcementPoint from the given policies,
// evaluated in order.
func NewEnforcementPoint(policies ...Policy) *EnforcementPoint {
	return &EnforcementPoint{policies: policies}
}

// Policies returns the configured policies (useful for diagnostics/tests).
func (ep *EnforcementPoint) Policies() []Policy {
	return ep.policies
}

// Authorize evaluates every policy against req.
//
// It returns (DecisionAllow, nil) only when all policies allow. It returns
// (DecisionDeny, nil) when a policy explicitly denies, and
// (DecisionDeny, err) when a policy could not reach a decision — callers
// should distinguish the two (403 vs 503) so an outage is never silently
// treated as a policy violation.
func (ep *EnforcementPoint) Authorize(ctx context.Context, req Request) (Decision, error) {
	for _, p := range ep.policies {
		decision, err := p.Evaluate(ctx, req)
		if err != nil {
			return DecisionDeny, fmt.Errorf("policy %q: %w", p.Name(), err)
		}
		if decision == DecisionDeny {
			return DecisionDeny, nil
		}
	}
	return DecisionAllow, nil
}
