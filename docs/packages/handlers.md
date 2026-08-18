# Package: `handlers`

**Files:** `internal/handlers/chat.go`, `internal/handlers/metrics.go`

**Package:** `handlers`

## Overview

Handles HTTP requests. Concerned only with HTTP — validates the request, reads the body, calls the guardrail enforcement layer, forwards to the proxy layer, and writes the response. Knows nothing about OpenAI or provider-specific logic.

## Imports

| Import | Usage |
|--------|-------|
| `context` | `context.Background()` for guardrail enforcement calls |
| `encoding/json` | `json.Valid()` to validate request body, encode error responses |
| `fmt` | `fmt.Sprintf()` for error formatting and request IDs |
| `io` | `io.ReadAll()` to read the request body |
| `log` | `log.Printf()` for error logging |
| `net/http` | `http.ResponseWriter`, `http.Request`, `http.Error()`, status codes |
| `time` | Timing for observability events |
| `github.com/dexterhere04/AgentPlane/internal/guardrail` | Guardrail enforcement types: `EnforcementPoint`, `GuardrailSet`, `Direction`, `Result` |
| `github.com/dexterhere04/AgentPlane/internal/observability` | Event publishing for observability |
| `github.com/dexterhere04/AgentPlane/internal/proxy` | Call `provider.Forward()` |

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

**Line:** `internal/handlers/chat.go:17`

**Registered at:** `cmd/server/main.go` as a closure wrapping `handlers.Chat`.

**Behavior (step by step):**

| Step | Action | Error Response |
|------|--------|---------------|
| 1 | Validates HTTP method is POST | 405 Method Not Allowed |
| 2 | Reads entire request body with `io.ReadAll(r.Body)` | 500 Failed to read request body |
| 3 | Defers `r.Body.Close()` | — |
| 4 | Publishes parsed body to EventBus (if valid JSON) | — |
| 5 | Validates body is valid JSON with `json.Valid(body)` | 400 Invalid JSON in request body |
| 6 | Runs input guardrails via `enforcement.Evaluate(ctx, requestID, DirectionInput, body, inputSet)` | 403 Forbidden (blocked) or 503 (guardrail error) |
| 7 | Calls `provider.Forward(ctx, body, requestID)` to forward to provider | 502 with error message |
| 8 | Runs output guardrails via `enforcement.Evaluate(ctx, requestID, DirectionOutput, respBody, outputSet)` | 403 Forbidden (blocked) or 503 (guardrail error) |
| 9 | Sets `Content-Type: application/json` header | — |
| 10 | Writes HTTP 200 status code | — |
| 11 | Writes the response body | — |

**Returns:** Nothing. Writes HTTP response directly.

**Called by:** The Go HTTP server when a request hits `POST /chat`.

**Guardrail handling details:**

- If `enforcement` is `nil` or a `GuardrailSet` has no guards, that direction's guardrail evaluation is skipped.
- On `DecisionBlock`: returns 403 with JSON error body including guardrail name, message, and findings.
- On `DecisionRedact`: replaces body with redacted version, continues pipeline.
- On `DecisionWarn`: publishes a warn event to EventBus, continues pipeline.
- On guardrail error (mandatory guardrail): returns 503 with `guardrail_unavailable` error.

### `writeGuardrailBlock()`

Writes a 403 Forbidden response with structured JSON:

```json
{
  "error": {
    "type": "guardrail_blocked",
    "message": "guardrail blocked: <reason>",
    "guardrail": "<name>",
    "findings": [...]
  }
}
```

### `writeGuardrailError()`

Writes a 503 Service Unavailable response when a mandatory guardrail encounters an error:

```json
{
  "error": {
    "type": "guardrail_unavailable",
    "message": "Request could not be evaluated by mandatory security controls"
  }
}
```

### `MetricsHandler()`

**Line:** `internal/handlers/metrics.go`

Returns a Prometheus-style metrics endpoint at `GET /metrics` that includes all guardrail counters from `guardrail.DefaultMetrics.Snapshot()`:
- `guardrail_evaluations_total`
- `guardrail_blocks_total`
- `guardrail_redactions_total`
- `guardrail_warns_total`
- `guardrail_passed_total`
- `guardrail_errors_total`
- `guardrail_evaluation_duration_avg_ms`

**Calls:**
- `io.ReadAll(r.Body)`
- `json.Valid(body)`
- `enforcement.Evaluate(ctx, requestID, direction, body, set)`
- `provider.Forward(ctx, body, requestID)`
- `w.Header().Set()`, `w.WriteHeader()`, `w.Write()`
- `http.Error()`

**Purpose:** Acts as the "receptionist" — validates incoming HTTP traffic, enforces guardrail policies, and delegates to the right back-end component. Ensures only valid POST requests with parseable JSON bodies that pass security checks reach the provider proxy.

**Must NOT:**
- Know how OpenAI works
- Hardcode provider URLs
- Store or manage API keys
