# Package: `config`

**File:** `internal/config/config.go` (9 lines)

**Package:** `config`

## Overview

Provides configuration values to the rest of the application. Currently uses environment variables, but the abstraction allows future swap to other config backends without changing callers.

## Imports

| Import | Usage |
|--------|-------|
| `os` | `os.Getenv("OPENAI_API_KEY")` |

## Functions

### `OpenAIKey()`

```go
func OpenAIKey() string
```

**Line:** `internal/config/config.go:7`

**Signature:** `func OpenAIKey() string`

**Behavior:**
1. Calls `os.Getenv("OPENAI_API_KEY")` (`config/config.go:8`)
2. Returns the value as a string

**Returns:** `string` — The OpenAI API key, or `""` if not set.

**Called by:**
- `proxy.ForwardChat()` in `internal/proxy/openai.go:19`

**Purpose:** Single source of truth for the OpenAI API key. Callers do not need to know where the key comes from (environment, file, vault, etc.).

**Design note:** Currently this is a one-liner that delegates to `os.Getenv()`. The function exists as an abstraction point so that future configuration sources (YAML, Vault, Kubernetes Secrets, AWS Secrets Manager) can be plugged in without changing the `proxy` package.
