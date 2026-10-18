package clickhouse

import (
	"context"
	"fmt"
)

// This file adds batched writers and a guarded truncate to the ClickHouse
// client. The singular Insert* methods in client.go prepare, append one row
// and send — a round trip per record. Backfilling analytics (demos, imports,
// migrations) needs thousands of rows at once, so these methods append an
// entire slice into a single batch and send it once.

// batchInsert is the shared append/send loop. It is intentionally unexported so
// every public batch method keeps its own column list and row mapping.
func batchInsert[T any](
	ctx context.Context,
	prepare func(context.Context, string) (batchAppender, error),
	query string,
	rows []T,
	appendRow func(batchAppender, *T) error,
) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := prepare(ctx, query)
	if err != nil {
		return err
	}

	for i := range rows {
		if err := appendRow(batch, &rows[i]); err != nil {
			return err
		}
	}

	return batch.Send()
}

// batchAppender is the slice of driver.Batch these helpers rely on. Keeping it
// narrow makes batchInsert testable without a live server.
type batchAppender interface {
	Append(...any) error
	Send() error
}

// InsertTracesBatch inserts many trace records in a single batch.
func (c *Client) InsertTracesBatch(ctx context.Context, traces []TraceEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.traces (
        trace_id, request_id, timestamp, user_id, username, organization_id, project_id,
        provider, model, latency_ms, status, cache_hit,
        input_tokens, output_tokens, total_tokens, estimated_cost,
        guardrail_action, route, upstream, attempt, failover, failover_from, failover_to
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, traces, func(batch batchAppender, t *TraceEvent) error {
		return batch.Append(
			t.TraceID, t.RequestID, t.Timestamp, t.UserID, t.Username, t.OrgID, t.ProjectID,
			t.Provider, t.Model, t.LatencyMs, t.Status, t.CacheHit,
			t.InputTokens, t.OutputTokens, t.TotalTokens, t.EstimatedCost,
			t.GuardrailAction, t.Route, t.Upstream, t.Attempt, t.Failover != 0, t.FailoverFrom, t.FailoverTo,
		)
	})
}

// InsertPromptsBatch inserts many prompt capture events in a single batch.
func (c *Client) InsertPromptsBatch(ctx context.Context, prompts []PromptEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.prompt_events (
        trace_id, user_id, username, prompt_blob, prompt_text, prompt_hash, prompt_bytes, compressed_size, capture_mode, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, prompts, func(batch batchAppender, p *PromptEvent) error {
		return batch.Append(
			p.TraceID, p.UserID, p.Username, p.PromptBlob, p.PromptText,
			p.PromptHash, p.PromptBytes, p.CompressedSize, p.CaptureMode, p.CreatedAt,
		)
	})
}

// InsertResponsesBatch inserts many response capture events in a single batch.
func (c *Client) InsertResponsesBatch(ctx context.Context, responses []ResponseEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.response_events (
        trace_id, response_blob, response_hash, response_bytes, compressed_size, capture_mode, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?)
`, responses, func(batch batchAppender, r *ResponseEvent) error {
		return batch.Append(
			r.TraceID, r.ResponseBlob, r.ResponseHash, r.ResponseBytes,
			r.CompressedSize, r.CaptureMode, r.CreatedAt,
		)
	})
}

// InsertGuardrailsBatch inserts many guardrail events in a single batch.
func (c *Client) InsertGuardrailsBatch(ctx context.Context, events []GuardrailEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.guardrail_events (
        trace_id, guardrail_name, phase, action, reason, created_at
    ) VALUES (?, ?, ?, ?, ?, ?)
`, events, func(batch batchAppender, g *GuardrailEvent) error {
		return batch.Append(
			g.TraceID, g.GuardrailName, g.Phase, g.Action, g.Reason, g.CreatedAt,
		)
	})
}

// InsertUsageBatch inserts many usage events in a single batch.
func (c *Client) InsertUsageBatch(ctx context.Context, events []UsageEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.usage_events (
        trace_id, provider, model, input_tokens, output_tokens,
        reasoning_tokens, cached_input_tokens, estimated_cost,
        route, upstream, attempt, failover, failover_from, failover_to, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, events, func(batch batchAppender, u *UsageEvent) error {
		return batch.Append(
			u.TraceID, u.Provider, u.Model, u.InputTokens, u.OutputTokens,
			u.ReasoningTokens, u.CachedInputTokens, u.EstimatedCost,
			u.Route, u.Upstream, u.Attempt, u.Failover, u.FailoverFrom, u.FailoverTo, u.CreatedAt,
		)
	})
}

// InsertToolCallsBatch inserts many tool-call events in a single batch.
func (c *Client) InsertToolCallsBatch(ctx context.Context, events []ToolCallEvent) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return batchInsert(ctx, func(ctx context.Context, query string) (batchAppender, error) {
		return c.conn.PrepareBatch(ctx, query)
	}, `
    INSERT INTO agentplane.tool_call_events (
        trace_id, tool_name, tool_input, tool_output, success, latency_ms, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?)
`, events, func(batch batchAppender, t *ToolCallEvent) error {
		success := uint8(0)
		if t.Success {
			success = 1
		}
		return batch.Append(
			t.TraceID, t.ToolName, t.ToolInput, t.ToolOutput, success, t.LatencyMs, t.CreatedAt,
		)
	})
}

// truncatableTables is the allow-list TruncateTables accepts. Table names are
// interpolated into DDL (they cannot be bound as parameters), so they are
// validated against this fixed set rather than trusted from callers.
var truncatableTables = map[string]bool{
	"traces":           true,
	"prompt_events":    true,
	"response_events":  true,
	"tool_call_events": true,
	"guardrail_events": true,
	"usage_events":     true,
}

// TruncateTables empties the named analytics tables. It is destructive and
// intended for demo/backfill tooling, never for request paths. Unknown table
// names are rejected so a caller cannot inject DDL.
func (c *Client) TruncateTables(ctx context.Context, tables ...string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, table := range tables {
		if !truncatableTables[table] {
			return fmt.Errorf("clickhouse: refusing to truncate unknown table %q", table)
		}
		if err := c.conn.Exec(ctx, "TRUNCATE TABLE IF EXISTS agentplane."+table); err != nil {
			return fmt.Errorf("clickhouse: truncate %s: %w", table, err)
		}
	}
	return nil
}
