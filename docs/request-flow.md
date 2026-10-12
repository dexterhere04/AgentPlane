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
  | 8. Resolve effective model (request "model" or DEFAULT_MODEL), then
  |    authorize it (policyEP.Authorize, permission "model:<name>")
  |    - no resolvable model → 400 model_required
  |    - deny → 403 policy_denied, evaluation error → 503 policy_unavailable
  |
  | 9. Run input guardrails (enforcement.Evaluate, DirectionInput)
  |    - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |    - Required guardrail error → 503 (fail-closed)
  |
  | 10. Build RouteRequest (model, user, roles) into context,
  |     then provider.Forward(ctx, body, requestID) → Router
  |
  v
proxy.Router.Forward()              [internal/proxy/router.go]
  |
  | 11. Extract RouteRequest from context
  | 12. Match routing rule by priority (first match wins)
  |     - no rule → default provider  |  deny rule → 503 (routing denied)
  | 13. Resolve model + provider group from rule
  | 14. Registry.Candidates(model, group) → eligible upstreams
  |     - none eligible → 503 (no provider)
  | 15. WeightedSelector selects the primary upstream
  |
  v
proxy.Upstream.ForwardWithConfig()  [internal/proxy/upstream.go]
  |
  | 16. Apply per-route timeout (context deadline)
  | 17. Retry retryable failures with backoff + jitter (bounded)
  | 18. Trip/recover circuit breaker on 5xx/failures
  | 19. On exhaustion → Router.Failover to next upstream in chain
  |     - failover emits routing_failover event + increments counters
  |
  v
proxy.OpenAIProvider.Forward()      [internal/proxy/openai.go]
  |
  | 20. Build request, set Content-Type + Authorization headers
  | 21. Send (60s provider-client timeout)
  | 22. Read response; non-200 → error
  |     (streaming: consume SSE, rebuild one completion)
  |
  v
Back in handlers.Chat()
  |
  | 23. Proxy/routing error → 502/503
  | 24. Run output guardrails (enforcement.Evaluate, DirectionOutput)
  |     - Block → 403, Redact → rewrite body, Warn/LogOnly → continue
  |     - Required guardrail error → 503 (fail-closed)
  | 25. Write 200 + response body
  |
  v
Client receives response
```

Routing is a forwarding concern: authorization (steps 4 & 8) has already decided *whether*
the request may proceed; the router decides *where* to send it.

## Policy Enforcement Flow (steps 4 & 8)

Authorization is deny-by-default: a user with no assigned role holds no permissions.

1. `EnforcementPoint.Require("chat:invoke")` runs after authentication. It reads the user from context (401 if absent) and calls `Authorize`.
2. `Authorize` runs each registered `Policy` in order; the first explicit deny wins. A policy error fails closed and is surfaced as 503.
3. RBAC (`internal/policy/rbac`) loads the user's effective permissions from `user_roles → role_permissions` and allows when any granted permission matches the requested one (`*` matches all; `model:*` is a prefix wildcard).
4. Inside `handlers.Chat`, after the body is validated, the effective model is resolved (the request `model` field, or `DEFAULT_MODEL` when the request omits it) and the same enforcement point authorizes `model:<name>`. A request with no resolvable model is rejected with `400 model_required` rather than forwarded.

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
| 8 | No resolvable model | 400 | `{"error":{"type":"model_required",...}}` |
| 8 | Model not permitted | 403 | `{"error":{"type":"policy_denied",...}}` |
| 8 | Policy evaluation error | 503 | `{"error":{"type":"policy_unavailable",...}}` |
| 9 | Input guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 9 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |
| 12 | No rule matches → default provider | — | (falls through) |
| 12 | `deny` / `force_deny` rule matches | 503 | routing denied |
| 14 | No eligible upstream (registry empty / all circuit-open) | 503 | no provider |
| 16–19 | Upstream request/network/non-200, retries exhausted, all candidates failed | 502 | wrapped error message |
| 24 | Output guardrail blocks | 403 | `{"error":{"type":"guardrail_blocked",...}}` |
| 24 | Required guardrail error | 503 | `{"error":{"type":"guardrail_unavailable",...}}` |

## Timing

- **Server startup:** establishes a Postgres pool, runs migrations, wires all packages, starts the mux.
- **Provider-client timeout:** 60 seconds (proxy HTTP client default).
- **Per-route timeouts:** applied by the upstream from the matched route/provider
  `timeout_ms` (or its override).
- **No request timeout on the server side** (inherits Go's default, which is none).
- **Retry backoff:** exponential with jitter, capped at the configured max; bounded by
  `max_retries` (see `resilience-and-retry.md`).
