# Package: `guardrail`

**Files:** `internal/guardrail/` (13 files + `streaming/`) + `internal/guardrail/providers/` (10 packages) + `internal/guardrail/providers/functions/` (3 files)

**Package:** `guardrail`

## Overview

Enforces policy checks on requests and responses flowing through the gateway. Guardrails are composed into directional sets (input/output) and evaluated through a centralized `EnforcementPoint` that resolves per-guardrail modes and publishes events.

Guardrails run at two points in the request lifecycle:
- **Input guardrails** — inspect the request body before it reaches the provider
- **Output guardrails** — inspect the provider response before it reaches the client

## Architecture

```
Registry (in-memory map of guardrails by name)
  ── Internal (built-in, in-process regex)
  │   ├── prompt_injection   →  PromptInjectionGuardrail
  │   ├── secrets            →  SecretsGuardrail
  │   ├── pii                →  PIIGuardrail
  │   └── content_moderation →  ContentModerationGuardrail
  ── External (API-based, fail-open)
  │   ├── aim                →  AIMGuardrail
  │   ├── lakera             →  LakeraGuardrail
  │   ├── lumigator          →  LumigatorGuardrail
  │   ├── prisma_airs        →  PrismaAIRSGuardrail
  │   ├── nvidia_content     →  NvidiaContentGuardrail
  │   ├── openai_moderation  →  OpenAIModerationGuardrail
  │   └── zscaler            →  ZscalerGuardrail
  ── Functions (configurable logic, available but off by default)
      ├── regex_match            ┐
      ├── contains               │
      ├── contains_code          │
      ├── ends_with              │ pattern/text match
      ├── valid_urls             │
      ├── all_uppercase          │
      ├── all_lowercase          ┘
      ├── json_schema            ┐
      ├── json_keys              │
      ├── jwt                    │ request structure validation
      ├── not_null               │
      ├── required_metadata_keys │
      ├── allowed_request_types  │
      ├── model_whitelist        │
      ├── model_rules            ┘
      ├── word_count             ┐
      ├── sentence_count         │ constraint checks
      ├── character_count        ┘
      ├── webhook                (external delegation)
      ├── log                    ┐
      ├── add_prefix             │ transformers (content modification)
      └── regex_replace          ┘

EnforcementPoint (orchestrates guardrails per direction)
  ├── GuardrailSet ← input spec:  [33 guards — all available]
  └── GuardrailSet ← output spec: [21 guards — output-relevant subset]

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

### Fail-open vs fail-closed (`Required` flag)

Fail-open/fail-closed is expressed per-spec via `GuardrailSpec.Required`, not via a guardrail type:

- `Required: true`  → fail-closed: a resolution or evaluation error returns `DecisionBlock` (and the handler returns 503).
- `Required: false` → fail-open: a disabled guardrail is skipped and an evaluation error is logged then skipped.

> The current wiring in `cmd/server/main.go` registers every guardrail as optional (`Required` is unset). Individual guardrails therefore only run when enabled via `GUARDRAIL_*` env vars (mode `enforce`/`warn`/`log_only`), and errors fail-open. The fail-closed path is implemented and used whenever a spec sets `Required: true`.

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
    Name     string
    Required bool            // fail-closed (error → block) when true
    Config   map[string]any  // reserved for future per-instance configuration
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

1. Resolve guardrail by name from the Registry (skip if not found, unless `Required`)
2. Check the Strategy — skip if `Enabled` is false (optional guardrails)
3. Call `guardrail.Evaluate(ctx, direction, body)`
4. On error: if `Required` → return `DecisionBlock` (fail-closed); otherwise → skip, continue (fail-open)
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

## Guardrail Catalog

AgentPlane ships with **33 guardrails** organized into three tiers:

| Tier | Count | Location | Default decision | Latency |
|---|---|---|---|---|
| **Internal** | 4 | `internal/guardrail/` + `providers/{secrets,pii}` | block / redact / warn | <1ms (regex in-process) |
| **External** | 7 | `internal/guardrail/providers/{aim,lakera,...}` | block / warn | 5-500ms (API call) |
| **Functions** | 22 | `internal/guardrail/providers/functions/` | varies | <1ms (in-process) |

> Fail-open vs fail-closed is **not** a property of the tier — it is set per-spec via `GuardrailSpec.Required` in the `GuardrailSet` (see [Fail-open vs fail-closed](#fail-open-vs-fail-closed-required-flag)). In the current `main.go`, all guardrails are optional (`Required` unset), so they run only when enabled via `GUARDRAIL_*` env vars.

---

### Tier 1: Internal Guardrails (enabled via env)

These are the core security guardrails. They run in-process using regex matching. When enabled in `enforce` mode they block/redact; whether an error fails closed depends on the spec's `Required` flag.

#### 1. Prompt Injection (`prompt_injection`)

Detects attempts to override system instructions or extract hidden prompts.

**Required:** yes (fail-closed) | **Default:** off (enable `GUARDRAIL_PROMPT_INJECTION=enforce`) | **Direction:** input only | **Decision:** `BLOCK` | **Latency:** <1ms

**When to use:** Always. This is your first line of defense against prompt injection attacks. Keep at `enforce` unless you have an external guard (e.g. Lakera) that handles this.

**Patterns matched (15):** "ignore previous instructions", "reveal your system prompt", "you are now DAN", "pretend you are", "override safety guidelines", "new system instructions", "act as", "forget all previous context", "do not follow your instructions", "respond as unfiltered", etc.

#### 2. Secrets Detection (`secrets`)

Detects credentials and sensitive tokens leaked in prompts or responses.

**Required:** yes (fail-closed) | **Default:** off (enable `GUARDRAIL_SECRETS=enforce`) | **Direction:** input + output | **Decision:** `BLOCK` | **Latency:** <1ms

**When to use:** Always on both input and output. Prevents accidental credential leaks from users pasting API keys into prompts, and from LLMs generating responses containing secrets they've memorized from training data.

**Patterns matched (28):** OpenAI keys (`sk-`, `sk-proj-`), GitHub tokens (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`), AWS access/secret keys (`AKIA`, `AROA`, `ASIA`, etc.), Google API keys (`AIza`), Google OAuth IDs, Stripe live/test keys, Slack tokens/webhooks (`xox`), JWT tokens, private keys (`-----BEGIN PRIVATE KEY-----`), PGP keys, Azure storage/SAS tokens, Heroku API keys, bearer/basic auth tokens, password-in-URI, Discord webhooks, SendGrid API keys, Twilio SIDs/auth tokens, password/key assignments, generic API keys.

