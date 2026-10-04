package observability

import "fmt"

// AnalyticsQueries provides reusable SQL query building for common observability questions.
// These are helper functions for building ClickHouse queries. The actual execution
// is delegated to the ClickHouse client/store.
type AnalyticsQueries struct{}

// TraceCountQuery returns SQL to count traces in a time window.
// Parameters:
// - hoursBack: how many hours to look back (e.g., 24 for last day)
// Returns: SQL query string
func (a *AnalyticsQueries) TraceCountQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT count() AS trace_count FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
`, hoursBack)
}

// AverageLatencyQuery returns SQL for average request latency.
func (a *AnalyticsQueries) AverageLatencyQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  avg(latency_ms) AS avg_latency_ms,
  min(latency_ms) AS min_latency_ms,
  max(latency_ms) AS max_latency_ms
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
`, hoursBack)
}

// FailedRequestsQuery returns SQL for failed request count and distribution.
func (a *AnalyticsQueries) FailedRequestsQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  status,
  count() AS failed_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR AND status != 'success'
GROUP BY status
ORDER BY failed_count DESC
`, hoursBack)
}

// TokenUsageQuery returns SQL for total token usage aggregation.
func (a *AnalyticsQueries) TokenUsageQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  sum(input_tokens) AS total_input_tokens,
  sum(output_tokens) AS total_output_tokens,
  sum(total_tokens) AS total_tokens_sum,
  count() AS request_count,
  avg(total_tokens) AS avg_tokens_per_request
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
`, hoursBack)
}

// EstimatedCostQuery returns SQL for cost aggregation.
func (a *AnalyticsQueries) EstimatedCostQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT 
  sum(estimated_cost) AS total_cost,
  avg(estimated_cost) AS avg_cost_per_request,
  max(estimated_cost) AS max_cost_per_request,
  count() AS request_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
`, hoursBack)
}

// TopModelsQuery returns SQL for most-used models.
func (a *AnalyticsQueries) TopModelsQuery(hoursBack, limit int) string {
	return fmt.Sprintf(`
SELECT 
  model,
  count() AS request_count,
  sum(total_tokens) AS total_tokens,
  sum(estimated_cost) AS total_cost,
  avg(latency_ms) AS avg_latency_ms
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR AND model != ''
GROUP BY model
ORDER BY request_count DESC
LIMIT %d
`, hoursBack, limit)
}

// ProviderUsageQuery returns SQL for provider distribution.
func (a *AnalyticsQueries) ProviderUsageQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT 
  provider,
  count() AS request_count,
  sum(total_tokens) AS total_tokens,
  sum(estimated_cost) AS total_cost,
  avg(latency_ms) AS avg_latency_ms
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
GROUP BY provider
ORDER BY request_count DESC
`, hoursBack)
}

// GuardrailEventsQuery returns SQL for guardrail audit trail.
func (a *AnalyticsQueries) GuardrailEventsQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT 
  guardrail_name,
  phase,
  action,
  count() AS event_count
FROM agentplane.guardrail_events
WHERE created_at >= now() - INTERVAL %d HOUR
GROUP BY guardrail_name, phase, action
ORDER BY event_count DESC
`, hoursBack)
}

// ToolCallStatsQuery returns SQL for tool usage statistics.
func (a *AnalyticsQueries) ToolCallStatsQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT 
  tool_name,
  count() AS call_count,
  sum(IF(success = 1, 1, 0)) AS success_count,
  avg(latency_ms) AS avg_latency_ms
FROM agentplane.tool_call_events
WHERE created_at >= now() - INTERVAL %d HOUR
GROUP BY tool_name
ORDER BY call_count DESC
`, hoursBack)
}

// ErrorRateQuery returns SQL for error rate by status code.
func (a *AnalyticsQueries) ErrorRateQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT 
  status,
  count() AS count,
  count() * 100.0 / (SELECT count() FROM agentplane.traces WHERE timestamp >= now() - INTERVAL %d HOUR) AS percentage
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
GROUP BY status
ORDER BY count DESC
`, hoursBack, hoursBack)
}

// NewAnalyticsQueries returns a new AnalyticsQueries helper.
func NewAnalyticsQueries() *AnalyticsQueries {
	return &AnalyticsQueries{}
}

// UserUsageQuery aggregates spend and token usage per user.
//
// Rows are keyed by (user_id, username) so the result can be attributed to a
// human identity without a Postgres lookup. The `?`-free body uses only the
// Sprintf'd hours window; callers execute it directly.
func (a *AnalyticsQueries) UserUsageQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  user_id,
  username,
  count() AS request_count,
  sum(input_tokens) AS input_tokens,
  sum(output_tokens) AS output_tokens,
  sum(total_tokens) AS total_tokens,
  sum(estimated_cost) AS total_cost,
  avg(latency_ms) AS avg_latency_ms,
  toString(max(timestamp)) AS last_seen
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND user_id != ''
GROUP BY user_id, username
ORDER BY total_cost DESC
`, hoursBack)
}

