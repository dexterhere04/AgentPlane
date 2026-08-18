# Docker & AppRole Workflow

## Overview

The gateway ships as a lightweight Docker image based on `alpine:3.20`. A `docker-compose.yml` bundles it with a dev-mode Vault instance preconfigured for AppRole authentication and a PostgreSQL instance, demonstrating the full secrets bootstrapping and database flow.

## Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Builds the gateway image from a pre-built binary |
| `docker-compose.yml` | Orchestrates Vault + init + PostgreSQL + gateway |
| `docker-entrypoint.sh` | Runtime entrypoint that sources AppRole credentials |
| `.dockerignore` | Excludes unnecessary files from the build context |

## Dockerfile

```dockerfile
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY agentplane /agentplane
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 3001

ENTRYPOINT ["/docker-entrypoint.sh"]
```

**Key points:**

- **Alpine 3.20** — minimal base (~3 MB) with `ca-certificates` for HTTPS/TLS to external APIs and Vault
- **Pre-built binary** — the `agentplane` binary is compiled on the host (`CGO_ENABLED=0 GOOS=linux go build`) and copied in, avoiding a full Go toolchain in the image
- **`docker-entrypoint.sh`** as `ENTRYPOINT` — runs before the binary to inject environment variables at container start, not build time

### Building

```bash
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o agentplane ./cmd/server/
docker build -t agentplane-gateway:latest .
```

---

## docker-entrypoint.sh

```shell
#!/bin/sh
set -e

if [ -f /vault-creds/creds.env ]; then
  set -a
  . /vault-creds/creds.env
  set +a
fi

exec /agentplane
```

### What it does

1. **`set -e`** — exit immediately if any command fails
2. **Checks for `/vault-creds/creds.env`** — this file is written by the `vault-init` container into a shared Docker volume
3. **`set -a` + `. /vault-creds/creds.env` + `set +a`** — sources the file in auto-export mode, so every `KEY=VALUE` line becomes an exported environment variable in the gateway process
4. **`exec /agentplane`** — replaces the shell with the gateway binary (no lingering shell process)

### Why a separate entrypoint script?

AppRole credentials (`VAULT_ROLE_ID`, `VAULT_SECRET_ID`) are generated dynamically by the `vault-init` container at compose startup. They are written to a shared volume, not known at build time. The entrypoint bridges this gap: it reads the volume at container start and makes the values available as environment variables before `main.go` evaluates them.

---

## docker-compose.yml Flow

The stack has four services orchestrated in sequence:

```
┌───────────┐     ┌──────────────┐     ┌──────────────┐
│   vault   │────>│  vault-init  │────>│   gateway    │
│ (dev mode)│     │ (bootstrap)  │     │ (agentplane) │
└───────────┘     └──────────────┘     └──────────────┘
    8200                                    3001

┌───────────┐
│ postgres  │────> gateway (DATABASE_URL)
│    16     │
└───────────┘
```

### 1. vault

Starts Vault in dev mode (in-memory, unsealed, root token `dev-root-token`). A healthcheck polls `vault status` until ready.

```yaml
vault:
  image: hashicorp/vault:1.18
  environment:
    VAULT_DEV_ROOT_TOKEN_ID: dev-root-token
    VAULT_DEV_LISTEN_ADDRESS: 0.0.0.0:8200
  healthcheck:
    test: ["CMD", "vault", "status", "-address=http://127.0.0.1:8200"]
```

### 2. vault-init

Depends on vault being healthy. Runs once to completion, then exits. It:

1. **Enables the KV v2 secrets engine** at path `agentplane`
2. **Enables AppRole auth method**
3. **Creates a policy** granting read access to `agentplane/data/*`
4. **Creates an AppRole role** with a 1h TTL
5. **Stores a test secret** (`OPENAI_API_KEY=sk-test-from-vault`)
6. **Generates a Role ID and Secret ID** via the AppRole endpoint
7. **Writes the credentials** to `/vault-creds/creds.env` on the shared volume

```yaml
vault-init:
  image: hashicorp/vault:1.18
  depends_on:
    vault:
      condition: service_healthy
  environment:
    VAULT_ADDR: http://vault:8200
    VAULT_TOKEN: dev-root-token       # uses root token for setup
  volumes:
    - vault-creds:/vault-creds        # shared with gateway
```

