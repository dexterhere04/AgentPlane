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
policy.EnforcementPoint.Require("chat:invoke")   [internal/policy/middleware.go]
  |
  | 4. Authorize the authenticated user for chat:invoke (RBAC)
  |    - deny → 403 policy_denied, evaluation error → 503 policy_unavailable
  |
  v
handlers.Chat()                     [internal/handlers/chat.go]
  |
  | 5. Validate HTTP method (POST) → else 405
  | 6. Read request body → read failure 500
  | 7. Validate JSON → invalid 400
  |
  | 8. Authorize model (policyEP.Authorize, permission "model:<name>")
  |    - deny → 403 policy_denied, evaluation error → 503 policy_unavailable
  |    - no "model" field → skipped
  |
  | 9. Run input guardrails (enforcement.Evaluate, DirectionInput)
  |    - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |    - Required guardrail error → 503 (fail-closed)
  |
  | 10. provider.Forward(ctx, body, requestID)
  |
  v
proxy.OpenAIProvider.Forward()      [internal/proxy/openai.go]
  |
  | 11. Check API key set → empty error
  | 12. Dispatch streaming vs non-streaming
  | 13. Build request, set Content-Type + Authorization headers
  | 14. Send (60s timeout)
  | 15. Read response; non-200 → error
  |     (streaming: consume SSE, rebuild one completion)
  |
  v
Back in handlers.Chat()
  |
  | 16. Proxy error → 502 Bad Gateway
  | 17. Run output guardrails (enforcement.Evaluate, DirectionOutput)
  |     - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |     - Required guardrail error → 503 (fail-closed)
  | 18. Write 200 + response body
  |
  v
Client receives response
```

## Policy Enforcement Flow (steps 4 & 8)

Authorization is deny-by-default: a user with no assigned role holds no permissions.

1. `EnforcementPoint.Require("chat:invoke")` runs after authentication. It reads the user from context (401 if absent) and calls `Authorize`.
2. `Authorize` runs each registered `Policy` in order; the first explicit deny wins. A policy error fails closed and is surfaced as 503.
3. RBAC (`internal/policy/rbac`) loads the user's effective permissions from `user_roles → role_permissions` and allows when any granted permission matches the requested one (`*` matches all; `model:*` is a prefix wildcard).
4. Inside `handlers.Chat`, after the body is validated, the same enforcement point authorizes `model:<name>` using the `model` field from the request.

> The `chat:invoke` check is static (middleware, before the body is read); the model check is dynamic (needs the parsed body).

## Guardrail Enforcement Flow (steps 9 & 17)

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
| 4 | Not permitted `chat:invoke` | 403 | `{"error":{"type":"policy_denied",...}}` |
| 4 | Policy evaluation error | 503 | `{"error":{"type":"policy_unavailable",...}}` |
| 5 | Method is not POST | 405 | `Method not allowed` |
| 6 | Body read fails | 500 | `Failed to read request body` |
| 7 | Body is not valid JSON | 400 | `Invalid JSON in request body` |
| 8 | Model not permitted | 403 | `{"error":{"type":"policy_denied",...}}` |
| 8 | Policy evaluation error | 503 | `{"error":{"type":"policy_unavailable",...}}` |
| 9 | Input guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 9 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |
| 11 | API key not set | 502 | `OPENAI_API_KEY is not set` |
| 13–15 | Request/network/read/non-200 | 502 | wrapped error message |
| 17 | Output guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 17 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |

## Timing

- **Server startup:** establishes a Postgres pool, runs migrations, wires all packages, starts the mux.
- **Request timeout:** 60 seconds (proxy HTTP client).
- **No request timeout on the server side** (inherits Go's default, which is none).
