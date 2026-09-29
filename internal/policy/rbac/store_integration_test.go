//go:build integration

package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/dexterhere04/AgentPlane/migrations"
	"github.com/google/uuid"
)

// TestStore_Integration runs against a real PostgreSQL database
// (DATABASE_URL). It applies migrations (idempotently) so the RBAC tables
// exist, then exercises role creation, permission grants, user-role
// assignment, and the deny-by-default behavior through the engine.
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags=integration ./internal/policy/rbac/...
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
	defer pool.Close()

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	userStore := users.NewStore(pool)
	store := NewStore(pool)

	suffix := uuid.NewString()
	username := "itest_rbac_" + suffix
	email := username + "@example.com"

	user, err := userStore.CreateUser(ctx, users.CreateUserParams{
		Username: username,
		Email:    &email,
	})
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	roleName := "itest_role_" + suffix

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM roles WHERE name = $1`, roleName)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	// Deny-by-default: a fresh user with no roles can do nothing.
	engine := New(store)
	decision, err := engine.Evaluate(ctx, policy.Request{UserID: user.ID, Permission: "chat:invoke"})
	if err != nil {
		t.Fatalf("engine evaluate failed: %v", err)
	}
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny before any role assigned, got %s", decision)
	}

	role, err := store.CreateRole(ctx, roleName, "integration test role", []string{"chat:invoke", "model:gpt-4o"})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if len(role.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(role.Permissions))
	}

	// Duplicate role creation is rejected.
	if _, err := store.CreateRole(ctx, roleName, "", nil); !errors.Is(err, ErrDuplicateRole) {
		t.Fatalf("expected ErrDuplicateRole, got %v", err)
	}

	if err := store.AssignRole(ctx, user.ID, roleName); err != nil {
		t.Fatalf("AssignRole failed: %v", err)
	}

	permissions, err := store.LoadPermissions(ctx, user.ID)
	if err != nil {
		t.Fatalf("LoadPermissions failed: %v", err)
	}
	if !policy.MatchAny(permissions, "chat:invoke") {
		t.Fatalf("expected chat:invoke in %v", permissions)
	}
	if !policy.MatchAny(permissions, "model:gpt-4o") {
		t.Fatalf("expected model:gpt-4o in %v", permissions)
	}
	if policy.MatchAny(permissions, "model:gpt-4o-mini") {
		t.Fatalf("did not expect model:gpt-4o-mini in %v", permissions)
	}

	byUser, err := store.RolesByUserIDs(ctx, []uuid.UUID{user.ID})
	if err != nil {
		t.Fatalf("RolesByUserIDs failed: %v", err)
	}
	if got := byUser[user.ID]; len(got) != 1 || got[0] != roleName {
		t.Fatalf("expected roles-by-user to contain %q, got %v", roleName, got)
	}

	decision, err = engine.Evaluate(ctx, policy.Request{UserID: user.ID, Permission: "model:gpt-4o"})
	if err != nil {
		t.Fatalf("engine evaluate failed: %v", err)
	}
	if decision != policy.DecisionAllow {
		t.Fatalf("expected allow for assigned role, got %s", decision)
	}

	// Revoking the role restores deny-by-default.
	if err := store.RevokeRole(ctx, user.ID, roleName); err != nil {
		t.Fatalf("RevokeRole failed: %v", err)
	}
	decision, err = engine.Evaluate(ctx, policy.Request{UserID: user.ID, Permission: "chat:invoke"})
	if err != nil {
		t.Fatalf("engine evaluate failed: %v", err)
	}
	if decision != policy.DecisionDeny {
		t.Fatalf("expected deny after role revoked, got %s", decision)
	}

	// Unknown roles are reported distinctly from other errors.
	if err := store.AssignRole(ctx, user.ID, "does-not-exist"); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected ErrRoleNotFound, got %v", err)
	}
}

// TestStore_SeededRoles verifies the migration seeds the built-in admin and
// member roles with their expected permissions.
func TestStore_SeededRoles(t *testing.T) {
	ctx := context.Background()

	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	store := NewStore(pool)
	roles, err := store.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles failed: %v", err)
	}

	byName := make(map[string]Role, len(roles))
	for _, r := range roles {
		byName[r.Name] = r
	}

	admin, ok := byName["admin"]
	if !ok {
		t.Fatal("expected seeded admin role")
	}
	if !policy.MatchAny(admin.Permissions, "anything:at:all") {
		t.Fatalf("expected admin wildcard permission, got %v", admin.Permissions)
	}

	member, ok := byName["member"]
	if !ok {
		t.Fatal("expected seeded member role")
	}
	if !policy.MatchAny(member.Permissions, "chat:invoke") {
		t.Fatalf("expected member to have chat:invoke, got %v", member.Permissions)
	}
	if !policy.MatchAny(member.Permissions, "model:gpt-4o") {
		t.Fatalf("expected member model:* to match model:gpt-4o, got %v", member.Permissions)
	}
}
