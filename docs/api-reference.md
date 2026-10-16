# API Reference

All endpoints are served from `http://localhost:3001` by default.

## Authentication

- `/chat` requires a bearer API key: `Authorization: Bearer ap_live_<key_id>_<secret>`.
- Admin endpoints accept either the static admin token (`Authorization: Bearer <AGENTPLANE_ADMIN_TOKEN>`) or any database-backed API key whose user holds the `admin` role. The browser-facing `/events` and `/dashboard` also accept the credential via `?token=<...>` (EventSource and top-level navigation cannot set request headers).

Keys are minted via `POST /provision/user`; roles are granted via `POST /admin/users/{id}/roles`.

## Authorization

`/chat` is gated by the policy layer. The authenticated user must hold the `chat:invoke` permission, and a matching `model:<name>` permission for the request's effective model — the request's `model` field, or the configured `DEFAULT_MODEL` when the request omits it. A request with no resolvable model is rejected. Access is **deny-by-default** — a user with no assigned role is permitted nothing. See [Policy Layer & RBAC](policy-rbac.md) for roles and permissions.

Grant access by assigning a role (for example the built-in `member`) via `POST /admin/users/{id}/roles`.

---

## `POST /chat`

Forward a chat completion request through the router to a configured upstream provider.

### Headers

| Header          | Value              | Required |
| --------------- | ------------------ | -------- |
| `Authorization` | `Bearer <api-key>` | Yes      |
| `Content-Type`  | `application/json` | Yes      |

### Request Body

Any valid JSON object. The body is authorized against the policy layer (see
[Authorization](#authorization)), checked by the input guardrails, then
forwarded through the `Router` to the upstream selected for the request's
model/user attributes. Set `"stream": true` to use the streaming path — the
gateway coalesces the stream and returns a single completion (see
`docs/packages/proxy.md`). Routing behavior is described in
[Provider Routing & Router](provider-routing-and-router.md).

**Minimal example:**

```json
{
  "model": "gpt-4o",
  "messages": [{ "role": "user", "content": "Hello, world!" }]
}
```

### Responses

#### 200 OK

The provider response is returned (possibly redacted). Content-Type is `application/json`.

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "gpt-4o",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 10,
    "total_tokens": 30
  }
}
```

#### 400 Bad Request

Returned when the request body is not valid JSON, or when no effective model can be resolved (the request omits `model` and no `DEFAULT_MODEL` is configured).

```
Invalid JSON in request body
```

```json
{
  "error": {
    "type": "model_required",
    "message": "a resolvable model is required to authorize this request"
  }
}
```

#### 401 Unauthorized

Returned when the `Authorization` header is missing, malformed, or the API key is invalid/inactive/revoked/expired.

#### 403 Forbidden — Guardrail Blocked

Returned when a guardrail blocks the request or response.

```json
{
  "error": {
    "type": "guardrail_blocked",
    "message": "Request blocked by guardrail policy",
    "guardrail": "prompt_injection",
    "findings": [
      {
        "guardrail": "prompt_injection",
        "type": "prompt_injection",
        "severity": "high",
        "start": 0,
        "end": 28,
        "entity": "injection_attempt"
      }
    ]
  }
}
```

#### 403 Forbidden — Policy Denied

Returned when the authenticated user is not permitted to perform the request (missing `chat:invoke`, or the requested model is not granted).

```json
{
  "error": {
    "type": "policy_denied",
    "message": "not permitted to perform this action",
    "permission": "model:gpt-4o-mini"
  }
}
```

#### 405 Method Not Allowed

Returned when the HTTP method is not `POST`.

#### 500 Internal Server Error

Returned when the request body cannot be read.

#### 502 Bad Gateway

Returned when the proxy fails to communicate with the upstream provider. The response contains a generic message; raw upstream bodies and connection details are not returned.

#### 503 Service Unavailable — Guardrail Error

Returned when a `Required` guardrail encounters an internal error (fail-closed).

```json
{
  "error": {
    "type": "guardrail_unavailable",
    "message": "Request could not be evaluated by mandatory security controls"
  }
}
```

#### 503 Service Unavailable — Policy Error

Returned when the authorization layer cannot evaluate a request (for example, a database error). Policies fail closed.

```json
{
  "error": {
    "type": "policy_unavailable",
    "message": "authorization could not be evaluated",
    "permission": "chat:invoke"
  }
}
```

### Example: curl

```bash
curl -X POST http://localhost:3001/chat \
  -H "Authorization: Bearer ap_live_<key_id>_<secret>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Say hi"}]
  }'
