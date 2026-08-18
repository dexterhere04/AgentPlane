# User Provisioning, API Key Authentication, Revocation, and Usage Tracking

Location: `internal/api/`, `internal/auth/`, `internal/provisioning/`, `internal/handlers/`, `internal/config/`, `cmd/server/`, `migrations/`, `tests/`

## 1. What was built

Implemented the application-level API key lifecycle and authentication flow for AgentPlane.

The delivered functionality includes:

- One-to-one ownership mapping between each API key and a user through `api_keys.user_id`.
- Support for multiple API keys belonging to the same user.
- Bearer-token authentication middleware for protected HTTP endpoints.
- Resolution of the authenticated user from the presented API key.
- Attachment of the authenticated user to the request context.
- API key status, expiration, and revocation checks during authentication.
- `last_used_at` tracking whenever a valid API key successfully authenticates.
- User provisioning that creates a user and API key and returns the plaintext key once.
- Integration tests against real PostgreSQL for API key creation, revocation, authentication, and revoked-key rejection.
- PostgreSQL schema/migration support for API key lifecycle fields.
- Administrative HTTP revocation endpoint protected by an admin token.
- User creation with application-level validation for required fields and email shape.
- Database-enforced uniqueness for usernames and email addresses, with PostgreSQL unique-constraint errors translated into application-level errors.
- End-to-end provisioning orchestration that creates the user first, then generates and persists an API key associated with that user.
- Bounded retry of API-key creation when an extremely rare `key_id` collision occurs.
- Explicit handling of the sequential provisioning model: user creation and API-key creation are separate database operations rather than one transaction.
- The plaintext API key exists only transiently during generation/provisioning and is returned once to the caller.

The selected architecture keeps API key persistence, authentication, provisioning, and HTTP handling separated into their respective packages.

## 2. Files delivered

| File | Purpose |
|---|---|
| `internal/api/store.go` | Persists API key records, including creation and application-level revocation. Defines `APIKeyRecord`, status constants, and database errors. |
| `internal/api/api_generation.go` | Generates API key material and provides the hashing implementation used for secret storage and verification. |
| `internal/api/store_integration_test.go` | Real PostgreSQL integration coverage for API key creation, hash-only storage, and revocation. |
| `internal/auth/authenticator.go` | Parses and verifies API keys, checks status/revocation/expiration, resolves the owning user, and performs constant-time hash comparison. |
| `internal/auth/store.go` | Looks up authentication credentials from PostgreSQL and updates `last_used_at` after successful authentication. |
| `internal/auth/middleware.go` | Extracts `Authorization: Bearer <api-key>`, authenticates it, and attaches the resolved user to request context. |
| `internal/auth/middleware_test.go`, `internal/auth/middleware_integration_test.go`, `internal/auth/last_used_integration_test.go` | Unit and integration coverage for valid, invalid, missing, empty, and revoked API keys plus context behavior. |
| `internal/provisioning/provisioning.go` | Orchestrates user creation, API-key generation, persistence, and bounded key-ID collision retry, while ensuring only the generated plaintext key is returned to the caller. |
| `internal/handlers/provision.go` | HTTP provisioning endpoint at `POST /provision/user`. |
| `internal/handlers/revoke.go` | HTTP administrative revocation endpoint at `POST /admin/api-keys/revoke`. |
| `internal/auth/admin.go` | Validates the configured administrative token for protected revocation operations. |
| `internal/config/admin_token.go` | Loads the administrative token configuration. |
| `internal/config/pepper.go` | Loads and validates `API_KEY_PEPPER`. |
| `internal/users/store.go` | Persists users and performs application-level validation/defaulting for user creation. |
| `internal/users/store_test.go` | Unit tests for user validation and defaulting behavior. |
| `internal/provisioning/provisioning_integration_test.go` | Real PostgreSQL integration coverage for end-to-end user and API-key provisioning. |
| `migrations/000002_create_api_keys_table.up.sql` | Defines the PostgreSQL API key schema, including `user_id`, `secret_hash`, `status`, `expires_at`, `last_used_at`, and `revoked_at`. |
| `cmd/server/main.go` | Registers the provisioning, authentication, chat, events, dashboard, and administrative revocation HTTP routes. |
| `docs/full-request-authentication.md` | Documents the request/authentication flow and bearer-key handling. |
| `Readme.md` | Updated project-level API/authentication documentation where applicable. |

