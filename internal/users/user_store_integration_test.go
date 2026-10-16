package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/db"
)

func TestCreateUser_RealPostgres(t *testing.T) {
	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("DATABASE_URL not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	store := NewStore(pool)

	username := "integration_test_" + time.Now().Format("20060102150405.000000000")
	email := username + "@example.com"

	user, err := store.CreateUser(ctx, CreateUserParams{
		Username: username,
		Email:    &email,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.ID == [16]byte{} {
		t.Fatal("expected PostgreSQL to generate a user ID")
	}

	if user.Username != username {
		t.Errorf("expected username %q, got %q", username, user.Username)
	}

	if user.Email == nil || *user.Email != email {
		t.Errorf("expected email %q, got %v", email, user.Email)
	}

	if user.Status != StatusActive {
		t.Errorf("expected status %q, got %q", StatusActive, user.Status)
	}

	if user.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated")
	}

	if user.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be populated")
	}

	var count int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM users WHERE id = $1`,
		user.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify user in PostgreSQL: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected exactly 1 user row in PostgreSQL, got %d", count)
	}

	// Keep the integration database clean after the test.
	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
	if err != nil {
		t.Fatalf("failed to clean up integration-test user: %v", err)
	}

}

func TestListUsers_RealPostgres(t *testing.T) {
	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("DATABASE_URL not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	// Registered via t.Cleanup rather than defer: t.Cleanup callbacks are LIFO,
	// and a deferred close would shut the pool before the row cleanup below runs.
	t.Cleanup(func() { pool.Close() })

	store := NewStore(pool)

	suffix := time.Now().Format("20060102150405.000000000")
	username := "listusers_test_" + suffix
	email := username + "@example.com"

	created, err := store.CreateUser(ctx, CreateUserParams{Username: username, Email: &email})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	// Clean up with a fresh context: the test's ctx is cancelled by defer cancel()
	// before t.Cleanup runs, which previously made this delete fail silently and
	// leak a user row on every run.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, created.ID)
	})

	list, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	var found *User
	for i := range list {
		if list[i].ID == created.ID {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected newly created user %s in ListUsers result", created.ID)
	}
	if found.Username != username {
		t.Errorf("expected username %q, got %q", username, found.Username)
	}
	if found.Status != StatusActive {
		t.Errorf("expected status %q, got %q", StatusActive, found.Status)
	}
}

// TestDeleteUser_RealPostgres verifies that deleting a user removes every row
// bound to that identity (API keys and IAM bindings), and that a second delete
// reports ErrUserNotFound.
func TestDeleteUser_RealPostgres(t *testing.T) {
	databaseURL, err := config.DatabaseURL()
	if err != nil {
		t.Skipf("DATABASE_URL not configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	// LIFO: this runs after the row cleanups registered below.
	t.Cleanup(func() { pool.Close() })

	store := NewStore(pool)

	suffix := time.Now().Format("20060102150405.000000000")
	username := "deleteuser_test_" + suffix
	email := username + "@example.com"

	user, err := store.CreateUser(ctx, CreateUserParams{Username: username, Email: &email})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Bind a policy, a role and a group to the user, plus one API key, so the
	// cascade is actually exercised.
	var policyID, roleID, groupID string
	if err := pool.QueryRow(ctx, `INSERT INTO iam_policies (name, document) VALUES ($1, '{}'::jsonb) RETURNING id`, "deleteuser_policy_"+suffix).Scan(&policyID); err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO iam_roles (name, description) VALUES ($1, 'test') RETURNING id`, "deleteuser_role_"+suffix).Scan(&roleID); err != nil {
		t.Fatalf("failed to seed role: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO iam_groups (name) VALUES ($1) RETURNING id`, "deleteuser_group_"+suffix).Scan(&groupID); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM iam_policies WHERE id = $1`, policyID)
		_, _ = pool.Exec(bg, `DELETE FROM iam_roles WHERE id = $1`, roleID)
		_, _ = pool.Exec(bg, `DELETE FROM iam_groups WHERE id = $1`, groupID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, user.ID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO api_keys (key_id, user_id, name, secret_hash, status) VALUES ($1, $2, 'test', 'hash', 'active')`, "deleteuser_key_"+suffix, user.ID); err != nil {
		t.Fatalf("failed to seed api key: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO iam_policy_attachments (policy_id, principal_type, principal_id) VALUES ($1, 'user', $2)`, policyID, user.ID); err != nil {
		t.Fatalf("failed to seed policy attachment: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO iam_role_assignments (role_id, principal_type, principal_id) VALUES ($1, 'user', $2)`, roleID, user.ID); err != nil {
		t.Fatalf("failed to seed role assignment: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO iam_group_memberships (group_id, user_id) VALUES ($1, $2)`, groupID, user.ID); err != nil {
		t.Fatalf("failed to seed group membership: %v", err)
	}

	if err := store.DeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	checks := []struct {
		name  string
		query string
	}{
		{"users", `SELECT COUNT(*) FROM users WHERE id = $1`},
		{"api_keys", `SELECT COUNT(*) FROM api_keys WHERE user_id = $1`},
		{"iam_policy_attachments", `SELECT COUNT(*) FROM iam_policy_attachments WHERE principal_type = 'user' AND principal_id = $1`},
		{"iam_role_assignments", `SELECT COUNT(*) FROM iam_role_assignments WHERE principal_type = 'user' AND principal_id = $1`},
		{"iam_group_memberships", `SELECT COUNT(*) FROM iam_group_memberships WHERE user_id = $1`},
	}
	for _, c := range checks {
		var count int
		if err := pool.QueryRow(ctx, c.query, user.ID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", c.name, err)
		}
		if count != 0 {
			t.Errorf("expected 0 rows in %s after delete, got %d", c.name, count)
		}
	}

	// Deleting again must report a missing user, not a silent success.
	if err := store.DeleteUser(ctx, user.ID); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound on second delete, got %v", err)
	}
}