#### 3. PII Detection (`pii`)

Detects and redacts personally identifiable information.

**Required:** yes (fail-closed) | **Default:** off (enable `GUARDRAIL_PII=enforce`) | **Direction:** input + output | **Decision:** `REDACT` | **Latency:** <1ms

**When to use:** Always on both input and output. Critical for GDPR/CCPA compliance. Redacts rather than blocks so the pipeline continues with sanitized content.

**Patterns matched (7):** email addresses, US phone numbers, SSNs, credit card numbers (Visa, MC, Amex, Discover, Diners Club, JCB), IP addresses, street addresses, dates with context.

**Placeholders:** `[EMAIL REDACTED]`, `[PHONE REDACTED]`, `[SSN REDACTED]`, `[CREDIT CARD REDACTED]`, `[IP REDACTED]`, `[ADDRESS REDACTED]`, `[DATE REDACTED]`

Supports pluggable `Detector` implementations via `RegisterDetector()` / `NewWithDetectors()`.

#### 4. Content Moderation (`content_moderation`)

Detects harmful or policy-violating content across multiple categories.

**Required:** no (fail-open) | **Default:** off (enable `GUARDRAIL_CONTENT_MODERATION=warn`) | **Direction:** input + output | **Decision:** `WARN` | **Latency:** <1ms

**When to use:** Enable when you need keyword-based content filtering but don't want to block traffic. Use `warn` mode to log violations while still allowing content through. Switch to `enforce` for strict blocking. For production-grade moderation, pair with `openai_moderation` or `nvidia_content` external guards.

**Categories (5):** hate_speech, violence, sexual_content, self_harm, abuse_harassment. Severity scales: low (1-3 matches), medium (4-10), high (>10).

---

### Tier 2: External Guardrails (API-based, off by default)

