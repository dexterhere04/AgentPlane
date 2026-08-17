package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Credential contains the database information required to authenticate
// an API key. The plaintext API key is never stored here.
type Credential struct {
	KeyID      string
	UserID     uuid.UUID
	SecretHash string
	Status     string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// Store provides authentication-related database lookups.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates an authentication Store backed by PostgreSQL.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// FindCredential looks up an API key by its public key_id.
//
// The database stores only the secret hash. The caller is responsible for
// hashing the supplied plaintext secret and comparing the result with
// SecretHash.
func (s *Store) FindCredential(ctx context.Context, keyID string) (*Credential, error) {
	if keyID == "" {
		return nil, fmt.Errorf("auth: key_id is required")
	}

	const query = `
		SELECT
			key_id,
			user_id,
			secret_hash,
			status,
			expires_at,
			revoked_at
		FROM api_keys
		WHERE key_id = $1
	`

	var credential Credential

	err := s.pool.QueryRow(ctx, query, keyID).Scan(
		&credential.KeyID,
		&credential.UserID,
		&credential.SecretHash,
		&credential.Status,
		&credential.ExpiresAt,
		&credential.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("auth: credential not found")
		}

		return nil, fmt.Errorf("auth: failed to find credential: %w", err)
	}

	return &credential, nil
}
