-- Create database if not exists
CREATE DATABASE IF NOT EXISTS agentplane;

-- 1. Traces table: Core request metadata (small & frequently queried)
CREATE TABLE IF NOT EXISTS agentplane.traces (
    trace_id String,
    request_id String,
    timestamp DateTime DEFAULT now(),
    user_id String,
    username String DEFAULT '',
    organization_id String,
    project_id String,
    provider String,
    model String,
    latency_ms UInt32,
    status String,
    cache_hit UInt8,
    input_tokens UInt32,
    output_tokens UInt32,
    total_tokens UInt32,
    estimated_cost Float64,
    guardrail_action String DEFAULT '',
    route String DEFAULT ''
) ENGINE = MergeTree()
ORDER BY (timestamp, organization_id, user_id)
TTL timestamp + INTERVAL 365 DAY;

-- 2. Prompts table: Full prompt text (compressed, shorter retention)
CREATE TABLE IF NOT EXISTS agentplane.prompt_events (
    trace_id String,
    user_id String DEFAULT '',
    username String DEFAULT '',
    prompt_blob String CODEC(ZSTD),
    prompt_text String DEFAULT '',
    prompt_hash String,
    prompt_bytes UInt32,
    compressed_size UInt32,
    capture_mode String DEFAULT 'full',
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (created_at, trace_id)
TTL created_at + INTERVAL 30 DAY;

-- 3. Responses table: Full response text (compressed, shorter retention)
CREATE TABLE IF NOT EXISTS agentplane.response_events (
    trace_id String,
    response_blob String CODEC(ZSTD),
    response_hash String,
    response_bytes UInt32,
    compressed_size UInt32,
    capture_mode String DEFAULT 'full',
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (created_at, trace_id)
TTL created_at + INTERVAL 30 DAY;

-- 4. Tool calls table: Tool invocations and results
CREATE TABLE IF NOT EXISTS agentplane.tool_call_events (
    trace_id String,
    tool_name String,
    tool_input String CODEC(ZSTD),
    tool_output String CODEC(ZSTD),
    success UInt8,
    latency_ms UInt32,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (created_at, trace_id)
TTL created_at + INTERVAL 30 DAY;

-- 5. Guardrail events: Security/policy outcomes
CREATE TABLE IF NOT EXISTS agentplane.guardrail_events (
    trace_id String,
    guardrail_name String,
    phase String,  -- 'input', 'output'
    action String,  -- 'allowed', 'blocked', 'redacted'
    reason String,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (created_at, trace_id)
TTL created_at + INTERVAL 60 DAY;

-- 6. Usage events: Token counts and cost
CREATE TABLE IF NOT EXISTS agentplane.usage_events (
    trace_id String,
    provider String,
    model String,
    input_tokens UInt32,
    output_tokens UInt32,
    reasoning_tokens UInt32 DEFAULT 0,
    cached_input_tokens UInt32 DEFAULT 0,
    estimated_cost Float64,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (created_at, provider, model)
TTL created_at + INTERVAL 365 DAY;

-- Indices for fast queries
ALTER TABLE agentplane.traces ADD INDEX IF NOT EXISTS idx_org (organization_id) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.traces ADD INDEX IF NOT EXISTS idx_user (user_id) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.traces ADD INDEX IF NOT EXISTS idx_model (model) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.prompt_events ADD INDEX IF NOT EXISTS idx_trace (trace_id) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.response_events ADD INDEX IF NOT EXISTS idx_trace (trace_id) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.tool_call_events ADD INDEX IF NOT EXISTS idx_trace (trace_id) TYPE bloom_filter GRANULARITY 1;
ALTER TABLE agentplane.guardrail_events ADD INDEX IF NOT EXISTS idx_trace (trace_id) TYPE bloom_filter GRANULARITY 1;

-- Columns added later for user-based analytics. ADD COLUMN IF NOT EXISTS is
-- idempotent so these apply to existing tables created before this change.
ALTER TABLE agentplane.traces ADD COLUMN IF NOT EXISTS username String DEFAULT '';
ALTER TABLE agentplane.prompt_events ADD COLUMN IF NOT EXISTS user_id String DEFAULT '';
ALTER TABLE agentplane.prompt_events ADD COLUMN IF NOT EXISTS username String DEFAULT '';
ALTER TABLE agentplane.prompt_events ADD COLUMN IF NOT EXISTS prompt_text String DEFAULT '';