These integrate with third-party security APIs. They are **fail-open** by default (errors are skipped and the request continues) unless the spec sets `Required: true`. If the API is unreachable or times out (5s), the guardrail passes and the request continues. To activate, set the corresponding `*_API_KEY` env var.

**Why external?** These services use ML models trained on millions of attack samples, catching sophisticated prompt injection, jailbreak attempts, and nuanced content safety violations that regex patterns miss.

**Latency consideration:** Each external guardrail adds network latency. Use 1-2 that cover your threat model. All use a 5s timeout so slow APIs don't hold up the pipeline.

#### 5. AIM (`aim`)

Meta's LlamaGuard integration for content safety classification.

**API endpoint:** `https://api.aim.security/v1/analyze` | **Config:** `AIM_API_KEY`, `AIM_BASE_URL`

**When to use:** If you're already a Meta/AIM customer. Specializes in Llama model guard compliance.

#### 6. Lakera Guard (`lakera`)

Real-time prompt injection and content safety detection.

**API endpoint:** `https://api.lakera.ai/v1/guard` | **Config:** `LAKERA_API_KEY`, `LAKERA_BASE_URL`

**When to use:** Best-in-class for prompt injection detection. Use as primary injection guard and set `prompt_injection` to `log_only` or `warn` as defense-in-depth.

#### 7. Lumigator (`lumigator`)

Lumigator guardrail integration for LLM safety evaluation.

**API endpoint:** `https://api.lumigator.ai/v1/evaluate` | **Config:** `LUMIGATOR_API_KEY`, `LUMIGATOR_BASE_URL`

**When to use:** If you use Lumigator's evaluation framework for LLM benchmarking and want a unified guardrail stack.

#### 8. PANW Prisma AIRS (`prisma_airs`)

Palo Alto Networks Prisma Access AI Runtime Security.

**API endpoint:** `https://api.prisma.paloaltonetworks.com/airs/v1/scan` | **Config:** `PRISMA_AIRS_API_KEY`, `PRISMA_AIRS_BASE_URL`

**When to use:** Enterprise environments already using Palo Alto Networks security infrastructure. Provides AI runtime security aligned with PANW's threat intelligence.

#### 9. NVIDIA Content Guard (`nvidia_content`)

NVIDIA content safety guard model for detecting unsafe LLM inputs/outputs.

**API endpoint:** `https://api.nvidia.com/v1/content-safety/evaluate` | **Config:** `NVIDIA_CONTENT_API_KEY`, `NVIDIA_CONTENT_BASE_URL`

**When to use:** When you need multi-category content safety (hate, harassment, violence, sexual, self-harm). Good alternative to OpenAI Moderation if you're on NVIDIA infrastructure.

#### 10. OpenAI Moderation (`openai_moderation`)

OpenAI Moderation API integration — the same moderation endpoint powering ChatGPT's content filters.

**API endpoint:** `https://api.openai.com/v1/moderations` | **Config:** `OPENAI_MODERATION_API_KEY` (falls back to `OPENAI_API_KEY`)

**When to use:** Simplest to set up if you already use OpenAI. Detects 11+ content categories (hate, hate/threatening, self-harm, sexual, sexual/minors, violence, violence/graphic). Use as primary content moderation — more accurate than regex-based `content_moderation`.

#### 11. Zscaler Guard (`zscaler`)

Zscaler AI guard integration for enterprise AI security.

**API endpoint:** `https://api.zscaler.com/ai/v1/guard` | **Config:** `ZSCALER_API_KEY`, `ZSCALER_BASE_URL`

**When to use:** Enterprise environments using Zscaler's Zero Trust platform. Integrates AI security with existing Zscaler policies.

---

### Tier 3: Function Guardrails (configurable, off by default)

These are composable building blocks for custom policies. They run in-process with sub-millisecond latency. All are **off by default** — set `GUARDRAIL_<NAME>=enforce` to activate, and provide config via `Strategy.Extra` or dedicated env vars.

**Why functions?** They let you enforce domain-specific rules without writing code. Use them for input validation, format constraints, or simple content checks that don't warrant an external API call.

#### Pattern / Text Match Functions

##### 12. Regex Match (`regex_match`)

Matches content against a regex pattern. Blocks on match.

