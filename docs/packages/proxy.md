# Package: `proxy`

**Files:** `internal/proxy/provider.go`, `internal/proxy/openai.go`

**Package:** `proxy`

## Overview

Communicates with AI providers. Encapsulates all provider-specific knowledge — URLs, authentication headers, request semantics, streaming, and error handling. For V0, only the OpenAI-compatible provider is implemented behind a `Provider` interface.

## Types

### `Provider` (interface)

```go
type Provider interface {
    Name() string
    Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
}
```

**File:** `internal/proxy/provider.go`

The abstraction handlers depend on. `handlers.Chat` takes a `proxy.Provider`, so additional providers can be added without touching the handler.

### `OpenAIProvider`

```go
type OpenAIProvider struct {
    apiKey     string
    baseURL    string
    httpClient *http.Client
}
```

Constructors:

- `NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider` — explicit key and base URL (empty `baseURL` defaults to `https://api.openai.com/v1`). Uses a 60-second HTTP client timeout.
- `NewOpenAIProviderFromEnv() *OpenAIProvider` — reads the key from `config.OpenAIKey()` and the base URL from `OPENAI_BASE_URL`.

## Functions

### `Forward()`

```go
func (p *OpenAIProvider) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
```

Targets `{baseURL}/chat/completions`.

**Behavior:**

1. Errors immediately if `apiKey` is empty (`OPENAI_API_KEY is not set`).
2. Detects `"stream": true` in the request body and dispatches to streaming or non-streaming path.

**Non-streaming** (`forwardNonStreaming`):

1. Builds a `POST` request with the raw body.
2. Sets `Content-Type: application/json` and `Authorization: Bearer <key>`.
3. Sends via the 60-second HTTP client.
4. Reads and returns the response body.
5. Non-200 statuses are returned as errors (`OpenAI returned status <code>: <body>`).

**Streaming** (`forwardStreaming`):

1. Sends the request and reads the SSE stream (`data: ` lines) until `[DONE]`.
2. Accumulates chunks and rebuilds a single structured `chat.completion` response via `buildStructuredResponse` (the gateway currently returns a non-streaming JSON response, not a live SSE stream to the client).

## Error Handling

Errors are wrapped with `%w` to preserve the chain. `handlers.Chat` logs the error and returns HTTP 502.

## Design notes

- The 60-second timeout is hardcoded; future versions may make it configurable.
- The response body is fully buffered before being returned.
- Streaming responses are coalesced into a single completion for now (see the roadmap for real SSE streaming back to the client).
