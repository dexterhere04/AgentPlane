# Future Roadmap

## Planned Pipeline

The architecture is designed so new stages can be inserted between the handler and proxy layers without changing existing code. Stages marked ✅ are implemented in the current codebase.

```
Incoming Request
        |
        v
  Authentication      ✅  API-key bearer auth (internal/auth)
        |
        v
  Rate Limiting       [per-user, per-token quotas]
        |
        v
  Logging             [request/response audit trail]
        |
        v
  Guardrails (Input)  ✅  internal/guardrail (input set)
        |
        v
  Agent Routing       [route to specific agent pipelines]
        |
        v
  Provider Selection  [choose OpenAI vs Anthropic vs Gemini]
        |
        v
  API Key Injection   ✅  config.OpenAIKey() → proxy
        |
        v
  Provider Proxy      ✅  proxy.OpenAIProvider.Forward()
        |
        v
  Guardrails (Output) ✅  internal/guardrail (output set)
        |
        v
  Response            [live SSE streaming back to client]
```

## Provider Expansion

Add new provider types implementing the `Provider` interface (`Name()`, `Forward()`) in `internal/proxy/`:

| File | Provider | Endpoint |
|------|----------|----------|
| `openai.go` | OpenAI (any OpenAI-compatible API) | `{OPENAI_BASE_URL}/chat/completions` |
| `anthropic.go` (planned) | Anthropic | `https://api.anthropic.com/v1/messages` |
| `gemini.go` (planned) | Google Gemini | `https://generativelanguage.googleapis.com/` |
| `groq.go` (planned) | Groq | `https://api.groq.com/openai/v1/chat/completions` |
| `ollama.go` (planned) | Ollama (local) | `http://localhost:11434/api/chat` |

## Configuration Sources

Implemented backends are available via `SECRET_STORE` (see `docs/secrets-backends.md`):

- ✅ Environment variables (`env`)
- ✅ Secret files (`file`, `chain`)
- ✅ HashiCorp Vault (`vault`, with AppRole support)
- ✅ AWS Secrets Manager (`aws`)
- ✅ Azure Key Vault (`azure`)
- YAML configuration files (not yet)

## Streaming Support

The gateway currently detects `"stream": true` and coalesces the upstream SSE stream into a single completion (see `docs/packages/proxy.md`). True end-to-end SSE streaming back to the client is planned:

```
POST /chat (stream: true)
  → SSE response streaming tokens as they arrive
```

## Other Candidates

- Transactional user + key provisioning (currently sequential by design)
- Per-key rate limiting and quotas
- Key rotation UI / admin dashboard CRUD
- Configurable provider timeout