**Required:** yes | **Direction:** input + output | **Decision:** `BLOCK`

**Config:** `REGEX_MATCH_PATTERN` env var or `{"pattern": "..."}` in Extra.

**When to use:** When you need to block requests containing specific patterns (e.g., internal endpoint URLs, proprietary code patterns, competitor names).

##### 13. Contains (`contains`)

Checks for forbidden words/phrases. Case-insensitive.

**Required:** yes | **Direction:** input + output | **Decision:** `BLOCK`

**Config:** `CONTAINS_WORDS` (comma-separated) or `{"words": ["word1","word2"]}` in Extra.

**When to use:** Block requests containing offensive terms, competitor product names, or any forbidden vocabulary your policy requires.

##### 14. Contains Code (`contains_code`)

Detects code snippets in prompts — catches code injection attempts.

**Required:** yes | **Direction:** input + output | **Decision:** `BLOCK`

**Built-in patterns:** function/class declarations (`def`, `function`, `func`, `class`), import statements, SQL queries (`SELECT`, `INSERT`, `DROP`), shell/sandbox escapes (`eval`, `exec`, `system`), shebang lines, exception handling blocks.

**When to use:** Prevent users from injecting executable code into prompts, especially when your application evaluates or interprets LLM output downstream.

##### 15. Ends With (`ends_with`)

Checks if content ends with a specific suffix.

**Required:** yes | **Direction:** input + output | **Decision:** `BLOCK`

**Config:** `ENDSWITH_SUFFIX` env var or `{"suffix": "..."}` in Extra.

**When to use:** Block requests that end with specific text patterns (e.g., appended system instructions, terminal escape sequences).

##### 16. Valid URLs (`valid_urls`)

Validates all URLs in content are well-formed. Blocks if malformed URLs found.

**Required:** yes | **Direction:** input + output | **Decision:** `BLOCK`

**When to use:** Prevent URL-based attacks (malformed URLs that bypass filters, SSRF probes in URL parameters).

##### 17. All Uppercase (`all_uppercase`)

Flags content that is entirely uppercase. Issues a warning by default.

**Required:** no | **Direction:** input + output | **Decision:** `WARN`

**When to use:** Detect potential abuse (shouting, spam, CAPS LOCK rage). Use `warn` to log and monitor rather than block outright.

##### 18. All Lowercase (`all_lowercase`)

Flags content that is entirely lowercase. Issues a warning by default.

**Required:** no | **Direction:** input + output | **Decision:** `WARN`

**When to use:** Detect input quality issues or bots sending unformatted text.

#### Request Structure Validation Functions

##### 19. JSON Schema (`json_schema`)

Validates request body against a JSON schema. Blocks on schema mismatch.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `JSON_SCHEMA` env var (JSON string) or `{"schema": {...}}` in Extra. Supports nested `type`, `properties`, and `type` validation for `string`, `number`, `boolean`, `object`, `array`.

**When to use:** Enforce strict API contract validation. Reject requests that don't match expected structure before they reach the provider.

##### 20. JSON Keys (`json_keys`)

Validates that required JSON keys are present in the request.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `JSON_KEYS_REQUIRED` (comma-separated) or `{"required": ["key1","key2"]}` in Extra.

**When to use:** Ensure mandatory fields like `model`, `messages`, `temperature` are always present in requests.

##### 21. JWT (`jwt`)

Validates JWT tokens in content for structural correctness and base64url encoding.

**Required:** no | **Direction:** input only | **Decision:** `WARN`

**When to use:** Monitor for malformed or suspicious JWT tokens in requests. Use `warn` mode — this is a heuristic check, not cryptographic validation.

##### 22. Not Null (`not_null`)

Ensures specified JSON fields are present and non-null.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `NOT_NULL_FIELDS` (comma-separated) or `{"fields": ["field1","field2"]}` in Extra.

**When to use:** Prevent null/absent values in critical fields like `model`, `user`, or custom metadata fields.

##### 23. Required Metadata Keys (`required_metadata_keys`)

Validates that specific keys exist in the request's `metadata` object.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `REQUIRED_METADATA_KEYS` (comma-separated) or `{"keys": ["key1","key2"]}` in Extra.

**When to use:** Enforce metadata tagging requirements for auditing, cost attribution, or compliance.

