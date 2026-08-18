# Package: `handlers`

**Files:** `internal/handlers/chat.go`, `provision.go`, `revoke.go`, `metrics.go`

**Package:** `handlers`

## Overview

Handles HTTP requests. Concerned only with HTTP — validates the request, reads the body, calls the guardrail enforcement layer, forwards to the proxy layer, and writes the response. Knows nothing about OpenAI or provider-specific logic.

## Functions

### `Chat()`

```go
func Chat(
    w http.ResponseWriter,
    r *http.Request,
    enforcement *guardrail.EnforcementPoint,
    inputSet guardrail.GuardrailSet,
    outputSet guardrail.GuardrailSet,
    provider proxy.Provider,
)
```

**File:** `internal/handlers/chat.go`

**Registered at:** `cmd/server/main.go`, wrapped in `authenticator.Middleware`.

**Behavior (step by step):**

| Step | Action | Error Response |
|------|--------|---------------|
| 1 | Publishes `request_received` + authenticated-user events | — |
| 2 | Validates HTTP method is POST | 405 Method Not Allowed |
| 3 | Reads entire request body with `io.ReadAll(r.Body)` | 500 Failed to read request body |
| 4 | Validates body is valid JSON with `json.Valid(body)` | 400 Invalid JSON in request body |
| 5 | Runs input guardrails (`enforcement.Evaluate`, `DirectionInput`) | 403 (blocked) or 503 (Required guardrail error) |
| 6 | Calls `provider.Forward(ctx, body, requestID)` | 502 with error message |
| 7 | Runs output guardrails (`enforcement.Evaluate`, `DirectionOutput`) | 403 (blocked) or 503 (Required guardrail error) |
| 8 | Writes 200 + response body (possibly redacted) | — |

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

Returns a JSON snapshot from `guardrail.DefaultMetrics.Snapshot()`:

```json
{
  "guardrail_evaluations_total": 0,
  "guardrail_blocks_total": 0,
  "guardrail_redactions_total": 0,
  "guardrail_warns_total": 0,
  "guardrail_passed_total": 0,
  "guardrail_errors_total": 0,
  "guardrail_evaluation_duration_avg_ms": 0
}
```

## Helper functions

- `writeGuardrailBlock()` — writes a 403 JSON error of `type: "guardrail_blocked"` with guardrail name, message, and findings.
- `writeGuardrailError()` — writes a 503 JSON error of `type: "guardrail_unavailable"`.

## Must NOT

- Know how OpenAI works
- Hardcode provider URLs
- Store or manage API keys
