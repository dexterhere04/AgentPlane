# Architecture

## Overview

AgentPlane uses a layered architecture following the Single Responsibility Principle. Each package owns exactly one concern.

```
                 Client
                    |
          Authorization: Bearer <api-key>
                    |
                    v
          +------------------+
          |  Auth Middleware |  (internal/auth/middleware.go)
          +--------+---------+
                    |
                    v
          +------------------+
          |     Handler      |  (internal/handlers/chat.go)
          +--------+---------+
                    |
                    v
          +------------------+
          |   Guardrails     |  (internal/guardrail/) input + output
          +--------+---------+
                    |
                    v
          +------------------+
          |  Provider Proxy  |  (internal/proxy/openai.go)
          +--------+---------+
                    |
                    v
             OpenAI API
```

## Project Structure

```
agentplane/
|
+-- cmd/
|   +-- server/
|   |   +-- main.go            # composition root
|   +-- keygeneration/
|       +-- main.go            # one-off API key generator
|
+-- internal/
|   +-- config/                # configuration + secret-store wiring
|   |   +-- config.go, store.go, database.go, admin_token.go, pepper.go
|   +-- db/                    # Postgres pool + embedded migrations
|   +-- api/                   # API-key generation + persistence
|   +-- auth/                  # API-key authentication + admin middleware
|   +-- users/                 # user persistence
|   +-- provisioning/          # user + API-key provisioning flow
|   +-- handlers/              # chat, provision, revoke, metrics
|   +-- guardrail/             # guardrail pipeline + providers
|   +-- observability/         # event bus / SSE
|   +-- proxy/                 # provider forwarding
|   +-- secrets/               # secret stores (env, file, Vault, AWS, Azure)
|   +-- dashboard/             # embedded HTML dashboard
|
+-- migrations/                # embedded SQL migrations
+-- testing/                   # local mock API (separate module)
+-- .env.example
+-- docker-compose.yml
+-- Dockerfile
+-- go.mod
+-- Readme.md
+-- docs/
    +-- ...
```

## Layer Responsibilities

| Layer | File(s) | Responsibility | Must NOT |
|-------|---------|---------------|----------|
| `cmd/server` | `main.go` | Composition root, route registration, startup | Call OpenAI, read request bodies, store configuration |
| `config` | `config/` | Provide configuration + select secret store | Implement business logic |
| `auth` | `auth/` | API-key parsing/verification, admin token check | Touch provider or handler logic |
| `handlers` | `handlers/` | HTTP validation, body reading, response writing | Know provider-specific details |
| `guardrail` | `guardrail/` | Policy enforcement (input + output) | Forward requests |
| `proxy` | `proxy/` | Provider communication, key injection, forwarding | Handle HTTP concerns beyond outbound calls |
| `api` | `api/` | API-key generation and persistence (hashes only) | Know about users or HTTP |
| `users` | `users/` | User persistence | Know about API keys or HTTP |
| `provisioning` | `provisioning/` | Orchestrate user + key creation | Do HTTP |
| `db` | `db/` | Connection pool + migrations | Know any table schema |
| `secrets` | `secrets/` | Read secrets from configured backend | Know business logic |

## Design Principles

### Single Responsibility

Each package has exactly one reason to change:
- Change the provider → only `proxy` changes
- Change the HTTP framework → only `handlers` changes
- Change configuration/secret source → only `config`/`secrets` changes
- Change the server mux → only `cmd/server` changes
- Change key format → only `api` changes

### Provider Abstraction

The `proxy` package encapsulates provider-specific knowledge behind a `Provider` interface. Adding a new provider means adding a new type implementing `Name()` + `Forward()` without touching handlers.

### Config Abstraction

The `config` package wraps secret access behind a pluggable `secrets.SecretStore`. Callers use `config.OpenAIKey()`, `config.KeyPepper()`, etc. and never depend on where secrets live (env, file, Vault, AWS, Azure).

### Hash-only Secret Storage

API-key plaintext is never persisted. Only `SHA-256(secret + pepper)` is stored; the full key is returned to the caller exactly once at provisioning time.