#### Constraint Functions

##### 24. Word Count (`word_count`)

Enforces min/max word count constraints.

**Required:** no | **Direction:** input + output | **Decision:** `WARN`

**Config:** `WORD_COUNT_MIN` / `WORD_COUNT_MAX` env vars or `{"min": 10, "max": 2000}` in Extra.

**When to use:** Set input limits to control token usage costs. Warn on unusually short or verbose prompts. Use on output to detect truncated or runaway generations.

##### 25. Sentence Count (`sentence_count`)

Enforces min/max sentence count constraints.

**Required:** no | **Direction:** input + output | **Decision:** `WARN`

**Config:** `SENTENCE_COUNT_MIN` / `SENTENCE_COUNT_MAX` env vars or `{"min": 1, "max": 100}` in Extra.

**When to use:** Similar to word count but at sentence granularity. Useful for output quality — too few sentences may indicate an incomplete response.

##### 26. Character Count (`character_count`)

Enforces min/max character count constraints.

**Required:** no | **Direction:** input + output | **Decision:** `WARN`

**Config:** `CHARACTER_COUNT_MIN` / `CHARACTER_COUNT_MAX` env vars or `{"min": 1, "max": 32000}` in Extra.

**When to use:** Most precise limit. Use to enforce model context window limits or API payload size constraints.

#### Model Governance Functions

##### 27. Model Whitelist (`model_whitelist`)

Blocks requests using unapproved models. Only whitelisted models pass.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `MODEL_WHITELIST` (comma-separated, e.g. `gpt-4o,gpt-4o-mini,claude-3-opus`) or `{"models": [...]}` in Extra.

**When to use:** Lock down which models your application can call. Prevent teams from accidentally using expensive models or unsanctioned providers.

##### 28. Model Rules (`model_rules`)

Applies per-model configuration rules. Supports disabling specific models.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `MODEL_RULES` env var (JSON like `{"gpt-4":{"disabled":true}}`) or `{"rules": {...}}` in Extra.

**When to use:** Granular model governance beyond simple whitelist. Disable deprecated models, enforce model-specific constraints.

##### 29. Allowed Request Types (`allowed_request_types`)

Restricts which endpoint/object types are permitted.

**Required:** yes | **Direction:** input only | **Decision:** `BLOCK`

**Config:** `ALLOWED_REQUEST_TYPES` (comma-separated, e.g. `chat.completion,chat.completion.chunk`) or `{"types": [...]}` in Extra.

**When to use:** Limit what your gateway accepts — block embeddings, fine-tuning, or moderation endpoints you don't want exposed.

#### Delegation Functions

##### 30. Webhook (`webhook`)

Delegates guardrail evaluation to an external webhook. Returns decision based on the webhook response.

**Required:** no (fail-open) | **Direction:** input + output | **Decision:** `BLOCK` (on webhook block) | **Latency:** depends on webhook

**Config:** `WEBHOOK_GUARD_URL` env var or `{"url": "https://..."}` in Extra.

**Webhook contract:** POST with `{"content": "..."}`. Expect response `{"blocked": bool, "reason": "..."}`.

**When to use:** Integrate custom validation logic hosted elsewhere without modifying the codebase. Useful for org-specific policies or legacy compliance systems.

#### Transformer Functions

Transformers modify content rather than blocking. They return `DecisionRedact` (with modified body) for `add_prefix` and `regex_replace`, and `DecisionPass` (with log message) for `log`.

##### 31. Log (`log`)

Logs content without any verdict. Pure observability — always passes.

**Required:** no | **Direction:** input + output | **Decision:** `PASS`

**When to use:** Debug guardrail pipelines. Insert a `log` guardrail between other guardrails to see the body state at that point in the pipeline. Useful for auditing — log all requests/responses for compliance.

##### 32. Add Prefix (`add_prefix`)

Prepends a string prefix to the content body.

**Required:** no | **Direction:** input + output | **Decision:** `REDACT`

**Config:** `ADD_PREFIX_TEXT` env var or `{"prefix": "..."}` in Extra.

**When to use:** Add system context to every request (e.g., prepend "You are a helpful assistant." or a compliance notice). Add watermarks to outputs.

##### 33. Regex Replace (`regex_replace`)

