package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockKey is an arbitrary advisory-lock key used to serialize
// concurrent startup migrations so two instances of the server never apply
// the same migration at the same time.
const migrationLockKey = 8152019

// Migrate applies any pending *.up.sql migrations found in fsys, in
// filename order, and records each applied version in a schema_migrations
// table. It is idempotent and safe to call on every startup.
//
// Each migration runs inside its own transaction along with the insert of
// its version into schema_migrations, so a failed migration leaves no
// partial state behind.
func Migrate(ctx context.Context, pool *Pool, fsys fs.FS) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("db: creating schema_migrations: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("db: acquiring migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("db: acquiring migration lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey) }()

	versions, err := upMigrationFiles(fsys)
	if err != nil {
		return err
	}

	for _, version := range versions {
		applied, err := migrationApplied(ctx, conn, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := fs.ReadFile(fsys, version)
		if err != nil {
			return fmt.Errorf("db: reading migration %s: %w", version, err)
		}

		if err := applyMigration(ctx, conn, version, string(sqlBytes)); err != nil {
			return err
		}
	}

	return nil
}

// upMigrationFiles returns the *.up.sql files in fsys sorted by filename.
func upMigrationFiles(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("db: reading migrations: %w", err)
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		versions = append(versions, e.Name())
	}
	sort.Strings(versions)
	return versions, nil
}

func migrationApplied(ctx context.Context, conn *pgxpool.Conn, version string) (bool, error) {
	var applied bool
	err := conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)",
		version,
	).Scan(&applied)
	if err != nil {
		return false, fmt.Errorf("db: checking migration %s: %w", version, err)
	}
	return applied, nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, version, sql string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: beginning migration %s: %w", version, err)
	}

	if _, err := tx.Exec(ctx, sql); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("db: applying migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1)",
		version,
	); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("db: recording migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: committing migration %s: %w", version, err)
	}

	return nil
}
