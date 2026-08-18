# Package: `cmd/server`

**File:** `cmd/server/main.go`

**Package:** `main`

## Overview

The application entry point and composition root. Loads configuration, connects to PostgreSQL, applies migrations, wires together all packages (auth, provisioning, guardrails, proxy), registers routes, and starts the HTTP server on port 3001.

There is also a second executable, `cmd/keygeneration/main.go`, which generates a single API key and prints it (full key, key ID, and secret hash).

## Startup sequence

1. Load `.env` via `godotenv.Load()` (missing file is non-fatal).
2. Read `config.DatabaseURL()` and create the pool via `db.NewPool()`.
3. Apply embedded migrations via `db.Migrate(ctx, pool, migrations.FS)`.
4. Build the stores: `users.NewStore`, `api.NewStore`, `auth.NewStore`.
5. Read `config.AdminToken()`.
6. Configure the secret store via `config.ConfigureSecretStore()` and load `config.KeyPepper()`.
7. Construct the `provisioning.Provisioner` and `auth.Authenticator`.
8. Build the provider via `proxy.NewOpenAIProviderFromEnv()`.
9. Load guardrail config, register all guardrails, and create the `EnforcementPoint` plus the input/output `GuardrailSet`s.
10. Register routes on an `http.ServeMux` and call `http.ListenAndServe(":"+port, mux)`.

## Routes

| Path | Handler | Auth |
|------|---------|------|
| `/chat` | `handlers.Chat` wrapped in `authenticator.Middleware` | Bearer API key |
| `/provision/user` | `handlers.ProvisionUser` wrapped in `auth.AdminMiddleware` | Bearer admin token |
| `/admin/api-keys/revoke` | `handlers.RevokeAPIKey` wrapped in `auth.AdminMiddleware` | Bearer admin token |
| `/events` | `observability.SSEHandler` | — |
| `/dashboard` | inline handler serving `dashboard.HTML` | — |
| `/metrics` | `handlers.MetricsHandler` | — |

## Functions

### `main()`

**Signature:** `func main()`

**Behavior:** Wires the full dependency graph (described above) and starts the server. Logs the gateway, events, dashboard, metrics, and mock-API URLs.

**Returns:** Does not return. Exits fatally on any startup error.

**Must NOT:**
- Call OpenAI directly
- Read request bodies
- Implement business logic
- Store configuration
