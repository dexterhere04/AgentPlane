package iam

// Decision is the outcome of evaluating policies. It mirrors policy.Decision
// but is kept local so the pure evaluator has no dependency on the generic
// policy package.
type Decision int

const (
	// DecisionAllow means an explicit Allow matched and nothing denied.
	DecisionAllow Decision = iota
	// DecisionDeny means an explicit Deny matched, a boundary disallowed, or
	// nothing allowed (implicit deny).
	DecisionDeny
)

// String renders the decision for logs and error messages.
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

// Request is an access attempt evaluated against a set of policies.
type Request struct {
	// Action is the concrete action, e.g. "ap:InvokeModel".
	Action string
	// Resource is the concrete resource ARN, e.g. "arn:ap:model/gpt-4o".
	Resource string
	// Context carries attribute values for condition evaluation. It is unused
	// in v1 (conditions are rejected at write time) but threaded through so v2
	// can add operators without an API change.
	Context map[string]any
}

// applies reports whether a statement is relevant to a request: its action,
// resource, and condition all match.
func (st Statement) applies(req Request) bool {
	return st.matchesAction(req.Action) &&
		st.matchesResource(req.Resource) &&
		st.conditionMatches(req.Context)
}

// matchesAction implements Action / NotAction semantics.
func (st Statement) matchesAction(action string) bool {
	if len(st.NotAction) > 0 {
		for _, p := range st.NotAction {
			if MatchAction(p, action) {
				return false
			}
		}
		return true
	}
	for _, p := range st.Action {
		if MatchAction(p, action) {
			return true
		}
	}
	return false
}

// matchesResource implements Resource / NotResource semantics.
func (st Statement) matchesResource(resource string) bool {
	if len(st.NotResource) > 0 {
		for _, p := range st.NotResource {
			if MatchResource(p, resource) {
				return false
			}
		}
		return true
	}
	for _, p := range st.Resource {
		if MatchResource(p, resource) {
			return true
		}
	}
	return false
}

// conditionMatches reports whether a statement's condition is satisfied.
//
// In v1 the evaluator implements no condition operators, so a statement that
// carries a Condition never applies (it cannot be shown to be satisfied). This
// is fail-closed: an un-evaluatable Deny is treated as "not applicable" and a
// lone Allow is not honoured — but the real protection is that Validate rejects
// Condition blocks from being stored in the first place, so this path is only
// reachable for in-memory documents.
func (st Statement) conditionMatches(ctx map[string]any) bool {
	if len(st.Condition) == 0 {
		return true
	}
	return evaluateCondition(st.Condition, ctx)
}

// evaluateCondition is the v2 extension point. It returns false today, meaning
// "not satisfied", which is the fail-closed choice.
func evaluateCondition(_ map[string]map[string]any, _ map[string]any) bool {
	return false
}

// EvaluateDocuments applies the IAM evaluation algorithm to a set of identity
// policies:
//
//  1. If any applicable statement is an explicit Deny → deny.
//  2. Otherwise, if any applicable statement is an Allow → allow.
//  3. Otherwise → implicit deny.
//
// This is the core difference from RBAC's allow-only chain: a single explicit
// Deny anywhere overrides every Allow.
func EvaluateDocuments(docs []*PolicyDocument, req Request) Decision {
	if hasMatchingDeny(docs, req) {
		return DecisionDeny
	}
	if hasMatchingAllow(docs, req) {
		return DecisionAllow
	}
	return DecisionDeny
}

// EvaluateWithBoundary evaluates identity policies subject to an optional
// permission boundary, which caps the maximum permissions an identity can
// have:
//
//  1. An explicit Deny in the identity policies or the boundary → deny.
//  2. If a boundary is set and does not allow the request → deny.
//  3. If the identity policies allow → allow.
//  4. Otherwise → implicit deny.
func EvaluateWithBoundary(docs []*PolicyDocument, boundary *PolicyDocument, req Request) Decision {
	var boundaryDocs []*PolicyDocument
	if boundary != nil {
		boundaryDocs = append(boundaryDocs, boundary)
	}

	if hasMatchingDeny(docs, req) || hasMatchingDeny(boundaryDocs, req) {
		return DecisionDeny
	}
	if boundary != nil && !hasMatchingAllow(boundaryDocs, req) {
		return DecisionDeny
	}
	if hasMatchingAllow(docs, req) {
		return DecisionAllow
	}
	return DecisionDeny
}

// hasMatchingDeny reports whether any document contains an applicable Deny.
func hasMatchingDeny(docs []*PolicyDocument, req Request) bool {
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, st := range doc.Statement {
			if st.Effect == EffectDeny && st.applies(req) {
				return true
			}
		}
	}
	return false
}

// hasMatchingAllow reports whether any document contains an applicable Allow.
func hasMatchingAllow(docs []*PolicyDocument, req Request) bool {
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		for _, st := range doc.Statement {
			if st.Effect == EffectAllow && st.applies(req) {
				return true
			}
		}
	}
	return false
}
