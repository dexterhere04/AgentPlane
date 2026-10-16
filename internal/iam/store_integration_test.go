//go:build integration

package iam

import (
	"context"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/dexterhere04/AgentPlane/migrations"
	"github.com/google/uuid"
)

// TestStore_Integration runs against a real PostgreSQL database (DATABASE_URL).
// It exercises the IAM store and engine end to end: policy CRUD, attachments,
// group inheritance, role grants, permission boundaries, and the deny-wins
// evaluation through Engine.
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags=integration ./internal/iam/...
func TestStore_Integration(t *testing.T) {
	ctx := context.Background()

	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	store := NewStore(pool)
	engine := New(store)
	userStore := users.NewStore(pool)

	if err := store.SeedBuiltins(ctx); err != nil {
		t.Fatalf("seed builtins: %v", err)
	}
	// SeedBuiltins must be idempotent.
	if err := store.SeedBuiltins(ctx); err != nil {
		t.Fatalf("seed builtins (second call): %v", err)
	}

	suffix := uuid.NewString()
	username := "itest_iam_" + suffix
	email := username + "@example.com"

	user, err := userStore.CreateUser(ctx, users.CreateUserParams{Username: username, Email: &email})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	policyAllow := "itest_allow_" + suffix
	policyDeny := "itest_deny_" + suffix
	policyBoundary := "itest_boundary_" + suffix
	groupName := "itest_group_" + suffix
	roleName := "itest_role_" + suffix

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_policies WHERE name = ANY($1)`, []string{policyAllow, policyDeny, policyBoundary})
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_groups WHERE name = $1`, groupName)
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_roles WHERE name = $1`, roleName)
		// Reuse the production delete path so the user's bindings go with them.
		_ = users.NewStore(pool).DeleteUser(context.Background(), user.ID)
	})

	mustPolicy := func(name string, doc string) {
		t.Helper()
		d, err := ParseDocument([]byte(doc))
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if _, err := store.CreatePolicy(ctx, name, "integration test", d); err != nil {
			t.Fatalf("create policy %s: %v", name, err)
		}
	}

	eval := func(action, resource string) policy.Decision {
		t.Helper()
		d, err := engine.Evaluate(ctx, policy.Request{UserID: user.ID, Action: action, Resource: resource})
		if err != nil {
			t.Fatalf("evaluate %s: %v", action, err)
		}
		return d
	}

	// Deny-by-default: no policies attached.
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionDeny {
		t.Fatalf("expected implicit deny for unprovisioned user, got %s", got)
	}

	// Direct attachment of an allow policy.
	mustPolicy(policyAllow, `{"Version":"2026-01-01","Statement":[{"Effect":"Allow","Action":"ap:InvokeModel","Resource":"arn:ap:model/*"}]}`)
	if err := store.AttachPolicy(ctx, policyAllow, PrincipalUser, user.ID); err != nil {
		t.Fatalf("attach allow: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionAllow {
		t.Fatalf("expected allow after attach, got %s", got)
	}
	if got := eval(ActionInvokeChat, "*"); got != policy.DecisionDeny {
		t.Fatalf("expected deny for non-granted action, got %s", got)
	}

	// Explicit deny overrides the allow.
	mustPolicy(policyDeny, `{"Statement":[{"Effect":"Deny","Action":"ap:InvokeModel","Resource":"arn:ap:model/gpt-4o"}]}`)
	if err := store.AttachPolicy(ctx, policyDeny, PrincipalUser, user.ID); err != nil {
		t.Fatalf("attach deny: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionDeny {
		t.Fatalf("expected explicit deny to win, got %s", got)
	}
	// A different model is still allowed (deny is resource-specific).
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o-mini"); got != policy.DecisionAllow {
		t.Fatalf("expected allow for non-denied model, got %s", got)
	}

	// Detach the deny → allow returns.
	if err := store.DetachPolicy(ctx, policyDeny, PrincipalUser, user.ID); err != nil {
		t.Fatalf("detach deny: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionAllow {
		t.Fatalf("expected allow after detach, got %s", got)
	}

	// Permission boundary caps the allow.
	mustPolicy(policyBoundary, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"}]}`)
	if err := store.SetUserPermissionsBoundary(ctx, user.ID, policyBoundary); err != nil {
		t.Fatalf("set boundary: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionDeny {
		t.Fatalf("expected boundary to deny InvokeModel, got %s", got)
	}
	if err := store.SetUserPermissionsBoundary(ctx, user.ID, ""); err != nil {
		t.Fatalf("clear boundary: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionAllow {
		t.Fatalf("expected allow after clearing boundary, got %s", got)
	}

	// Group inheritance: a policy attached to a group the user belongs to.
	policyGroup := "itest_group_policy_" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_policies WHERE name = $1`, policyGroup)
	})
	mustPolicy(policyGroup, `{"Statement":[{"Effect":"Allow","Action":"ap:InvokeChat","Resource":"*"}]}`)
	if _, err := store.CreateGroup(ctx, groupName, "integration test group"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := store.AttachPolicy(ctx, policyGroup, PrincipalGroup, groupUUID(ctx, t, store, groupName)); err != nil {
		t.Fatalf("attach group policy: %v", err)
	}
	if err := store.AddUserToGroup(ctx, groupName, user.ID); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if got := eval(ActionInvokeChat, "*"); got != policy.DecisionAllow {
		t.Fatalf("expected allow via group, got %s", got)
	}

	// Role grant: a role carries a policy; granting the role grants access.
	policyRole := "itest_role_policy_" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM iam_policies WHERE name = $1`, policyRole)
	})
	mustPolicy(policyRole, `{"Statement":[{"Effect":"Allow","Action":"ap:ListProviders","Resource":"arn:ap:*"}]}`)
	role, err := store.CreateRole(ctx, roleName, "integration test role", nil, nil)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := store.AttachPolicy(ctx, policyRole, PrincipalRole, role.ID); err != nil {
		t.Fatalf("attach role policy: %v", err)
	}
	if got := eval(ActionListProviders, "arn:ap:provider/openai"); got != policy.DecisionDeny {
		t.Fatalf("expected deny before role grant, got %s", got)
	}
	if err := store.GrantRole(ctx, roleName, PrincipalUser, user.ID); err != nil {
		t.Fatalf("grant role: %v", err)
	}
	if got := eval(ActionListProviders, "arn:ap:provider/openai"); got != policy.DecisionAllow {
		t.Fatalf("expected allow via granted role, got %s", got)
	}

	// UserRoles reflects the grant.
	roles, err := store.UserRoles(ctx, user.ID)
	if err != nil {
		t.Fatalf("user roles: %v", err)
	}
	if len(roles) != 1 || roles[0] != roleName {
		t.Fatalf("expected [%s], got %v", roleName, roles)
	}
}

