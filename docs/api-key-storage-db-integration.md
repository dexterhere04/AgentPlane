# API Key Storage & Database Integration 
Location: `internal/apikey_management/store.go`, `internal/db/db.go`,
`internal/config/database.go`, `migrations/`

## 1. What was built

A persistence layer so AgentPlane can store API keys as hashes only, never plaintext,
matching the `api_keys` schema (with `users` as its Foreign Key dependency), on top of a shared Postgres connection pool wired into server startup.

This covers:
- Inserting a new API key record (`Store.CreateAPIKey`)
- The shared connection pool (`db.NewPool`)
- Database configuration (`config.DatabaseURL`)
- Migrations to create `users` and `api_keys`
- End-to-end validation against a real, running PostgreSQL instance

## 2. Files delivered

| File | Purpose |
|---|---|
| `internal/apikey_management/store.go` | `Store.CreateAPIKey` — inserts a new `api_keys` row from `KeyID` + `SecretHash` only |
| `internal/db/db.go` | Shared `pgx/v5` connection pool: `NewPool`, pings on creation, closed on shutdown |
| `internal/config/database.go` | `config.DatabaseURL()` — reads and validates the Postgres connection string |
| `migrations/000001_create_users_table.{up,down}.sql` | Creates `users` (dependency of `api_keys` via FK) |
| `migrations/000002_create_api_keys_table.{up,down}.sql` | Creates `api_keys` matching the schema exactly |
| `.env` / `.env.example` | Local database configuration; `.env` is git-ignored, `.env.example` is the non-secret template |

## 3. Decisions and rationale

### 3.1 Driver: pgx / pgxpool
Modern, high-performance Postgres driver with no `database/sql` wrapper overhead.

### 3.2 Storage code location: `internal/apikey_management/store.go`
Kept next to the generation code rather than a separate `internal/store` package, so
everything related to an API key's lifecycle — generate, hash, persist — lives in one
package.

### 3.3 Connection pool location: `internal/db`
The pool itself knows nothing about `api_keys` or `users` — it's a single-responsibility
package (pool setup only) that any future package can depend on, mirroring how `config`
is already shared rather than duplicated per-package.

### 3.4 Migrations: golang-migrate format
Two numbered migrations:
- `000001_create_users_table` — `users` table.
- `000002_create_api_keys_table` — `api_keys` table matching the schema, with a `UNIQUE`
  constraint on `key_id` (which also serves as its index — no separate index needed) and
  a Foreign Key to `users(id)`.

### 3.5 How "never store plaintext" is enforced, not just followed
`CreateAPIKeyParams` (the only way to call `Store.CreateAPIKey`) has no field for a
plaintext secret or full key — only `KeyID` and `SecretHash`. Storing a plaintext secret
is a compile error, not just a convention someone could forget. The returned
`APIKeyRecord` also omits `secret_hash` entirely, so the hash doesn't casually flow
through logs or downstream code that only needs the rest of the record.

## 4. Testing & validation

The implementation was validated against the real AgentPlane codebase and a live
PostgreSQL 18.4 database. `go build ./...`, `go vet ./...`, and the complete `go test ./...` suite all pass.

- Migrations were run against an isolated `agentplane_test` database, successfully
  creating `users` and `api_keys` with the expected UUID defaults, constraints, unique
  indexes, and FK relationship.
- A dedicated integration test exercised `Store.CreateAPIKey` through the real pgx/v5
  connection and confirmed API-key records persist correctly while the plaintext secret
  is never stored.

### 4.1 Bugs found and fixed during validation

**Test bug — invalid delimiter assumption.**
`TestGenerateAPIKey_Format` originally split the generated key on `_` and assumed
exactly four parts (`ap`, `live`, `key_id`, `secret`). This is wrong: base64url
permits `_` as a valid character *within* `key_id` or `secret`, so a legitimately
formatted key like `ap_live_fgftrwd_bdenkxmw_fekdmlweldf` can split into more than
four parts even though it correctly follows `ap_live_<key_id>_<secret>`. Fixed by
building the expected string directly from the already-generated `KeyID` and `Secret`
and comparing it to `FullKey`, instead of parsing the key apart — this tests the
actual contract of the generator without imposing an invalid character restriction.

## 5. Database integration setup

AgentPlane now has a dedicated Postgres connection wired into the running server, plus
a persistent local development database separate from the disposable test database.

- `DATABASE_URL` is supplied via a local `.env` file, git-ignored via `.gitignore`.
- `config.DatabaseURL()` reads and validates `DATABASE_URL` without hardcoding
  credentials anywhere in source.
- `db.NewPool()` creates the shared pool and verifies connectivity with `Ping`.
- The server loads `.env` at startup, initializes the pool via
  `config.DatabaseURL()` + `db.NewPool()`, and closes the pool on shutdown.
- `agentplane` was created as the persistent local dev database, kept separate from
  `agentplane_test` so the test database can stay disposable. Both migrations were
  applied successfully to `agentplane`, creating `users`, `api_keys`, and
  `schema_migrations`.

## 6. Next steps

**User creation dependency.** `api_keys.user_id` is a required FK referencing
`users.id`, so an API key can only be created after an application user exists. The
intended production flow:

1. Create user
2. Obtain the new user's ID
3. Generate an API key (`GenerateAPIKey`)
4. Persist `key_id` and `secret_hash` against that user (`Store.CreateAPIKey`)
5. Return the complete plaintext key to the user exactly once, then discard it

