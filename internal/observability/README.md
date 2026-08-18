# ClickHouse Observability

## Overview

This package provides optional ClickHouse-backed observability for AgentPlane, storing request traces, prompts, responses, and usage analytics. Observability is entirely optional and failures do not impact user `/chat` requests.

## Architecture

The observability layer is vendor-neutral:

```
HTTP Handler (/chat)
    ↓
observability.CapturePromptPayload() / CaptureResponsePayload()
    ↓
observability.RecordTrace()
    ↓
Store interface (abstract)
    ↓
ClickHouseAdapter (concrete)
    ↓
internal/clickhouse.Client (native ClickHouse client)
```

## Enabling Observability

Set environment variables at startup:

```bash
export CLICKHOUSE_HOST=127.0.0.1      # (required)
export CLICKHOUSE_PORT=9000            # (required; native protocol)
export OBSERVE_PROMPT_MODE=full        # (optional: disabled|full|sampled|hash_only, default: full)
export OBSERVE_RESPONSE_MODE=full      # (optional: same modes)
export OBSERVE_SAMPLE_RATE=1.0         # (optional: 0.0-1.0, default: 1.0, used with sampled mode)
export PRICING_OPENAI_GPT4O=0.003:0.006  # (optional: input_rate:output_rate per 1k tokens)
```

If `CLICKHOUSE_HOST` and `CLICKHOUSE_PORT` are not set, observability is disabled but the application continues normally.

## ClickHouse Schema

All tables reside in the `agentplane` database. See `migrations/clickhouse/001_create_tables.sql` for the authoritative schema.

### traces

High-level request metadata (small, frequently queried, 365-day retention):

- `trace_id` — unique request ID
- `request_id` — alternate request ID
- `timestamp` — request start time
- `user_id` — user context (empty if no auth)
- `organization_id` — org context (empty if no auth)
- `project_id` — project context (empty if no auth)
- `provider` — LLM provider (e.g., "openai")
- `model` — model name (e.g., "gpt-4o")
- `latency_ms` — request latency in milliseconds
- `status` — "success", "http_NNN", or error code
- `cache_hit` — whether response was cached (0 if not implemented)
- `input_tokens`, `output_tokens`, `total_tokens` — token counts from provider
- `estimated_cost` — dollars (0.0 if pricing unavailable; not for billing)
- `guardrail_action` — "allowed", "blocked", "redacted" (empty if no guardrails)
- `route` — e.g., "/chat"

### prompt_events

Request prompts (compressed, 30-day retention):

- `trace_id` — reference to traces table
- `prompt_blob` — compressed prompt JSON (gzip → ClickHouse ZSTD)
- `prompt_hash` — SHA256 of original (decompressed) prompt
- `prompt_bytes` — original (uncompressed) size in bytes
- `compressed_size` — size after application compression
- `capture_mode` — "full", "hash_only", "disabled", "sampled"
- `created_at` — storage timestamp

### response_events

LLM responses (compressed, 30-day retention):

- `trace_id` — reference to traces table
- `response_blob` — compressed response JSON (gzip → ClickHouse ZSTD)
- `response_hash` — SHA256 of original (decompressed) response
- `response_bytes` — original (uncompressed) size in bytes
- `compressed_size` — size after application compression
- `capture_mode` — "full", "hash_only", "disabled", "sampled"
- `created_at` — storage timestamp

### usage_events

Normalized token counts (365-day retention):

- `trace_id` — reference to traces table
- `provider`, `model` — from request
- `input_tokens`, `output_tokens`, `total_tokens` — normalized from provider response
- `reasoning_tokens`, `cached_input_tokens` — advanced usage (0 if unsupported by provider)
- `estimated_cost` — dollars (same caveat as traces)
- `created_at` — storage timestamp

### tool_call_events

Tool invocations (30-day retention; **currently not instrumented**):

- `trace_id` — reference to traces table
- `tool_name` — which tool was invoked
- `tool_input`, `tool_output` — compressed (ZSTD)
- `success` — 1 if successful, 0 if failed
- `latency_ms` — execution time
- `created_at` — storage timestamp

### guardrail_events

Policy/security outcomes (60-day retention; **currently not instrumented**):

- `trace_id` — reference to traces table
- `guardrail_name` — which guardrail rule
- `phase` — "input" or "output"
- `action` — "allowed", "blocked", "redacted"
- `reason` — why the action was taken
- `created_at` — storage timestamp

## Payload Compression

Prompts and responses are compressed before storage:

1. Original payload → gzip compression (application-side)
2. Compressed bytes → stored in ClickHouse ZSTD codec (ClickHouse-side)
3. Both `prompt_bytes` (original) and `compressed_size` (after gzip) are stored for analysis

Decompression for hashing is automatic in the adapter. Retrieval requires ClickHouse decompression.

## Token Usage and Cost Estimation

### Token Normalization

Currently supports OpenAI-style `usage` objects in responses:

```json
{
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

Other providers are not yet normalized. If a provider does not return usage, tokens default to 0.

### Cost Calculation

Cost is estimated based on configurable pricing:

```go
// internal/config/pricing.go
cost = (inputTokens / 1000.0) * inputCostPer1k + (outputTokens / 1000.0) * outputCostPer1k
```

**Important:** Estimated costs are **NOT billing data**. Use only for analytics and dashboarding. Actual charges should come from provider invoices.

Pricing can be:

- Set in code via `config.SetDefaultPricing(table)`
- Overridden via environment: `PRICING_OPENAI_GPT4O=0.003:0.006` (input:output per 1k tokens)
- Queried via `config.GetPricing("openai", "gpt-4o")` (returns nil if not found)

If pricing is unavailable for a model, cost defaults to 0.0 and is logged.

## Capture Modes

Payload capture can be configured globally via environment variables:

- `OBSERVE_PROMPT_MODE` and `OBSERVE_RESPONSE_MODE` accept:
  - `disabled` — do not capture payloads
  - `full` — capture all payloads (default)
  - `sampled` — capture a sample (use `OBSERVE_SAMPLE_RATE`)
  - `hash_only` — capture hash only, skip large payloads

- `OBSERVE_SAMPLE_RATE` (0.0–1.0, default 1.0) — percentage of requests to sample

Currently all captures default to `full` mode. Per-org/project configuration is not yet implemented.

## Retention

Retention is configured via ClickHouse TTL directives in the migration:

- `traces`: 365 days (small, searchable metadata)
- `prompt_events`, `response_events`: 30 days (large compressed payloads)
- `tool_call_events`: 30 days
- `guardrail_events`: 60 days
- `usage_events`: 365 days

Changing retention requires updating the migration and redeploying. Existing data is not retroactively aged.

## Analytics

The `internal/observability/analytics.go` package provides SQL query builders for common questions:

```go
// In HTTP handler
q := analytics.TraceCountQuery(24)  // Last 24 hours
q := analytics.AverageLatencyQuery(24)
q := analytics.FailedRequestsQuery(24)
q := analytics.TokenUsageQuery(24)
q := analytics.EstimatedCostQuery(24)
q := analytics.TopModelsQuery(24, 10)
q := analytics.ProviderUsageQuery(24)
q := analytics.GuardrailEventsQuery(24)  // (only if guardrails exist)
q := analytics.ToolCallEventsQuery(24)   // (only if tools exist)
// Execute q.Query with q.Params against ClickHouse
```

No analytics endpoints are built into the server; add them as needed.

## Error Handling and Observability Resilience

**Critical:** If ClickHouse is unavailable, the `/chat` request must still succeed.

All observability operations are asynchronous and fire-and-forget:

```go
go observability.RecordTrace(t)  // Does not block
go observability.CapturePromptPayload(p)  // Does not block
```

The adapter logs errors to stdout/stderr:

```
observability: error storing trace (trace_id=req-1234): connection refused
```

Operators should monitor these logs to detect observability failures. Do NOT silently swallow errors; log them.

## Not Yet Implemented

### Tool Calls

No tool execution system exists in AgentPlane yet. The schema and adapter are ready; instrumentation can be added when a tool-call framework is implemented.

### Guardrails

No guardrail/policy system exists yet. Add to the observability layer when guardrail framework is built.

### User/Org/Project Context

No authentication middleware exists in AgentPlane. The schema supports `user_id`, `organization_id`, `project_id`, but they remain empty. Build authentication and set these fields when multi-tenant support is added.

### Cache Tracking

No caching layer exists. The `cache_hit` field defaults to false. Wire it when caching is implemented.

### Per-org/project Capture Configuration

Capture modes are currently global. Per-tenant configuration requires multi-tenant infrastructure.

## Testing

End-to-end verification requires:

1. ClickHouse running (locally or remote)
2. AgentPlane started with `CLICKHOUSE_HOST` and `CLICKHOUSE_PORT` set
3. A `/chat` request sent to AgentPlane
4. Query ClickHouse for rows in `traces`, `prompt_events`, `response_events`, `usage_events`

Example queries:

```sql
SELECT trace_id, model, latency_ms, status FROM agentplane.traces ORDER BY timestamp DESC LIMIT 1;
SELECT trace_id, prompt_hash, prompt_bytes, compressed_size FROM agentplane.prompt_events ORDER BY created_at DESC LIMIT 1;
SELECT trace_id, provider, input_tokens, output_tokens, estimated_cost FROM agentplane.usage_events ORDER BY created_at DESC LIMIT 1;
```

Verify `trace_id` values match across tables and payloads are compressed (compressed_size < prompt_bytes).

## Configuration Reference

| Env Var                     | Type   | Default       | Example                                    |
| --------------------------- | ------ | ------------- | ------------------------------------------ |
| `CLICKHOUSE_HOST`           | string | (none)        | `127.0.0.1`                                |
| `CLICKHOUSE_PORT`           | int    | 9000          | `9000`                                     |
| `OBSERVE_PROMPT_MODE`       | string | `full`        | `full`, `disabled`, `sampled`, `hash_only` |
| `OBSERVE_RESPONSE_MODE`     | string | `full`        | `full`, `disabled`, `sampled`, `hash_only` |
| `OBSERVE_SAMPLE_RATE`       | float  | 1.0           | `0.1` (10%)                                |
| `PRICING_OPENAI_GPT4O`      | string | `0.003:0.006` | `input_rate:output_rate`                   |
| `PRICING_OPENAI_GPT4_TURBO` | string | `0.01:0.03`   | `input_rate:output_rate`                   |
| (etc. for other models)     |        |               |                                            |

## References

- ClickHouse Docs: https://clickhouse.com/docs/
- ClickHouse Go Driver: https://github.com/ClickHouse/clickhouse-go/
- AgentPlane Schema: `migrations/clickhouse/001_create_tables.sql`
