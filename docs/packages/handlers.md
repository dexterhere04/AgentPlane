# Package: `handlers`

**Files:** `internal/handlers/chat.go`, `provision.go`, `revoke.go`, `apikeys.go`,
`metrics.go`, `analytics.go`, `user_analytics.go`, `routing.go`, `secrets.go`,
`roles.go`, `users.go`

**Package:** `handlers`

## Overview

Handles HTTP requests. Concerned only with HTTP — validates the request, reads the body,
calls the guardrail enforcement layer, forwards to the proxy/router layer, and writes the
response. Knows nothing about OpenAI or provider-specific logic.

Endpoint functions fall into three groups: the chat pipeline, admin CRUD (roles, users,
providers, routing rules), and observability (metrics, analytics).

## Functions

### `Chat()` / `ChatWithRouting()`

```go
func Chat(w, r, enforcement, policyEP, inputSet, outputSet, provider proxy.Provider)
func ChatWithRouting(
    w, r, enforcement, policyEP, inputSet, outputSet,
    provider proxy.Provider,
    rbacStore *rbac.Store,
    providerRegistry *proxy.Registry,
    selector *proxy.WeightedSelector,
)
```

**File:** `internal/handlers/chat.go`

**Registered at:** `cmd/server/main.go`, wrapped in `authenticator.Middleware` and
`policyEP.Require("chat:invoke")`. The server registers `ChatWithRouting`, passing the
`Router` as the `provider` (the router implements `proxy.Provider`), so the handler is
provider-agnostic. `Chat()` is the fallback that supplies no routing/RBAC dependencies.

**Behavior (step by step):**

| Step | Action | Error Response |
|------|--------|---------------|
| 1 | Publishes `request_received` + authenticated-user events | — |
| 2 | Validates HTTP method is POST | 405 Method Not Allowed |
| 3 | Reads entire request body with `io.ReadAll(r.Body)` | 500 Failed to read request body |
| 4 | Validates body is valid JSON with `json.Valid(body)` | 400 Invalid JSON in request body |
| 5 | Resolves the effective model (`proxy.ApplyDefaultModel`), loads the user's roles for routing | 503 routing unavailable |
| 6 | Builds a `proxy.RouteRequest` (model, user, roles) into the context | — |
| 7 | Runs the model authorization check (`policyEP.Authorize`, `model:<name>`) | 400/403 (policy denied) |
| 8 | Runs input guardrails (`enforcement.Evaluate`, `DirectionInput`) | 403 (blocked) or 503 (Required guardrail error) |
| 9 | Calls `provider.Forward(ctx, body, requestID)` — the Router matches a rule, selects an upstream, and forwards with timeout/retry/failover | 502 (upstream error), 503 (`no provider`), 503 (`routing denied`) |
| 10 | Runs output guardrails (`enforcement.Evaluate`, `DirectionOutput`) | 403 (blocked) or 503 (Required guardrail error) |
| 11 | Writes 200 + response body (possibly redacted) | — |

**Guardrail handling details:**

- If `enforcement` is `nil` or a `GuardrailSet` has no guards, that direction's evaluation is skipped.
- On `DecisionBlock`: returns 403 with JSON error body including guardrail name, message, and findings.
- On `DecisionRedact`: replaces body with redacted version, continues pipeline.
- On `DecisionWarn`: publishes a warn event, continues pipeline.
- On guardrail error (Required guardrail): returns 503 with `guardrail_unavailable` error.

### `ProvisionUser()`

```go
func ProvisionUser(provisioner *provisioning.Provisioner) http.Handler
```

**File:** `internal/handlers/provision.go`

Handles `POST /provision/user`. Decodes `{username, email?, key_name}`, calls `provisioner.ProvisionUserWithAPIKey`, and returns `{user_id, key_id, api_key}` with 201. The full `api_key` is returned exactly once.

### `RevokeAPIKey()`

```go
func RevokeAPIKey(apiKeys *api.Store) http.Handler
```

**File:** `internal/handlers/revoke.go`

Handles `POST /admin/api-keys/revoke`. Decodes `{key_id}`, calls `apiKeys.RevokeAPIKey`, and returns `{key_id, status:"revoked"}`. Returns 404 for unknown/already-revoked keys.

### `MetricsHandler()`

```go
func MetricsHandler() http.HandlerFunc
```

**File:** `internal/handlers/metrics.go`

Returns a JSON snapshot merging `guardrail.DefaultMetrics.Snapshot()` and
`proxy.DefaultMetrics.Snapshot()`:

```json
{
  "guardrail_evaluations_total": 0,
  "guardrail_blocks_total": 0,
  "guardrail_redactions_total": 0,
  "guardrail_warns_total": 0,
  "guardrail_passed_total": 0,
  "guardrail_errors_total": 0,
  "guardrail_evaluation_duration_avg_ms": 0,
  "routing_decisions_total": 0,
  "routing_failovers_total": 0,
  "routing_retries_total": 0,
  "routing_breaker_trips_total": 0,
  "routing_breaker_recoveries_total": 0,
  "routing_no_provider_total": 0,
  "routing_denied_total": 0,
  "routing_upstreams": []
}
```

Routing counter semantics are documented in `routing-metrics.md`.

### `RoutingAdmin` (`routing.go`)

```go
func NewRoutingAdmin(providers *proxy.ProviderStore, rules *proxy.RoutingRuleStore,
    registry *proxy.Registry, reload func(context.Context) error) *RoutingAdmin
```

Handles the provider and routing-rule CRUD surface:

| Method | Path |
|--------|------|
| GET/POST | `/admin/providers`, `/admin/routing-rules` |
| GET/PUT | `/admin/providers/{id}`, `/admin/routing-rules/{id}` |
| POST | `/admin/providers/{id}/enable`, `/admin/providers/{id}/disable` |
| POST | `/admin/routing-rules/{id}/enable`, `/admin/routing-rules/{id}/disable` |
| DELETE | `/admin/providers/{id}`, `/admin/routing-rules/{id}` |
| GET | `/admin/upstreams` (read-only live registry snapshot) |

Every mutation persists and then invokes the `reload` callback to rebuild the runtime
registry + rule set. Schemas: [Routing Administration](../routing-admin.md).

### `AnalyticsHandler` (`analytics.go`)

```go
func AnalyticsHandler(chURL string, defaultType string) http.Handler
```

Proxies whitelisted ClickHouse queries to `POST {chURL}/?query=...` with basic auth, adding
`FORMAT JSON`. Serves `/admin/analytics?type=...&hours=N` and the pinned
`/analytics/traces_count`. Query types and shapes: [Routing Metrics](../routing-metrics.md).

### `UserAnalyticsHandler` (`user_analytics.go`)

Serves `/admin/analytics/users` — per-user spend, token usage, latency, and a searchable
prompt log from ClickHouse.

### `ListAPIKeys` (`apikeys.go`)

Serves `GET /admin/api-keys` with non-secret key metadata (id, user, active/revoked state).

### `StoreProviderSecret` (`secrets.go`)

Handles `POST /admin/secrets/provider`. Maps a provider slug to a secret-store key name
(`providerSecretKeys`, extensible) and writes it via the configured `secrets.SecretWriter`.
A caller may override the key name with `secret_key`.

## Helper functions

- `writeGuardrailBlock()` — writes a 403 JSON error of `type: "guardrail_blocked"` with guardrail name, message, and findings.
- `writeGuardrailError()` — writes a 503 JSON error of `type: "guardrail_unavailable"`.

## Must NOT

- Know how OpenAI works
- Hardcode provider URLs
- Store or manage API keys
