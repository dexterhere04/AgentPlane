# Package: `guardrail`

**Files:** `internal/guardrail/` (16 files)

**Package:** `guardrail`

## Overview

Enforces policy checks on requests and responses flowing through the gateway. Guardrails are composed into directional sets (input/output) and evaluated through a centralized `EnforcementPoint` that resolves per-guardrail modes and publishes events.

Guardrails run at two points in the request lifecycle:
- **Input guardrails** — inspect the request body before it reaches the provider
- **Output guardrails** — inspect the provider response before it reaches the client

## Architecture

```
Registry (in-memory map of guardrails by name)
  ├── prompt_injection   →  PromptInjectionGuardrail (TypeMandatory)
  ├── secrets            →  SecretsGuardrail           (TypeMandatory)
  ├── pii                →  PIIGuardrail               (TypeMandatory)
  └── content_moderation →  ContentModerationGuardrail (TypePolicy)

EnforcementPoint (orchestrates guardrails per direction)
  ├── GuardrailSet ← input spec:  [prompt_injection, secrets, pii]
  └── GuardrailSet ← output spec: [secrets, pii]

Request flow:
  Body → EnforcementPoint.Evaluate(input set)  → Provider → EnforcementPoint.Evaluate(output set) → Response
              ↓ block / redact                                      ↓ block / redact
             403  / modified body                                 403  / modified body
```

## Core Types

### Decision

```go
type Decision int

const (
    DecisionPass    Decision = iota  // allow through
    DecisionBlock                     // return HTTP 403
    DecisionRedact                    // replace matched content, continue pipeline
    DecisionLogOnly                   // record without interfering
    DecisionWarn                      // surface warning, continue pipeline
)
```

### Mode (per-guardrail)

```go
type Mode string

const (
    ModeEnforce Mode = "enforce"   // honor the guardrail's decision exactly
    ModeLogOnly Mode = "log_only"  // downgrade all decisions to LOG_ONLY
    ModeWarn    Mode = "warn"      // downgrade BLOCK → WARN, REDACT → LOG_ONLY
    ModeOff     Mode = "off"       // skip the guardrail entirely
)
```

### GuardrailType (fail-open vs fail-closed)

```go
type GuardrailType int

const (
    TypeMandatory GuardrailType = iota  // fail-closed: errors result in DecisionBlock
    TypePolicy                           // fail-open:   errors result in DecisionPass (skip)
)
```

### Direction

```go
type Direction string

const (
    DirectionInput       Direction = "input"
    DirectionOutput      Direction = "output"
    DirectionMCPRequest  Direction = "mcp_request"
    DirectionMCPResponse Direction = "mcp_response"
)
```

### Guardrail Interface

```go
type Guardrail interface {
    Name() string
    Type() GuardrailType
    Evaluate(ctx context.Context, dir Direction, body []byte) (*Result, error)
}
```

Each guardrail receives the `Direction` and decides internally whether to inspect. Guardrails that don't apply to a direction (e.g. prompt_injection on output) return `DecisionPass`.

### Result

```go
type Result struct {
    Guardrail string         `json:"guardrail"`
    Decision  Decision       `json:"decision"`
    Message   string         `json:"message"`
    Details   map[string]any `json:"details,omitempty"`
    Findings  []Finding      `json:"findings,omitempty"`
    Redacted  []byte         `json:"-"` // modified body when DecisionRedact
}
```

### GuardrailSpec / GuardrailSet

```go
type GuardrailSpec struct {
    Name   string
    Config map[string]any  // reserved for future per-instance configuration
}

type GuardrailSet struct {
    Guards []GuardrailSpec
}
```

`GuardrailSpec` identifies a guardrail by name. `GuardrailSet` defines which guardrails run for a given direction (input or output).

### Registry

```go
type Registry struct { /* thread-safe map of guardrails by name */ }

func NewRegistry() *Registry
func (r *Registry) Register(g Guardrail)              // register a guardrail
func (r *Registry) Get(name string) (Guardrail, bool)  // lookup by name
func (r *Registry) Resolve(spec GuardrailSpec) (Guardrail, error) // resolve from spec
```

### EnforcementPoint

```go
type EnforcementPoint struct { /* registry, metrics, event bus, config */ }

func NewEnforcementPoint(registry *Registry, cfg Config, bus *observability.EventBus) *EnforcementPoint

func (ep *EnforcementPoint) Evaluate(
    ctx context.Context,
    requestID string,
    dir Direction,
    body []byte,
    set GuardrailSet,
) (*Result, error)
```