## 3. Decisions and rationale

### 3.1 Provisioning orchestration

User creation and API-key creation are coordinated by `internal/provisioning.Provisioner`.

The provisioning flow is:

```
POST /provision/user
→ Provisioner.ProvisionUserWithAPIKey
→ users.Store.CreateUser
→ api.GenerateAPIKey
→ api.Store.CreateAPIKey
→ return Result.FullKey
```

The users and API packages remain independent. The provisioning package owns the orchestration between them.

This keeps persistence concerns in the stores while preventing HTTP handlers from containing user/key creation logic.

### 3.2 Sequential provisioning rather than a single transaction

User creation and API-key creation are currently separate database operations.

If user creation succeeds but API-key persistence subsequently fails, the user row is not automatically rolled back. The provisioning result preserves the created user's identity so the caller can retry API-key creation for the existing user rather than creating another user.

This was chosen to keep the initial provisioning implementation simple and to keep the `users` and `api` stores independently responsible for their own persistence.

A future implementation could introduce a transaction if atomic user-plus-key provisioning becomes a requirement.

### 3.3 User validation and database uniqueness

User creation performs lightweight validation in Go for required fields and basic email shape.

Uniqueness of `username` and `email` is delegated to PostgreSQL `UNIQUE` constraints rather than being pre-checked in application code. This avoids duplicating database uniqueness logic and avoids race conditions between a check-then-insert sequence.

PostgreSQL unique-constraint violations are translated into application-level errors so callers can distinguish duplicate usernames and duplicate email addresses.

### 3.4 Hash-only secret storage

Plaintext API key secrets are never written to the `api_keys` table.

`CreateAPIKeyParams` deliberately contains `KeyID` and `SecretHash`, but no plaintext secret field. This makes accidental persistence of the full key harder at the type/interface level.

The plaintext key is returned only during provisioning/generation and is not required for future authentication or revocation.

### 3.5 Constant-time credential comparison

Authentication hashes the supplied secret using the configured pepper and compares the resulting hash with the stored hash using `crypto/subtle.ConstantTimeCompare`.

This avoids using an ordinary string comparison for the credential verification operation.

### 3.6 User ownership

Each API key contains a `user_id` foreign-key relationship to the users table.

Authentication therefore follows:

`API key -> api_keys.user_id -> users.id`

This gives each key exactly one owner while still allowing one user to have multiple API keys.

### 3.7 Middleware-based authentication

Authentication is implemented as HTTP middleware rather than duplicated inside individual handlers.

Protected routes receive:

`Authorization: Bearer <api-key>`

The middleware authenticates the credential before invoking the downstream handler.

On success, the resolved `users.User` is stored in the request context and can be retrieved with `UserFromContext`.

This keeps authentication concerns separate from endpoint business logic.

### 3.8 Application-level revocation

Revocation is implemented through the application method:

`Store.RevokeAPIKey(ctx, keyID)`

The method performs the database update itself and sets `revoked_at = now()` only for keys that are not already revoked.

The administrative HTTP endpoint calls this application-level function rather than directly issuing SQL from the handler.

Authentication checks `RevokedAt` and rejects revoked credentials.

This is important because a database row changing is not, by itself, an application feature. The application now owns the revocation operation and exposes it through an authenticated administrative endpoint.

### 3.9 `last_used_at`

`last_used_at` is updated after successful API key authentication.

The database schema contains the field, and the authentication store performs the application-level update.

The value was manually validated against PostgreSQL:

- Before authentication, `last_used_at` was `NULL`.
- After a successful authenticated `/chat` request, `last_used_at` contained the request time.
- Revocation preserved the usage timestamp and populated `revoked_at`.

### 3.10 Administrative revocation authentication

The revocation endpoint is not exposed as an unauthenticated operation.

It requires an administrative bearer token loaded through configuration.

