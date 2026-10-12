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
          |   Policy (RBAC)  |  (internal/policy/) authorize identity
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
           |     Router       |  (internal/proxy/router.go) match rule, select
           +--------+---------+
                     |
                     v
           +------------------+
           |  Upstreams       |  (internal/proxy/upstream.go) timeout,
           |  (chain)         |   retry, circuit breaker, failover
           +--------+---------+
                     |
                     v
              Provider API
```

Routing is a **forwarding** concern: authorization (`policy`) decides whether a
principal may act; guardrails check content; the router decides *where* the
already-authorized request is sent. See `provider-routing-and-router.md`.

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
|   +-- policy/                # identity-based authorization (RBAC)
|   |   +-- rbac/              # roles, permissions, assignments
|   +-- users/                 # user persistence
|   +-- provisioning/          # user + API-key provisioning flow
|   +-- handlers/              # chat, routing admin, analytics, provision/revoke, metrics, secrets
|   +-- guardrail/             # guardrail pipeline + providers
|   +-- observability/         # event bus / SSE + analytics queries
|   +-- clickhouse/            # tracing persistence (traces, prompt/response, tool calls)
|   +-- proxy/                 # provider abstraction, router, registry, upstream, selector,
|   |                          # routing rules/store, routing metrics (registry.go, router.go,
|   |                          # upstream.go, selector.go, routing.go, routing_store.go, metrics.go)
|   +-- secrets/               # secret stores (env, file, Vault, AWS, Azure)
|   +-- dashboard/             # embedded HTML dashboard (Go)
|
+-- web/                       # in-app dashboard (React + Vite, served on :3000)
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
| `policy` | `policy/` | Identity-based authorization (RBAC roles/permissions) | Inspect request content or forward requests |
| `handlers` | `handlers/` | HTTP validation, body reading, response writing | Know provider-specific details |
| `guardrail` | `guardrail/` | Policy enforcement (input + output) | Forward requests |
| `proxy` | `proxy/` | Provider communication, routing (router/registry/selector), resilient forwarding (timeout/retry/breaker/failover) | Handle HTTP concerns beyond outbound calls |
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

### Identity vs Content Policy

Two policy layers with distinct concerns:

- `guardrail` inspects **content** — is this request/response body safe and allowed?
- `policy` inspects **identity** — is this principal permitted this action?

They are separate packages because they change for different reasons and operate on different inputs. Both use a chain/registry evaluated with a `Decision`, and both fail closed, so the codebase stays consistent without coupling them.

### Hash-only Secret Storage

API-key plaintext is never persisted. Only `SHA-256(secret + pepper)` is stored; the full key is returned to the caller exactly once at provisioning time.
