# Setup & Running

## Prerequisites

- Go 1.26.5 or later
- A PostgreSQL instance (for users and API keys)
- An OpenAI-compatible API key

## Configuration

Copy `.env.example` to `.env` and fill in real values. The file is git-ignored.

Required environment variables:

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string. Tables are created automatically on startup via embedded migrations. |
| `API_KEY_PEPPER` | Server-side secret mixed into API-key hashes (`SHA-256(secret + pepper)`). |
| `AGENTPLANE_ADMIN_TOKEN` | Bearer token that authorizes admin endpoints (`/provision/user`, `/admin/api-keys/revoke`). |
| `OPENAI_API_KEY` | Upstream provider key (loaded via the configured secret store). |

Optional:

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Upstream API base URL. |
| `SECRET_STORE` | `vault` | Secret backend (`env`, `file`, `vault`, `aws`, `azure`, `chain`). |
| `PORT` | `3001` | HTTP listen port. |

See `docs/secrets-backends.md` for the full secret-store configuration matrix.

## Database

Start a local Postgres (e.g. via Docker):

```bash
docker compose up -d postgres
```

Migrations in `migrations/` are applied automatically on server startup — no separate migration step is required.

## Build

```bash
go build -o bin/agentplane ./cmd/server/
```

## Run

### Development (direct)

```bash
go run ./cmd/server/
```

### Production (binary)

```bash
./bin/agentplane
```

### Dev script

`./dev.sh` builds a local mock API (on port 3002), points `OPENAI_BASE_URL` at it, and starts the gateway with `SECRET_STORE=env` and a few guardrails enabled. It still requires PostgreSQL (`docker compose up -d postgres`).

### Docker Compose

```bash
docker compose up -d
```

Brings up Vault (dev mode), `vault-init`, PostgreSQL, and the gateway. See `docs/docker-approle.md`.

## Endpoints

| Method | Path | Auth |
|--------|------|------|
| POST | `/chat` | Bearer API key |
| POST | `/provision/user` | Bearer admin token |
| POST | `/admin/api-keys/revoke` | Bearer admin token |
| GET | `/events` | — |
| GET | `/dashboard` | — |
| GET | `/metrics` | — |

## Verify

Provision a user and key, then use the returned key:

```bash
curl -X POST http://localhost:3001/provision/user \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","email":"alice@example.com","key_name":"dev"}'
```

```bash
curl -X POST http://localhost:3001/chat \
  -H "Authorization: Bearer <api-key-from-above>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}'
```

## Troubleshooting

### OPENAI_API_KEY is not set

Ensure the key is available through the configured secret store. For `SECRET_STORE=env`, check:

```bash
echo $OPENAI_API_KEY
```

### Connection refused / database errors

The server fails fast at startup if `DATABASE_URL` is missing or the database is unreachable. Confirm PostgreSQL is up and `DATABASE_URL` is correct.

### 401 Unauthorized

`/chat` requires a valid `ap_live_*` API key; admin endpoints require the admin token. Provision a key first via `POST /provision/user`.

### Unsupported Go version

Run `go version` and ensure it's 1.26.5+. The `go.mod` file can be edited to match your installed Go version by changing the `go` directive.
