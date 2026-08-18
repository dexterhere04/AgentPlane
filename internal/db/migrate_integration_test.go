//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/dexterhere04/AgentPlane/migrations"
)

// TestMigrate_Integration runs the embedded migrations against a real
// PostgreSQL database and verifies the schema is created and that Migrate
// is idempotent.
//
// Run with:
//
//	DATABASE_URL=postgres://... go test -tags=integration ./internal/db/...
func TestMigrate_Integration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()

	pool, err := NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Migrate must be idempotent.
	if err := Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}

	for _, table := range []string{"users", "api_keys", "schema_migrations"} {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %q to exist", table)
		}
	}
}
