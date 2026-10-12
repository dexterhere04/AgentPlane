# AgentPlane Routing Metrics

## 1. Purpose

AgentPlane exposes two complementary views of routing activity:

* **In-process counters** — They reflect everything that has happened since the
  gateway process started and do **not** depend on ClickHouse or the tracing
  pipeline.
* **ClickHouse analytics** — windowed, historical breakdowns served from
  `GET /admin/analytics`. They answer "in the last N hours, what did routing
  actually do" broken down by provider, route, and failover.

```text
Router / Upstream
      |
      +--> proxy.DefaultMetrics  (atomic counters, in-process)
      |            |
      |            v
      |        GET /metrics        -> live cumulative snapshot
      |
      +--> observability bus -> ClickHouse agentplane.traces
                       |
                       v
            GET /admin/analytics?type=...  -> windowed breakdowns
```

The two sources are documented in `provider-routing-and-router.md` (routing
behavior) and `resilience-and-retry.md` (failover/retry mechanics). This
document is the metrics reference for both.

## 2. `GET /metrics` — In-Process Routing Counters

### Response shape

The endpoint merges the guardrail snapshot and the routing snapshot into a
single JSON object. The two use disjoint field-name prefixes
(`guardrail_*` vs `routing_*`) so nothing collides:

```json
{
  "guardrail_evaluations_total": 0,
  "guardrail_blocks_total": 0,
  "guardrail_redactions_total": 0,
  "guardrail_warns_total": 0,
  "guardrail_passed_total": 0,
  "guardrail_errors_total": 0,
  "guardrail_evaluation_duration_avg_ms": 0,
  "routing_decisions_total": 0,
  "routing_failovers_total": 0,
  "routing_retries_total": 0,
  "routing_breaker_trips_total": 0,
  "routing_breaker_recoveries_total": 0,
  "routing_no_provider_total": 0,
  "routing_denied_total": 0,
  "routing_upstreams": []
}
```

### Field semantics

| Field | Meaning | Recorded when |
|-------|---------|---------------|
| `routing_decisions_total` | Primary upstream selections — the initial attempt for a request, before any failover | `router.go` — `RecordDecision`, at primary selection |
| `routing_failovers_total` | Switches away from a failing upstream | `router.go` — `RecordFailover`, at each failover hop |
| `routing_retries_total` | Retries on the same upstream | `upstream.go` — `RecordRetry`, per retry attempt |
| `routing_breaker_trips_total` | Transitions of a circuit breaker into the open state | `upstream.go` — `RecordBreakerTrip` |
| `routing_breaker_recoveries_total` | Transitions of a circuit breaker back to closed | `upstream.go` — `RecordBreakerRecovery` |
| `routing_no_provider_total` | Requests that found no eligible upstream | `router.go` — `RecordNoProvider`, when candidate lookup is empty |
| `routing_denied_total` | Requests rejected by a `deny` / `force_deny` routing rule | `router.go` — `RecordDenied`, before any provider is selected |

### Per-upstream breakdown (`routing_upstreams`)

Each registered upstream carries its own counters. Sorted by name so the
output is stable across polls:

```json
{
  "routing_upstreams": [
    {
      "name": "openai-primary",
      "decisions": 120,
      "failovers": 3,
      "retries": 9,
      "breaker_trips": 1,
      "breaker_recoveries": 1
    }
  ]
}
```

Semantics:

* `decisions` — how often this upstream was the primary choice.
* `failovers` — how often traffic **moved off** this upstream. The count is
  attributed to the upstream being left, so it answers "how often did this
  provider get abandoned for another".
* `retries` — in-place retries issued against this upstream.
* `breaker_trips` / `breaker_recoveries` — opens and closes of this
  upstream's circuit breaker.

### Properties

* Counters are cumulative for the life of the process and reset on restart.
* Recording is synchronous and atomic; snapshotting never blocks recording
  for long (`internal/proxy/metrics.go`).
* The endpoint is served without touching ClickHouse, so it stays live even
  when the analytics database is down.

## 3. `GET /admin/analytics` — ClickHouse Routing Analytics

The analytics endpoint runs whitelisted queries against the `agentplane.traces`
table and returns ClickHouse `FORMAT JSON` output verbatim.

### Parameters

