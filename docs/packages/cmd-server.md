# Package: `cmd/server`

**File:** `cmd/server/main.go` (19 lines)

**Package:** `main`

## Overview

The application entry point. Creates an HTTP server, registers routes, and starts listening on port 3001.

## Imports

| Import | Usage |
|--------|-------|
| `log` | Log startup message and fatal errors |
| `net/http` | Create HTTP mux, register routes, start server |
| `github.com/dexterhere04/AgentPlane/internal/handlers` | Import the Chat handler |

## Functions

### `main()`

```go
func main()
```

**Line:** `cmd/server/main.go:10`

**Signature:** `func main()`

**Behavior:**
1. Creates a new `http.ServeMux` (`cmd/server/main.go:11`)
2. Registers `handlers.Chat` on the `/chat` path (`cmd/server/main.go:12`)
3. Logs `"Starting AgentPlane on :3001"` (`cmd/server/main.go:14`)
4. Calls `http.ListenAndServe(":3001", mux)` to start the server (`cmd/server/main.go:15`)
5. If `ListenAndServe` returns an error, logs it fatally and exits (`cmd/server/main.go:16-18`)

**Returns:** Does not return. Exits on fatal error.

**Called by:** The Go runtime (program entry point).

**Calls:**
- `http.NewServeMux()`
- `mux.HandleFunc("/chat", handlers.Chat)`
- `http.ListenAndServe(":3001", mux)`

**Purpose:** Wires the minimal dependency graph for V0 and starts the server. Acts as the composition root.

**Must NOT:**
- Call OpenAI directly
- Read request bodies
- Implement business logic
- Store configuration