`EnforcementPoint.Evaluate` runs all guardrails in the `GuardrailSet` in order, resolving each by name from the Registry. The pipeline short-circuits on `DecisionBlock`. Redacted bodies are passed to subsequent guardrails.

### Strategy / Config

```go
type Strategy struct {
    Name    string
    Mode    Mode
    Enabled bool
    Extra   map[string]any
}

type Config struct {
    Strategies map[string]Strategy
}

func (c Config) Strategy(name string) Strategy  // returns the strategy for a guardrail name
func LoadConfig() Config                        // reads from env vars
```

### Finding / Severity

```go
type Severity string

const (
    SeverityInfo     Severity = "info"
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

type Finding struct {
    Guardrail string   `json:"guardrail"`
    Type      string   `json:"type"`      // e.g. "email", "github_pat", "hate_speech"
    Severity  Severity `json:"severity"`
    Start     int      `json:"start"`
    End       int      `json:"end"`
    Entity    string   `json:"entity"`    // e.g. "email_address", "secret"
    Value     string   `json:"value"`     // snippet (truncated for sensitive data)
}
```

### Metrics / MetricsSnapshot

```go
type Metrics struct { /* atomic counters for evaluations, blocks, redactions, warns, passes, errors; latency ring buffer */ }

func (m *Metrics) Snapshot() MetricsSnapshot

type MetricsSnapshot struct {
    EvaluationsTotal int64 `json:"guardrail_evaluations_total"`
    BlocksTotal      int64 `json:"guardrail_blocks_total"`
    RedactionsTotal  int64 `json:"guardrail_redactions_total"`
    WarnsTotal       int64 `json:"guardrail_warns_total"`
    PassedTotal      int64 `json:"guardrail_passed_total"`
    ErrorsTotal      int64 `json:"guardrail_errors_total"`
    AvgLatencyMs     int64 `json:"guardrail_evaluation_duration_avg_ms"`
}
```

### Action

```go
type Action int

const (
    ActionDetect  Action = iota
    ActionBlock
    ActionRedact
    ActionWarn
)

func DecisionToAction(d Decision) Action  // maps Decision → Action
```

### Content Extraction

```go
type ContentType string

const (
    ContentTypeSystem     ContentType = "system"
    ContentTypeUser       ContentType = "user"
    ContentTypeAssistant  ContentType = "assistant"
    ContentTypeToolCall   ContentType = "tool_call"
    ContentTypeToolResult ContentType = "tool_result"
)

type Content struct {
    Type   ContentType `json:"type"`
    Text   string      `json:"text"`
    Role   string      `json:"role,omitempty"`
    Source string      `json:"source,omitempty"`
}

func ExtractContent(body []byte) ([]Content, error)
```

`ExtractContent` parses structured chat request bodies (with a `messages` array) and returns extracted `Content` segments. Non-JSON or unrecognized formats fall back to a single `ContentTypeUser` entry with the raw body. Tool call arguments are extracted into separate `Content` entries.

### GuardrailError

```go
type GuardrailError struct {
    Guardrail string
    Err       error
}

var ErrGuardrailUnavailable = fmt.Errorf("request could not be evaluated by mandatory security controls")
```

## EnforcementPoint (Pipeline Execution)

### Setup example

```go
import (
    "github.com/dexterhere04/AgentPlane/internal/guardrail"
    guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
    guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
    "github.com/dexterhere04/AgentPlane/internal/observability"
)

func main() {
    bus := observability.DefaultBus
    cfg := guardrail.LoadConfig()

    // 1. Create registry and register guardrails
    registry := guardrail.NewRegistry()
    registry.Register(guardrail.NewPromptInjectionGuardrail(cfg.Strategy("prompt_injection")))
    registry.Register(guardsecrets.New(cfg.Strategy("secrets")))
    registry.Register(guardpii.New(cfg.Strategy("pii")))
    registry.Register(guardrail.NewContentModerationGuardrail(cfg.Strategy("content_moderation")))

    // 2. Create enforcement point
    enforcement := guardrail.NewEnforcementPoint(registry, cfg, bus)

    // 3. Define directional guardrail sets
    inputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            {Name: "prompt_injection"},
            {Name: "secrets"},
            {Name: "pii"},
        },
    }
    outputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            {Name: "secrets"},
            {Name: "pii"},
        },
    }

    // 4. Use in handler
    enforcement.Evaluate(ctx, requestID, guardrail.DirectionInput, body, inputSet)
    // ... forward to provider ...
    enforcement.Evaluate(ctx, requestID, guardrail.DirectionOutput, respBody, outputSet)
}
```

### Execution flow for each guardrail

