# AgentPlane Documentation

> **Version:** 0.1.0

## Index

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | High-level architecture, project structure, and design principles |
| [API Reference](api-reference.md) | Endpoint specifications |
| [Request Flow](request-flow.md) | Step-by-step request lifecycle |
| [Setup & Running](setup.md) | Build, configure, and run the server |
| [Secrets Backends](secrets-backends.md) | Secret-store configuration (env, file, Vault, AWS, Azure, chain) |
| [Docker & AppRole](docker-approle.md) | Docker image and Vault AppRole bootstrap flow |
| [API Key Design](api-key-design.md) | Key format design evaluation and decision |
| [API Key Generation](api-key-generation.md) | Key generation design and decisions |
| [API Key Storage & DB](api-key-storage-db-integration.md) | Hash-only persistence and database integration |
| [API Key Lifecycle](api-key-lifecycle.md) | Full lifecycle: provisioning → auth → revocation |
| [Full Request Authentication](full-request-authentication.md) | Provisioning, authentication, revocation, usage tracking |
| [Policy Layer & RBAC](policy-rbac.md) | Identity-based authorization: roles, permissions, enforcement |
| [Future Roadmap](future-roadmap.md) | Planned pipeline stages and evolution |
| [go.mod](go-mod.md) | Module definition and dependency management |
| [.gitignore](gitignore.md) | Version control exclusion rules |

## Package Reference

| Package | Path | Document |
|---------|------|----------|
| `cmd/server` | `cmd/server/main.go` | [docs](packages/cmd-server.md) |
| `config` | `internal/config/` | [docs](packages/config.md) |
| `handlers` | `internal/handlers/` | [docs](packages/handlers.md) |
| `guardrail` | `internal/guardrail/` | [docs](packages/guardrail.md) |
| `policy` | `internal/policy/` | [policy-rbac.md](policy-rbac.md), [docs](packages/policy.md) |
| `proxy` | `internal/proxy/` | [docs](packages/proxy.md) |
| `auth` | `internal/auth/` | [full-request-authentication.md](full-request-authentication.md) |
| `api` | `internal/api/` | [api-key-generation.md](api-key-generation.md), [api-key-storage-db-integration.md](api-key-storage-db-integration.md) |
| `users` | `internal/users/` | [full-request-authentication.md](full-request-authentication.md) |
| `provisioning` | `internal/provisioning/` | [api-key-lifecycle.md](api-key-lifecycle.md) |
| `db` | `internal/db/` | [api-key-storage-db-integration.md](api-key-storage-db-integration.md) |
| `secrets` | `internal/secrets/` | [secrets-backends.md](secrets-backends.md) |

## Quick Links

- **Entry point:** `cmd/server/main.go`
- **HTTP handler:** `internal/handlers/chat.go` — function `Chat()`
- **Provider proxy:** `internal/proxy/openai.go` — `OpenAIProvider.Forward()`
- **Authorization:** `internal/policy/` — `EnforcementPoint`, `Require()`; RBAC in `internal/policy/rbac/`
- **Configuration:** `internal/config/` — `OpenAIKey()`, `KeyPepper()`, `DatabaseURL()`, `AdminToken()`, `ConfigureSecretStore()`
- **Default port:** `3001`
- **Required env:** `DATABASE_URL`, `API_KEY_PEPPER`, `AGENTPLANE_ADMIN_TOKEN`, `OPENAI_API_KEY`

## Project Summary

AgentPlane is an AI Gateway — a middleware layer between AI applications and LLM providers. It authenticates callers via API keys, authorizes them through a role-based policy layer, runs configurable guardrails on input and output, injects the upstream provider key, forwards to an OpenAI-compatible `chat/completions` endpoint, and returns the response.

Built in Go 1.26.5 with PostgreSQL for user/API-key/RBAC persistence, a pluggable secrets layer (env, file, Vault, AWS, Azure), an extensible guardrail pipeline, and a deny-by-default authorization layer.