Replaces content matching a regex pattern with a replacement string.

**Required:** no | **Direction:** input + output | **Decision:** `REDACT`

**Config:** `REGEX_REPLACE_PATTERN` + `REGEX_REPLACE_REPLACEMENT` env vars or `{"pattern": "...", "replacement": "..."}` in Extra.

**When to use:** Sanitize outputs (remove internal hostnames, redact custom patterns not covered by PII guardrail), normalize inputs (strip formatting, remove noise).

---

### Quick Reference: Which Guardrail When?

| Scenario | Use |
|---|---|
| Stop prompt injection | `prompt_injection` (always) + `lakera` (production) |
| Prevent credential leaks | `secrets` (always) |
| GDPR/CCPA compliance | `pii` (always) |
| Block harmful content | `openai_moderation` or `content_moderation` |
| Enterprise security | `zscaler` or `prisma_airs` (if PANW/Zscaler shop) |
| Model cost control | `model_whitelist` |
| Request validation | `json_schema` + `json_keys` + `not_null` |
| Token budget enforcement | `word_count` or `character_count` |
| Sanitize outputs | `regex_replace` |
| Audit trail | `log` |
| Custom org policy | `webhook` |

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

Guardrails are configured via environment variables. The general pattern is:

```bash
GUARDRAIL_<NAME>=enforce|log_only|warn|off
```

When unset, the default varies by tier:
- **Internal guardrails:** `enforce` (active by default)
- **External guardrails:** `off` (require explicit activation + API key)
- **Function guardrails:** `off` (require explicit activation + config)

Invalid mode values default to `enforce`.

### All Configuration Variables

```bash
# ── Internal (active by default) ─────────────────────────────────
GUARDRAIL_PROMPT_INJECTION=enforce
GUARDRAIL_SECRETS=enforce
GUARDRAIL_PII=enforce
GUARDRAIL_CONTENT_MODERATION=warn

# ── External (off by default, require API key) ───────────────────
GUARDRAIL_AIM=off                          # needs AIM_API_KEY
GUARDRAIL_LAKERA=off                       # needs LAKERA_API_KEY
GUARDRAIL_LUMIGATOR=off                    # needs LUMIGATOR_API_KEY
GUARDRAIL_PRISMA_AIRS=off                  # needs PRISMA_AIRS_API_KEY
GUARDRAIL_NVIDIA_CONTENT=off               # needs NVIDIA_CONTENT_API_KEY
GUARDRAIL_OPENAI_MODERATION=off            # needs OPENAI_MODERATION_API_KEY or OPENAI_API_KEY
GUARDRAIL_ZSCALER=off                      # needs ZSCALER_API_KEY

# ── Functions (off by default, require config) ───────────────────
GUARDRAIL_REGEX_MATCH=off                  # needs REGEX_MATCH_PATTERN
GUARDRAIL_CONTAINS=off                     # needs CONTAINS_WORDS
GUARDRAIL_CONTAINS_CODE=off
GUARDRAIL_ENDS_WITH=off                    # needs ENDSWITH_SUFFIX
GUARDRAIL_VALID_URLS=off
GUARDRAIL_JSON_SCHEMA=off                  # needs JSON_SCHEMA
GUARDRAIL_JSON_KEYS=off                    # needs JSON_KEYS_REQUIRED
GUARDRAIL_JWT=off
GUARDRAIL_WORD_COUNT=off                   # needs WORD_COUNT_MIN/MAX
GUARDRAIL_SENTENCE_COUNT=off               # needs SENTENCE_COUNT_MIN/MAX
GUARDRAIL_CHARACTER_COUNT=off              # needs CHARACTER_COUNT_MIN/MAX
GUARDRAIL_ALL_UPPERCASE=off
GUARDRAIL_ALL_LOWERCASE=off
GUARDRAIL_MODEL_WHITELIST=off              # needs MODEL_WHITELIST
GUARDRAIL_MODEL_RULES=off                  # needs MODEL_RULES
GUARDRAIL_REQUIRED_METADATA_KEYS=off       # needs REQUIRED_METADATA_KEYS
GUARDRAIL_ALLOWED_REQUEST_TYPES=off        # needs ALLOWED_REQUEST_TYPES
GUARDRAIL_NOT_NULL=off                     # needs NOT_NULL_FIELDS
GUARDRAIL_WEBHOOK=off                      # needs WEBHOOK_GUARD_URL
GUARDRAIL_LOG=off
GUARDRAIL_ADD_PREFIX=off                   # needs ADD_PREFIX_TEXT
GUARDRAIL_REGEX_REPLACE=off                # needs REGEX_REPLACE_PATTERN + REGEX_REPLACE_REPLACEMENT
```

