# AgentPlane Weighted Selection and Failover

## 1. Purpose

AgentPlane supports weighted provider selection, provider-level retries, circuit breakers, and cross-provider failover.

These mechanisms provide reliability when a selected provider is unavailable, times out, or returns a retryable error.

The responsibility is divided between the selector, upstream, and Router:

```text
WeightedSelector
    |
    +--> Select initial provider

Upstream
    |
    +--> Retry the same provider
    +--> Enforce timeout
    +--> Enforce circuit breaker

Router
    |
    +--> Fail over to another provider
```

## 2. Weighted Provider Selection

The Router receives eligible upstreams from the Registry and passes them to the weighted selector.

Each upstream has a configured weight.

For example:

```text
Provider A   weight = 70
Provider B   weight = 20
Provider C   weight = 10
```

The selector chooses providers proportionally to their configured weights.

Only upstreams with a positive weight participate in selection.

If no upstream has a positive weight, selection fails with:

```text
ErrNoWeightedUpstream
```

The selector does not perform retries or failover. Its responsibility ends after selecting an upstream.

## 3. Retry vs Failover

Retries and failover are separate mechanisms.

A retry means:

```text
Same Provider
    |
    +--> attempt 1
    |
    +--> attempt 2
    |
    +--> attempt 3
```

Failover means:

```text
Provider A
    |
    +--> retries exhausted
    |
    v
Provider B
    |
    +--> retries
    |
    v
Provider C
```

The `Upstream` owns retries for a single provider.

The Router owns failover between different upstreams.

This separation prevents the Router from needing to understand provider-specific retry behavior.

## 4. Upstream Retry Behavior

Each upstream has a configurable `MaxRetries`.

If `MaxRetries` is `1`, the request can make up to two attempts:

```text
Initial attempt
      |
      +--> failure
              |
              v
           Retry
              |
              v
         Final attempt
```

Retries use an increasing backoff based on the upstream's configured backoff base.

The upstream also applies its configured timeout to provider requests.

A retry is attempted only when the failure is classified as retryable.

## 5. Failure Classification

Provider failures are classified based on the resulting error.

The current classification is:

| Failure              | Retryable | Breaker Failure |
| -------------------- | --------: | --------------: |
| HTTP 408             |       Yes |             Yes |
| HTTP 429             |       Yes |             Yes |
| HTTP 5xx             |       Yes |             Yes |
| HTTP 401             |        No |             Yes |
| HTTP 403             |        No |             Yes |
| Other HTTP 4xx       |        No |              No |
| Network error        |       Yes |             Yes |
| Timeout              |       Yes |             Yes |
| Context cancellation |        No |              No |
| Context deadline     |        No |              No |

This distinction is important because not every provider failure should trigger another attempt.

For example, an authentication failure such as HTTP 401 should not cause another provider attempt through normal retry logic.

## 6. Cross-Provider Failover

After an upstream has exhausted its own retries, the Router can fail over to another eligible upstream.

The Router maintains the remaining candidates for the current request.

Conceptually:

```text
Candidates:

[A, B, C]

Select A
  |
  +--> A fails after retries
  |
  v
Remove A

Remaining:

[B, C]

Select B
  |
  +--> B succeeds
  |
  v
Return response
```

The failed upstream is removed from the candidate set for that request.

This prevents the Router from immediately selecting the same failed upstream again.

## 7. Multiple Failovers

Failover can continue through multiple providers.

For example:

```text
Provider A
   |
   +--> retry
   |
   +--> failure
   |
   v
Provider B
   |
   +--> retry
   |
   +--> failure
   |
   v
Provider C
   |
   +--> success
   |
   v
Response
```

If all eligible upstreams fail, the Router returns the final provider error.

The Router does not continue indefinitely because each failed upstream is removed from the remaining candidate set.

## 8. When Failover Does Not Occur

The Router does not fail over for non-retryable client failures.

For example:

```text
Provider A
    |
    +--> HTTP 400
    |
    v
Return error
```

There is no reason to send the same invalid request to another provider when the failure represents a client-side request problem.

Failover is also stopped when the request context is canceled or its deadline expires.

```text
Provider A
    |
    +--> context canceled
    |
    v
Stop processing
```

## 9. Circuit Breaker Interaction

Each upstream has its own circuit breaker.

The circuit breaker prevents requests from being sent to an upstream that is currently considered unhealthy.

Before an upstream attempt is executed, the upstream checks whether its circuit breaker allows the request.

If the circuit is open, the upstream returns an error without making a provider request.

The Registry also checks circuit-breaker readiness when building the candidate set.

This means an unhealthy upstream can be excluded before weighted selection occurs.

The overall flow is:

```text
Registry
    |
    +--> exclude disabled upstreams
    +--> exclude unsupported models
    +--> exclude wrong provider groups
    +--> exclude circuit-open upstreams
    |
    v
Eligible candidates
    |
    v
Weighted selection
```

## 10. Retry and Failover Interaction

The complete reliability flow is:

```text
                  Weighted Selection
                         |
                         v
                   Provider A
                         |
                    attempt 1
                         |
                     failure
                         |
                       retry
                         |
                    attempt 2
                         |
                     failure
                         |
                  retries exhausted
                         |
                         v
                  Cross-provider
                     failover
                         |
                         v
                   Provider B
                         |
                    attempt 1
                         |
                     failure
                         |
                       retry
                         |
                    attempt 2
                         |
                      success
                         |
                         v
                     Response
```

The important distinction is:

```text
Retry   = same upstream
Failover = different upstream
```

## 11. Telemetry

Retry and failover are exposed as separate observability events.

Retry events use:

```text
provider_retry
```

Failover events use:

```text
provider_failover
```

A retry event represents another attempt against the same provider.

A failover event represents the Router moving from one upstream to another.

This allows operational telemetry to distinguish:

```text
Provider instability
        |
        +--> retries

Provider failure requiring another provider
        |
        +--> failover
```

## 12. Deterministic Selection for Testing

The production Router uses the weighted selector, but the Router depends on the `UpstreamSelector` interface rather than directly requiring a concrete selector implementation.

Conceptually:

```go
type UpstreamSelector interface {
    Select([]*Upstream) (*Upstream, error)
}
```

This allows tests to provide a deterministic selector.

That is important for failover tests because weighted random selection should not determine which provider is selected during a unit test.

Tests can therefore explicitly verify flows such as:

```text
Select A
  |
  +--> fail
  |
  v
Select B
  |
  +--> fail
  |
  v
Select C
  |
  +--> success
```

This makes failover behavior deterministic and avoids flaky tests.

## 13. End-to-End Reliability Flow

The complete routing and reliability path is:

```text
HTTP Request
     |
     v
Router
     |
     v
Routing Rule
     |
     v
Provider Group
     |
     v
Registry
     |
     +--> enabled?
     +--> correct group?
     +--> supports model?
     +--> circuit available?
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
     +--> timeout
     +--> circuit breaker
     +--> provider request
     +--> retry if retryable
     |
     +--> failure after retries
             |
             v
        Router Failover
             |
             v
        Next Upstream
             |
             +--> retries
             |
             +--> additional failover
             |
             v
          Response/Error
```

The architecture therefore separates the responsibilities cleanly:

```text
Routing
    -> Router

Candidate discovery
    -> Registry

Initial provider choice
    -> WeightedSelector

Same-provider reliability
    -> Upstream

Cross-provider reliability
    -> Router

Operational visibility
    -> Observability events
```