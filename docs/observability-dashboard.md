# Observability Analytics API

The observability view is served at `/observability` and reads from the authenticated, read-only API. The existing live dashboard remains at `/dashboard` with its live request pipeline, guardrail panels, events, and chat view.

## Authorization

All routes under `/api/observability/` require `Authorization: Bearer <AGENTPLANE_ADMIN_TOKEN>`. The compatibility route `/analytics/traces_count` uses the same admin middleware. AgentPlane currently has one global admin token, not operator RBAC or tenant-scoped authorization. API-key credentials do not grant access to analytics.

All responses are JSON and use `Cache-Control: no-store`. Raw database errors, SQL, connection details, bearer tokens, and payload blobs are never returned.

## Endpoints

All endpoints are GET-only. Non-GET requests receive `405`.

| Endpoint                                  | Result                                                                                                                                        |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/observability/health`           | Reachability, data state, and latest trace timestamp. ClickHouse query/ping failures return an `unavailable` state without driver details.    |
| `GET /api/observability/overview`         | Request, success/error, error-rate, latency percentile, token, cost, and latest telemetry aggregates.                                         |
| `GET /api/observability/series`           | Bucketed request, success/error, rate, latency, token, and cost time series plus the selected bucket size.                                    |
| `GET /api/observability/breakdowns`       | Top provider, model, and route rows with request/error, token, cost, and latency metrics. Each category is capped at 20 rows.                 |
| `GET /api/observability/failures`         | Newest failed traces; maximum 100 rows.                                                                                                       |
| `GET /api/observability/guardrails`       | Guardrail action/phase aggregates and newest safe guardrail metadata; maximum 100 recent events.                                              |
| `GET /api/observability/traces`           | Newest trace summaries with keyset pagination; maximum 100 rows per page.                                                                     |
| `GET /api/observability/traces/{traceID}` | Trace summary, usage metadata, prompt/response capture metadata, guardrail events, and tool metadata. The ID may be a trace ID or request ID. |

The legacy `GET /analytics/traces_count?hours=1|6|24|168` remains available for compatibility and returns `{"count":N}` through the same analytics store. Its old raw ClickHouse HTTP SQL forwarding was removed.

## Filters and Pagination

Common query parameters apply to aggregate, series, breakdown, failure, guardrail, and trace queries:

- `range`: `15m`, `1h`, `6h`, `24h`, or `7d`; defaults to `24h`. Other values are rejected.
- `status`: `all`, `success`, or `error`; `error` includes non-success trace statuses and persisted guardrail blocks.
- `provider`, `model`, `route`, `guardrail_action`, `user_id`, `organization_id`, `project_id`: exact-match filters when the corresponding telemetry is populated.

`failures` and `guardrails` also accept `limit` from 1 to 100 (default 50). `traces` accepts the same `limit` and an opaque `cursor` returned as `next_cursor`. The cursor is bound to timestamp and trace ID for descending keyset pagination. List-only parameters are rejected on unrelated endpoints. Repeated, unknown, oversized, malformed, and unsupported parameters receive `400`.

Example:

```http
GET /api/observability/overview?range=24h&provider=openai&model=gpt-4o&status=error
Authorization: Bearer <admin-token>
```

## Response Shapes

Overview fields include `requests`, `successes`, `errors`, fractional `error_rate`, `latency_ms` (`p50_ms`, `p95_ms`, `p99_ms`), `tokens` (`input`, `output`, `total`), `estimated_cost`, and optional `latest_telemetry_at`.

Series returns `{ "bucket_size": "5m", "buckets": [...] }`; each bucket contains timestamp, request/success/error counts, fractional error rate, latency percentiles, token totals, and estimated cost. Bucket sizes are fixed by range: 1 minute for 15m/1h, 5 minutes for 6h, 15 minutes for 24h, and 1 hour for 7d. Buckets are UTC-aligned, include zero-valued intervals, and end strictly before the selected window's exclusive end.

Breakdowns return `provider`, `model`, and `route` arrays. Trace pages return `items` and optional `next_cursor`. Trace detail contains the trace summary, a `usage` object, `prompt` and `response` capture metadata, and bounded `guardrails` and `tools` arrays. Capture metadata includes availability, stored mode, hash, original bytes, and compressed size when present. It never contains the payload blob.

Errors use a stable shape such as `{"error":{"code":"analytics_unavailable","message":"analytics data is temporarily unavailable"}}`. Missing traces return `404`; empty aggregates are successful zero-valued responses. Health returns `status`, `clickhouse_reachable`, `data_state` (`fresh`, `stale`, `empty`, or `unavailable`), and `checked_at`. Telemetry is marked stale when the latest matching trace is more than 15 minutes old.

## Metrics and Query Strategy

ClickHouse `quantileTDigest` percentiles are approximate. Recorded latency starts at chat-handler entry and ends when the provider call completes; output guardrail evaluation and response writing are not included.

ClickHouse `quantileTDigest` percentiles are approximate. Recorded latency starts at chat-handler entry and ends when the provider call completes; output guardrail evaluation and response writing are not included.

ClickHouse `quantileTDigest` percentiles are approximate. The trace latency interval begins at chat-handler entry and ends when the provider call completes; it does not include output guardrail evaluation or response writing.

Request count is the number of filtered rows in `agentplane.traces`; usage events never add request counts. Successes are successful traces without a persisted block outcome. Errors include any non-success trace status and persisted guardrail blocks, including blocks after a provider call. Error rate is `errors / requests`, or zero for a zero-request window. Latency percentiles use `quantileTDigest` at 0.50, 0.95, and 0.99 over the same filtered request rows, in milliseconds. Input/output/total tokens and estimated cost come from the request-level trace row, avoiding double-counting asynchronous usage events. Cost is estimated USD, not billing data. Usage-event detail supplements reasoning/cached token metadata.

All queries use validated, fixed ranges and bound values. The shared filtered trace source applies time, status, provider, model, route, guardrail action, and tenant filters consistently. Its guardrail summary is grouped by trace ID and bounded by the selected event-time window. The timestamp-leading MergeTree sort key and existing TTL/index definitions are reused. Result sizes are capped: 20 rows per breakdown, 100 failures/recent guardrail rows/trace rows, and at most 168 generated time buckets. Each ClickHouse operation has a five-second context deadline that remains active while rows are scanned; each HTTP operation has an eight-second deadline. Time-series buckets are UTC-aligned, selected server-side, zero-filled for empty intervals, and emitted only before the requested end time. Trace IDs are looked up within the selected time window using the existing schema; no new index, projection, materialized view, or application cache is added without measured evidence.

Tables read by the API: `traces` is canonical for request, status, latency, tokens, cost, and identity; `guardrail_events` supplies effective block/action/phase outcomes; `usage_events` supplies supplemental reasoning/cached-token metadata; `prompt_events` and `response_events` supply capture metadata only; and `tool_call_events` supplies safe tool summaries when instrumented. No analytics endpoint writes to ClickHouse.

ClickHouse query errors are converted to generic API-unavailable responses. Health distinguishes connection/query unavailability from empty or stale data. API reads are separate from asynchronous `/chat` telemetry writes; read failures do not propagate to request handlers. Trace, usage, payload, and guardrail writes remain asynchronous, so related detail rows can appear shortly after their trace. Write failures are logged with correlation IDs, never payload content.

## Retention and Privacy

The current migration sets 365-day TTL for traces and usage, 30 days for prompt/response/tool events, and 60 days for guardrail events. Existing tables and indexes are reused.

Capture modes remain governed by `OBSERVE_PROMPT_MODE`, `OBSERVE_RESPONSE_MODE`, and `OBSERVE_SAMPLE_RATE`:

- `disabled`: no capture row is written.
- `full`: payload and hash are stored in ClickHouse.
- `sampled`: selected payloads and hashes are stored; unselected requests have no capture row.
- `hash_only`: hash and size metadata are stored without a payload blob.

The API never selects or returns prompt/response blobs, regardless of mode. The observability UI renders only capture metadata: disabled means no payload was captured; hash-only shows hash/size without payload; sampled and full modes state that payload retrieval is not exposed. Missing capture metadata is described as unavailable/ambiguous, not inferred to mean disabled. Guardrail reason/details and tool inputs/outputs are also omitted.

## Dashboard Frontend

Use the navigation in either page to switch between **Live dashboard** and **Observability**. The observability view is an embedded HTML/CSS/JavaScript page; it adds no frontend framework or chart dependency. Canvas charts render request volume, success/error rate, p50/p95/p99 latency, input/output token use, and estimated cost from the series endpoint. KPI cards show request/success/error counts and rates, latency percentiles, token totals, estimated cost, and latest telemetry time.

One shared filter state drives every API request: range (`15m`, `1h`, `6h`, `24h`, `7d`), status, provider, model, route, guardrail action, user ID, organization ID, and project ID. Range/status changes apply immediately; text filters apply with **Apply filters** or Enter. The page loads each endpoint once per refresh and refreshes every 30 seconds while connected. Breakdown panels show top providers/models/routes. Recent failures and guardrail events are bounded sections.

The trace explorer lists one backend page at a time, newest first. It supports local text search within the currently loaded page, exact trace/request ID lookup through the detail endpoint, copy buttons for IDs, and keyset next/previous pagination. Selecting an ID opens metadata, usage, capture, guardrail, and tool details. Durable lifecycle timeline events are not present in ClickHouse and are explicitly shown as unavailable.

Analytics requires the existing admin bearer token. The operator enters it into the password field on the observability page; JavaScript holds it only in page memory, clears the field after connecting, sends it only as the `Authorization` header to same-origin API requests, and discards it on disconnect or authorization failure. It is not written to a URL, browser storage, page source, or logs. The page does not expose raw payloads. Empty data is shown separately from failed API requests; partial endpoint failures are identified without replacing successful sections, and stale telemetry is flagged from the health response. The page polls at a modest 30-second interval and also provides a manual refresh.

The existing `/events` stream remains available to the live view and is unauthenticated; it does not emit CORS headers, strips structured event `data`, removes guardrail detail text, and suppresses upstream URLs while retaining lifecycle stage, safe summary, status, and request ID. The live event log HTML-escapes message text. The live chat widget accepts an API key in a password field and sends it only in the `/chat` Authorization header; it is not saved to browser storage or a URL.

## Known Telemetry Limitations

SSE is not an authorization mechanism: CORS is disabled for cross-origin browser clients, structured data and sensitive message details are stripped, but same-origin clients can still observe safe lifecycle summaries and request IDs.

- Authenticated user ID is persisted for requests reaching the provider and input guardrail blocks. Organization and project IDs are not synthesized and remain empty until AgentPlane has authoritative values.
- Guardrail action/phase persistence is connected to evaluations. Effective `block` outcomes count as request errors even when the provider trace status was successful; trace detail and guardrail analytics use the event table when the trace column is empty.
- Malformed JSON and authentication failures occur before durable trace creation and are not included in trace analytics. Input guardrail blocks have a trace; output guardrail outcomes correlate with the existing provider trace.
- Tool-call tables exist but AgentPlane does not currently instrument tool execution.
- Only the current OpenAI-compatible provider path normalizes usage. Missing provider usage produces zero token counts.
- Sampled capture omissions cannot be distinguished from disabled, expired, or failed capture from a trace detail response alone.
- There is no ClickHouse integration-test service configured in the repository. Query mapping and handler behavior are covered with deterministic unit seams; production-like ClickHouse query validation remains an operational check.

## Troubleshooting

- `401` on analytics: provide the configured `AGENTPLANE_ADMIN_TOKEN`; an AgentPlane API key is not sufficient.
- Health `unavailable`: verify `CLICKHOUSE_HOST`, native `CLICKHOUSE_PORT`, credentials/access, and that the observability tables exist. The API intentionally omits host and driver details.
- Health `empty`: ClickHouse is reachable but there are no matching trace rows in the selected window/filters.
- Health `stale`: traces exist, but the newest matching trace is older than 15 minutes.
- Missing prompt/response metadata: capture may be disabled, unsampled, expired, or unavailable; absence is intentionally ambiguous.
- Missing guardrail/tool detail: guardrail events are persisted after evaluation; tool execution is not currently instrumented.
