# API Reference

## `POST /chat`

Forward a chat completion request to OpenAI.

### URL

```
POST http://localhost:3001/chat
```

### Headers

| Header | Value | Required |
|--------|-------|----------|
| `Content-Type` | `application/json` | Yes |

### Request Body

Any valid JSON object. The body is forwarded unchanged to `https://api.openai.com/v1/chat/completions`.

**Minimal example:**

```json
{
  "model": "gpt-4o",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ]
}
```

**Full example** (any OpenAI chat completion params):

```json
{
  "model": "gpt-4o",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is the capital of France?"}
  ],
  "temperature": 0.7,
  "max_tokens": 100
}
```

### Responses

#### 200 OK

The OpenAI response is returned unchanged. Content-Type is `application/json`.

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

#### 405 Method Not Allowed

Returned when the HTTP method is not `POST`.

```
Method not allowed
```

#### 500 Internal Server Error

Returned when the request body cannot be read.

```
Failed to read request body
```

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

#### 503 Service Unavailable — Guardrail Error

Returned when a mandatory guardrail encounters an internal error (fail-closed).

```json
{
  "error": {
    "type": "guardrail_unavailable",
    "message": "Request could not be evaluated by mandatory security controls"
  }
}
```

#### 502 Bad Gateway

Returned when the proxy fails to communicate with the upstream provider. The response body contains the error message.

Possible causes:
- `OPENAI_API_KEY` environment variable not set
- Network connectivity issues
- Provider API returns a non-200 status code (rate limiting, auth errors, etc.)
- Request timeout (60 seconds)

### Example: curl

```bash
curl -X POST http://localhost:3001/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Say hi"}]
  }'
```
