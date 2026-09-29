package clickhouse

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Client wraps the ClickHouse connection
type Client struct {
	conn driver.Conn
	mu   sync.RWMutex
}

var (
	instance *Client
	once     sync.Once
)

// New creates or returns the singleton ClickHouse client
func New(host string, port int) (*Client, error) {
	var err error
	once.Do(func() {
		user, password := Credentials()
		conn, connErr := clickhouse.Open(&clickhouse.Options{
			Addr: []string{fmt.Sprintf("%s:%d", host, port)},
			Auth: clickhouse.Auth{
				// Connect to the server's default database; the "agentplane"
				// database is created by the schema migration at init time and
				// all table writes are fully qualified (agentplane.*).
				Username: user,
				Password: password,
			},
			ClientInfo: clickhouse.ClientInfo{
				Products: []struct{ Name, Version string }{
					{Name: "agentplane", Version: "1.0"},
				},
			},
		})
		if connErr != nil {
			err = connErr
			return
		}

		// Verify connection
		if pingErr := conn.Ping(context.Background()); pingErr != nil {
			err = pingErr
			return
		}

		instance = &Client{conn: conn}
		log.Println("ClickHouse connected")
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

// GetInstance returns the singleton client
func GetInstance() *Client {
	return instance
}

// Close closes the ClickHouse connection
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Exec executes a statement without returning rows
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn.Exec(ctx, query, args...)
}

// Query executes a query and returns rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn.Query(ctx, query, args...)
}

// InsertTrace inserts a trace record
func (c *Client) InsertTrace(ctx context.Context, trace *TraceEvent) error {
	query := `
		INSERT INTO agentplane.traces (
			trace_id, request_id, timestamp, user_id, username, organization_id, project_id,
			provider, model, latency_ms, status, cache_hit,
			input_tokens, output_tokens, total_tokens, estimated_cost,
			guardrail_action, route
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	return c.Exec(ctx, query,
		trace.TraceID, trace.RequestID, trace.Timestamp, trace.UserID, trace.Username, trace.OrgID, trace.ProjectID,
		trace.Provider, trace.Model, trace.LatencyMs, trace.Status, boolToUint8(trace.CacheHit),
		trace.InputTokens, trace.OutputTokens, trace.TotalTokens, trace.EstimatedCost,
		trace.GuardrailAction, trace.Route,
	)
}

// InsertPrompt inserts a prompt event
func (c *Client) InsertPrompt(ctx context.Context, p *PromptEvent) error {
	query := `
		INSERT INTO agentplane.prompt_events (
			trace_id, user_id, username, prompt_blob, prompt_text, prompt_hash, prompt_bytes, compressed_size, capture_mode
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	return c.Exec(ctx, query, p.TraceID, p.UserID, p.Username, p.PromptBlob, p.PromptText, p.PromptHash, p.PromptBytes, p.CompressedSize, p.CaptureMode)
}

// InsertResponse inserts a response event
func (c *Client) InsertResponse(ctx context.Context, r *ResponseEvent) error {
	query := `
		INSERT INTO agentplane.response_events (
			trace_id, response_blob, response_hash, response_bytes, compressed_size, capture_mode
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	return c.Exec(ctx, query, r.TraceID, r.ResponseBlob, r.ResponseHash, r.ResponseBytes, r.CompressedSize, r.CaptureMode)
}

// InsertToolCall inserts a tool call event
func (c *Client) InsertToolCall(ctx context.Context, t *ToolCallEvent) error {
	query := `
		INSERT INTO agentplane.tool_call_events (
			trace_id, tool_name, tool_input, tool_output, success, latency_ms
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	success := uint8(0)
	if t.Success {
		success = 1
	}
	return c.Exec(ctx, query, t.TraceID, t.ToolName, t.ToolInput, t.ToolOutput, success, t.LatencyMs)
}

// InsertGuardrail inserts a guardrail event
func (c *Client) InsertGuardrail(ctx context.Context, g *GuardrailEvent) error {
	query := `
		INSERT INTO agentplane.guardrail_events (
			trace_id, guardrail_name, phase, action, reason
		) VALUES (?, ?, ?, ?, ?)
	`
	return c.Exec(ctx, query, g.TraceID, g.GuardrailName, g.Phase, g.Action, g.Reason)
}

// InsertUsage inserts a usage event
func (c *Client) InsertUsage(ctx context.Context, u *UsageEvent) error {
	query := `
		INSERT INTO agentplane.usage_events (
			trace_id, provider, model, input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, estimated_cost
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	return c.Exec(ctx, query, u.TraceID, u.Provider, u.Model, u.InputTokens, u.OutputTokens, u.ReasoningTokens, u.CachedInputTokens, u.EstimatedCost)
}

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
