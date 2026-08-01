# AgentPlane Documentation

> **Version:** 0.1.0

## Index

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | High-level architecture, project structure, and design principles |
| [API Reference](api-reference.md) | `POST /chat` endpoint specification |
| [Request Flow](request-flow.md) | Step-by-step request lifecycle |
| [Setup & Running](setup.md) | Build, configure, and run the server |
| [Future Roadmap](future-roadmap.md) | Planned pipeline stages and evolution |
| [go.mod](go-mod.md) | Module definition and dependency management |
| [.gitignore](gitignore.md) | Version control exclusion rules |

## Package Reference

| Package | Path | Document |
|---------|------|----------|
| `cmd/server` | `cmd/server/main.go` | [docs](packages/cmd-server.md) |
| `config` | `internal/config/config.go` | [docs](packages/config.md) |
| `handlers` | `internal/handlers/chat.go` | [docs](packages/handlers.md) |
| `proxy` | `internal/proxy/openai.go` | [docs](packages/proxy.md) |

## Quick Links

- **Entry point:** `cmd/server/main.go`
- **HTTP handler:** `internal/handlers/chat.go` — function `Chat()`
- **Provider proxy:** `internal/proxy/openai.go` — function `ForwardChat()`
- **Configuration:** `internal/config/config.go` — function `OpenAIKey()`
- **Default port:** `3001`
- **Required env:** `OPENAI_API_KEY`

## Project Summary

AgentPlane is an AI Gateway — a middleware layer between AI applications and LLM providers. It accepts `POST /chat` requests, injects the OpenAI API key, forwards to `https://api.openai.com/v1/chat/completions`, and returns the response unchanged.

Built in Go 1.26.5 with zero external dependencies (standard library only).
