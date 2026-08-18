# API Reference

All endpoints are served from `http://localhost:3001` by default.

## Authentication

- `/chat` requires a bearer API key: `Authorization: Bearer ap_live_<key_id>_<secret>`.
- Admin endpoints require the admin token: `Authorization: Bearer <AGENTPLANE_ADMIN_TOKEN>`.

Keys are minted via `POST /provision/user`.

---

## `POST /chat`

Forward a chat completion request to the OpenAI-compatible upstream provider.

### Headers

| Header | Value | Required |
|--------|-------|----------|
| `Authorization` | `Bearer <api-key>` | Yes |
| `Content-Type` | `application/json` | Yes |

### Request Body

Any valid JSON object. The body is forwarded (possibly after guardrail redaction) to `{OPENAI_BASE_URL}/chat/completions` (default `https://api.openai.com/v1/chat/completions`). Set `"stream": true` to use the streaming path — the gateway coalesces the stream and returns a single completion (see `docs/packages/proxy.md`).

**Minimal example:**

```json
{
  "model": "gpt-4o",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ]
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

Returned when the request body is not valid JSON.

```
Invalid JSON in request body
```

#### 401 Unauthorized

Returned when the `Authorization` header is missing, malformed, or the API key is invalid/inactive/revoked/expired.

#### 403 Forbidden — Guardrail Blocked

Returned when a guardrail blocks the request or response.

```json
{
  "error": {
    "type": "guardrail_blocked",
    "message": "guardrail blocked: potential prompt injection detected",
    "guardrail": "prompt_injection",
    "findings": [
      {
        "guardrail": "prompt_injection",
        "type": "prompt_injection",
        "severity": "high",
        "start": 0,
        "end": 28,
        "entity": "injection_attempt",
        "value": "ignore previous instructions"
      }
    ]
  }
}
```

#### 405 Method Not Allowed

Returned when the HTTP method is not `POST`.

#### 500 Internal Server Error

Returned when the request body cannot be read.

#### 502 Bad Gateway

Returned when the proxy fails to communicate with the upstream provider. The response body contains the error message.

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

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `username` | string | Yes | Unique |
| `email` | string | No | Unique if set |
| `key_name` | string | Yes | Human-readable key label |

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

## `GET /events`

Server-sent event stream of request lifecycle events. Optionally filter with `?request_id=<id>`.

## `GET /dashboard`

HTML dashboard (embeds the SSE stream and a chat widget).

## `GET /metrics`

JSON snapshot of guardrail metrics (evaluations, blocks, redactions, warns, passes, errors, average latency).
