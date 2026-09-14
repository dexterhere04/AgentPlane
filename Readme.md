# AgentPlane V0 Architecture

> **Version:** 0.1.0
> **Status:** Design Document

---

# Overview

AgentPlane is an **AI Gateway** that sits between AI applications and Large Language Model (LLM) providers.

Instead of applications communicating directly with OpenAI, Anthropic, Gemini, or other providers, they communicate with AgentPlane. The gateway is responsible for securely forwarding requests to the appropriate provider.

The gateway:

> Accepts an HTTP request, authenticates the caller's API key, runs configurable
> guardrails, attaches the provider API key, forwards the request to the LLM
> provider, and returns the response.

Request-level authentication (API keys mapped to users) and a PostgreSQL
backend for users and API keys are included.

---

# High-Level Architecture

```text
                   Client

             POST /chat
                    │
                    ▼
        ┌──────────────────────┐
        │      AgentPlane      │
        │                      │
        │   HTTP Server        │
        │   HTTP Handler       │
        │   Provider Proxy     │
        └──────────┬───────────┘
                   │
                   ▼
             OpenAI API
                   │
                   ▼
                Response
```

---

# Project Structure

```text
agentplane/
│
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── keygeneration/
│       └── main.go
│
├── internal/
│   ├── config/          # configuration + secret-store wiring
│   ├── db/              # Postgres pool + embedded migrations
│   ├── api/             # API-key generation + persistence
│   ├── auth/            # API-key authentication + middleware
│   ├── policy/          # identity-based authorization (RBAC roles/permissions)
│   ├── users/           # user persistence
│   ├── provisioning/    # user + API-key provisioning flow
│   ├── handlers/        # HTTP handlers (chat, provision, revoke, metrics)
│   ├── guardrail/       # guardrail pipeline + providers
│   ├── observability/   # event bus / SSE
│   ├── proxy/           # provider forwarding
│   └── secrets/         # secret stores (Vault, AWS, Azure, env, file)
│
├── migrations/          # embedded SQL migrations
├── .env
├── .env.example
├── .gitignore
├── go.mod
└── README.md
```

---

# Folder Responsibilities

## cmd/

Contains executable applications.

For V0 there is only one executable:

```text
cmd/
└── server/
    └── main.go
```

### Responsibility

* Application entry point
* Initialise configuration
* Create HTTP server
* Register routes
* Start listening for requests

### Should NOT

* Call OpenAI directly
* Read request bodies
* Implement business logic
* Store configuration

Think of `main.go` as the person who wires everything together.

---

## internal/

Contains the implementation of AgentPlane.

Everything that makes the gateway work lives here.

---

## internal/config/

```text
config/
└── config.go
```

### Responsibility

Provide configuration to the rest of the application.

Initially configuration is loaded from environment variables.

Example:

```text
OPENAI_API_KEY=sk-xxxxxxxx
```

Instead of every package reading environment variables directly,

```go
os.Getenv(...)
```

the rest of the application should call

```go
config.OpenAIKey()
```

This allows configuration sources to change later without affecting other packages.

Future configuration sources may include:

* YAML
* HashiCorp Vault
* Kubernetes Secrets
* AWS Secrets Manager

---

## internal/handlers/

```text
handlers/
└── chat.go
```

### Responsibility

Handle HTTP requests.

A handler should only be concerned with HTTP.

Responsibilities include:

* Accept incoming requests
* Validate HTTP methods
* Read request body
* Call the provider proxy
* Return the response

Handlers should not know how OpenAI works.

Think of the handler as the receptionist of the application.

```
HTTP Request

↓

Handler

↓

Provider
```

---

## internal/proxy/

```text
proxy/
└── openai.go
```

### Responsibility

Communicate with AI providers.

This package knows:

* Provider URL
* Required headers
* Authentication
* Request forwarding
* Response handling

For V0 only OpenAI is supported.

Future versions may include:

```text
providers/

openai.go

anthropic.go

gemini.go

groq.go

ollama.go
```

The rest of the application should not need to know provider-specific implementation details.

---

# Root Files

## .env

Temporary development configuration.

Example:

```text
OPENAI_API_KEY=sk-xxxxxxxx
```

Never commit this file.

---

## .gitignore

Prevents unnecessary files from being committed.

Typical entries:

```text
.env
bin/
build/
logs/
```

---

## go.mod

Defines the Go module and manages project dependencies.

---

## README.md

Provides project documentation.

Should include:

* Project overview
* Installation
* Running the server
* Example requests
* Roadmap

---

# Request Flow

The following sequence describes how a request travels through the application.

