// Package iam implements an AWS-style identity and access management layer for
// AgentPlane: JSON policy documents with Allow/Deny effects, action and
// resource (ARN) matching, permission boundaries, and the deny-wins evaluation
// algorithm.
//
// The package is deliberately layered:
//
//   - arn.go / action.go   — pure string matchers (glob semantics)
//   - document.go          — the policy document JSON model
//   - validate.go          — write-time validation
//   - evaluator.go         — the pure, I/O-free evaluation algorithm
//   - engine.go            — adapts the evaluator to the generic policy.Policy
//   - store.go             — persistence (policies, roles, groups, attachments)
//
// It replaces internal/policy/rbac. RBAC expressed an identity's access as a
// flat list of "<resource>:<action>" strings with allow-only semantics; IAM
// instead evaluates independently-formatted Action and Resource patterns and,
// crucially, honours an explicit Deny that overrides any Allow.
package iam

// arnPrefix is the resource namespace. Resources have the form
// "arn:ap:{type}/{id}", e.g. arn:ap:model/gpt-4o, arn:ap:provider/*,
// arn:ap:*.
const arnPrefix = "arn:ap:"

// ResourceTypes is the set of resource types the gateway understands. It is
// used for validation only; matching is pure string globbing. It is exported so
// callers (and the admin API) can enumerate valid resource types.
var ResourceTypes = map[string]bool{
	"model":    true,
	"provider": true,
	"route":    true,
	"apikey":   true,
	"user":     true,
	"group":    true,
	"role":     true,
	"policy":   true,
	"secret":   true,
	"chat":     true,
}

// matchGlob reports whether pattern matches s using IAM wildcard semantics:
// '*' matches any run of characters (including ':' and '/'), and '?' matches
// exactly one character. The match is anchored at both ends.
//
// It uses the classic linear-backtracking wildcard algorithm: O(len(pattern))
// space and O(len(pattern)*len(s)) worst-case time, which is more than fast
// enough for policy evaluation.
func matchGlob(pattern, s string) bool {
	pi, si := 0, 0
	star, mark := -1, 0

	for si < len(s) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == s[si]):
			pi++
			si++
		case pi < len(pattern) && pattern[pi] == '*':
			star = pi
			mark = si
			pi++
		case star != -1:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}

	// Consume any trailing '*'s in the pattern.
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// MatchResource reports whether the resource pattern grants access to the
// concrete resource. Both sides are matched with glob semantics.
func MatchResource(pattern, resource string) bool {
	if pattern == "" || resource == "" {
		return false
	}
	return matchGlob(pattern, resource)
}

// ValidResourcePattern reports whether s is a syntactically valid resource
// pattern: either the "*" shorthand, or an "arn:ap:..." form. Wildcards are
// permitted in the type and id segments.
func ValidResourcePattern(s string) bool {
	if s == "*" {
		return true
	}
	if len(s) < len(arnPrefix) || s[:len(arnPrefix)] != arnPrefix {
		return false
	}
	rest := s[len(arnPrefix):]
	if rest == "" {
		return false
	}

	typ, id, hasID := cut(rest, '/')
	if typ == "" {
		return false
	}
	// A bare type is only valid as the "*" wildcard (arn:ap:*).
	if !hasID {
		return typ == "*"
	}
	return id != ""
}