```

---

## `POST /provision/user`

Create a user and mint an API key. Requires the admin token.

### Request Body

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "key_name": "dev-key"
}
```

| Field      | Type   | Required | Notes                    |
| ---------- | ------ | -------- | ------------------------ |
| `username` | string | Yes      | Unique                   |
| `email`    | string | No       | Unique if set            |
| `key_name` | string | Yes      | Human-readable key label |

### Response — 201 Created

```json
{
  "user_id": "8b0f...",
  "key_id": "fgftrwd",
  "api_key": "ap_live_fgftrwd_..."
}
```

`api_key` is returned exactly once; only `key_id` and a hash are persisted.

---

## `POST /admin/api-keys/revoke`

Revoke an API key by its public `key_id`. Requires the admin token.

### Request Body

```json
{
  "key_id": "fgftrwd"
}
```

### Response — 200 OK

```json
{
  "key_id": "fgftrwd",
  "status": "revoked"
}
```

Returns 404 if the key is not found or already revoked.

---

## Role Management

All endpoints below require the admin token and manage the RBAC policy layer (see [Policy Layer & RBAC](policy-rbac.md)).

### `GET /admin/roles`

List every role with its permissions.

```json
{
  "roles": [
    {"name": "admin", "description": "Full access to every gateway resource", "permissions": ["*"]},
    {"name": "member", "description": "Standard access: invoke chat and use any model", "permissions": ["chat:invoke", "model:*"]}
  ]
}
```

### `POST /admin/roles`

Create a role, optionally with initial permissions.

