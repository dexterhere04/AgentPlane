# Architecture

## Overview

AgentPlane uses a layered architecture following the Single Responsibility Principle. Each package owns exactly one concern.

```
                 Client
                    |
              POST /chat
                    |
                    v
          +------------------+
          |   HTTP Server    |  (cmd/server/main.go)
          +--------+---------+
                   |
                   v
          +------------------+
          |     Handler      |  (internal/handlers/chat.go)
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
|       +-- main.go
|
+-- internal/
|   +-- config/
|   |   +-- config.go
|   |
|   +-- handlers/
|   |   +-- chat.go
|   |
|   +-- proxy/
|       +-- openai.go
|
+-- .env
+-- .gitignore
+-- go.mod
+-- Readme.md
+-- docs/
    +-- ...
```

## Layer Responsibilities

| Layer | File | Responsibility | Must NOT |
|-------|------|---------------|----------|
| `cmd/server` | `main.go` | Application entry point, route registration, server startup | Call OpenAI, read request bodies, store configuration |
| `config` | `config.go` | Provide configuration values to the application | Implement business logic |
| `handlers` | `chat.go` | HTTP request validation, body reading, response writing | Know provider-specific details |
| `proxy` | `openai.go` | Provider communication, API key injection, request forwarding | Handle HTTP concerns beyond outbound calls |

## Design Principles

### Single Responsibility

Each package has exactly one reason to change:
- Change the provider → only `proxy` changes
- Change the HTTP framework → only `handlers` changes
- Change configuration source → only `config` changes
- Change the server mux → only `cmd/server` changes

### Zero Dependencies

V0 uses only the Go standard library. No external packages are imported. This keeps the binary small, build times fast, and the attack surface minimal.

### Provider Abstraction

The `proxy` package encapsulates all provider-specific knowledge (URLs, auth headers, error formats). Adding a new provider means adding a new file to `proxy/` without touching handlers or config.

### Config Abstraction

The `config` package wraps `os.Getenv()` behind functions like `OpenAIKey()`. This allows a future swap from env vars to YAML, Vault, or Kubernetes Secrets without changing any callers.