1. Resolve guardrail by name from the Registry (skip if not found)
2. Check the Strategy — skip if `Enabled` is false
3. Call `guardrail.Evaluate(ctx, direction, body)`
4. On error: if `TypeMandatory` → return `DecisionBlock` (fail-closed); if `TypePolicy` → skip, continue (fail-open)
5. Record metrics (evaluation count, latency)
6. Publish the raw result to EventBus
7. Resolve effective decision based on configured `Mode`:
   - `ModeEnforce` → pass raw result unchanged
   - `ModeLogOnly` → downgrade to `DecisionLogOnly`, clear redacted body
   - `ModeWarn` → downgrade `BLOCK` → `WARN`, `REDACT` → `LOG_ONLY`
8. If effective decision is `DecisionBlock` → stop pipeline, return result immediately
9. If effective decision is `DecisionRedact` → replace body with redacted version, continue
10. If body was modified by any guardrail → return `DecisionRedact` with final body; else `DecisionPass`

### Mode Resolution Table

| Guardrail Decision | Mode `enforce` | Mode `log_only` | Mode `warn` |
|---|---|---|---|
| `BLOCK` | BLOCK (stop) | LOG_ONLY (continue) | WARN (continue) |
| `REDACT` | REDACT (modify) | LOG_ONLY (continue) | LOG_ONLY (continue) |
| `WARN` | WARN (continue) | LOG_ONLY (continue) | WARN (continue) |
| `LOG_ONLY` | LOG_ONLY (continue) | LOG_ONLY (continue) | LOG_ONLY (continue) |
| `PASS` | PASS (continue) | PASS (continue) | PASS (continue) |

## Guardrail Implementations

### 1. Prompt Injection (`prompt_injection.go`)

Detects attempts to override system instructions or extract hidden prompts using regex keyword patterns. Uses `ExtractContent` to inspect individual message segments.

**Type:** `TypeMandatory` (fail-closed)
**Default mode:** `enforce`
**Direction:** input only
**Decision:** `BLOCK` on match, `PASS` otherwise

**Patterns matched:** "ignore previous instructions", "reveal your system prompt", "you are now DAN", "pretend you are", "override safety guidelines", "new system instructions", "act as", "forget all previous context", "do not follow your instructions", "respond as unfiltered", etc. (15 patterns)

### 2. Secrets Detection (`providers/secrets/secrets.go`)

Detects credentials and sensitive tokens using regex patterns (28 patterns).

**Type:** `TypeMandatory` (fail-closed)
**Default mode:** `enforce`
**Direction:** input and output
**Decision:** `BLOCK` on match, `PASS` otherwise

