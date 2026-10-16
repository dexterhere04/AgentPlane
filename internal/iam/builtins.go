package iam

import (
	"context"
	"errors"
	"fmt"
)

// DefaultVersion is the policy document version written by the built-in
// policies. It mirrors AWS's "2012-10-17" version marker.
const DefaultVersion = "2026-01-01"

// Builtin names for the recommended, AWS-managed-policy-style policies.
const (
	PolicyAdmin     = "APAdmin"
	PolicyMember    = "APMember"
	PolicyDeveloper = "APDeveloper"
	PolicyReadOnly  = "APReadOnly"
	PolicyModelUser = "APModelUser"
)

// builtinPolicy is a recommended policy seeded on startup.
type builtinPolicy struct {
	name        string
	description string
	statements  []Statement
}

// BuiltinPolicies returns the recommended policies. They are seeded idempotently
// by SeedBuiltins and shown in the admin API so operators can attach a sensible
// least-privilege starting point instead of hand-writing JSON.
func BuiltinPolicies() []builtinPolicy {
	return []builtinPolicy{
		{
			name:        PolicyAdmin,
			description: "Full administrative access to every gateway resource.",
			statements: []Statement{{
				Sid: "FullAccess", Effect: EffectAllow,
				Action:   StringOrSlice{"ap:*"},
				Resource: StringOrSlice{"arn:ap:*"},
			}},
		},
		{
			name:        PolicyMember,
			description: "Invoke chat and use any model.",
			statements: []Statement{
				{Sid: "InvokeChat", Effect: EffectAllow, Action: StringOrSlice{ActionInvokeChat}, Resource: StringOrSlice{"*"}},
				{Sid: "InvokeAnyModel", Effect: EffectAllow, Action: StringOrSlice{ActionInvokeModel}, Resource: StringOrSlice{"arn:ap:model/*"}},
			},
		},
		{
			name:        PolicyDeveloper,
			description: "Invoke chat and models, and read providers, routes, and metrics.",
			statements: []Statement{
				{Sid: "Invoke", Effect: EffectAllow, Action: StringOrSlice{ActionInvokeChat, ActionInvokeModel}, Resource: StringOrSlice{"arn:ap:*"}},
				{Sid: "ReadConfiguration", Effect: EffectAllow,
					Action:   StringOrSlice{ActionListProviders, ActionListRoutingRules, ActionViewMetrics},
					Resource: StringOrSlice{"arn:ap:*"}},
			},
		},
		{
			name:        PolicyReadOnly,
			description: "Read-only access to gateway configuration and metrics.",
			statements: []Statement{{
				Sid: "ReadOnly", Effect: EffectAllow,
				Action: StringOrSlice{
					ActionListProviders, ActionListRoutingRules, ActionListUsers,
					ActionListApiKeys, ActionViewMetrics,
				},
				Resource: StringOrSlice{"arn:ap:*"},
			}},
		},
		{
			name:        PolicyModelUser,
			description: "Invoke chat and any model, but no administrative actions.",
			statements: []Statement{
				{Sid: "InvokeChat", Effect: EffectAllow, Action: StringOrSlice{ActionInvokeChat}, Resource: StringOrSlice{"*"}},
				{Sid: "InvokeAnyModel", Effect: EffectAllow, Action: StringOrSlice{ActionInvokeModel}, Resource: StringOrSlice{"arn:ap:model/*"}},
			},
		},
	}
}

// SeedBuiltins idempotently creates the recommended policies and the built-in
// admin/member roles, attaching APAdmin to the admin role and APMember to the
// member role. It is safe to call on every startup.
//
// The migration in 000010 already creates admin/member from the legacy RBAC
// seed; SeedBuiltins reconciles that with the recommended policy names so a
// fresh install and an upgraded install converge on the same state.
func (s *Store) SeedBuiltins(ctx context.Context) error {
	for _, bp := range BuiltinPolicies() {
		doc := &PolicyDocument{Version: DefaultVersion, Statement: bp.statements}
		if _, err := s.CreatePolicy(ctx, bp.name, bp.description, doc); err != nil {
			if !errors.Is(err, ErrDuplicatePolicy) {
				return fmt.Errorf("iam: seeding policy %q: %w", bp.name, err)
			}
		}
		if err := s.markBuiltin(ctx, bp.name); err != nil {
			return err
		}
	}

	// Ensure the built-in roles exist.
	for _, r := range []struct{ name, description string }{
		{"admin", "Full access to every gateway resource."},
		{"member", "Standard access: invoke chat and use any model."},
	} {
		if _, err := s.CreateRole(ctx, r.name, r.description, nil, nil); err != nil && !errors.Is(err, ErrDuplicateRole) {
			return fmt.Errorf("iam: seeding role %q: %w", r.name, err)
		}
	}

	// Attach the recommended policy to each built-in role.
	if err := s.attachRolePolicy(ctx, "admin", PolicyAdmin); err != nil {
		return err
	}
	if err := s.attachRolePolicy(ctx, "member", PolicyMember); err != nil {
		return err
	}
	return nil
}

// markBuiltin flags a policy as built-in.
func (s *Store) markBuiltin(ctx context.Context, name string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE iam_policies SET is_builtin = TRUE WHERE name = $1`, name); err != nil {
		return fmt.Errorf("iam: marking %q builtin: %w", name, err)
	}
	return nil
}

// attachRolePolicy attaches a policy to a role by name, tolerating a missing
// role or policy (the caller seeds both).
func (s *Store) attachRolePolicy(ctx context.Context, roleName, policyName string) error {
	const query = `
		INSERT INTO iam_policy_attachments (policy_id, principal_type, principal_id)
		SELECT p.id, 'role', r.id
		FROM iam_policies p, iam_roles r
		WHERE p.name = $1 AND r.name = $2
		ON CONFLICT DO NOTHING
	`
	if _, err := s.pool.Exec(ctx, query, policyName, roleName); err != nil {
		return fmt.Errorf("iam: attaching %q to role %q: %w", policyName, roleName, err)
	}
	return nil
}
