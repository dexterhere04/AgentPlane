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
  sum(total_tokens) AS total_tokens,
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
