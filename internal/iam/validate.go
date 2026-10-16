package iam

import (
	"encoding/json"
	"fmt"
)

// MaxDocumentBytes caps a policy document, analogous to AWS's 6144-character
// managed-policy limit (measured in bytes here).
const MaxDocumentBytes = 6144

// ValidateOptions tunes write-time validation.
type ValidateOptions struct {
	// AllowUnknownActions permits actions outside KnownActions. Off by default
	// so a typo cannot produce a policy that silently grants nothing (or, with
	// wildcards, more than intended).
	AllowUnknownActions bool

	// AllowConditions permits Condition blocks even though the v1 evaluator
	// cannot evaluate them. Off by default: an un-evaluatable deny must never
	// be stored, because the evaluator would treat it as "not applicable" and
	// the deny would be silently dropped.
	AllowConditions bool
}

// Validate checks a document's structure and, by default, that every action is
// a known gateway action. It returns a descriptive error on the first problem.
func Validate(doc *PolicyDocument, opts ValidateOptions) error {
	if doc == nil {
		return fmt.Errorf("iam: nil policy document")
	}

	if raw, err := json.Marshal(doc); err == nil && len(raw) > MaxDocumentBytes {
		return fmt.Errorf("iam: policy document is %d bytes, exceeds limit of %d", len(raw), MaxDocumentBytes)
	}

	if len(doc.Statement) == 0 {
		return fmt.Errorf("iam: policy document has no statements")
	}

	seenSid := make(map[string]bool, len(doc.Statement))
	for i, st := range doc.Statement {
		where := fmt.Sprintf("statement %d", i+1)
		if st.Sid != "" {
			if seenSid[st.Sid] {
				return fmt.Errorf("iam: duplicate Sid %q", st.Sid)
			}
			seenSid[st.Sid] = true
			where = fmt.Sprintf("statement %q", st.Sid)
		}

		if st.Effect != EffectAllow && st.Effect != EffectDeny {
			return fmt.Errorf("iam: %s: Effect must be %q or %q", where, EffectAllow, EffectDeny)
		}

		if len(st.Action) > 0 && len(st.NotAction) > 0 {
			return fmt.Errorf("iam: %s: Action and NotAction are mutually exclusive", where)
		}
		if len(st.Action) == 0 && len(st.NotAction) == 0 {
			return fmt.Errorf("iam: %s: one of Action or NotAction is required", where)
		}

		if len(st.Resource) > 0 && len(st.NotResource) > 0 {
			return fmt.Errorf("iam: %s: Resource and NotResource are mutually exclusive", where)
		}
		if len(st.Resource) == 0 && len(st.NotResource) == 0 {
			return fmt.Errorf("iam: %s: one of Resource or NotResource is required", where)
		}

		for _, a := range actionsOf(st) {
			if !ValidActionPattern(a) {
				return fmt.Errorf("iam: %s: invalid action %q", where, a)
			}
			if !opts.AllowUnknownActions && !actionPatternKnown(a) {
				return fmt.Errorf("iam: %s: unknown action %q", where, a)
			}
		}

		for _, r := range resourcesOf(st) {
			if !ValidResourcePattern(r) {
				return fmt.Errorf("iam: %s: invalid resource %q", where, r)
			}
		}

		if len(st.Condition) > 0 && !opts.AllowConditions {
			return fmt.Errorf("iam: %s: conditions are not supported yet", where)
		}
	}

	return nil
}

// actionsOf returns every action pattern in a statement (Action or NotAction).
func actionsOf(st Statement) []string {
	out := make([]string, 0, len(st.Action)+len(st.NotAction))
	out = append(out, st.Action...)
	out = append(out, st.NotAction...)
	return out
}

// resourcesOf returns every resource pattern in a statement (Resource or
// NotResource).
func resourcesOf(st Statement) []string {
	out := make([]string, 0, len(st.Resource)+len(st.NotResource))
	out = append(out, st.Resource...)
	out = append(out, st.NotResource...)
	return out
}
