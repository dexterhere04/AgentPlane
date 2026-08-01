# Future Roadmap

## Planned Pipeline

V0 is a minimal proxy. The architecture is designed so new stages can be inserted between the handler and proxy layers without changing existing code.

```
Incoming Request
        |
        v
  Authentication      [validate API keys, JWT, OAuth]
        |
        v
  Rate Limiting       [per-user, per-token quotas]
        |
        v
  Logging             [request/response audit trail]
        |
        v
  Agent Routing       [route to specific agent pipelines]
        |
        v
  Provider Selection  [choose OpenAI vs Anthropic vs Gemini]
        |
        v
  API Key Injection   [existing — config.OpenAIKey()]
        |
        v
  Provider Proxy      [existing — proxy.ForwardChat()]
        |
        v
  Response            [response transformation, streaming]
```

## Provider Expansion

Add new provider files to `internal/proxy/`:

| File | Provider | Endpoint |
|------|----------|----------|
| `openai.go` | OpenAI | `https://api.openai.com/v1/chat/completions` |
| `anthropic.go` (planned) | Anthropic | `https://api.anthropic.com/v1/messages` |
| `gemini.go` (planned) | Google Gemini | `https://generativelanguage.googleapis.com/` |
| `groq.go` (planned) | Groq | `https://api.groq.com/openai/v1/chat/completions` |
| `ollama.go` (planned) | Ollama (local) | `http://localhost:11434/api/chat` |

## Configuration Sources

Swap `os.Getenv()` for richer config backends:

- YAML configuration files
- HashiCorp Vault
- Kubernetes Secrets
- AWS Secrets Manager

## Streaming Support

Add Server-Sent Events (SSE) streaming for real-time token delivery:

```
POST /chat (stream: true)
  → SSE response streaming tokens as they arrive
```
