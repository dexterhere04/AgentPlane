# Package: `proxy`

**File:** `internal/proxy/openai.go` (50 lines)

**Package:** `proxy`

## Overview

Communicates with AI providers. Encapsulates all provider-specific knowledge — URLs, authentication headers, request semantics, and error handling. For V0, only OpenAI is supported.

## Imports

| Import | Usage |
|--------|-------|
| `bytes` | `bytes.NewReader(body)` to create request body reader |
| `fmt` | `fmt.Errorf()` for wrapping errors with context |
| `io` | `io.ReadAll()` to read OpenAI response body |
| `net/http` | Create and send HTTP requests, read responses |
| `time` | `60 * time.Second` client timeout |
| `github.com/dexterhere04/AgentPlane/internal/config` | `config.OpenAIKey()` to get the API key |

## Constants

### `openAIURL`

```go
const openAIURL = "https://api.openai.com/v1/chat/completions"
```

**Line:** `internal/proxy/openai.go:13`

**Type:** `string`

**Value:** The OpenAI Chat Completions API endpoint.

**Usage:** Used as the target URL in `http.NewRequest()` at line 24.

## Functions

### `ForwardChat()`

```go
func ForwardChat(body []byte) ([]byte, error)
```

**Line:** `internal/proxy/openai.go:18`

**Signature:** `func ForwardChat(body []byte) ([]byte, error)`

**Parameters:**
- `body []byte` — The raw JSON request body from the client (already validated as JSON by the handler).

**Returns:**
- `[]byte` — The raw JSON response body from OpenAI (on success).
- `error` — Wrapped error describing what went wrong (on failure).

**Called by:**
- `handlers.Chat()` in `internal/handlers/chat.go:35`

**Calls:**
- `config.OpenAIKey()` to retrieve the API key
- `http.NewRequest(http.MethodPost, openAIURL, bytes.NewReader(body))` to create the outbound request
- `client.Do(req)` to send the request (with 60s timeout)
- `io.ReadAll(resp.Body)` to read the response

**Behavior (step by step):**

| Step | Line | Action | Error Returned |
|------|------|--------|---------------|
| 1 | 19 | Gets API key from `config.OpenAIKey()` | `"OPENAI_API_KEY is not set"` if empty |
| 2 | 24 | Creates HTTP request with POST method, OpenAI URL, and body reader | `fmt.Errorf("creating request: %w", err)` |
| 3 | 29 | Sets `Content-Type: application/json` header | — |
| 4 | 30 | Sets `Authorization: Bearer <api_key>` header | — |
| 5 | 32 | Creates HTTP client with 60-second timeout | — |
| 6 | 34 | Sends the request via `client.Do(req)` | `fmt.Errorf("sending request: %w", err)` |
| 7 | 38 | Defers `resp.Body.Close()` | — |
| 8 | 40 | Reads response body with `io.ReadAll(resp.Body)` | `fmt.Errorf("reading response: %w", err)` |
| 9 | 45 | Checks if status code is not 200 | `fmt.Errorf("OpenAI returned status %d: %s", code, body)` |
| 10 | 49 | Returns response body on success | `nil` |

**Error Handling:**

All errors use `fmt.Errorf` with `%w` to wrap the underlying error, preserving the full error chain for debugging. The caller (`handlers.Chat`) logs the error and returns HTTP 502.

**Purpose:** The single provider-specific module. All knowledge of the OpenAI API (URL, authentication mechanism, content type, timeout) lives here. Future providers (Anthropic, Gemini, Groq, Ollama) will each get their own file in this package.

**Design notes:**
- The 60-second timeout is hardcoded. Future versions may make this configurable via the `config` package.
- Non-200 status codes from OpenAI are treated as errors and returned with the full response body for debugging.
- The response body is fully buffered in memory before being returned. For very large responses, streaming support would be needed (future roadmap).