```json
{
  "name": "gpt4o-only",
  "description": "gpt-4o access",
  "permissions": ["chat:invoke", "model:gpt-4o"]
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | Yes | Unique |
| `description` | string | No | Human-readable |
| `permissions` | string[] | No | Initial permission set |

Response — `201 Created`:

```json
{"name": "gpt4o-only", "description": "gpt-4o access", "permissions": ["chat:invoke", "model:gpt-4o"]}
```

Returns `409` if the role already exists.

### `POST /admin/roles/{name}/permissions`

Add a permission to a role.

```json
{"permission": "model:gpt-4o-mini"}
```

Response — `200 OK`:

```json
{"role": "gpt4o-only", "permission": "model:gpt-4o-mini"}
```

Returns `404` if the role does not exist.

### `DELETE /admin/roles/{name}/permissions/{permission}`

Remove a permission from a role. The `permission` path segment is the permission string verbatim (for example `model:*`); quote it in a shell to avoid glob expansion.

Response — `200 OK`:

```json
{"role": "gpt4o-only", "permission": "model:gpt-4o-mini"}
```

### `GET /admin/users`

List every user with the roles assigned to them. Access is deny-by-default, so a user with an empty `roles` array currently has no access.

```json
{
  "users": [
    {
      "id": "8b0f...",
      "username": "alice",
      "email": "alice@example.com",
      "status": "active",
      "created_at": "2026-01-02T15:04:05Z",
      "roles": ["member"]
    }
  ]
}
```

### `GET /admin/users/{id}/roles`

List the roles assigned to a user.

```json
{"user_id": "8b0f...", "roles": ["member"]}
```

### `POST /admin/users/{id}/roles`

Assign a role to a user. This is how access is granted — users have no roles by default.

```json
{"role": "member"}
```

Response — `200 OK`:

```json
{"user_id": "8b0f...", "role": "member"}
```

Returns `404` if the role does not exist.

### `DELETE /admin/users/{id}/roles/{role}`

Remove a role from a user.

```json
{"user_id": "8b0f...", "role": "member"}
```

### `DELETE /admin/users/{id}`

Delete a user together with every identity bound to that user: their API keys,
IAM policy attachments, role assignments and group memberships. The deletion is
atomic and final — the user's API keys stop authenticating immediately.

Requires `ap:DeleteUser`. Only the built-in `APAdmin` policy grants it by
default (it matches `ap:*`); `ap:ReadOnly` does not.

Response — `204 No Content`. Returns `404` when the user does not exist and
`400` for a malformed id.

---

## `GET /events`

Server-sent event stream of request lifecycle events (including `routing_decision` and `routing_failover` events). Requires the admin token via `Authorization: Bearer <admin-token>` or, for browser clients that cannot set headers, `?token=<admin-token>`. Optionally filter with `?request_id=<id>`.

## `GET /dashboard`

HTML dashboard (embeds the SSE stream and a chat widget). Requires the admin token via `Authorization: Bearer <admin-token>` or, for top-level browser navigation, `?token=<admin-token>`.

## `GET /observability`

Embedded observability view. The operator enters the admin bearer token in-page; it is held in memory and sent only to authenticated analytics API requests. Requires the admin token via `Authorization: Bearer <admin-token>` or, for top-level browser navigation, `?token=<admin-token>`.

## `GET /metrics`

JSON snapshot of metrics, merging guardrail counters (`guardrail_*`) and routing counters (`routing_*`), including a per-upstream breakdown in `routing_upstreams`. Counter semantics are documented in [Routing Metrics](routing-metrics.md). Requires the admin token via `Authorization: Bearer <admin-token>`.

## `GET /admin/analytics`

Windowed analytics over the ClickHouse `agentplane.traces` table. Requires the admin token. Returns ClickHouse `FORMAT JSON` output verbatim.

### Query Params

| Param | Default | Range |
|-------|---------|-------|
| `type` | — | whitelisted query (see below) |
| `hours` | `24` | `1`–`720` (30 days) |

Accepted `type` values and their result shapes are listed in [Routing Metrics](routing-metrics.md) (routing queries such as `provider_traffic`, `route_traffic`, `failover_count`, `failover_transitions`, `tokens_by_provider`, `tokens_by_route`, `cost_by_provider`, `cost_by_route`) plus the general observability queries (`traces_count`, `latency`, `latency_percentiles`, `token_usage`, `cost`, `top_models`, `provider_usage`, `guardrail_events`, `error_rate`, `error_rate_summary`, `tool_calls`). A pinned, single-query route also exists at `GET /analytics/traces_count`.

## `GET /admin/upstreams`

Read-only, poll-safe snapshot of every registered runtime upstream and its live circuit-breaker state. Response shape and semantics: [Routing Administration](routing-admin.md) §5.

## Provider & Routing-Rule Administration

CRUD + enable/disable for `providers` and `routing_rules` (create, read, update, delete on `/admin/providers`, `/admin/providers/{id}`, `/admin/routing-rules`, `/admin/routing-rules/{id}`, with `POST .../enable` and `POST .../disable`). Request/response schemas, matcher JSON forms, and examples: [Routing Administration](routing-admin.md) §3–§4.

## `POST /admin/secrets/provider`

Store a provider API key in the secret store. Requires the admin token.

```json
{ "provider": "openai", "api_key": "<provider-api-key>" }
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `provider` | string | Yes | Provider slug (e.g. `openai`, `anthropic`, `gemini`, ...) mapping to a default secret-store key |
| `secret_key` | string | No | Override the secret-store key name |
| `api_key` | string | Yes | The provider credential |

Provider records reference the stored credential via their `secret_key` field.

## Observability Analytics

Read-only, typed analytics endpoints under `/api/observability/*` require the admin bearer token. Supported ranges are `15m`, `1h`, `6h`, `24h`, and `7d`; common filters include status, provider, model, route, guardrail action, user, organization, and project. The API includes health, overview, series, breakdowns, failures, guardrails, trace listing, and trace detail. Trace detail exposes capture metadata only, never raw prompt/response blobs.

See [observability-dashboard.md](observability-dashboard.md) for request parameters, response fields, privacy behavior, retention, and failure semantics.
