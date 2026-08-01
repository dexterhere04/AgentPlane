# Package: `handlers`

**File:** `internal/handlers/chat.go` (45 lines)

**Package:** `handlers`

## Overview

Handles HTTP requests. Concerned only with HTTP — validates the request, reads the body, calls the proxy layer, and writes the response. Knows nothing about OpenAI or provider-specific logic.

## Imports

| Import | Usage |
|--------|-------|
| `encoding/json` | `json.Valid()` to validate request body is well-formed JSON |
| `io` | `io.ReadAll()` to read the request body |
| `log` | `log.Printf()` for error logging |
| `net/http` | `http.ResponseWriter`, `http.Request`, `http.Error()`, status codes |
| `github.com/dexterhere04/AgentPlane/internal/proxy` | Call `proxy.ForwardChat()` |

## Functions

### `Chat()`

```go
func Chat(w http.ResponseWriter, r *http.Request)
```

**Line:** `internal/handlers/chat.go:15`

**Signature:** `func Chat(w http.ResponseWriter, r *http.Request)`

**Registered at:** `cmd/server/main.go:12` as `mux.HandleFunc("/chat", handlers.Chat)`

**Behavior (step by step):**

| Step | Line | Action | Error Response |
|------|------|--------|---------------|
| 1 | 16 | Validates HTTP method is POST | 405 Method Not Allowed |
| 2 | 21 | Reads entire request body with `io.ReadAll(r.Body)` | 500 Failed to read request body |
| 3 | 27 | Defers `r.Body.Close()` | — |
| 4 | 30 | Validates body is valid JSON with `json.Valid(body)` | 400 Invalid JSON in request body |
| 5 | 35 | Calls `proxy.ForwardChat(body)` to forward to OpenAI | 502 with error message |
| 6 | 42 | Sets `Content-Type: application/json` header | — |
| 7 | 43 | Writes HTTP 200 status code | — |
| 8 | 44 | Writes the response body from OpenAI | — |

**Returns:** Nothing. Writes HTTP response directly.

**Called by:** The Go HTTP server when a request hits `POST /chat`.

**Calls:**
- `io.ReadAll(r.Body)`
- `json.Valid(body)`
- `proxy.ForwardChat(body)`
- `w.Header().Set()`, `w.WriteHeader()`, `w.Write()`
- `http.Error()`

**Purpose:** Acts as the "receptionist" — validates incoming HTTP traffic and delegates to the right back-end component. Ensures only valid POST requests with parseable JSON bodies reach the provider proxy.

**Must NOT:**
- Know how OpenAI works
- Hardcode provider URLs
- Store or manage API keys