| Param | Default | Range |
|-------|---------|-------|
| `type` | — (or a pinned default on dedicated routes) | one of the whitelisted queries below |
| `hours` | `24` | integer between `1` and `720` (30 days) |

### Routing-relevant trace columns

Routing analytics are derived from the columns the tracing pipeline records
per request (`internal/clickhouse/client.go` — `InsertTrace`):

| Column | Type | Meaning |
|--------|------|---------|
| `provider` | String | Provider/upstream that handled the request |
| `route` | String | Routing rule that matched (empty for default-provider traffic) |
| `upstream` | String | Upstream selected at the final attempt |
| `attempt` | UInt64 | Attempt number (1 = primary, N = after retry/failover) |
| `failover` | UInt8 | `1` when the request experienced failover |
| `failover_from` / `failover_to` | String | Providers involved in the failover |
| `status` | String | `success` or error/status code |
| `latency_ms` | Float64 | End-to-end request latency |
| `input_tokens` / `output_tokens` / `total_tokens` | UInt64 | Token usage |
| `estimated_cost` | Float64 | Estimated spend |

### Whitelisted analytics types

| `type` | Returns | Per |
|--------|---------|-----|
| `provider_traffic` | `request_count`, `traffic_percentage` | provider |
| `route_traffic` | `request_count`, `traffic_percentage` | route |
| `latency_percentiles` | `p50_latency_ms`, `p95_latency_ms`, `p99_latency_ms` | provider |
| `error_rate` | average error rate for the window | overall |
| `error_rate_summary` | `total_requests`, `error_count`, `error_rate_percentage` | provider |
| `failover_count` | `failover_count`, `total_requests`, `failover_percentage` | window summary |
| `failover_transitions` | `failover_from`, `failover_to`, `failover_count` | pair |
| `tokens_by_provider` | `input_tokens`, `output_tokens`, `total_tokens`, `request_count` | provider |
| `tokens_by_route` | `input_tokens`, `output_tokens`, `total_tokens`, `request_count` | route |
| `cost_by_provider` | `total_cost`, `avg_cost_per_request`, `request_count` | provider |
| `cost_by_route` | `total_cost`, `avg_cost_per_request`, `request_count` | route |

Percentages are computed with `nullIf()` division so that zero-traffic windows
return `null` rather than a divide-by-zero (clients render these as `0`).

## 4. Web Dashboard Consumption

* **Overview → Routing widget**: polls `/metrics` every 5s for the six live
  cumulative counters, and polls `provider_traffic`, `failover_count`, and
  `error_rate_summary` (24h window) every 15s for provider traffic bars and
  failover/error stats.
* **Routing tab**: polls `/metrics` every 5s for the live-counters card (which
  explicitly requires no ClickHouse), and refreshes the analytics block
  every 15s — `provider_traffic`, `latency`, `error_rate_summary`, `failovers`,
  `tokens_by_route`, `tokens_by_provider`, `cost_by_route`, `cost_by_provider`,
  and `failover_transitions` — over the selected window.
* **Event log**: the `/events` SSE stream carries per-request routing events
  (`routing_decision`, `routing_failover`) for live, per-request visibility,
  complementing the aggregate counters above.

## 5. Authentication

Both endpoints require admin authorization:

```text
Authorization: Bearer <admin-token-or-admin-role-api-key>
GET /metrics
GET /admin/analytics?type=provider_traffic&hours=24
```

The static bootstrap admin token and any API key whose user holds the `admin`
role are accepted. Some routes (e.g. `/events`) also accept `/events?token=...`
as a query-parameter alternative.

## 6. Operational Notes

* **Restart resets** the in-process `/metrics` routing counters; ClickHouse
  analytics survive restarts because they are derived from persisted traces.
* `/metrics` remains available when ClickHouse is unhealthy; `/admin/analytics`
  returns the ClickHouse error (typically HTTP 502) in that case.
* Analytics windows are truncated to `1..720` hours; values outside that range
  are rejected with `400`.
* Lead diagnostics: a high `routing_failovers_total` with rising per-upstream
  `failovers` → check `failover_transitions` to see which pairs are involved;
  a high `routing_no_provider_total` → check `route_traffic` and the registry
  candidate filter in `provider-routing-and-router.md`.