# go.mod

**File:** `go.mod`

## Content

```
module github.com/dexterhere04/AgentPlane

go 1.26.5
```

## Module

| Field | Value |
|-------|-------|
| Module path | `github.com/dexterhere04/AgentPlane` |
| Go version | `1.26.5` |
| Dependencies | None (zero external dependencies) |

## Import Path

All internal imports use the module path prefix:

```go
import "github.com/dexterhere04/AgentPlane/internal/handlers"
import "github.com/dexterhere04/AgentPlane/internal/proxy"
import "github.com/dexterhere04/AgentPlane/internal/config"
```

## Zero Dependencies

AgentPlane V0 uses only the Go standard library:

| Package | Used By |
|---------|---------|
| `net/http` | `main.go`, `chat.go`, `openai.go` |
| `encoding/json` | `chat.go` |
| `io` | `chat.go`, `openai.go` |
| `log` | `main.go`, `chat.go` |
| `os` | `config.go` |
| `bytes` | `openai.go` |
| `fmt` | `openai.go` |
| `time` | `openai.go` |

## Building

```bash
go build ./cmd/server/
```

The `go` directive in `go.mod` specifies the expected language version. If your installed Go is older, change the directive or update Go.