// PromptSearchQuery returns prompts joined with their trace metadata so the
// user can search prompt text and attribute results to a user.
//
// user and q are bound as parameters by the caller:
//   - args[0], args[1] filter by username (empty = no filter)
//   - args[2], args[3] search prompt_text case-insensitively (empty = no filter)
func (a *AnalyticsQueries) PromptSearchQuery(hoursBack, limit int) string {
	return fmt.Sprintf(`
SELECT
  p.trace_id,
  p.prompt_text,
  toString(p.created_at) AS created_at,
  coalesce(t.username, '') AS username,
  coalesce(t.model, '') AS model,
  toUInt64(coalesce(t.total_tokens, 0)) AS total_tokens,
  coalesce(t.estimated_cost, 0.0) AS estimated_cost
FROM agentplane.prompt_events p
LEFT JOIN agentplane.traces t ON t.trace_id = p.trace_id
WHERE p.created_at >= now() - INTERVAL %d HOUR
  AND p.prompt_text != ''
  AND (? = '' OR t.username = ?)
  AND (? = '' OR positionCaseInsensitive(p.prompt_text, ?) > 0)
ORDER BY p.created_at DESC
LIMIT %d
`, hoursBack, limit)
}

// ProviderTrafficQuery returns request volume and traffic percentage per provider.
func (a *AnalyticsQueries) ProviderTrafficQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  provider,
  count() AS request_count,
  count() * 100.0 /
    nullIf((SELECT count() FROM agentplane.traces
            WHERE timestamp >= now() - INTERVAL %d HOUR), 0) AS traffic_percentage
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND provider != ''
GROUP BY provider
ORDER BY request_count DESC
`, hoursBack, hoursBack)
}

// RouteTrafficQuery returns request volume and traffic percentage per route.
func (a *AnalyticsQueries) RouteTrafficQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  route,
  count() AS request_count,
  count() * 100.0 /
    nullIf((SELECT count() FROM agentplane.traces
            WHERE timestamp >= now() - INTERVAL %d HOUR), 0) AS traffic_percentage
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND route != ''
GROUP BY route
ORDER BY request_count DESC
`, hoursBack, hoursBack)
}

func (a *AnalyticsQueries) LatencyPercentilesQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  provider,
  quantile(0.50)(latency_ms) AS p50_latency_ms,
  quantile(0.95)(latency_ms) AS p95_latency_ms,
  quantile(0.99)(latency_ms) AS p99_latency_ms
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND provider != ''
GROUP BY provider
ORDER BY provider
`, hoursBack)
}

func (a *AnalyticsQueries) ErrorRateSummaryQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  provider,
  count() AS total_requests,
  countIf(status != 'success') AS error_count,
  countIf(status != 'success') * 100.0 /
    nullIf(count(), 0) AS error_rate_percentage
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND provider != ''
GROUP BY provider
ORDER BY provider
`, hoursBack)
}

func (a *AnalyticsQueries) FailoverTransitionsQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  failover_from,
  failover_to,
  count() AS failover_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND failover = 1
  AND failover_from != ''
  AND failover_to != ''
GROUP BY failover_from, failover_to
ORDER BY failover_count DESC
`, hoursBack)
}

// FailoverCountQuery returns the number of requests that experienced failover.
func (a *AnalyticsQueries) FailoverCountQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  countIf(failover = 1) AS failover_count,
  count() AS total_requests,
  countIf(failover = 1) * 100.0 /
    nullIf(count(), 0) AS failover_percentage
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
`, hoursBack)
}

// TokensByProviderQuery returns token usage grouped by provider.
func (a *AnalyticsQueries) TokensByProviderQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  provider,
  sum(input_tokens) AS input_tokens,
  sum(output_tokens) AS output_tokens,
  sum(total_tokens) AS total_tokens,
  count() AS request_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND provider != ''
GROUP BY provider
ORDER BY total_tokens DESC
`, hoursBack)
}

// TokensByRouteQuery returns token usage grouped by route.
func (a *AnalyticsQueries) TokensByRouteQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  route,
  sum(input_tokens) AS input_tokens,
  sum(output_tokens) AS output_tokens,
  sum(total_tokens) AS total_tokens,
  count() AS request_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND route != ''
GROUP BY route
ORDER BY total_tokens DESC
`, hoursBack)
}

// CostByProviderQuery returns estimated cost grouped by provider.
func (a *AnalyticsQueries) CostByProviderQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  provider,
  sum(estimated_cost) AS total_cost,
  avg(estimated_cost) AS avg_cost_per_request,
  count() AS request_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND provider != ''
GROUP BY provider
ORDER BY total_cost DESC
`, hoursBack)
}

// CostByRouteQuery returns estimated cost grouped by route.
func (a *AnalyticsQueries) CostByRouteQuery(hoursBack int) string {
	return fmt.Sprintf(`
SELECT
  route,
  sum(estimated_cost) AS total_cost,
  avg(estimated_cost) AS avg_cost_per_request,
  count() AS request_count
FROM agentplane.traces
WHERE timestamp >= now() - INTERVAL %d HOUR
  AND route != ''
GROUP BY route
ORDER BY total_cost DESC
`, hoursBack)
}
