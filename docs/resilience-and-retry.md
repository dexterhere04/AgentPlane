# Resilience, Timeout, Retry, and Failover

## 1. Purpose

The gateway needs to tolerate temporary upstream failures without immediately failing a request or unnecessarily switching providers.

This part adds a gateway-style resilience layer around `Provider.Forward`. The resilience behavior is implemented at the `Upstream` level, while cross-provider failover remains the responsibility of the `Router`.

The execution model is:

```text
Router
  |
  +-- select upstream
       |
       +-- Timeout
            |
            +-- Retry
                 |
                 +-- Provider.Forward
```

When all retry attempts for the selected upstream are exhausted, control returns to the Router, which may fail over to another eligible upstream.

## 2. Resilience Execution Flow

For a routed request, the Router first selects an upstream using the configured weighted selection policy.

The selected upstream then executes its own resilience policy:

```text
Router
  |
  v
Selected Upstream
  |
  +-- Retry attempt 1
  |     |
  |     +-- timeout
  |           |
  |           +-- Provider.Forward
  |
  +-- backoff + jitter
  |
  +-- Retry attempt 2
  |
  +-- ...
  |
  +-- retries exhausted
        |
        v
      Router
        |
        +-- failover to another upstream
```

This separation is important. Retries are intended to recover from transient failures on the same provider. Failover is used only after that provider's retry policy has been exhausted.

## 3. Timeout Handling

Each upstream has its own timeout configuration.

The upstream creates a derived context using:

```go
context.WithTimeout(ctx, timeout)
```

The timeout is applied to each individual provider attempt.

The default timeout is configured on the `Upstream` and originates from the provider configuration stored in PostgreSQL.

A routing rule can override the upstream default through `timeout_retry_overrides`.

For example:

```json
{
  "timeout_ms": 30000
}
```

This allows different routes to use different timeout policies without modifying the shared upstream configuration.

The route override is applied per request. It does not mutate the `Upstream`, which allows the same upstream to safely serve concurrent requests using different routing rules.

## 4. Retry Policy

Retries are performed only for failures considered transient.

The failure classifier distinguishes between retryable failures and client/policy failures.

Retryable failures include:

```text
408 Request Timeout
429 Too Many Requests
5xx server errors
connection failures
network errors
provider timeouts
other unknown transport/provider failures
```

Ordinary client errors are not retried:

```text
400 Bad Request
404 Not Found
and other ordinary 4xx responses
```

Authentication and authorization errors are also not retried:

```text
401 Unauthorized
403 Forbidden
```

However, 401 and 403 responses count as upstream failures for circuit-breaker purposes because they can indicate an upstream configuration or credential problem.

Provider HTTP status errors are represented internally by `StatusError`, allowing the resilience layer to classify them without depending on provider-specific error strings.

## 5. Exponential Backoff and Jitter

Retry attempts use bounded exponential backoff.

The default progression is approximately:

```text
attempt 1 → immediate
attempt 2 → 100ms + jitter
attempt 3 → 200ms + jitter
attempt 4 → 400ms + jitter
attempt 5 → 800ms + jitter
...
```

The delay is capped by a configurable maximum.

Jitter is added to avoid multiple gateway requests retrying at exactly the same time, which can otherwise create synchronized retry bursts against an unhealthy provider.

The implementation uses bounded positive jitter. With the default 20% jitter factor, a 100ms retry delay can become a value between approximately 100ms and 120ms, subject to the configured maximum.

The following parameters are configurable:

```text
MaxRetries
BackoffBase
BackoffMax
JitterFactor
```

They are configured on the upstream by default and can be overridden by a routing rule.

## 6. Route-Level Resilience Overrides

Routing rules contain a JSONB field named:

```text
timeout_retry_overrides
```

The field defaults to an empty JSON object.

Supported overrides are:

```json
{
  "timeout_ms": 30000,
  "max_retries": 3,
  "backoff_base_ms": 100,
  "backoff_max_ms": 2000,
  "jitter_percent": 20
}
```

An empty object means that the upstream's normal resilience configuration is used.

Explicit zero values are supported where appropriate. In particular:

```json
{
  "max_retries": 0
}
```

means that the route intentionally disables retries. The implementation distinguishes an unset value from an explicitly configured zero.

This distinction is necessary because `0` is a valid retry configuration and must not accidentally be replaced by the upstream default.

## 7. Retry and Failover

Retry and failover operate at different levels.

Retry operates inside a single upstream:

```text
Provider A
  ├── attempt 1 → 503
  ├── backoff
  ├── attempt 2 → 503
  └── retry limit reached
```

Failover operates outside that loop:

```text
Provider A
  ├── retry
  ├── retry
  └── exhausted
        |
        v
Provider B
  ├── retry
  └── success
```

Therefore, a transient failure does not immediately cause provider switching. The selected provider gets the number of attempts allowed by its resilience configuration first.

If the final failure is retryable, the Router can remove that upstream from the remaining candidates and select another eligible provider.

Multiple failovers are supported.

## 8. Circuit Breaker Interaction

Each upstream has its own circuit breaker.

Retryable upstream failures contribute to the upstream's breaker failure count. Successful requests reset the breaker state according to the existing circuit-breaker implementation.

A normal client error such as `400` does not trip the breaker because the upstream is not considered unhealthy merely because a client sent an invalid request.

Authentication failures such as `401` and `403` are different: they are not retried, but they count as breaker failures because they may indicate an invalid provider configuration or credential.

The circuit breaker therefore operates independently for each provider and participates in upstream selection and request execution.

## 9. Configuration Ownership

There are two levels of configuration.

Upstream configuration provides the normal defaults:

```text
Provider
  ├── timeout
  ├── max retries
  ├── backoff base
  ├── backoff maximum
  └── jitter
```

Routing configuration can override those values for a particular request:

```text
Routing Rule
  └── timeout_retry_overrides
```

The effective configuration is calculated per request:

```text
Route override
      |
      | if unset
      v
Upstream default
```

The calculated configuration is passed into the request execution path without modifying the shared upstream object.

## 10. Observability

Retry and failover are recorded as separate observability stages.

A retry produces:

```text
provider_retry
```

while cross-provider switching produces:

```text
provider_failover
```

This distinction allows operational metrics to answer two different questions:

```text
How often are providers recovering through retries?

How often are requests requiring another provider?
```

This is useful when diagnosing provider reliability, network instability, rate limiting, or configuration problems.

## 11. Testing

The resilience implementation is covered by unit tests for:

* retrying transient 5xx failures
* refusing to retry ordinary client errors
* circuit-breaker behavior
* bounded exponential backoff
* jitter bounds
* route-level resilience configuration
* explicit `max_retries: 0`
* timeout configuration
* retry exhaustion
* failover after the selected provider exhausts its retries
* successful requests through a fallback provider

The complete repository test suite is validated with:

```bash
go test ./...
```

## 12. Result

The gateway now provides layered resilience without mixing retry and failover responsibilities.

The final request path is:

```text
Router
  |
  +-- weighted upstream selection
  |
  v
Upstream
  |
  +-- timeout
  |
  +-- retry transient failures
  |     |
  |     +-- exponential backoff
  |     +-- bounded jitter
  |
  +-- Provider.Forward
  |
  +-- retry exhaustion
        |
        v
      Router failover
        |
        +-- next eligible upstream
```

This gives each provider its own timeout, retry, backoff, jitter, and circuit-breaker behavior while allowing routing rules to apply request-specific resilience overrides.