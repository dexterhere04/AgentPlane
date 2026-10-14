# AgentPlane Provider Routing and Router

## 1. Purpose

AgentPlane supports routing requests across multiple model providers using persistent routing configuration and runtime provider abstractions.

The routing layer is responsible for:

* Identifying the routing rule for a request.
* Resolving the provider group associated with that rule.
* Finding eligible upstream providers.
* Selecting the initial provider using weighted selection.
* Falling back to the configured default provider when no routing rule applies.

Retry and cross-provider failover are handled separately and are documented in `weighted-selection-and-failover.md`.

## 2. Provider Abstraction

All model providers implement the common `Provider` interface:

```go
type Provider interface {
    Name() string
    Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
}
```

This allows the router and proxy layer to operate independently of the provider implementation.

The Router also implements the `Provider` interface. This allows existing handlers to continue calling a provider without needing to know whether the request is being routed to a specific provider or handled by the default provider.

The request flow is therefore:

```text
HTTP Handler
    |
    v
Router
    |
    +----> selected Upstream
    |
    +----> default Provider
```

## 3. PostgreSQL Routing Configuration

Provider routing configuration is stored in PostgreSQL.

The provider configuration contains information such as:

* Provider name
* Provider group
* Weight
* Supported models
* Timeout
* Maximum retries
* Enabled state

The database provides the persistent source of routing configuration. Runtime components load this configuration into provider and upstream objects.

The provider store is responsible for loading enabled providers and constructing their runtime `Upstream` representations.

## 4. Provider Store

The ProviderStore loads provider configuration from PostgreSQL and creates runtime upstreams.

Conceptually:

```text
PostgreSQL
    |
    v
ProviderStore
    |
    v
Provider + Upstream
    |
    v
Registry
```

The current implementation creates OpenAI-backed providers from the database configuration.

The store applies runtime configuration including:

* Provider group
* Weight
* Supported models
* Timeout
* Maximum retries
* Enabled state

This keeps database configuration separate from request-routing logic.

## 5. Routing Rules

Routing rules determine which provider group should handle a request.

A rule can match request attributes such as:

* Model
* API key
* Tenant
* Request metadata
* Other configured routing attributes

When a request matches a rule, the rule provides the provider group used for upstream selection.

Conceptually:

```text
Request
   |
   v
Routing Rule Matching
   |
   v
Provider Group
```

If no rule matches, the Router falls back to the configured default provider.

## 6. Registry

The runtime `Registry` maintains the available upstream providers.

The registry is responsible for provider lookup and candidate discovery. It does not perform weighted selection.

For a requested model and provider group, the registry returns eligible upstreams based on:

* Provider enabled state
* Provider group
* Model support
* Circuit-breaker readiness

Conceptually:

```text
Registry.Candidates(model, providerGroup)
                 |
                 v
        Eligible Upstreams
```

The registry preserves the registration order of the candidates. Selection policy is handled separately by the selector.

## 7. Upstream Runtime Model

Each provider is represented at runtime by an `Upstream`.

An upstream contains:

* Provider implementation
* Provider name
* Provider group
* Weight
* Supported models
* Timeout
* Maximum retries
* Circuit breaker

The `Upstream` is responsible for executing requests against a provider and enforcing provider-level reliability controls.

The upstream's retry behavior and circuit breaker are covered in `weighted-selection-and-failover.md`.

## 8. Router

The Router is the central request-routing component.

Its responsibilities are:

1. Extract routing information from the request context.
2. Match the request against routing rules.
3. Resolve the provider group.
4. Resolve the requested model.
5. Ask the Registry for eligible upstreams.
6. Select the initial upstream using the weighted selector.
7. Forward the request to the selected upstream.
8. Use the default provider when routing is not applicable.

The Router does not contain provider-specific API logic.

Conceptually:

```text
Request
   |
   v
Router
   |
   +--> Match Routing Rule
   |
   +--> Resolve Provider Group
   |
   +--> Resolve Model
   |
   +--> Registry.Candidates()
   |
   +--> WeightedSelector.Select()
   |
   v
Selected Upstream
```

## 9. Weighted Primary Selection

After the Registry returns eligible candidates, the Router uses a weighted selector to choose the initial upstream.

The selector considers only candidates with a positive weight.

For example:

```text
Provider A   weight = 70
Provider B   weight = 20
Provider C   weight = 10
```

The approximate selection distribution is:

```text
Provider A   70%
Provider B   20%
Provider C   10%
```

The selector is responsible only for choosing an upstream. It does not perform retries or failover.

The separation is:

```text
Registry
  -> finds eligible providers

WeightedSelector
  -> chooses initial provider

Upstream
  -> retries requests to that provider

Router
  -> performs cross-provider failover
```

## 10. Default Provider

The Router supports a default provider for requests that do not require routing.

If:

* routing information is unavailable,
* no routing rule matches, or
* no provider group is specified by the matching rule,

the Router forwards the request to the configured default provider.

This preserves the existing single-provider behavior while allowing multi-provider routing to be introduced incrementally.

## 11. Request Execution Flow

The complete Task 4 routing flow is:

```text
HTTP Request
    |
    v
Chat Handler
    |
    v
Router
    |
    +--> Extract RouteRequest
    |
    +--> Match Routing Rules
    |
    +--> Resolve Provider Group
    |
    +--> Resolve Model
    |
    v
Registry
    |
    +--> Check enabled state
    +--> Check provider group
    +--> Check model support
    +--> Check circuit-breaker readiness
    |
    v
Eligible Upstreams
    |
    v
WeightedSelector
    |
    v
Primary Upstream
    |
    v
Provider Request
```

Provider-level retries and cross-provider failover begin after the primary upstream has been selected.

## 12. Overall Architecture

The routing architecture separates configuration, routing, selection, and provider execution:

```text
                    PostgreSQL
                        |
                        v
                 ProviderStore
                        |
                        v
                Runtime Upstreams
                        |
                        v
                    Registry
                        ^
                        |
HTTP Request --> Router
                  |
                  +--> Routing Rules
                  |
                  +--> Model Resolution
                  |
                  +--> Weighted Selection
                  |
                  v
             Primary Upstream
                  |
                  v
              Provider API
```

This separation keeps the Router focused on routing decisions while provider-specific execution and reliability behavior remain inside the upstream layer.
