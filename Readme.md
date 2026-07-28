# AgentPlane V0 Architecture

> **Version:** 0.1.0
> **Status:** Design Document

---

# Overview

AgentPlane is an **AI Gateway** that sits between AI applications and Large Language Model (LLM) providers.

Instead of applications communicating directly with OpenAI, Anthropic, Gemini, or other providers, they communicate with AgentPlane. The gateway is responsible for securely forwarding requests to the appropriate provider.

The first version (V0) intentionally focuses on doing **one thing well**:

> Accept an HTTP request, attach the provider API key, forward it to the LLM provider, and return the response.

No request transformation, authentication, routing, or databases are included in this version.

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
│   └── server/
│       └── main.go
│
├── internal/
│   ├── config/
│   │   └── config.go
│   │
│   ├── handlers/
│   │   └── chat.go
│   │
│   └── proxy/
│       └── openai.go
│
├── .env
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

    │

    ▼

main.go

    │

Registers HTTP routes

    │

    ▼

handlers.Chat()

    │

Validates request

Reads request body

    │

    ▼

proxy.ForwardChat()

    │

Loads API key

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

proxy

    │

Returns response

    │

    ▼

handler

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
| `proxy`      | Provider communication   |

Because responsibilities are isolated, changing one package should not require changes to others.

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
Rate Limiting
        │
        ▼
Logging
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
