# Package: `proxy`

**Files:** `internal/proxy/provider.go`, `openai.go`, `registry.go`, `router.go`,
`upstream.go`, `selector.go`, `routing.go`, `routing_store.go`, `metrics.go`

**Package:** `proxy`

## Overview

Communicates with AI providers and routes requests across them. The package owns the
`Provider` abstraction, the provider registry, the router, weighted selection, and the
resilience layer (timeout, retry, circuit breaker, failover). It is split into four groups:

```text
provider.go   Provider interface + OpenAI-compatible implementation
registry.go   registered upstreams + candidate resolution
router.go     rule matching + weighted selection + failover orchestration
upstream.go   per-upstream execution: timeout, retry, circuit breaker
selector.go   weighted primary selection
routing.go    in-memory routing-rule set + matchers (model/role/user)
routing_store.go  Postgres persistence for rules (and providers)
metrics.go    in-process routing counters (/metrics)
```

## Types

### `Provider` (interface)

```go
type Provider interface {
    Name() string
    Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
}
```

**File:** `internal/proxy/provider.go`

The abstraction handlers depend on. `handlers.ChatWithRouting` takes a `proxy.Provider`;
the `Router` implements it, so switching between single-provider and routed forwarding
does not change the handler.

### `Registry`

```go
type Registry struct { /* upstreams map[string]*Upstream */ }
func (r *Registry) RegisterProvider(name string, p Provider, opts ...UpstreamOption) error
func (r *Registry) Candidates(model, providerGroup string) []*Upstream
func (r *Registry) Statuses() []UpstreamStatus
```

Holds all registered upstreams and returns eligible candidates for a request, filtering
by enabled state, provider group, model support, and circuit-breaker readiness. Register
via `RegisterProvider`, which wraps any `Provider` implementation with routing metadata
(`weight`, `models`, `timeout`, `max_retries`, breaker threshold).

### `Router`

```go
type Router struct { /* ruleSet, registry, selector, default provider */ }
func (r *Router) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
```

Implements `proxy.Provider`. `Forward` extracts a `RouteRequest` from the context, matches
the rule set by priority (first match wins), resolves the provider group and model, asks
the registry for eligible upstreams, selects the primary via the `WeightedSelector`, and
runs `forwardWithFailover` down the candidate chain. Records metrics and routing telemetry
along the way. See `provider-routing-and-router.md`.

### `Upstream`

```go
func NewUpstream(name string, p Provider, opts ...UpstreamOption) *Upstream
func (u *Upstream) Forward(ctx, body, requestID) ([]byte, error)
func (u *Upstream) ForwardWithConfig(ctx, body, requestID, ResilienceConfig) ([]byte, error)
func (u *Upstream) Available() bool
```

A concrete destination: provider + weight + models + resilience settings + a passive
`CircuitBreaker`. `Forward` applies an optional deadline, retries retryable failures with
exponential backoff + jitter, trips/recoveries the breaker, and returns typed errors
(`ErrCircuitOpen`, `ErrUpstreamDisabled`). See `resilience-and-retry.md`.

### `WeightedSelector`

```go
func NewWeightedSelector(seed int64) *WeightedSelector
func (s *WeightedSelector) Select(candidates []*Upstream) (*Upstream, error)
```

Chooses an upstream proportional to weight; ignores non-positive weights; returns
`ErrNoWeightedUpstream` when nothing eligible remains.

### `CircuitBreaker`

```go
type CircuitBreaker struct { /* failure threshold + cooldown */ }
func NewCircuitBreaker(config BreakerConfig) *CircuitBreaker
```

Passive breaker: `closed → open → half-open → closed`, with half-open probe recovery.

### `RuleSet` / `RoutingRule` / `RoutingRuleStore`

Rules match on `model_matcher`/`role_matcher`/`user_matcher` (JSON `equals`/`in`/`prefix`)
with `action` `route`|`weighted`|`deny`, evaluated in priority order. `RoutingRuleStore`
persists `routing_rules` and `providers` rows in PostgreSQL; changes are reloaded into the
runtime `Router` via `Load()`. See `routing-admin.md` for the schema and admin API.

### `Metrics`

```go
var DefaultMetrics = &Metrics{...}
func (m *Metrics) Snapshot() RoutingMetricsSnapshot
```

Cumulative, ClickHouse-independent counters (`routing_*_total` + per-upstream breakdown),
served from `/metrics`. See `routing-metrics.md`.

### `OpenAIProvider`

```go
type OpenAIProvider struct {
    apiKey     string
    baseURL    string
    httpClient *http.Client
}
```

Constructors:

- `NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider` — explicit key and base URL (empty `baseURL` defaults to `https://api.openai.com/v1`). Uses a 60-second HTTP client timeout.
- `NewOpenAIProviderFromEnv() *OpenAIProvider` — reads the key from `config.OpenAIKey()` and the base URL from `OPENAI_BASE_URL`.

## Functions

### `Forward()`

```go
func (p *OpenAIProvider) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
```

Targets `{baseURL}/chat/completions`.

**Behavior:**

1. Errors immediately if `apiKey` is empty (`OPENAI_API_KEY is not set`).
2. Detects `"stream": true` in the request body and dispatches to streaming or non-streaming path.

**Non-streaming** (`forwardNonStreaming`):

1. Builds a `POST` request with the raw body.
2. Sets `Content-Type: application/json` and `Authorization: Bearer <key>`.
3. Sends via the 60-second HTTP client.
4. Reads and returns the response body.
5. Non-200 statuses are returned as errors (`OpenAI returned status <code>: <body>`).

**Streaming** (`forwardStreaming`):

1. Sends the request and reads the SSE stream (`data: ` lines) until `[DONE]`.
2. Accumulates chunks and rebuilds a single structured `chat.completion` response via `buildStructuredResponse` (the gateway currently returns a non-streaming JSON response, not a live SSE stream to the client).

## Error Handling

Errors are wrapped with `%w` to preserve the chain. The router / upstream returns typed
errors (`ErrCircuitOpen`, `ErrUpstreamDisabled`, `ErrNoProvider`); the handler surfaces
upstream failures as HTTP 502 and routing failures as 503.

## Design notes

- The 60-second HTTP-client timeout is the provider-client default; per-route/per-upstream
  timeouts are applied by the upstream's `ForwardWithConfig`.
- The response body is fully buffered before being returned.
- Streaming responses are coalesced into a single completion for now (see the roadmap for
  real SSE streaming back to the client).
