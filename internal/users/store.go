// Package users owns persistence for the users table: creating users and
// mapping database rows to Go types. It mirrors internal/apikey_management's
// structure (types + Store) and knows nothing about API keys, HTTP, or
// anything outside the users table.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values for users.status. Callers should use these constants
// rather than hardcoding strings.
const (
	StatusActive = "active"
)

// postgresUniqueViolationCode is the Postgres error code for a unique
// constraint violation (23505).
const postgresUniqueViolationCode = "23505"

// Postgres constraint names for users' unique columns, used to tell a
// username collision apart from an email collision after a 23505 error.
const (
	usernameUniqueConstraint = "users_username_key"
	emailUniqueConstraint    = "users_email_key"
)

// ErrDuplicateUsername is returned when the requested username is already
// taken.
var ErrDuplicateUsername = errors.New("users: username already exists")

// ErrDuplicateEmail is returned when the requested email is already in use.
var ErrDuplicateEmail = errors.New("users: email already exists")

// User mirrors a row in the users table.
type User struct {
	ID        uuid.UUID
	Username  string
	Email     *string // nullable in the schema
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateUserParams is the input for creating a new user.
type CreateUserParams struct {
	// Username is required and must be unique.
	Username string

	// Email is optional (nullable in the schema) but must be unique if set.
	Email *string

	// Status is the initial status. If empty, defaults to StatusActive.
	Status string
}

// Store persists users. It holds only a connection pool, consistent with
// apikey_management.Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool
// (see db.NewPool).
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateUser inserts a new users row and returns the stored record.
//
// Validation performed in Go before hitting the database:
//   - Username must be non-empty.
//   - Email, if provided, must pass a lightweight format sanity check
//     (has a non-empty local part and domain, and the domain contains a
//     dot). This is intentionally not full RFC 5322 validation — it's a
//     cheap check to catch obvious mistakes before a round trip to the DB.
//
// Uniqueness (username, email) is NOT re-checked in Go; it's enforced by
// the database's UNIQUE constraints, and a resulting 23505 violation is
// translated into ErrDuplicateUsername or ErrDuplicateEmail based on which
// constraint fired.
func (s *Store) CreateUser(ctx context.Context, params CreateUserParams) (*User, error) {
	params, err := validateCreateUserParams(params)
	if err != nil {
		return nil, err
	}

	const query = `
		INSERT INTO users (id, username, email, status, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, now(), now())
		RETURNING id, username, email, status, created_at, updated_at
	`

	var u User
	err = s.pool.QueryRow(ctx, query,
		params.Username,
		params.Email,
		params.Status,
	).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			switch pgErr.ConstraintName {
			case usernameUniqueConstraint:
				return nil, ErrDuplicateUsername
			case emailUniqueConstraint:
				return nil, ErrDuplicateEmail
			default:
				return nil, fmt.Errorf("users: unique constraint violated: %w", err)
			}
		}
		return nil, fmt.Errorf("users: failed to insert user: %w", err)
	}

	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("users: user ID is required")
	}

	const query = `
		SELECT id, username, email, status, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var u User

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.Status,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("users: failed to get user: %w", err)
	}

	return &u, nil
}

// ListUsers returns every user, newest first. It is used by the admin
// user-management surface and returns only user-table columns — callers that
// need role assignments should query those separately (see
// internal/policy/rbac).
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	const query = `
		SELECT id, username, email, status, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("users: failed to list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.Email,
			&u.Status,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("users: failed to scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("users: failed to iterate users: %w", err)
	}

	return users, nil
}

// validateCreateUserParams checks required fields and applies defaults. It
// is pure (no I/O) so it can be unit tested without a database.
func validateCreateUserParams(params CreateUserParams) (CreateUserParams, error) {
	if params.Username == "" {
		return params, fmt.Errorf("users: Username is required")
	}

	if params.Status == "" {
		params.Status = StatusActive
	}

	if params.Email != nil {
		if !isValidEmail(*params.Email) {
			return params, fmt.Errorf("users: Email %q is not a valid email address", *params.Email)
		}
	}

	return params, nil
}

// isValidEmail is a lightweight sanity check, not full RFC 5322 validation:
// it requires a non-empty local part, a non-empty domain, and at least one
// dot in the domain (e.g. rejects "a@b" but accepts "a@b.com").
func isValidEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}

	local := email[:at]
	domain := email[at+1:]

	if local == "" || domain == "" {
		return false
	}

	if !strings.Contains(domain, ".") {
		return false
	}

	// Reject a second '@' (e.g. "a@b@c.com").
	if strings.ContainsRune(domain, '@') {
		return false
	}

	return true
}
