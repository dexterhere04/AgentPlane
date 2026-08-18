package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/users"
)

var (
	ErrInvalidAPIKey  = errors.New("auth: invalid API key")
	ErrInactiveAPIKey = errors.New("auth: API key is inactive")
	ErrRevokedAPIKey  = errors.New("auth: API key has been revoked")
	ErrExpiredAPIKey  = errors.New("auth: API key has expired")
)

// Authenticator verifies AgentPlane API keys and resolves their users.
type Authenticator struct {
	credentials *Store
	users       *users.Store
	pepper      string
}

// NewAuthenticator creates an API-key authenticator.
func NewAuthenticator(
	credentials *Store,
	users *users.Store,
	pepper string,
) *Authenticator {
	return &Authenticator{
		credentials: credentials,
		users:       users,
		pepper:      pepper,
	}
}

// Authenticate verifies a complete AgentPlane API key and returns the
// corresponding authenticated user.
func (a *Authenticator) Authenticate(
	ctx context.Context,
	fullKey string,
) (*users.User, error) {
	keyID, secret, err := parseAPIKey(fullKey)
	if err != nil {
		return nil, ErrInvalidAPIKey
	}

	rec, err := a.credentials.FindCredential(ctx, keyID)
	if err != nil {
		return nil, ErrInvalidAPIKey
	}

	if rec.Status != api.StatusActive {
		return nil, ErrInactiveAPIKey
	}

	if rec.RevokedAt != nil {
		return nil, ErrRevokedAPIKey
	}

	if rec.ExpiresAt != nil && !rec.ExpiresAt.After(time.Now()) {
		return nil, ErrExpiredAPIKey
	}

	expectedHash := hashSecret(secret, a.pepper)

	if subtle.ConstantTimeCompare(
		[]byte(expectedHash),
		[]byte(rec.SecretHash),
	) != 1 {
		return nil, ErrInvalidAPIKey
	}

	user, err := a.users.GetUserByID(ctx, rec.UserID)
	if err != nil {
		return nil, ErrInvalidAPIKey
	}
	if user.Status != users.StatusActive {
		return nil, ErrInvalidAPIKey
	}
	if err := a.credentials.MarkKeyUsed(ctx, rec.KeyID); err != nil {
		return nil, ErrInvalidAPIKey
	}

	return user, nil
}

// parseAPIKey separates:
//
//	ap_live_<key_id>_<secret>
//
// into key ID and secret.
func parseAPIKey(fullKey string) (string, string, error) {
	const prefix = "ap_live_"
	const keyIDLength = 11
	const secretLength = 43

	if !strings.HasPrefix(fullKey, prefix) {
		return "", "", ErrInvalidAPIKey
	}

	remaining := strings.TrimPrefix(fullKey, prefix)

	if len(remaining) != keyIDLength+1+secretLength {
		return "", "", ErrInvalidAPIKey
	}

	if remaining[keyIDLength] != '_' {
		return "", "", ErrInvalidAPIKey
	}

	keyID := remaining[:keyIDLength]
	secret := remaining[keyIDLength+1:]

	if keyID == "" || secret == "" {
		return "", "", ErrInvalidAPIKey
	}

	return keyID, secret, nil
}

// hashSecret must use the exact same hashing implementation used during
// API-key generation.
func hashSecret(secret, pepper string) string {
	return api.HashAPIKeySecret(secret, pepper)
}