This prevents an ordinary API key holder from arbitrarily revoking other credentials.

## 4. Testing & validation

The implementation was validated with both normal tests and tests against real PostgreSQL.

### Unit and package tests

Executed:

```text
go test ./...
```

Result:

- All packages passed.
- `internal/api` passed.
- `internal/auth` passed.
- `internal/users` passed.
- Existing project tests passed.
- Packages without tests were reported as `[no test files]`.

### Integration tests

Executed:

```text
go test -tags=integration ./internal/...
```

Result:

- `internal/api` integration tests passed.
- `internal/auth` integration tests passed.
- `internal/provisioning` integration tests passed.
- `internal/users` integration tests passed.

Important API integration cases include:

- Creating an API key in real PostgreSQL.
- Verifying the expected `key_id`.
- Verifying the owning user.
- Verifying active status.
- Verifying the stored value is a hash rather than the plaintext secret.
- Revoking an existing API key.
- Verifying the revoked timestamp.

Important authentication integration cases include:

- Valid API key authenticates successfully.
- Invalid API key returns unauthorized.
- Revoked API key returns unauthorized.
- Missing authorization header is rejected.
- Invalid authorization scheme is rejected.
- Empty bearer token is rejected.
- Authenticated user is resolved correctly.

### Provisioning integration validation

`internal/provisioning/provisioning_integration_test.go` validates the complete provisioning flow against real PostgreSQL.

The test verifies:

- A user is created successfully.
- An API key is created for that user.
- `api_keys.user_id` matches the created `users.id`.
- The API key's `key_id` is persisted.
- The stored secret is a hash.
- The stored hash is not the plaintext API key.
- The expected user fields are persisted.
- Test records are cleaned up after execution.

### Manual HTTP validation

The server was run in development mode and the following routes were manually exercised:

- `POST /provision/user`
- `POST /chat`
- `GET /events`
- `POST /admin/api-keys/revoke`

Manual validation confirmed:

- Missing credentials return HTTP 401.
- Invalid credentials return HTTP 401.
- A valid credential reaches the application authentication path.
- `/events` exposes the request lifecycle events.
- A missing Vault/OpenAI secret produces the expected upstream/configuration failure rather than bypassing authentication.
- Provisioning generates an API key that can subsequently be used for authentication.
- Revocation causes the database `revoked_at` field to be populated.
- Authentication rejects the revoked key.

### Formatting and build validation

Executed:

```text
gofmt -w .
go test ./...
go test -tags=integration ./internal/...
```

All commands completed successfully.

## 6. Integration/setup details

### Environment

The application uses PostgreSQL for users and API key records.

Required configuration includes:

```text
DATABASE_URL=<postgres connection string>
API_KEY_PEPPER=<secret pepper>
```

The administrative revocation endpoint additionally requires the configured administrative token.

### Starting the server

From the repository root:

```bash
go run ./cmd/server
```

Development mode exposes the configured HTTP server, including:

```text
POST /provision/user
POST /chat
GET  /events
GET  /dashboard
POST /admin/api-keys/revoke
```

### Provisioning a user and key

Example request:

```bash
curl -i -X POST http://localhost:3001/provision/user \
  -H 'Content-Type: application/json' \
  -d '{
    "username": "review-user",
    "email": "review@example.com",
    "key_name": "review-key"
  }'
```

The response contains the user ID, key ID, and plaintext API key.

The plaintext key should be treated as a secret and should not be committed, logged, or stored in source control.

### Authentication

A protected request uses:

```text
Authorization: Bearer <full-api-key>
```

### Revocation

Administrative revocation uses the application endpoint:

```bash
curl -i -X POST http://localhost:3001/admin/api-keys/revoke \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"key_id":"<key-id>"}'
```

The endpoint invokes the application-level `RevokeAPIKey` method.

The database can then be checked with:

```sql
SELECT key_id, status, last_used_at, revoked_at
FROM api_keys
WHERE key_id = '<key-id>';
```

A successfully revoked key has a non-NULL `revoked_at`.

A subsequent request using the same API key is rejected by the authentication layer.