**Patterns matched:** OpenAI keys (`sk-`, `sk-proj-`), GitHub tokens (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`), AWS access/secret keys (`AKIA`, `AROA`, `ASIA`, etc.), Google API keys (`AIza`), Google OAuth IDs, Stripe live/test keys (`sk_live_`, `rk_live_`, `sk_test_`), Slack tokens/ webhooks (`xox`), JWT tokens, private keys (`-----BEGIN PRIVATE KEY-----`), PGP keys, Azure storage/SAS tokens, Heroku API keys, bearer/basic auth tokens, password-in-URI, Discord webhooks, SendGrid API keys, Twilio SIDs/auth tokens, password/key assignments, generic API keys.

### 3. PII Detection (`providers/pii/pii.go`)

Detects and redacts personally identifiable information. Supports pluggable `Detector` implementations.

**Type:** `TypeMandatory` (fail-closed)
**Default mode:** `enforce`
**Direction:** input and output
**Decision:** `REDACT` on match, `PASS` otherwise

**Patterns matched:** email addresses, US phone numbers, SSNs, credit card numbers (Visa, MC, Amex, Discover, Diners Club, JCB), IP addresses, street addresses, dates with context. (7 rule types via `RegexDetector`)

Each PII type is replaced with a placeholder: `[EMAIL REDACTED]`, `[PHONE REDACTED]`, `[SSN REDACTED]`, `[CREDIT CARD REDACTED]`, `[IP REDACTED]`, `[ADDRESS REDACTED]`, `[DATE REDACTED]`.

#### Pluggable Detectors

The PII guardrail supports custom detector plugins via the `Detector` interface:

```go
type Detector interface {
    Name() string
    Detect(ctx context.Context, content string) ([]guardrail.Finding, error)
}

// Register a custom detector at any time
piiGuard := guardpii.New(strategy)
piiGuard.RegisterDetector(myCustomDetector{})

// Or create with detectors upfront
piiGuard := guardpii.NewWithDetectors(strategy, &guardpii.RegexDetector{}, myCustomDetector{})
```

### 4. Content Moderation (`content_moderation.go`)

Detects harmful or policy-violating content across multiple categories.

**Type:** `TypePolicy` (fail-open)
**Default mode:** `warn`
**Direction:** input and output
**Decision:** `WARN` on match, `PASS` otherwise

**Categories:** hate_speech, violence, sexual_content, self_harm, abuse_harassment. Each category has its own set of keyword patterns. Severity is calculated based on total match count across all categories: low (1-3 matches), medium (4-10), high (>10).

The result `Details` map includes per-category match counts and aggregate severity.

## Streaming Support (`streaming/buffer.go`)

### Buffer

The `Buffer` wraps a single guardrail for evaluating SSE (Server-Sent Events) streaming responses. It accumulates incoming chunks and evaluates the guardrail against the accumulated content.

```go
import "github.com/dexterhere04/AgentPlane/internal/guardrail/streaming"

type Buffer struct { /* thread-safe internal buffer, max size, guardrail reference */ }

func NewBuffer(maxSize int, g guardrail.Guardrail) *Buffer  // maxSize defaults to 4096

func (b *Buffer) Write(chunk []byte) ([]byte, error)  // accumulate chunk, evaluate guardrail; returns nil on block
func (b *Buffer) Flush() []byte                        // drain and return accumulated buffer
func (b *Buffer) Len() int                             // current buffer length
func (b *Buffer) IsBlocked() bool                      // whether the guardrail has blocked
func (b *Buffer) Findings() []guardrail.Finding        // copy of findings from blocked evaluation
func (b *Buffer) Reset()                               // clear buffer, unblock, clear findings
```

### Usage example

```go
buf := streaming.NewBuffer(8192, secretsGuardrail)

for {
    chunk := readSSEChunk()
    ssePayload, isSSE := streaming.ExtractSSEChunk(chunk)
    if !isSSE {
        continue
    }

    content, hasContent := streaming.ExtractDeltaContent(ssePayload)
    if !hasContent {
        continue
    }

    output, err := buf.Write([]byte(content))
    if err != nil {
        // guardrail blocked the stream
        break
    }

    // forward output chunk to client
    w.Write(output)
}

if buf.IsBlocked() {
    findings := buf.Findings()
    // log or surface findings to operator
}
```

### SSE helpers

```go
func ExtractSSEChunk(data []byte) ([]byte, bool)       // strips "data: " prefix, returns payload
func ExtractDeltaContent(payload []byte) (string, bool)  // extracts Choices[0].Delta.Content from JSON chunk
```

## Configuration

Guardrails are configured via environment variables:

```bash
GUARDRAIL_PROMPT_INJECTION=enforce   # enforce | log_only | warn | off
GUARDRAIL_SECRETS=enforce
GUARDRAIL_PII=enforce
GUARDRAIL_CONTENT_MODERATION=warn
```

Legacy suffix format is also supported:
```bash
GUARDRAIL_PROMPT_INJECTION_MODE=enforce
GUARDRAIL_SECRETS_MODE=log_only
GUARDRAIL_PII_MODE=off
```

When unset, the default is `off` (disabled). Invalid values default to `enforce`.

## Observability

All guardrail decisions are published to the EventBus with these stages:

- `guardrail_input` — input guardrail evaluation for one guardrail
- `guardrail_output` — output guardrail evaluation for one guardrail
- `guardrail_blocked` — a request or response was blocked

Each event includes the guardrail name, decision, and message. BLOCK events include the full error message. Non-PASS events with details include serialized JSON.

Metrics are tracked in the thread-safe `Metrics` struct. Call `Snapshot()` to get an atomic read of all counters:

```go
snapshot := guardrail.DefaultMetrics.Snapshot()
log.Printf("blocks=%d redactions=%d warns=%d passes=%d errors=%d avg_latency_ms=%d",
    snapshot.BlocksTotal, snapshot.RedactionsTotal, snapshot.WarnsTotal,
    snapshot.PassedTotal, snapshot.ErrorsTotal, snapshot.AvgLatencyMs)
```

## Integration

The enforcement pipeline is wired in `cmd/server/main.go` and injected into `handlers.Chat()`:

```go
import (
    "github.com/dexterhere04/AgentPlane/internal/guardrail"
    guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
    guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
    "github.com/dexterhere04/AgentPlane/internal/handlers"
    "github.com/dexterhere04/AgentPlane/internal/observability"
    "github.com/dexterhere04/AgentPlane/internal/proxy"
)

func main() {
    bus := observability.DefaultBus
    provider := proxy.NewOpenAIProviderFromEnv()

    cfg := guardrail.LoadConfig()

    registry := guardrail.NewRegistry()
    registry.Register(guardrail.NewPromptInjectionGuardrail(cfg.Strategy("prompt_injection")))
    registry.Register(guardsecrets.New(cfg.Strategy("secrets")))
    registry.Register(guardpii.New(cfg.Strategy("pii")))
    registry.Register(guardrail.NewContentModerationGuardrail(cfg.Strategy("content_moderation")))

    enforcement := guardrail.NewEnforcementPoint(registry, cfg, bus)

    inputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            {Name: "prompt_injection"},
            {Name: "secrets"},
            {Name: "pii"},
        },
    }
    outputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            {Name: "secrets"},
            {Name: "pii"},
        },
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
        handlers.Chat(w, r, enforcement, inputSet, outputSet, provider)
    })
}
```

The handler calls `enforcement.Evaluate()` for input guardrails after JSON validation and for output guardrails after the proxy returns. Passing `nil` for enforcement or empty `GuardrailSet` disables that direction's checks.

### Handler response behavior

| Decision | HTTP Status | Body |
|---|---|---|
| `DecisionBlock` | 403 Forbidden | `{"error": {"type": "guardrail_blocked", "message": "...", "guardrail": "...", "findings": [...]}}` |
| `DecisionRedact` | 200 OK (pipeline continues) | Redacted body forwarded to provider or client |
| `DecisionWarn` | 200 OK (pipeline continues) | Warning logged to EventBus, body passes through |
| `DecisionLogOnly` | 200 OK (pipeline continues) | Event logged, no body modification |
| Guardrail error (mandatory) | 503 Service Unavailable | `{"error": {"type": "guardrail_unavailable", "message": "..."}}` |

## Adding a Custom Guardrail

Implement the `Guardrail` interface and register it:

```go
package myguard