func groupUUID(ctx context.Context, t *testing.T, store *Store, name string) uuid.UUID {
	t.Helper()
	groups, err := store.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	for _, g := range groups {
		if g.Name == name {
			return g.ID
		}
	}
	t.Fatalf("group %q not found", name)
	return uuid.Nil
}

// TestBuiltinRoles_Integration verifies that the seeded built-in roles grant
// the access they advertise: "member" can invoke models but not administer,
// and "admin" (APAdmin) can do everything.
func TestBuiltinRoles_Integration(t *testing.T) {
	ctx := context.Background()

	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := NewStore(pool)
	engine := New(store)
	userStore := users.NewStore(pool)
	if err := store.SeedBuiltins(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	suffix := uuid.NewString()
	email := "itest_builtin_" + suffix + "@example.com"
	user, err := userStore.CreateUser(ctx, users.CreateUserParams{Username: "itest_builtin_" + suffix, Email: &email})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Delete through the production path so the user's role bindings are removed too.
	t.Cleanup(func() { _ = users.NewStore(pool).DeleteUser(context.Background(), user.ID) })

	eval := func(action, resource string) policy.Decision {
		t.Helper()
		d, err := engine.Evaluate(ctx, policy.Request{UserID: user.ID, Action: action, Resource: resource})
		if err != nil {
			t.Fatalf("evaluate %s: %v", action, err)
		}
		return d
	}

	// Before any grant: denied.
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionDeny {
		t.Fatalf("expected deny before grant, got %s", got)
	}

	// member: can invoke models, cannot administer.
	if err := store.GrantRole(ctx, "member", PrincipalUser, user.ID); err != nil {
		t.Fatalf("grant member: %v", err)
	}
	if got := eval(ActionInvokeModel, "arn:ap:model/gpt-4o"); got != policy.DecisionAllow {
		t.Fatalf("expected member to invoke a model, got %s", got)
	}
	if got := eval(ActionCreateProvider, "arn:ap:*"); got != policy.DecisionDeny {
		t.Fatalf("expected member to be denied CreateProvider, got %s", got)
	}

	// admin (APAdmin): can do everything.
	if err := store.GrantRole(ctx, "admin", PrincipalUser, user.ID); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	if got := eval(ActionCreateProvider, "arn:ap:*"); got != policy.DecisionAllow {
		t.Fatalf("expected admin to create a provider, got %s", got)
	}
	if got := eval(ActionListPolicies, "arn:ap:*"); got != policy.DecisionAllow {
		t.Fatalf("expected admin to list policies, got %s", got)
	}
}
