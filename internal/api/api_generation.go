// Package apikeymanagement implements secure generation of AgentPlane API keys.
//
// Key format: ap_live_<key_id>_<secret>
//
//   - key_id:  8 random bytes,  base64url-encoded (public, used only for DB lookup)
//   - secret:  32 random bytes, base64url-encoded (private, 256 bits of entropy)
//   - secret_hash stored in the DB = SHA-256(secret + serverPepper), hex-encoded
//   - The pepper is expected to come from config (e.g. config.KeyPepper()),
//     which retrieves it from the configured SecretStore.
//   - This package does not access configuration or secrets directly; the
//     pepper is supplied by the caller.

package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	// keyPrefix identifies this as a live AgentPlane API key.
	keyPrefix = "ap_live"

	// keyIDBytes is the number of random bytes used to generate key_id.
	keyIDBytes = 8

	// secretBytes is the number of random bytes used to generate the secret portion of the key.
	secretBytes = 32
)

// GeneratedAPIKey holds everything produced by a single key-generation call.

// FullKey is shown to the user EXACTLY ONCE at creation time and must never
// be persisted. Only KeyID and SecretHash should be written to the database
// (as api_keys.key_id and api_keys.secret_hash respectively).
type GeneratedAPIKey struct {
	// FullKey is the complete API key to hand back to the user.
	FullKey string

	// KeyID is the public identifier portion, stored in api_keys.key_id
	KeyID string

	// Secret is the raw, unhashed secret portion. It exists only transiently
	Secret string

	// SecretHash is SHA-256(secret + pepper), hex-encoded. This is what gets stored in api_keys.secret_hash.
	SecretHash string
}

// GenerateAPIKey creates a new AgentPlane API key.

// pepper is the server-side secret value mixed into the hash.
// It is expected to be obtained by the caller via config.KeyPepper().

// The returned GeneratedAPIKey.FullKey must be shown to the caller's end user immediately and then discarded — only KeyID and SecretHash should be persisted.
func GenerateAPIKey(pepper string) (*GeneratedAPIKey, error) {
	if pepper == "" {
		return nil, fmt.Errorf("apikeymanagement: pepper must not be empty")
	}

	keyID, err := generateRandomBase64URL(keyIDBytes)
	if err != nil {
		return nil, fmt.Errorf("apikeymanagement: failed to generate key_id: %w", err)
	}

	secret, err := generateRandomBase64URL(secretBytes)
	if err != nil {
		return nil, fmt.Errorf("apikeymanagement: failed to generate secret: %w", err)
	}

	secretHash := hashSecret(secret, pepper)

	fullKey := fmt.Sprintf("%s_%s_%s", keyPrefix, keyID, secret)

	return &GeneratedAPIKey{
		FullKey:    fullKey,
		KeyID:      keyID,
		Secret:     secret,
		SecretHash: secretHash,
	}, nil
}

// generateRandomBase64URL generates n cryptographically secure random bytes
// and returns them base64url-encoded (no padding).
func generateRandomBase64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashSecret computes SHA-256(secret + pepper) and returns the result
// as a hexadecimal string.
//
// The pepper is a server-side secret retrieved from the configured
// SecretStore and is never stored alongside the hash. A database dump
// alone is therefore insufficient to reproduce the stored hashes.
func hashSecret(secret, pepper string) string {
	sum := sha256.Sum256([]byte(secret + pepper))
	return hex.EncodeToString(sum[:])
}
