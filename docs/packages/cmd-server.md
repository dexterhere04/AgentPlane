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
4. Build the stores: `users.NewStore`, `api.NewStore`, `auth.NewStore`, `rbac.Store`.
5. Read `config.AdminToken()`.
6. Configure the secret store via `config.ConfigureSecretStore()` and load `config.KeyPepper()`.
7. Construct the `provisioning.Provisioner` and `auth.Authenticator`.
8. Wire the routing stack: `ProviderStore` + `RoutingRuleStore` backed by PostgreSQL, a `proxy.Registry` of runtime upstreams, a `weightedSelector` (`proxy.NewWeightedSelector`), and a `proxy.Router` that implements `proxy.Provider`. A `reload` callback rebuilds the registry/rule set after admin mutations.
9. Load guardrail config, register all guardrails, and create the `EnforcementPoint` plus the input/output `GuardrailSet`s.
10. Register the `adminAuth` helper: authorizes the static root admin token (constant-time compare) **or** a database-backed API key whose user holds the `admin` role. `adminOrRBAC` requires a header; `adminOrRBACQuery` also accepts `?token=` for browser clients.
11. Register routes on an `http.ServeMux` and call `http.ListenAndServe(":"+port, mux)`.

## Routes

| Path | Handler | Auth |
|------|---------|------|
| `/chat` | `handlers.ChatWithRouting` (router as provider) wrapped in `authenticator.Middleware` + `Require("chat:invoke")` | Bearer API key |
| `/provision/user` | `handlers.ProvisionUser` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/api-keys/revoke` | `handlers.RevokeAPIKey` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/api-keys` | `handlers.ListAPIKeys` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/roles` | role CRUD wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/users/{id}/roles` | role assignment wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/secrets/provider` | `handlers.StoreProviderSecret` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/providers` [+`/{id}`, `/{id}/enable`, `/{id}/disable`] | `routingAdmin` CRUD wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/upstreams` | `routingAdmin.ListUpstreams` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/routing-rules` [+`/{id}`, `/{id}/enable`, `/{id}/disable`] | `routingAdmin` CRUD wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/admin/analytics` | `handlers.AnalyticsHandler(chURL, "")` wrapped in `adminOrRBAC` (registered only when `CH_URL`/ClickHouse is configured) | admin token / admin-role key |
| `/admin/analytics/users` | `handlers.UserAnalyticsHandler` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/analytics/traces_count` | `handlers.AnalyticsHandler(chURL, "traces_count")` wrapped in `adminOrRBAC` | admin token / admin-role key |
| `/events` | `observability.SSEHandler` wrapped in `adminOrRBACQuery` | admin token / admin-role key (`?token=` allowed) |
| `/dashboard` | inline handler serving `dashboard.HTML` wrapped in `adminOrRBACQuery` | admin token / admin-role key (`?token=` allowed) |
| `/metrics` | `handlers.MetricsHandler` wrapped in `adminOrRBAC` | admin token / admin-role key |

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
