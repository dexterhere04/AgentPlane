package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
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

// MarkKeyUsed records the time at which an API key was successfully
// authenticated.
func (s *Store) MarkKeyUsed(ctx context.Context, keyID string) error {
	if keyID == "" {
		return fmt.Errorf("auth: key_id is required")
	}

	const query = `
		UPDATE api_keys
		SET last_used_at = now()
		WHERE key_id = $1
		  AND status = $2
		  AND revoked_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, keyID, api.StatusActive)
	if err != nil {
		return fmt.Errorf("auth: failed to update last_used_at: %w", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("auth: API key not found or inactive")
	}

	return nil
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
