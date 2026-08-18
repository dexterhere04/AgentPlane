# Package: `config`

**Files:** `internal/config/config.go`, `store.go`, `database.go`, `admin_token.go`, `pepper.go`

**Package:** `config`

## Overview

Centralizes configuration for the rest of the application. Secrets are read through a pluggable `secrets.SecretStore` (installed via `ConfigureSecretStore()`), so callers never depend on a specific backend (env, file, Vault, AWS, Azure).

## Functions

### `ConfigureSecretStore()`

```go
func ConfigureSecretStore() error
```

**File:** `internal/config/store.go`

Selects the secret backend from the `SECRET_STORE` env var and installs it into `Store`. Backends: `env`, `file`, `aws`, `azure`, `chain`. When `SECRET_STORE` is unset or unrecognized, it falls back to HashiCorp Vault.

### `OpenAIKey()`

```go
func OpenAIKey() (string, error)
```

**File:** `internal/config/config.go`

Returns the upstream provider key from the configured secret store (`Store.GetSecret("OPENAI_API_KEY")`).

### `OpenAIBaseURL()`

```go
func OpenAIBaseURL() string
```

**File:** `internal/config/config.go`

Returns `OPENAI_BASE_URL`, defaulting to `https://api.openai.com/v1`.

### `KeyPepper()`

```go
func KeyPepper() (string, error)
```

**File:** `internal/config/pepper.go`

Returns the `API_KEY_PEPPER` secret from the configured store. Returns an error if missing or empty, so the app fails fast at startup rather than hashing keys with an empty pepper.

### `DatabaseURL()`

```go
func DatabaseURL() (string, error)
```

**File:** `internal/config/database.go`

Returns the `DATABASE_URL` Postgres connection string. Returns an error if unset or empty.

### `AdminToken()`

```go
func AdminToken() (string, error)
```

**File:** `internal/config/admin_token.go`

Returns the `AGENTPLANE_ADMIN_TOKEN` used to authorize admin endpoints. Returns an error if unset or empty.

## Design note

`Store` is a package-level `secrets.SecretStore` variable defaulting to `secrets.EnvStore{}`. `SetStore()` replaces it. This mirrors the abstraction the original `OpenAIKey()` design intended: the backend can change (YAML, Vault, Kubernetes, AWS, Azure) without changing callers.
