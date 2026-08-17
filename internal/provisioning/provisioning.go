// Package provisioning orchestrates the user-to-API-key flow: create a
// user, generate an API key for them, and persist only the key's hash.
//
// It depends on both internal/users and internal/api but neither of those
// packages depends on it or on each other — this is the one place that
// knows both steps need to happen together.
//
// Steps are sequential, not transactional (by design — see the project
// owner's decision below): if user creation succeeds but API key
// generation/persistence fails, the user is NOT rolled back. The returned
// error wraps the user's ID so the caller can retry key creation for that
// same user rather than creating a duplicate user.
package provisioning

import (
	"context"
	"errors"
	"fmt"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/users"
)

// maxKeyGenerationAttempts bounds how many times we'll regenerate a fresh
// key_id/secret and retry if we hit an (extremely unlikely) key_id
// collision — see api.ErrDuplicateKeyID. Not asked about explicitly; a
// small, bounded retry seemed better than failing the whole provisioning
// flow on a one-in-many-billions collision.
const maxKeyGenerationAttempts = 3

// Result holds everything produced by a successful (or partially
// successful) provisioning call.
type Result struct {
	// User is set whenever user creation succeeded, even if a later step
	// failed. Callers should check this on error to find out whether a
	// user now exists that needs a key created for it separately.
	User *users.User

	// APIKeyRecord is set only on full success — the persisted row
	// (key_id, user_id, etc., no secret).
	APIKeyRecord *api.APIKeyRecord

	// FullKey is the complete plaintext API key (e.g. "ap_live_..."). It
	// is set only on full success and is NEVER persisted anywhere — not
	// in the database, not logged. The caller must show it to the end
	// user immediately and then let it go out of scope; nothing in this
	// package or api/users retains it after this call returns.
	FullKey string
}

// Provisioner wires together a users.Store, an api.Store, and the
// server-side pepper needed to generate API keys.
type Provisioner struct {
	users   *users.Store
	apiKeys *api.Store
	pepper  string
}

// NewProvisioner creates a Provisioner. pepper should come from
// config.KeyPepper(), same as any other direct caller of
// api.GenerateAPIKey.
func NewProvisioner(usersStore *users.Store, apiKeyStore *api.Store, pepper string) *Provisioner {
	return &Provisioner{
		users:   usersStore,
		apiKeys: apiKeyStore,
		pepper:  pepper,
	}
}

// ProvisionUserWithAPIKey creates a new user and a new API key for them,
// in that order:
//
//  1. users.Store.CreateUser — creates the user, obtaining user.ID.
//  2. api.GenerateAPIKey — generates key_id, secret, and secret_hash
//     in memory. The plaintext secret/full key never leaves this call.
//  3. api.Store.CreateAPIKey — persists key_id, secret_hash, and
//     user_id. The plaintext secret is never passed to this step at all
//     (api.CreateAPIKeyParams has no field for it), so it's structurally
//     impossible for this step to store it.
//
// On success, Result.FullKey holds the complete plaintext key — the only
// place it will ever exist outside this call stack. The caller must
// return/display it to the end user immediately; it cannot be recovered
// afterward, by design.
//
// If step 1 fails, both User and the error are nil/set appropriately and
// no key is generated. If step 2 or 3 fails after step 1 succeeded, the
// user IS left created (see package doc) — Result.User will be non-nil
// even though the overall call returns an error, so the caller can see
// which user now needs a key.
func (p *Provisioner) ProvisionUserWithAPIKey(ctx context.Context, userParams users.CreateUserParams, keyName string) (*Result, error) {
	user, err := p.users.CreateUser(ctx, userParams)
	if err != nil {
		return nil, fmt.Errorf("provisioning: failed to create user: %w", err)
	}

	var generated *api.GeneratedAPIKey
	var record *api.APIKeyRecord

	for attempt := 1; attempt <= maxKeyGenerationAttempts; attempt++ {
		generated, err = api.GenerateAPIKey(p.pepper)
		if err != nil {
			return &Result{User: user}, fmt.Errorf("provisioning: user %s created but failed to generate api key: %w", user.ID, err)
		}

		record, err = p.apiKeys.CreateAPIKey(ctx, api.CreateAPIKeyParams{
			UserID:     user.ID,
			Name:       keyName,
			KeyID:      generated.KeyID,
			SecretHash: generated.SecretHash,
		})
		if err == nil {
			break
		}
		if errors.Is(err, api.ErrDuplicateKeyID) {
			continue // regenerate a fresh key_id/secret and retry
		}
		return &Result{User: user}, fmt.Errorf("provisioning: user %s created but failed to persist api key: %w", user.ID, err)
	}

	if err != nil {
		return &Result{User: user}, fmt.Errorf("provisioning: user %s created but failed to persist api key after %d attempts: %w", user.ID, maxKeyGenerationAttempts, err)
	}

	return &Result{
		User:         user,
		APIKeyRecord: record,
		FullKey:      generated.FullKey,
	}, nil
}
