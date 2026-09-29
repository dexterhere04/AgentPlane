package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/clickhouse"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

const (
	minAnalyticsHours = 1
	maxAnalyticsHours = 720 // 30 days
)

// analyticsHTTPClient forwards analytics queries to ClickHouse's HTTP interface.
var analyticsHTTPClient = &http.Client{Timeout: 30 * time.Second}

// AnalyticsHandler serves pre-built, whitelisted ClickHouse analytics queries.
// It is admin-gated upstream because it queries the observability database.
//
// Query params:
//   - type:  one of traces_count, latency, token_usage, cost, top_models,
//     provider_usage, guardrail_events, error_rate, tool_calls
//   - hours: integer window between 1 and 720 (default 24)
//
// When defaultType is non-empty it is used instead of the "type" query param,
// which lets a dedicated route (e.g. /analytics/traces_count) pin a single query.
func AnalyticsHandler(chURL string, defaultType string) http.Handler {
	queries := observability.NewAnalyticsQueries()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hours := 24
		if v := r.URL.Query().Get("hours"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < minAnalyticsHours || n > maxAnalyticsHours {
				http.Error(w, fmt.Sprintf("hours must be an integer between %d and %d", minAnalyticsHours, maxAnalyticsHours), http.StatusBadRequest)
				return
			}
			hours = n
		}

		typ := r.URL.Query().Get("type")
		if typ == "" {
			typ = defaultType
		}

		query, ok := analyticsQuery(queries, typ, hours)
		if !ok {
			http.Error(w, "unsupported analytics type", http.StatusBadRequest)
			return
		}
		query += " FORMAT JSON"

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, chURL+"/?query="+url.QueryEscape(query), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if user, pass := clickhouse.Credentials(); pass != "" {
			req.SetBasicAuth(user, pass)
		}

		resp, err := analyticsHTTPClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

func analyticsQuery(q *observability.AnalyticsQueries, typ string, hours int) (string, bool) {
	switch typ {
	case "traces_count":
		return q.TraceCountQuery(hours), true
	case "latency":
		return q.AverageLatencyQuery(hours), true
	case "token_usage":
		return q.TokenUsageQuery(hours), true
	case "cost":
		return q.EstimatedCostQuery(hours), true
	case "top_models":
		return q.TopModelsQuery(hours, 10), true
	case "provider_usage":
		return q.ProviderUsageQuery(hours), true
	case "guardrail_events":
		return q.GuardrailEventsQuery(hours), true
	case "error_rate":
		return q.ErrorRateQuery(hours), true
	case "tool_calls":
		return q.ToolCallStatsQuery(hours), true
	default:
		return "", false
	}
}
