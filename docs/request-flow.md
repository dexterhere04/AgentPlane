# Request Flow

The complete lifecycle of a `POST /chat` request through AgentPlane.

## Sequence Diagram

```
Client
  |
  | POST /chat (JSON body)
  | Authorization: Bearer ap_live_<key_id>_<secret>
  |
  v
Authenticator.Middleware()          [internal/auth/middleware.go]
  |
  | 1. Require Authorization: Bearer <api-key>
  | 2. Authenticate (parse key → hash → constant-time compare → resolve user)
  | 3. Attach user to request context
  |    - failure → 401 Unauthorized
  |
  v
handlers.Chat()                     [internal/handlers/chat.go]
  |
  | 4. Validate HTTP method (POST) → else 405
  | 5. Read request body → read failure 500
  | 6. Validate JSON → invalid 400
  |
  | 7. Run input guardrails (enforcement.Evaluate, DirectionInput)
  |    - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |    - Required guardrail error → 503 (fail-closed)
  |
  | 8. provider.Forward(ctx, body, requestID)
  |
  v
proxy.OpenAIProvider.Forward()      [internal/proxy/openai.go]
  |
  | 9. Check API key set → empty error
  | 10. Dispatch streaming vs non-streaming
  | 11. Build request, set Content-Type + Authorization headers
  | 12. Send (60s timeout)
  | 13. Read response; non-200 → error
  |     (streaming: consume SSE, rebuild one completion)
  |
  v
Back in handlers.Chat()
  |
  | 14. Proxy error → 502 Bad Gateway
  | 15. Run output guardrails (enforcement.Evaluate, DirectionOutput)
  |     - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |     - Required guardrail error → 503 (fail-closed)
  | 16. Write 200 + response body
  |
  v
Client receives response
```

## Guardrail Enforcement Flow (steps 7 & 15)

For each `GuardrailSpec` in the `GuardrailSet`, the `EnforcementPoint` performs:

1. Resolve guardrail by name from the Registry.
   - Not found + `Required` → return `DecisionBlock` (fail-closed) → 503.
   - Not found + optional → skip.
2. For `Required` guardrails: evaluate; on error → fail-closed → 503.
3. For optional guardrails: check `Strategy.Enabled` → skip if disabled.
4. Call `guardrail.Evaluate(ctx, dir, body)` → record metrics and latency.
   - On error → skip and continue (fail-open).
5. Publish the raw result to EventBus.
6. Resolve effective decision via mode (`enforce`/`log_only`/`warn`):
   - `enforce` → raw result unchanged
   - `log_only` → downgrade to `DecisionLogOnly`
   - `warn` → downgrade `Block` → `Warn`, `Redact` → `LogOnly`
7. On `DecisionBlock` → stop pipeline, return (handler returns 403).
8. On `DecisionRedact` → update body, continue.
9. On `DecisionPass`/`Warn`/`LogOnly` → continue.

> Note: `main.go` currently registers all guardrails as optional (`Required` is unset), so individual guardrails run only when enabled via `GUARDRAIL_*` env vars and errors fail-open. The fail-closed (`Required`) path is implemented but not wired by default.

## Error Paths

| Step | Error Condition | HTTP Status | Body |
|------|----------------|-------------|------|
| 1–3 | Missing/invalid API key | 401 | `invalid API key` (or similar) |
| 4 | Method is not POST | 405 | `Method not allowed` |
| 5 | Body read fails | 500 | `Failed to read request body` |
| 6 | Body is not valid JSON | 400 | `Invalid JSON in request body` |
| 7 | Input guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 7 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |
| 9 | API key not set | 502 | `OPENAI_API_KEY is not set` |
| 11–13 | Request/network/read/non-200 | 502 | wrapped error message |
| 15 | Output guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 15 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |

## Timing

- **Server startup:** establishes a Postgres pool, runs migrations, wires all packages, starts the mux.
- **Request timeout:** 60 seconds (proxy HTTP client).
- **No request timeout on the server side** (inherits Go's default, which is none).
