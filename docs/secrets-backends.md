# Secrets Backends

## Overview

The `internal/secrets` package provides a pluggable secrets layer via the `SecretStore` interface:

```go
type SecretStore interface {
    GetSecret(key string) (string, error)
}
```

The gateway selects a backend at startup via the `SECRET_STORE` environment variable (see `config.ConfigureSecretStore()`). All callers go through `config.OpenAIKey()` (and `config.KeyPepper()`), which delegate to the configured store — callers never know which backend is in use.

> **Default:** when `SECRET_STORE` is unset or unrecognized, the gateway falls back to HashiCorp Vault.

## Backends

### EnvStore (`store.go`)

Reads secrets directly from environment variables.

| Env Var                 | Purpose                          |
|-------------------------|----------------------------------|
| `SECRET_STORE=env`      | Select this backend              |

**Example:**

```bash
export OPENAI_API_KEY=sk-...
go run ./cmd/server/
```

### FileStore (`store.go`)

Reads secrets from files in a directory. Useful for Docker/Kubernetes secrets mounted as files.

| Env Var              | Purpose                  |
|----------------------|--------------------------|
| `SECRET_STORE=file`  | Select this backend      |
| `SECRET_STORE_PATH`  | Directory containing secret files |

**Example:**

```bash
SECRET_STORE=file SECRET_STORE_PATH=/run/secrets go run ./cmd/server/
# reads /run/secrets/OPENAI_API_KEY
```

### ChainStore (`store.go`)

Tries multiple stores in order; returns the first successful result.

| Env Var               | Purpose             |
|-----------------------|---------------------|
| `SECRET_STORE=chain`  | Select this backend |
| `SECRET_STORE_PATH`   | Base path for FileStore fallback |

**Example:**

```bash
SECRET_STORE=chain SECRET_STORE_PATH=/run/secrets go run ./cmd/server/
# tries file → env in sequence
```

---

## Vault (`vault.go`)

Integrates with HashiCorp Vault KV (v1 and v2). Supports token-based and AppRole authentication. Uses no external Vault SDK — pure `net/http` + `encoding/json` against the Vault HTTP API.

### Authentication

**Static token** — set `VAULT_TOKEN` directly:

```bash
VAULT_TOKEN=hvs.xxx
```

**AppRole** — leave `VAULT_TOKEN` unset and provide role credentials. The gateway calls `VaultAppRoleLogin()` to exchange them for a short-lived token:

```bash
VAULT_ROLE_ID=<role-id>
VAULT_SECRET_ID=<secret-id>
```

### KV Versions

`vault.go` handles both KV engine formats transparently:

| KV Version    | URL pattern                        | Response extraction       |
|---------------|------------------------------------|---------------------------|
| v1            | `GET /v1/<mount>/<key>`           | `.data.value`             |
| v2 (default)  | `GET /v1/<mount>/data/<key>`      | `.data.data.value`        |

### Configuration

| Env Var              | Required | Default | Purpose                             |
|----------------------|----------|---------|-------------------------------------|
| `SECRET_STORE=vault` | No       | (fallback) | Select this backend; also the default when `SECRET_STORE` is unset |
| `VAULT_ADDR`         | No       | `http://127.0.0.1:8200` | Vault server URL (e.g. `http://vault:8200`) |
| `VAULT_MOUNT_PATH`   | No       | `agentplane` | KV mount path inside Vault          |
| `VAULT_TOKEN`        | No       | —       | Static token; skips AppRole login   |
| `VAULT_ROLE_ID`      | No       | —       | AppRole Role ID (mutual with SECRET_ID) |
| `VAULT_SECRET_ID`    | No       | —       | AppRole Secret ID (mutual with ROLE_ID) |
| `VAULT_KV_VERSION`   | No       | `2`     | Set to `1` for KV v1 engine         |

### Auth Flow

```
       ┌─────────┐                         ┌───────────┐
       │ Gateway │                         │   Vault   │
       └────┬────┘                         └─────┬─────┘
            │                                     │
            │  (if no VAULT_TOKEN)                │
            │  POST /v1/auth/approle/login       │
            │  {"role_id":"...","secret_id":"..."}│
            │ ──────────────────────────────────> │
            │                                     │
            │            {"auth":{"client_token":"..."}}
            │ <────────────────────────────────── │
            │                                     │
            │  (on each request)                  │
            │  GET /v1/<mount>/data/OPENAI_API_KEY│
            │  X-Vault-Token: <client_token>      │
            │ ──────────────────────────────────> │
            │                                     │
            │         {"data":{"data":{"value":"sk-..."}}}
            │ <────────────────────────────────── │
            │                                     │
```

### Implementation Detail

`GetSecret()` builds the URL based on `KVVersion`, attaches the token via `X-Vault-Token` header, and parses the response. KV1 data is unwrapped directly; KV2 data goes through an extra `data` wrapper.

`VaultAppRoleLogin()` (exported, not a method) sends a JSON POST to `/v1/auth/approle/login` and returns the `client_token` string. It is used by `main.go` during bootstrap when no static token is provided.

---

## AWS Secrets Manager (`aws.go`)

Reads secrets from AWS Secrets Manager using the AWS SDK.

### Configuration

| Env Var              | Required | Purpose                                |
|----------------------|----------|----------------------------------------|
| `SECRET_STORE=aws`   | Yes      | Select this backend                    |
| `AWS_REGION`         | Yes      | AWS region (e.g. `us-east-1`)          |
| `AWS_SECRET_ID`      | Yes      | Secret name or ARN in Secrets Manager  |
| `AWS_SECRET_JSON_KEY`| No       | JSON key to extract (if secret is JSON)|
| `AWS_ENDPOINT_URL`   | No       | Custom endpoint (e.g. LocalStack)      |

### Behavior

If `AWS_SECRET_JSON_KEY` is set, the secret value is parsed as JSON and the specified key is extracted (must be a string). Otherwise the raw SecretString is returned as-is.

### Example

```bash
SECRET_STORE=aws \
AWS_REGION=us-east-1 \
AWS_SECRET_ID=prod/openai/api-key \
go run ./cmd/server/
```

With a JSON secret:

```bash
SECRET_STORE=aws \
AWS_REGION=us-east-1 \
AWS_SECRET_ID=prod/credentials \
AWS_SECRET_JSON_KEY=OPENAI_API_KEY \
go run ./cmd/server/
```

---

## Azure Key Vault (`azure.go`)

Reads secrets from Azure Key Vault using the Azure SDK with `DefaultAzureCredential`.

### Configuration

| Env Var                | Required | Purpose                            |
|------------------------|----------|------------------------------------|
| `SECRET_STORE=azure`   | Yes      | Select this backend                |
| `AZURE_KEY_VAULT_URL`  | Yes      | Vault URL (e.g. `https://myvault.vault.azure.net`) |

### Authentication

Uses `azidentity.NewDefaultAzureCredential()`, which tries multiple credential sources in order: environment variables, managed identity, Azure CLI, etc. No explicit credentials are needed in configuration.

### Example

```bash
SECRET_STORE=azure \
AZURE_KEY_VAULT_URL=https://agentplane-vault.vault.azure.net \
go run ./cmd/server/
# reads secret named OPENAI_API_KEY from the vault
```

### Implementation Detail

`GetSecret()` creates a credential via `DefaultAzureCredential`, instantiates a secrets client, and calls `GetSecret()` on the key name. The `key` parameter passed by the config layer (i.e. `OPENAI_API_KEY`) maps directly to the Azure secret name.