```
Client

    │

POST /chat
Authorization: Bearer ap_live_<key_id>_<secret>

    │

    ▼

main.go

    │

Registers HTTP routes
(wraps /chat in auth middleware)

    │

    ▼

auth.Middleware

    │

Parses + verifies API key
Resolves user, attaches to context

    │

    ▼

handlers.Chat()

    │

Validates method + JSON body
Runs input guardrails

    │

    ▼

provider.Forward()

    │

Loads upstream API key

Creates outbound request

Adds Authorization header

Sends request

    │

    ▼

OpenAI API

    │

Processes request

    │

Returns response

    │

    ▼

handlers.Chat()

    │

Runs output guardrails

    │

Writes HTTP response

    │

    ▼

Client
```

---

# Component Interaction

```text
                 Client
                    │
                    ▼
          ┌────────────────┐
          │ HTTP Server    │
          └───────┬────────┘
                  │
                  ▼
          ┌────────────────┐
          │ Handler        │
          └───────┬────────┘
                  │
                  ▼
          ┌────────────────┐
          │ Provider Proxy │
          └───────┬────────┘
                  │
                  ▼
             OpenAI API
```

Each layer has a single responsibility.

* HTTP Server → Accepts connections.
* Handler → Processes HTTP requests.
* Proxy → Talks to AI providers.

---

# Why This Structure?

The architecture follows the **Single Responsibility Principle**.

Each package owns exactly one concern.

| Package      | Responsibility           |
| ------------ | ------------------------ |
| `cmd/server` | Application startup      |
| `config`     | Configuration management |
| `handlers`   | HTTP request handling    |
| `auth`       | API-key authentication   |
| `policy`     | Authorization (RBAC)     |
| `users`      | User persistence         |
| `api`        | API-key lifecycle        |
| `provisioning` | User + key provisioning |
| `db`         | Postgres pool + migrations |
| `guardrail`  | Policy enforcement       |
| `proxy`      | Provider communication   |
| `secrets`    | Secret-store backends    |
| `observability` | Event bus / SSE        |
| `dashboard`  | HTML dashboard           |

Because responsibilities are isolated, changing one package should not require changes to others.

---

# Authentication & Database

AgentPlane authenticates callers with API keys and persists users and keys in
PostgreSQL. Requests are then authorized through a deny-by-default RBAC policy
layer: a user must be assigned a role before they can use the gateway. See
`docs/policy-rbac.md`.

## Endpoints

| Method | Path                     | Auth              | Purpose                                    |
| ------ | ------------------------ | ----------------- | ------------------------------------------ |
| POST   | `/chat`                  | Bearer API key    | Forward a request to the LLM provider      |
| POST   | `/provision/user`        | Bearer admin token | Create a user and mint an API key          |
| POST   | `/admin/api-keys/revoke` | Bearer admin token | Revoke an API key by `key_id`              |
| GET    | `/admin/roles`           | Bearer admin token | List roles and their permissions           |
| POST   | `/admin/roles`           | Bearer admin token | Create a role                              |
| POST   | `/admin/users/{id}/roles`| Bearer admin token | Assign a role to a user                    |
| GET    | `/events`                | —                 | Server-sent event stream                   |
| GET    | `/dashboard`             | —                 | HTML dashboard                             |
| GET    | `/metrics`               | —                 | Metrics                                    |

## Configuration

Required environment variables are documented in `.env.example`:

- `DATABASE_URL` — PostgreSQL connection string (tables are created
  automatically on startup via the embedded migrations in `migrations/`).
- `API_KEY_PEPPER` — server-side secret mixed into API-key hashes.
- `AGENTPLANE_ADMIN_TOKEN` — token for the admin endpoints.
- `OPENAI_API_KEY` — upstream provider key (loaded via the configured secret
  store).

## Provisioning flow

```text
POST /provision/user   (admin token)
        │
        ▼
Create user ──► Generate API key ──► Store key_id + secret_hash only
        │
        ▼
Return full API key (shown exactly once)
```

Only the SHA-256 hash of the key secret (mixed with the pepper) is stored;
the plaintext key is returned once and never persisted.

---

# Future Evolution

The proxy built in V0 becomes the foundation for the full AgentPlane architecture.

Future request pipeline:

```text
Incoming Request
        │
        ▼
Authentication
        │
        ▼
Authorization (RBAC)
        │
        ▼
Rate Limiting
        │
        ▼
Logging
        │
        ▼
Guardrails (Input)
        │
        ▼
Agent Routing
        │
        ▼
Provider Selection
        │
        ▼
API Key Injection
        │
        ▼
Provider Proxy
        │
        ▼
Guardrails (Output)
        │
        ▼
Response
```

Each new feature can be inserted into the pipeline without changing the existing provider proxy.

---

# V0 Goals

The first milestone is complete when AgentPlane can:

* Start an HTTP server
* Accept `POST /chat`
* Read the request body
* Load the OpenAI API key from configuration
* Forward the request to OpenAI
* Return the provider's response unchanged

Once this works, AgentPlane becomes a functional AI Gateway that can be extended into a production-grade AI control plane.