import (
    "context"
    "github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type MyGuardrail struct{}

func New(strategy guardrail.Strategy) *MyGuardrail {
    return &MyGuardrail{}
}

func (g *MyGuardrail) Name() string {
    return "my_guard"
}

func (g *MyGuardrail) Type() guardrail.GuardrailType {
    return guardrail.TypePolicy  // or TypeMandatory for fail-closed
}

func (g *MyGuardrail) Evaluate(ctx context.Context, dir guardrail.Direction, body []byte) (*guardrail.Result, error) {
    if dir == guardrail.DirectionOutput {
        return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
    }

    if matches := detectSomething(body); len(matches) > 0 {
        var findings []guardrail.Finding
        for _, m := range matches {
            findings = append(findings, guardrail.Finding{
                Guardrail: g.Name(),
                Type:      "my_detection",
                Severity:  guardrail.SeverityHigh,
                Start:     m.Start,
                End:       m.End,
                Entity:    "my_entity",
                Value:     m.Value,
            })
        }
        return &guardrail.Result{
            Guardrail: g.Name(),
            Decision:  guardrail.DecisionBlock,
            Message:   "detected suspicious pattern",
            Findings:  findings,
        }, nil
    }

    return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}

// Register and use
registry.Register(myguard.New(cfg.Strategy("my_guard")))
```

## Testing

Tests cover each guardrail's detection patterns, enforcement order, mode resolution, block short-circuiting, redact chaining, disabled guardrail skipping, content extraction, findings, metrics snapshots, action conversions, config loading, and streaming buffer accumulation and detection. Run with:

```bash
go test ./internal/guardrail/ -v        # unit tests
go test ./tests/ -run Guardrail -v       # integration tests
```

Test files: `guardrail_test.go`, `guardrail_secrets_test.go`, `guardrail_pii_test.go`, `guardrail_streaming_test.go`, `handler_test.go` (guardrail integration), `helpers_test.go` (mock guardrail implementations).

## Design Principles

- **Modular:** Each guardrail is independent and composable. Add or remove without changing other checks.
- **Observable:** Every decision is published as an event through the existing EventBus. Metrics are tracked with atomic counters.
- **Configurable:** Per-guardrail mode via environment variables. Guardrail sets are defined per direction.
- **Extensible:** The `Guardrail` interface allows future integration with external engines (LiteLLM, Bifrost, etc.). PII supports pluggable `Detector` implementations. Custom guardrails implement a single `Evaluate` method.
- **Fail-safe:** Mandatory guardrails fail-closed (block on error); policy guardrails fail-open (pass on error).
- **Zero dependencies:** Uses only the Go standard library (except for the project-internal `observability` package).