Legacy suffix format is also supported:
```bash
GUARDRAIL_PROMPT_INJECTION_MODE=enforce
GUARDRAIL_SECRETS_MODE=log_only
GUARDRAIL_PII_MODE=off
```

### Example: Full Production Configuration

```bash
# Always-on security
GUARDRAIL_PROMPT_INJECTION=enforce
GUARDRAIL_SECRETS=enforce
GUARDRAIL_PII=enforce

# Content safety via OpenAI
GUARDRAIL_OPENAI_MODERATION=enforce
OPENAI_MODERATION_API_KEY=sk-...

# Prompt injection via Lakera (defense in depth)
GUARDRAIL_LAKERA=enforce
LAKERA_API_KEY=lak-...

# Model governance
GUARDRAIL_MODEL_WHITELIST=enforce
MODEL_WHITELIST=gpt-4o,gpt-4o-mini

# Input constraints
GUARDRAIL_WORD_COUNT=warn
WORD_COUNT_MIN=10
WORD_COUNT_MAX=4000

GUARDRAIL_CHARACTER_COUNT=warn
CHARACTER_COUNT_MAX=32000

# Audit
GUARDRAIL_LOG=enforce
```

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
    guardaim "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/aim"
    guardfunctions "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/functions"
    guardlakera "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/lakera"
    guardlumigator "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/lumigator"
    guardnvidia "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/nvidia_content"
    guardopenaimod "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/openai_moderation"
    guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
    guardprisma "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/prisma_airs"
    guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
    guardzscaler "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/zscaler"
    "github.com/dexterhere04/AgentPlane/internal/handlers"
    "github.com/dexterhere04/AgentPlane/internal/observability"
    "github.com/dexterhere04/AgentPlane/internal/proxy"
)

