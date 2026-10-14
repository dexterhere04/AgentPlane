package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/dexterhere04/AgentPlane/internal/clickhouse"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type userUsageRow struct {
	UserID       string  `json:"user_id"`
	Username     string  `json:"username"`
	RequestCount uint64  `json:"request_count"`
	InputTokens  uint64  `json:"input_tokens"`
	OutputTokens uint64  `json:"output_tokens"`
	TotalTokens  uint64  `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	LastSeen     string  `json:"last_seen"`
}

type userPromptRow struct {
	TraceID       string  `json:"trace_id"`
	Prompt        string  `json:"prompt"`
	CreatedAt     string  `json:"created_at"`
	Username      string  `json:"username"`
	Model         string  `json:"model"`
	TotalTokens   uint64  `json:"total_tokens"`
	EstimatedCost float64 `json:"estimated_cost"`
}

// UserAnalyticsHandler serves per-user spend/token analytics and a searchable
// prompt log, both backed by ClickHouse. It is admin-gated upstream.
//
// Query params:
//   - hours: window (1-720, default 24)
//   - user:  optional username filter for prompts
//   - q:     optional case-insensitive substring to search prompt text
func UserAnalyticsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		client := clickhouse.GetInstance()
		if client == nil {
			http.Error(w, "clickhouse not available", http.StatusServiceUnavailable)
			return
		}

		hours := 24
		if v := r.URL.Query().Get("hours"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= minAnalyticsHours && n <= maxAnalyticsHours {
				hours = n
			}
		}
		userFilter := r.URL.Query().Get("user")
		search := r.URL.Query().Get("q")

		q := observability.NewAnalyticsQueries()

		users, err := queryUserUsage(r.Context(), client, q.UserUsageQuery(hours))
		if err != nil {
			log.Printf("user usage analytics failed: %v", err)
			http.Error(w, "analytics query failed", http.StatusInternalServerError)
			return
		}

		prompts, err := queryUserPrompts(r.Context(), client, q.PromptSearchQuery(hours, 200), userFilter, search)
		if err != nil {
			log.Printf("prompt search analytics failed: %v", err)
			http.Error(w, "analytics query failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users":   users,
			"prompts": prompts,
		})
	})
}

func queryUserUsage(ctx context.Context, client *clickhouse.Client, sql string) ([]userUsageRow, error) {
	rows, err := client.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]userUsageRow, 0)
	for rows.Next() {
		var r userUsageRow
		if err := rows.Scan(
			&r.UserID, &r.Username, &r.RequestCount, &r.InputTokens,
			&r.OutputTokens, &r.TotalTokens, &r.TotalCost, &r.AvgLatencyMs, &r.LastSeen,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func queryUserPrompts(ctx context.Context, client *clickhouse.Client, sql, user, q string) ([]userPromptRow, error) {
	rows, err := client.Query(ctx, sql, user, user, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]userPromptRow, 0)
	for rows.Next() {
		var r userPromptRow
		if err := rows.Scan(
			&r.TraceID, &r.Prompt, &r.CreatedAt, &r.Username,
			&r.Model, &r.TotalTokens, &r.EstimatedCost,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
