package users

import (
	"context"
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
	defer pool.Close()

	store := NewStore(pool)

	suffix := time.Now().Format("20060102150405.000000000")
	username := "listusers_test_" + suffix
	email := username + "@example.com"

	created, err := store.CreateUser(ctx, CreateUserParams{Username: username, Email: &email})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, created.ID) })

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
