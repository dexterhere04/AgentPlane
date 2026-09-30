package observability

import (
	"context"
	"fmt"
	"time"
)

type AnalyticsRange string

const (
	Range15Minutes AnalyticsRange = "15m"
	Range1Hour     AnalyticsRange = "1h"
	Range6Hours    AnalyticsRange = "6h"
	Range24Hours   AnalyticsRange = "24h"
	Range7Days     AnalyticsRange = "7d"

	MaxTracePageSize     = 100
	DefaultTracePageSize = 50
)

func ParseAnalyticsRange(value string, now time.Time) (time.Time, time.Time, error) {
	var duration time.Duration
	switch AnalyticsRange(value) {
	case Range15Minutes:
		duration = 15 * time.Minute
	case Range1Hour:
		duration = time.Hour
	case Range6Hours:
		duration = 6 * time.Hour
	case Range24Hours:
		duration = 24 * time.Hour
	case Range7Days:
		duration = 7 * 24 * time.Hour
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported analytics range")
	}
	to := now.UTC()
	return to.Add(-duration), to, nil
}

type AnalyticsFilter struct {
	Range           AnalyticsRange
	From            time.Time
	To              time.Time
	Status          string
	Provider        string
	Model           string
	Route           string
	GuardrailAction string
	UserID          string
	OrganizationID  string
	ProjectID       string
}

type TraceCursor struct {
	Timestamp time.Time
	TraceID   string
}

type TracePageRequest struct {
	Filter AnalyticsFilter
	Limit  int
	Cursor *TraceCursor
}

type LatencyPercentiles struct {
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
}

type TokenTotals struct {
	Input  uint64 `json:"input"`
	Output uint64 `json:"output"`
	Total  uint64 `json:"total"`
}

type AnalyticsOverview struct {
	Requests          uint64             `json:"requests"`
	Successes         uint64             `json:"successes"`
	Errors            uint64             `json:"errors"`
	ErrorRate         float64            `json:"error_rate"`
	Latency           LatencyPercentiles `json:"latency_ms"`
	Tokens            TokenTotals        `json:"tokens"`
	EstimatedCost     float64            `json:"estimated_cost"`
	LatestTelemetryAt *time.Time         `json:"latest_telemetry_at,omitempty"`
}

type AnalyticsBucket struct {
	Timestamp     time.Time          `json:"timestamp"`
	Requests      uint64             `json:"requests"`
	Successes     uint64             `json:"successes"`
	Errors        uint64             `json:"errors"`
	ErrorRate     float64            `json:"error_rate"`
	Latency       LatencyPercentiles `json:"latency_ms"`
	Tokens        TokenTotals        `json:"tokens"`
	EstimatedCost float64            `json:"estimated_cost"`
}

type AnalyticsSeries struct {
	BucketSize string            `json:"bucket_size"`
	Buckets    []AnalyticsBucket `json:"buckets"`
}

type AnalyticsBreakdown struct {
	Key           string             `json:"key"`
	Requests      uint64             `json:"requests"`
	Successes     uint64             `json:"successes"`
	Errors        uint64             `json:"errors"`
	ErrorRate     float64            `json:"error_rate"`
	Tokens        TokenTotals        `json:"tokens"`
	EstimatedCost float64            `json:"estimated_cost"`
	Latency       LatencyPercentiles `json:"latency_ms"`
}

type TraceSummary struct {
	Timestamp       time.Time `json:"timestamp"`
	TraceID         string    `json:"trace_id"`
	RequestID       string    `json:"request_id"`
	UserID          string    `json:"user_id,omitempty"`
	OrganizationID  string    `json:"organization_id,omitempty"`
	ProjectID       string    `json:"project_id,omitempty"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	Route           string    `json:"route"`
	Status          string    `json:"status"`
	LatencyMS       uint32    `json:"latency_ms"`
	InputTokens     uint32    `json:"input_tokens"`
	OutputTokens    uint32    `json:"output_tokens"`
	TotalTokens     uint32    `json:"total_tokens"`
	EstimatedCost   float64   `json:"estimated_cost"`
	GuardrailAction string    `json:"guardrail_action,omitempty"`
}

type TracePage struct {
	Items      []TraceSummary `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type CaptureMetadata struct {
	Available      bool   `json:"available"`
	Mode           string `json:"mode,omitempty"`
	Hash           string `json:"hash,omitempty"`
	Bytes          uint32 `json:"bytes,omitempty"`
	CompressedSize uint32 `json:"compressed_size,omitempty"`
}

type GuardrailMetadata struct {
	Name      string    `json:"name"`
	Phase     string    `json:"phase"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

type ToolMetadata struct {
	Name      string    `json:"name"`
	Success   bool      `json:"success"`
	LatencyMS uint32    `json:"latency_ms"`
	Timestamp time.Time `json:"timestamp"`
}

type UsageMetadata struct {
	Available         bool      `json:"available"`
	Provider          string    `json:"provider,omitempty"`
	Model             string    `json:"model,omitempty"`
	ReasoningTokens   uint32    `json:"reasoning_tokens,omitempty"`
	CachedInputTokens uint32    `json:"cached_input_tokens,omitempty"`
	EstimatedCost     float64   `json:"estimated_cost,omitempty"`
	Timestamp         time.Time `json:"timestamp,omitempty"`
}

type TraceDetail struct {
	TraceSummary
	Prompt     CaptureMetadata     `json:"prompt"`
	Response   CaptureMetadata     `json:"response"`
	Usage      UsageMetadata       `json:"usage"`
	Guardrails []GuardrailMetadata `json:"guardrails"`
	Tools      []ToolMetadata      `json:"tools"`
}

type GuardrailBreakdown struct {
	Name   string `json:"name"`
	Phase  string `json:"phase"`
	Action string `json:"action"`
	Count  uint64 `json:"count"`
}

type GuardrailAnalytics struct {
	Breakdown []GuardrailBreakdown `json:"breakdown"`
	Recent    []GuardrailMetadata  `json:"recent"`
}

type ObservabilityHealth struct {
	Status              string     `json:"status"`
	ClickHouseReachable bool       `json:"clickhouse_reachable"`
	DataState           string     `json:"data_state"`
	LatestTelemetryAt   *time.Time `json:"latest_telemetry_at,omitempty"`
	CheckedAt           time.Time  `json:"checked_at"`
}

// AnalyticsStore is read-only by design and is backed by the configured telemetry store.
type AnalyticsStore interface {
	Health(context.Context, AnalyticsFilter) (ObservabilityHealth, error)
	Overview(context.Context, AnalyticsFilter) (AnalyticsOverview, error)
	Series(context.Context, AnalyticsFilter) (AnalyticsSeries, error)
	Breakdowns(context.Context, AnalyticsFilter) (map[string][]AnalyticsBreakdown, error)
	Failures(context.Context, AnalyticsFilter, int) ([]TraceSummary, error)
	Guardrails(context.Context, AnalyticsFilter, int) (GuardrailAnalytics, error)
	Traces(context.Context, TracePageRequest) (TracePage, error)
	Trace(context.Context, AnalyticsFilter, string) (TraceDetail, error)
}
