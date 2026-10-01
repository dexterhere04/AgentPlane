package observability

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const analyticsQueryTimeout = 5 * time.Second

type clickhouseAnalyticsStore struct {
	client AnalyticsQueryClient
}

type AnalyticsQueryClient interface {
	Query(context.Context, string, ...any) (driver.Rows, error)
	Ping(context.Context) error
}

func NewClickHouseAnalyticsStore(client AnalyticsQueryClient) AnalyticsStore {
	return &clickhouseAnalyticsStore{client: client}
}

func analyticsQuery(ctx context.Context, client AnalyticsQueryClient, query string, args ...any) (driver.Rows, error) {
	queryCtx, cancel := context.WithTimeout(ctx, analyticsQueryTimeout)
	rows, err := client.Query(queryCtx, query, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	return &timeoutRows{Rows: rows, cancel: cancel}, nil
}

type timeoutRows struct {
	driver.Rows
	cancel context.CancelFunc
	once   sync.Once
}

func (r *timeoutRows) Close() error {
	err := r.Rows.Close()
	r.once.Do(r.cancel)
	return err
}

func analyticsFilterSQL(filter AnalyticsFilter, alias string) (string, []any) {
	column := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	clauses := []string{column("timestamp") + " >= ?", column("timestamp") + " < ?"}
	args := []any{filter.From, filter.To}
	add := func(name, value string) {
		if value == "" {
			return
		}
		if name == "status" {
			if value == "error" {
				clauses = append(clauses, column("is_error")+" = 1")
			} else {
				clauses = append(clauses, column("is_error")+" = 0")
			}
			return
		}
		clauses = append(clauses, column(name)+" = ?")
		args = append(args, value)
	}
	add("status", filter.Status)
	add("provider", filter.Provider)
	add("model", filter.Model)
	add("route", filter.Route)
	if filter.GuardrailAction != "" {
		clauses = append(clauses, column("effective_guardrail_action")+" = ?")
		args = append(args, filter.GuardrailAction)
	}
	add("user_id", filter.UserID)
	add("organization_id", filter.OrganizationID)
	add("project_id", filter.ProjectID)
	return strings.Join(clauses, " AND "), args
}

func filteredTraceSource(filter AnalyticsFilter) (string, []any) {
	where, filterArgs := analyticsFilterSQL(filter, "t")
	source := `(SELECT traces.*,
if(traces.status = 'success' AND ifNull(guardrail_summary.has_block, 0) = 1, 'guardrail_blocked', traces.status) AS effective_status,
if(traces.status != 'success' OR ifNull(guardrail_summary.has_block, 0) = 1, 1, 0) AS is_error,
if(traces.guardrail_action != '', traces.guardrail_action,
   if(ifNull(guardrail_summary.has_block, 0) = 1, 'block', ifNull(guardrail_summary.last_action, ''))) AS effective_guardrail_action
FROM agentplane.traces AS traces
LEFT JOIN (
  SELECT trace_id, max(action = 'block') AS has_block, argMax(action, created_at) AS last_action
  FROM agentplane.guardrail_events
  WHERE created_at >= ? AND created_at < ?
  GROUP BY trace_id
) AS guardrail_summary ON traces.trace_id = guardrail_summary.trace_id) AS t`
	args := make([]any, 0, len(filterArgs)+2)
	args = append(args, filter.From, filter.To)
	args = append(args, filterArgs...)
	return source + " WHERE " + where, args
}

func (s *clickhouseAnalyticsStore) Health(ctx context.Context, filter AnalyticsFilter) (ObservabilityHealth, error) {
	health := ObservabilityHealth{Status: "unavailable", DataState: "unavailable", CheckedAt: time.Now().UTC()}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := s.client.Ping(pingCtx)
	cancel()
	if err != nil {
		return health, nil
	}
	health.ClickHouseReachable = true
	source, args := filteredTraceSource(filter)
	rows, err := analyticsQuery(ctx, s.client, "SELECT count(), max(timestamp) FROM "+source, args...)
	if err != nil {
		return health, nil
	}
	defer rows.Close()
	var count uint64
	var latest time.Time
	if rows.Next() {
		if err := rows.Scan(&count, &latest); err != nil {
			return health, nil
		}
	}
	if err := rows.Err(); err != nil {
		return health, nil
	}
	health.Status = "healthy"
	if count == 0 {
		health.DataState = "empty"
		return health, nil
	}
	health.LatestTelemetryAt = &latest
	health.DataState = "fresh"
	if time.Since(latest) > 15*time.Minute {
		health.DataState = "stale"
	}
	return health, nil
}

func (s *clickhouseAnalyticsStore) Overview(ctx context.Context, filter AnalyticsFilter) (AnalyticsOverview, error) {
	source, args := filteredTraceSource(filter)
	query := `SELECT count(), countIf(is_error = 0), countIf(is_error = 1),
quantileTDigest(0.50)(latency_ms), quantileTDigest(0.95)(latency_ms), quantileTDigest(0.99)(latency_ms),
sum(input_tokens), sum(output_tokens), sum(total_tokens), sum(estimated_cost), max(timestamp)
FROM ` + source
	rows, err := analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return AnalyticsOverview{}, err
	}
	defer rows.Close()
	var result AnalyticsOverview
	var latest time.Time
	if rows.Next() {
		if err := rows.Scan(&result.Requests, &result.Successes, &result.Errors,
			&result.Latency.P50, &result.Latency.P95, &result.Latency.P99,
			&result.Tokens.Input, &result.Tokens.Output, &result.Tokens.Total,
			&result.EstimatedCost, &latest); err != nil {
			return AnalyticsOverview{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return AnalyticsOverview{}, err
	}
	if result.Requests > 0 {
		result.ErrorRate = float64(result.Errors) / float64(result.Requests)
		result.LatestTelemetryAt = &latest
	}
	return result, nil
}

func bucketExpression(value AnalyticsRange) (string, string) {
	switch value {
	case Range15Minutes, Range1Hour:
		return "toStartOfInterval(timestamp, INTERVAL 1 MINUTE, 'UTC')", "1m"
	case Range6Hours:
		return "toStartOfInterval(timestamp, INTERVAL 5 MINUTE, 'UTC')", "5m"
	case Range24Hours:
		return "toStartOfInterval(timestamp, INTERVAL 15 MINUTE, 'UTC')", "15m"
	default:
		return "toStartOfInterval(timestamp, INTERVAL 1 HOUR, 'UTC')", "1h"
	}
}

func bucketDuration(value AnalyticsRange) time.Duration {
	switch value {
	case Range15Minutes, Range1Hour:
		return time.Minute
	case Range6Hours:
		return 5 * time.Minute
	case Range24Hours:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func (s *clickhouseAnalyticsStore) Series(ctx context.Context, filter AnalyticsFilter) (AnalyticsSeries, error) {
	bucket, label := bucketExpression(filter.Range)
	source, args := filteredTraceSource(filter)
	query := fmt.Sprintf(`SELECT %s, count(), countIf(is_error = 0), countIf(is_error = 1),
quantileTDigest(0.50)(latency_ms), quantileTDigest(0.95)(latency_ms), quantileTDigest(0.99)(latency_ms),
sum(input_tokens), sum(output_tokens), sum(total_tokens), sum(estimated_cost)
FROM %s GROUP BY %s ORDER BY %s LIMIT 10000`, bucket, source, bucket, bucket)
	rows, err := analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return AnalyticsSeries{}, err
	}
	defer rows.Close()
	result := AnalyticsSeries{BucketSize: label, Buckets: make([]AnalyticsBucket, 0)}
	for rows.Next() {
		var item AnalyticsBucket
		if err := rows.Scan(&item.Timestamp, &item.Requests, &item.Successes, &item.Errors,
			&item.Latency.P50, &item.Latency.P95, &item.Latency.P99,
			&item.Tokens.Input, &item.Tokens.Output, &item.Tokens.Total, &item.EstimatedCost); err != nil {
			return AnalyticsSeries{}, err
		}
		if item.Requests > 0 {
			item.ErrorRate = float64(item.Errors) / float64(item.Requests)
		}
		result.Buckets = append(result.Buckets, item)
	}
	if err := rows.Err(); err != nil {
		return AnalyticsSeries{}, err
	}
	result.Buckets = fillSeriesBuckets(result.Buckets, filter)
	return result, nil
}

func fillSeriesBuckets(existing []AnalyticsBucket, filter AnalyticsFilter) []AnalyticsBucket {
	interval := bucketDuration(filter.Range)
	from := filter.From.UTC().Truncate(interval)
	to := filter.To.UTC()
	byTime := make(map[int64]AnalyticsBucket, len(existing))
	for _, bucket := range existing {
		bucket.Timestamp = bucket.Timestamp.UTC().Truncate(interval)
		byTime[bucket.Timestamp.Unix()] = bucket
	}
	filled := make([]AnalyticsBucket, 0, int(to.Sub(from)/interval)+1)
	for timestamp := from; timestamp.Before(to); timestamp = timestamp.Add(interval) {
		bucket, ok := byTime[timestamp.Unix()]
		if !ok {
			bucket.Timestamp = timestamp
		}
		filled = append(filled, bucket)
	}
	return filled
}

func (s *clickhouseAnalyticsStore) Breakdowns(ctx context.Context, filter AnalyticsFilter) (map[string][]AnalyticsBreakdown, error) {
	result := make(map[string][]AnalyticsBreakdown, 3)
	source, args := filteredTraceSource(filter)
	for key, dimension := range map[string]string{"provider": "provider", "model": "model", "route": "route"} {
		query := fmt.Sprintf(`SELECT %s, count(), countIf(is_error = 0), countIf(is_error = 1),
sum(input_tokens), sum(output_tokens), sum(total_tokens), sum(estimated_cost),
quantileTDigest(0.50)(latency_ms), quantileTDigest(0.95)(latency_ms), quantileTDigest(0.99)(latency_ms)
FROM %s AND %s != '' GROUP BY %s ORDER BY count() DESC LIMIT 20`, dimension, source, dimension, dimension)
		rows, err := analyticsQuery(ctx, s.client, query, args...)
		if err != nil {
			return nil, err
		}
		items := make([]AnalyticsBreakdown, 0)
		for rows.Next() {
			var item AnalyticsBreakdown
			if err := rows.Scan(&item.Key, &item.Requests, &item.Successes, &item.Errors,
				&item.Tokens.Input, &item.Tokens.Output, &item.Tokens.Total, &item.EstimatedCost,
				&item.Latency.P50, &item.Latency.P95, &item.Latency.P99); err != nil {
				rows.Close()
				return nil, err
			}
			if item.Requests > 0 {
				item.ErrorRate = float64(item.Errors) / float64(item.Requests)
			}
			items = append(items, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		result[key] = items
	}
	return result, nil
}

func (s *clickhouseAnalyticsStore) Failures(ctx context.Context, filter AnalyticsFilter, limit int) ([]TraceSummary, error) {
	source, args := filteredTraceSource(filter)
	query := `SELECT timestamp, trace_id, request_id, user_id, organization_id, project_id, provider, model, route,
effective_status, latency_ms, input_tokens, output_tokens, total_tokens, estimated_cost, effective_guardrail_action
FROM ` + source + ` AND is_error = 1 ORDER BY timestamp DESC LIMIT ?`
	args = append(args, boundedLimit(limit))
	return scanTraceSummaries(ctx, s.client, query, args...)
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return DefaultTracePageSize
	}
	if limit > MaxTracePageSize {
		return MaxTracePageSize
	}
	return limit
}

func scanTraceSummaries(ctx context.Context, client AnalyticsQueryClient, query string, args ...any) ([]TraceSummary, error) {
	rows, err := analyticsQuery(ctx, client, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TraceSummary, 0)
	for rows.Next() {
		var item TraceSummary
		if err := rows.Scan(&item.Timestamp, &item.TraceID, &item.RequestID, &item.UserID,
			&item.OrganizationID, &item.ProjectID, &item.Provider, &item.Model, &item.Route,
			&item.Status, &item.LatencyMS, &item.InputTokens, &item.OutputTokens, &item.TotalTokens,
			&item.EstimatedCost, &item.GuardrailAction); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *clickhouseAnalyticsStore) Traces(ctx context.Context, request TracePageRequest) (TracePage, error) {
	source, args := filteredTraceSource(request.Filter)
	if request.Cursor != nil {
		source += " AND (timestamp < ? OR (timestamp = ? AND trace_id < ?))"
		args = append(args, request.Cursor.Timestamp, request.Cursor.Timestamp, request.Cursor.TraceID)
	}
	limit := boundedLimit(request.Limit)
	query := `SELECT timestamp, trace_id, request_id, user_id, organization_id, project_id, provider, model, route,
effective_status, latency_ms, input_tokens, output_tokens, total_tokens, estimated_cost, effective_guardrail_action
FROM ` + source + ` ORDER BY timestamp DESC, trace_id DESC LIMIT ?`
	args = append(args, limit+1)
	items, err := scanTraceSummaries(ctx, s.client, query, args...)
	if err != nil {
		return TracePage{}, err
	}
	page := TracePage{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		cursor, err := json.Marshal(TraceCursor{Timestamp: last.Timestamp, TraceID: last.TraceID})
		if err != nil {
			return TracePage{}, err
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	return page, nil
}

func (s *clickhouseAnalyticsStore) Guardrails(ctx context.Context, filter AnalyticsFilter, limit int) (GuardrailAnalytics, error) {
	source, args := filteredTraceSource(filter)
	result := GuardrailAnalytics{Breakdown: make([]GuardrailBreakdown, 0), Recent: make([]GuardrailMetadata, 0)}
	eventAction := ""
	if filter.GuardrailAction != "" {
		eventAction = " AND g.action = ?"
	}
	query := `SELECT g.guardrail_name, g.phase, g.action, count()
FROM agentplane.guardrail_events AS g INNER JOIN (SELECT * FROM ` + source + `) AS t ON g.trace_id = t.trace_id
WHERE g.created_at >= ? AND g.created_at < ?` + eventAction + `
GROUP BY g.guardrail_name, g.phase, g.action ORDER BY count() DESC LIMIT 100`
	args = append(args, filter.From, filter.To)
	if filter.GuardrailAction != "" {
		args = append(args, filter.GuardrailAction)
	}
	rows, err := analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return GuardrailAnalytics{}, err
	}
	for rows.Next() {
		var item GuardrailBreakdown
		if err := rows.Scan(&item.Name, &item.Phase, &item.Action, &item.Count); err != nil {
			rows.Close()
			return GuardrailAnalytics{}, err
		}
		result.Breakdown = append(result.Breakdown, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return GuardrailAnalytics{}, err
	}
	recentLimit := boundedLimit(limit)
	query = `SELECT g.guardrail_name, g.phase, g.action, g.created_at
FROM agentplane.guardrail_events AS g INNER JOIN (SELECT * FROM ` + source + `) AS t ON g.trace_id = t.trace_id
WHERE g.created_at >= ? AND g.created_at < ?` + eventAction + `
ORDER BY g.created_at DESC LIMIT ?`
	args = append(args, filter.From, filter.To)
	if filter.GuardrailAction != "" {
		args = append(args, filter.GuardrailAction)
	}
	args = append(args, recentLimit)
	rows, err = analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return GuardrailAnalytics{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item GuardrailMetadata
		if err := rows.Scan(&item.Name, &item.Phase, &item.Action, &item.Timestamp); err != nil {
			return GuardrailAnalytics{}, err
		}
		result.Recent = append(result.Recent, item)
	}
	return result, rows.Err()
}

func (s *clickhouseAnalyticsStore) Trace(ctx context.Context, filter AnalyticsFilter, id string) (TraceDetail, error) {
	source, args := filteredTraceSource(filter)
	source += " AND (trace_id = ? OR request_id = ?)"
	args = append(args, id, id)
	traces, err := scanTraceSummaries(ctx, s.client, `SELECT timestamp, trace_id, request_id, user_id, organization_id, project_id,
provider, model, route, effective_status, latency_ms, input_tokens, output_tokens, total_tokens, estimated_cost, effective_guardrail_action
FROM `+source+` ORDER BY timestamp DESC LIMIT 1`, args...)
	if err != nil {
		return TraceDetail{}, err
	}
	if len(traces) == 0 {
		return TraceDetail{}, sql.ErrNoRows
	}
	result := TraceDetail{TraceSummary: traces[0], Guardrails: make([]GuardrailMetadata, 0), Tools: make([]ToolMetadata, 0)}
	traceID := result.TraceID
	if result.Prompt, err = s.captureMetadata(ctx, "prompt_events", "prompt_hash", "prompt_bytes", traceID, filter); err != nil {
		return TraceDetail{}, err
	}
	if result.Response, err = s.captureMetadata(ctx, "response_events", "response_hash", "response_bytes", traceID, filter); err != nil {
		return TraceDetail{}, err
	}
	usageRows, err := analyticsQuery(ctx, s.client, `SELECT provider, model, reasoning_tokens, cached_input_tokens, estimated_cost, created_at
FROM agentplane.usage_events WHERE trace_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at DESC LIMIT 1`, traceID, filter.From, filter.To)
	if err != nil {
		return TraceDetail{}, err
	}
	if usageRows.Next() {
		if err := usageRows.Scan(&result.Usage.Provider, &result.Usage.Model, &result.Usage.ReasoningTokens,
			&result.Usage.CachedInputTokens, &result.Usage.EstimatedCost, &result.Usage.Timestamp); err != nil {
			usageRows.Close()
			return TraceDetail{}, err
		}
		result.Usage.Available = true
	}
	err = usageRows.Err()
	usageRows.Close()
	if err != nil {
		return TraceDetail{}, err
	}
	guardRows, err := analyticsQuery(ctx, s.client, `SELECT guardrail_name, phase, action, created_at FROM agentplane.guardrail_events
WHERE trace_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at ASC LIMIT 100`, traceID, filter.From, filter.To)
	if err != nil {
		return TraceDetail{}, err
	}
	for guardRows.Next() {
		var item GuardrailMetadata
		if err := guardRows.Scan(&item.Name, &item.Phase, &item.Action, &item.Timestamp); err != nil {
			guardRows.Close()
			return TraceDetail{}, err
		}
		result.Guardrails = append(result.Guardrails, item)
	}
	err = guardRows.Err()
	guardRows.Close()
	if err != nil {
		return TraceDetail{}, err
	}
	toolRows, err := analyticsQuery(ctx, s.client, `SELECT tool_name, success, latency_ms, created_at FROM agentplane.tool_call_events
WHERE trace_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at ASC LIMIT 100`, traceID, filter.From, filter.To)
	if err != nil {
		return TraceDetail{}, err
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var item ToolMetadata
		var success uint8
		if err := toolRows.Scan(&item.Name, &success, &item.LatencyMS, &item.Timestamp); err != nil {
			return TraceDetail{}, err
		}
		item.Success = success == 1
		result.Tools = append(result.Tools, item)
	}
	return result, toolRows.Err()
}

func (s *clickhouseAnalyticsStore) captureMetadata(ctx context.Context, table, hashColumn, bytesColumn, traceID string, filter AnalyticsFilter) (CaptureMetadata, error) {
	metadata := CaptureMetadata{}
	query := fmt.Sprintf("SELECT capture_mode, %s, %s, compressed_size FROM agentplane.%s WHERE trace_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at DESC LIMIT 1", hashColumn, bytesColumn, table)
	rows, err := analyticsQuery(ctx, s.client, query, traceID, filter.From, filter.To)
	if err != nil {
		return metadata, err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&metadata.Mode, &metadata.Hash, &metadata.Bytes, &metadata.CompressedSize); err != nil {
			return CaptureMetadata{}, err
		}
		metadata.Available = true
	}
	return metadata, rows.Err()
}

// UserUsage aggregates per-user spend and token usage over the given window.
func (s *clickhouseAnalyticsStore) UserUsage(ctx context.Context, filter AnalyticsFilter, limit int) ([]UserUsageSummary, error) {
	source, args := filteredTraceSource(filter)
	query := `SELECT user_id, username, count(), sum(input_tokens), sum(output_tokens), sum(total_tokens),
sum(estimated_cost), avg(latency_ms), max(timestamp)
FROM ` + source + ` AND user_id != ''
GROUP BY user_id, username
ORDER BY sum(estimated_cost) DESC LIMIT ?`
	args = append(args, boundedLimit(limit))
	rows, err := analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]UserUsageSummary, 0)
	for rows.Next() {
		var item UserUsageSummary
		if err := rows.Scan(&item.UserID, &item.Username, &item.RequestCount,
			&item.InputTokens, &item.OutputTokens, &item.TotalTokens,
			&item.TotalCost, &item.AvgLatencyMS, &item.LastSeen); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// PromptSearch returns prompts joined with their trace metadata, optionally
// filtered by username (user) and free-text (q), over the given window.
func (s *clickhouseAnalyticsStore) PromptSearch(ctx context.Context, filter AnalyticsFilter, user, q string, limit int) ([]PromptSearchResult, error) {
	query := `SELECT p.trace_id, p.prompt_text, p.created_at,
coalesce(t.username, ''), coalesce(t.model, ''), toUInt64(coalesce(t.total_tokens, 0)), coalesce(t.estimated_cost, 0.0)
FROM agentplane.prompt_events p
LEFT JOIN agentplane.traces t ON t.trace_id = p.trace_id
WHERE p.created_at >= ? AND p.created_at < ?
  AND p.prompt_text != ''
  AND (? = '' OR t.username = ?)
  AND (? = '' OR positionCaseInsensitive(p.prompt_text, ?) > 0)
ORDER BY p.created_at DESC LIMIT ?`
	args := []any{filter.From, filter.To, user, user, q, q, boundedLimit(limit)}
	rows, err := analyticsQuery(ctx, s.client, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PromptSearchResult, 0)
	for rows.Next() {
		var item PromptSearchResult
		if err := rows.Scan(&item.TraceID, &item.PromptText, &item.CreatedAt,
			&item.Username, &item.Model, &item.TotalTokens, &item.EstimatedCost); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ AnalyticsStore = (*clickhouseAnalyticsStore)(nil)