The generated `creds.env` file looks like:

```
VAULT_ROLE_ID=fb9ce36c-27db-6bce-1bd0-1be0b5efb6c8
VAULT_SECRET_ID=8172e089-0107-6e7e-b989-9a2faa166cfc
```

### 3. postgres

Runs PostgreSQL 16 and serves as the backing store for users and API keys. The gateway connects to it via `DATABASE_URL`, and migrations are applied automatically on startup.

```yaml
postgres:
  image: postgres:16-alpine
  environment:
    POSTGRES_USER: agentplane
    POSTGRES_PASSWORD: agentplane
    POSTGRES_DB: agentplane
  volumes:
    - pgdata:/var/lib/postgresql/data
```

### 4. gateway

Depends on `vault-init` completing successfully and `postgres` being healthy. It:

1. **Entrypoint sources** `creds.env` from the shared volume, exporting `VAULT_ROLE_ID` and `VAULT_SECRET_ID`
2. **`main.go` detects** `SECRET_STORE=vault` and constructs a `VaultStore`
3. **No static token** is set (`VAULT_TOKEN` env is empty), so it calls `VaultAppRoleLogin(addr, roleID, secretID)`
4. **VaultAppRoleLogin** POSTs to `/v1/auth/approle/login` and receives a `client_token`
5. The token is stored in `VaultStore.Token` and used for all subsequent secret reads
6. **`main.go` connects** to PostgreSQL via `DATABASE_URL` and applies embedded migrations

```yaml
gateway:
  build: .
  environment:
    SECRET_STORE: vault
    VAULT_ADDR: http://vault:8200
    VAULT_MOUNT_PATH: agentplane
    OPENAI_BASE_URL: https://api.openai.com/v1
    DATABASE_URL: postgres://agentplane:agentplane@postgres:5432/agentplane?sslmode=disable
    AGENTPLANE_ADMIN_TOKEN: dev-admin-token-change-me
  volumes:
    - vault-creds:/vault-creds       # receives creds from vault-init
  depends_on:
    vault-init:
      condition: service_completed_successfully
    postgres:
      condition: service_healthy
```

### Full Auth Timeline

```
t=0   vault starts (dev mode, unsealed)
t=1   vault-init:
        - enables KV v2 at agentplane/
        - enables AppRole auth
        - creates role + policy
        - stores OPENAI_API_KEY secret
        - generates role_id + secret_id
        - writes /vault-creds/creds.env
t=2   vault-init exits (success)
t=3   gateway starts:
        - entrypoint sources creds.env → VAULT_ROLE_ID, VAULT_SECRET_ID set
        - main.go connects to PostgreSQL, applies migrations (users, api_keys)
        - main.go: VaultStore has no Token
        - calls VaultAppRoleLogin(addr, roleID, secretID)
        - POST /v1/auth/approle/login → client_token
        - VaultStore.Token = client_token
t=4   gateway ready on :3001
t=5   POST /chat → config.OpenAIKey() → VaultStore.GetSecret("OPENAI_API_KEY")
        - GET /v1/agentplane/data/OPENAI_API_KEY (X-Vault-Token attached)
        - returns sk-test-from-vault
```

## Running

```bash
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o agentplane ./cmd/server/
docker compose up -d
```

## Verifying

```bash
curl -X POST http://localhost:3001/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}'
```

The response should succeed (or fail) without ever including the secret value in client-visible errors. To confirm Vault/AppRole wiring, verify via Vault audit/logs or by temporarily pointing `OPENAI_BASE_URL` at a local mock and checking that requests are authorized (without printing `OPENAI_API_KEY`).

## Production Notes

- The compose file uses Vault dev mode (in-memory, no persistence) for demonstration only
- For production, replace the dev Vault with a proper HA cluster and use a pre-provisioned AppRole with known Role ID
- Store the Secret ID in a secure location (Docker secret, Kubernetes secret, SOPS-encrypted file) rather than generating it at startup
- Consider reducing `token_ttl` and `token_max_ttl` to match your security posture
- The gateway's `VaultAppRoleLogin` happens once at startup; add a retry/renewal mechanism if running long-lived instances beyond the token TTL