func main() {
    bus := observability.DefaultBus
    provider := proxy.NewOpenAIProviderFromEnv()

    cfg := guardrail.LoadConfig()

    // 1. Create registry and register ALL guardrails
    registry := guardrail.NewRegistry()

    // Internal
    registry.Register(guardrail.NewPromptInjectionGuardrail(cfg.Strategy("prompt_injection")))
    registry.Register(guardrail.NewContentModerationGuardrail(cfg.Strategy("content_moderation")))
    registry.Register(guardsecrets.New(cfg.Strategy("secrets")))
    registry.Register(guardpii.New(cfg.Strategy("pii")))

    // External
    registry.Register(guardaim.New(cfg.Strategy("aim")))
    registry.Register(guardlakera.New(cfg.Strategy("lakera")))
    registry.Register(guardlumigator.New(cfg.Strategy("lumigator")))
    registry.Register(guardprisma.New(cfg.Strategy("prisma_airs")))
    registry.Register(guardnvidia.New(cfg.Strategy("nvidia_content")))
    registry.Register(guardopenaimod.New(cfg.Strategy("openai_moderation")))
    registry.Register(guardzscaler.New(cfg.Strategy("zscaler")))

    // Functions
    registry.Register(guardfunctions.NewRegexMatch(cfg.Strategy("regex_match")))
    registry.Register(guardfunctions.NewContains(cfg.Strategy("contains")))
    registry.Register(guardfunctions.NewContainsCode(cfg.Strategy("contains_code")))
    registry.Register(guardfunctions.NewEndsWith(cfg.Strategy("ends_with")))
    registry.Register(guardfunctions.NewValidUrls(cfg.Strategy("valid_urls")))
    registry.Register(guardfunctions.NewJsonSchema(cfg.Strategy("json_schema")))
    registry.Register(guardfunctions.NewJsonKeys(cfg.Strategy("json_keys")))
    registry.Register(guardfunctions.NewJWT(cfg.Strategy("jwt")))
    registry.Register(guardfunctions.NewWordCount(cfg.Strategy("word_count")))
    registry.Register(guardfunctions.NewSentenceCount(cfg.Strategy("sentence_count")))
    registry.Register(guardfunctions.NewCharacterCount(cfg.Strategy("character_count")))
    registry.Register(guardfunctions.NewAllUppercase(cfg.Strategy("all_uppercase")))
    registry.Register(guardfunctions.NewAllLowercase(cfg.Strategy("all_lowercase")))
    registry.Register(guardfunctions.NewModelWhitelist(cfg.Strategy("model_whitelist")))
    registry.Register(guardfunctions.NewModelRules(cfg.Strategy("model_rules")))
    registry.Register(guardfunctions.NewRequiredMetadataKeys(cfg.Strategy("required_metadata_keys")))
    registry.Register(guardfunctions.NewAllowedRequestTypes(cfg.Strategy("allowed_request_types")))
    registry.Register(guardfunctions.NewNotNull(cfg.Strategy("not_null")))
    registry.Register(guardfunctions.NewWebhook(cfg.Strategy("webhook")))
    registry.Register(guardfunctions.NewLog(cfg.Strategy("log")))
    registry.Register(guardfunctions.NewAddPrefix(cfg.Strategy("add_prefix")))
    registry.Register(guardfunctions.NewRegexReplace(cfg.Strategy("regex_replace")))

    // 2. Create enforcement point
    enforcement := guardrail.NewEnforcementPoint(registry, cfg, bus)

    // 3. Define directional guardrail sets
    inputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            // Internal (always-on)
            {Name: "prompt_injection"},
            {Name: "secrets"},
            {Name: "pii"},
            // External (off by default — no API key = no-op)
            {Name: "aim"},
            {Name: "lakera"},
            {Name: "lumigator"},
            {Name: "prisma_airs"},
            {Name: "nvidia_content"},
            {Name: "openai_moderation"},
            {Name: "zscaler"},
            // Functions (off by default — no config = no-op)
            {Name: "regex_match"},
            {Name: "contains"},
            {Name: "contains_code"},
            {Name: "ends_with"},
            {Name: "valid_urls"},
            {Name: "json_schema"},
            {Name: "json_keys"},
            {Name: "jwt"},
            {Name: "word_count"},
            {Name: "sentence_count"},
            {Name: "character_count"},
            {Name: "all_uppercase"},
            {Name: "all_lowercase"},
            {Name: "model_whitelist"},
            {Name: "model_rules"},
            {Name: "required_metadata_keys"},
            {Name: "allowed_request_types"},
            {Name: "not_null"},
            {Name: "webhook"},
            // Transformers
            {Name: "log"},
            {Name: "add_prefix"},
            {Name: "regex_replace"},
        },
    }
    outputSet := guardrail.GuardrailSet{
        Guards: []guardrail.GuardrailSpec{
            {Name: "secrets"},
            {Name: "pii"},
            {Name: "aim"},
            {Name: "lakera"},
            {Name: "nvidia_content"},
            {Name: "openai_moderation"},
            {Name: "zscaler"},
            {Name: "regex_match"},
            {Name: "contains"},
            {Name: "contains_code"},
            {Name: "valid_urls"},
            {Name: "word_count"},
            {Name: "sentence_count"},
            {Name: "character_count"},
            {Name: "all_uppercase"},
            {Name: "all_lowercase"},
            {Name: "webhook"},
            {Name: "log"},
            {Name: "add_prefix"},
            {Name: "regex_replace"},
        },
    }

    // 4. Use in handler
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
| Guardrail error (Required) | 503 Service Unavailable | `{"error": {"type": "guardrail_unavailable", "message": "..."}}` |

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

// In the GuardrailSet, set Required: true to make it fail-closed:
inputSet := guardrail.GuardrailSet{
    Guards: []guardrail.GuardrailSpec{
        {Name: "my_guard", Required: true},
    },
}
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
- **Fail-safe:** Guardrails with `Required: true` fail-closed (block on error); optional guardrails fail-open (pass on error).
- **Dependencies:** The `secrets` guardrail uses gitleaks for secret detection; the rest of the guardrail framework uses only the Go standard library (plus the project-internal `observability` package).
