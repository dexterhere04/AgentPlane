package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values for api_keys.status. Callers should use these constants
// rather than hardcoding strings.
const (
	StatusActive = "active"
)

// postgresUniqueViolationCode is the Postgres error code for a unique
// constraint violation (23505). Used to detect a key_id collision.
const postgresUniqueViolationCode = "23505"

// ErrDuplicateKeyID is returned when the generated key_id already exists
// in api_keys. key_id collisions should be extremely rare (see
// apikey.go's entropy notes), but callers should be prepared to catch
// this and retry with a freshly generated key rather than treating it as
// a fatal error.
var ErrDuplicateKeyID = errors.New("apikeymanagement: key_id already exists")

// Store persists API key records. It holds only a connection pool and has
// no knowledge of HTTP, key generation, or anything outside the api_keys
// table — consistent with the rest of the project keeping each package to
// a single responsibility.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool
// (see db.NewPool).
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateAPIKeyParams is the input for persisting a newly generated API key.
//
// SECURITY: there is deliberately no field here for a plaintext secret or
// full key (GeneratedAPIKey.Secret / GeneratedAPIKey.FullKey). Only KeyID
// (public, non-secret) and SecretHash (irreversible) are ever written to
// the database. Callers must show the full key to the end user exactly
// once at generation time and then discard it — CreateAPIKeyParams has no
// way to accidentally persist it.
type CreateAPIKeyParams struct {
	// UserID is the owning user. Required.
	UserID uuid.UUID

	// Name is a human-readable label for the key (e.g. "CI pipeline key").
	// Required.
	Name string

	// KeyID is the public lookup identifier, from GeneratedAPIKey.KeyID.
	// Required.
	KeyID string

	// SecretHash is SHA-256(secret + pepper), from GeneratedAPIKey.SecretHash.
	// Required.
	SecretHash string

	// Status is the initial status (e.g. StatusActive). If empty, defaults
	// to StatusActive.
	Status string

	// ExpiresAt is optional. Nil means the key does not expire.
	ExpiresAt *time.Time
}

// APIKeyRecord mirrors a row in api_keys, minus secret_hash.
//
// secret_hash is intentionally excluded from this struct. Once a key is
// stored, nothing in the generation/storage path needs to read the hash
// back out — only the future verification path should query for it, and
// it should do so narrowly and explicitly rather than have it flow through
// this general-purpose struct where it could end up logged or serialized
// by accident.
type APIKeyRecord struct {
	ID         uuid.UUID
	KeyID      string
	UserID     uuid.UUID
	Name       string
	Status     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// CreateAPIKey inserts a new api_keys row and returns the stored record.
//
// It never receives or persists a plaintext secret: CreateAPIKeyParams has
// no field for one, so this satisfies "store only hashes, never plaintext
// keys" at the type level, not just by convention.
//
// If the generated key_id collides with an existing row (a 23505 unique
// violation on api_keys.key_id), CreateAPIKey returns ErrDuplicateKeyID so
// the caller can generate a fresh key and retry.
func (s *Store) CreateAPIKey(ctx context.Context, params CreateAPIKeyParams) (*APIKeyRecord, error) {
	if params.UserID == uuid.Nil {
		return nil, fmt.Errorf("apikeymanagement: UserID is required")
	}
	if params.Name == "" {
		return nil, fmt.Errorf("apikeymanagement: Name is required")
	}
	if params.KeyID == "" {
		return nil, fmt.Errorf("apikeymanagement: KeyID is required")
	}
	if params.SecretHash == "" {
		return nil, fmt.Errorf("apikeymanagement: SecretHash is required")
	}

	status := params.Status
	if status == "" {
		status = StatusActive
	}

	const query = `
		INSERT INTO api_keys (id, key_id, user_id, name, secret_hash, status, created_at, expires_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), $6)
		RETURNING id, key_id, user_id, name, status, created_at, last_used_at, expires_at, revoked_at
	`

	var rec APIKeyRecord
	err := s.pool.QueryRow(ctx, query,
		params.KeyID,
		params.UserID,
		params.Name,
		params.SecretHash,
		status,
		params.ExpiresAt,
	).Scan(
		&rec.ID,
		&rec.KeyID,
		&rec.UserID,
		&rec.Name,
		&rec.Status,
		&rec.CreatedAt,
		&rec.LastUsedAt,
		&rec.ExpiresAt,
		&rec.RevokedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			return nil, ErrDuplicateKeyID
		}
		return nil, fmt.Errorf("apikeymanagement: failed to insert api key: %w", err)
	}

	return &rec, nil
}
