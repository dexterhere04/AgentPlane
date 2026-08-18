// Package db owns exactly one concern: creating and holding the shared
// Postgres connection pool. It knows nothing about api_keys, users, or any
// other table — that keeps it reusable by every future package that needs
// database access, in line with the project's single-responsibility split
// between config / handlers / proxy / (now) db.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the shared connection pool type. Callers (e.g.
// internal/apikey_management.NewStore) take a *db.Pool rather than
// constructing their own connection.
type Pool = pgxpool.Pool

// NewPool creates a Postgres connection pool for the given DSN
// (see config.DatabaseURL()) and verifies connectivity with a Ping before
// returning, so callers find out immediately if the database is
// unreachable rather than on the first query.
//
// The caller is responsible for calling Close() on the returned pool
// (typically via defer in main.go) during graceful shutdown.
func NewPool(ctx context.Context, databaseURL string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: failed to ping database: %w", err)
	}

	return pool, nil
}
