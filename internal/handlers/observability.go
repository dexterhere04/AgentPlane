package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type analyticsError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewObservabilityHandler(store observability.AnalyticsStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/observability/health", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			if store == nil {
				return observability.ObservabilityHealth{Status: "unavailable", DataState: "unavailable", CheckedAt: time.Now().UTC()}, nil
			}
			return store.Health(ctx, filter)
		})
	})
	mux.HandleFunc("/api/observability/overview", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			return store.Overview(ctx, filter)
		})
	})
	mux.HandleFunc("/api/observability/series", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			return store.Series(ctx, filter)
		})
	})
	mux.HandleFunc("/api/observability/breakdowns", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			return store.Breakdowns(ctx, filter)
		})
	})
	mux.HandleFunc("/api/observability/failures", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			limit, err := parseLimit(r.URL.Query().Get("limit"), 50)
			if err != nil {
				return nil, err
			}
			return store.Failures(ctx, filter, limit)
		})
	})
	mux.HandleFunc("/api/observability/guardrails", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			limit, err := parseLimit(r.URL.Query().Get("limit"), 50)
			if err != nil {
				return nil, err
			}
			return store.Guardrails(ctx, filter, limit)
		})
	})
	mux.HandleFunc("/api/observability/traces", func(w http.ResponseWriter, r *http.Request) {
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
			limit, err := parseLimit(r.URL.Query().Get("limit"), observability.DefaultTracePageSize)
			if err != nil {
				return nil, err
			}
			cursor, err := parseTraceCursor(r.URL.Query().Get("cursor"))
			if err != nil {
				return nil, err
			}
			return store.Traces(ctx, observability.TracePageRequest{Filter: filter, Limit: limit, Cursor: cursor})
		})
	})
	mux.HandleFunc("/api/observability/traces/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeAnalyticsError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/observability/traces/")
		if id == "" || strings.Contains(id, "/") {
			writeAnalyticsError(w, http.StatusNotFound, "not_found", "trace not found")
			return
		}
		decoded, err := url.PathUnescape(id)
		if err != nil || decoded == "" || len(decoded) > 256 {
			writeAnalyticsError(w, http.StatusBadRequest, "invalid_trace_id", "invalid trace identifier")
			return
		}
		serveAnalytics(w, r, store, func(ctx context.Context, filter observability.AnalyticsFilter) (any, error) {
		return store.Trace(ctx, filter, decoded)
	})
	})
	mux.HandleFunc("/api/observability/users", func(w http.ResponseWriter, r *http.Request) {
		serveUserUsage(w, r, store)
	})
	mux.HandleFunc("/api/observability/prompts", func(w http.ResponseWriter, r *http.Request) {
		servePromptSearch(w, r, store)
	})
	// Compatibility alias: retain the old route, but use the typed analytics store.
	mux.HandleFunc("/analytics/traces_count", func(w http.ResponseWriter, r *http.Request) {
		serveLegacyTraceCount(w, r, store)
	})
	return mux
}

type analyticsOperation func(context.Context, observability.AnalyticsFilter) (any, error)

func serveAnalytics(w http.ResponseWriter, r *http.Request, store observability.AnalyticsStore, operation analyticsOperation) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAnalyticsError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	filter, err := parseAnalyticsFilter(r.URL.Query(), time.Now())
	if err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	if err := validateAnalyticsEndpointQuery(r); err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	if store == nil && !strings.HasSuffix(r.URL.Path, "/health") {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "observability storage is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := operation(ctx, filter)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAnalyticsError(w, http.StatusNotFound, "trace_not_found", "trace not found in the selected time range")
			return
		}
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics data is temporarily unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		return
	}
}

func parseAnalyticsFilter(query url.Values, now time.Time) (observability.AnalyticsFilter, error) {
	allowed := map[string]bool{
		"range": true, "status": true, "provider": true, "model": true, "route": true,
		"guardrail_action": true, "user_id": true, "organization_id": true, "project_id": true,
		"limit": true, "cursor": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return observability.AnalyticsFilter{}, fmt.Errorf("unsupported or repeated query parameter")
		}
	}
	rangeValue := query.Get("range")
	if rangeValue == "" {
		rangeValue = string(observability.Range24Hours)
	}
	from, to, err := observability.ParseAnalyticsRange(rangeValue, now)
	if err != nil {
		return observability.AnalyticsFilter{}, err
	}
	filter := observability.AnalyticsFilter{Range: observability.AnalyticsRange(rangeValue), From: from, To: to}
	filter.Status = query.Get("status")
	if filter.Status == "all" {
		filter.Status = ""
	}
	if filter.Status != "" && filter.Status != "success" && filter.Status != "error" {
		return observability.AnalyticsFilter{}, fmt.Errorf("status must be all, success, or error")
	}
	fields := []struct {
		name string
		dest *string
		max  int
	}{
		{"provider", &filter.Provider, 128}, {"model", &filter.Model, 128}, {"route", &filter.Route, 256},
		{"guardrail_action", &filter.GuardrailAction, 64}, {"user_id", &filter.UserID, 128},
		{"organization_id", &filter.OrganizationID, 128}, {"project_id", &filter.ProjectID, 128},
	}
	for _, field := range fields {
		raw := query.Get(field.name)
		if strings.ContainsAny(raw, "\r\n\x00") {
			return observability.AnalyticsFilter{}, fmt.Errorf("%s contains invalid characters", field.name)
		}
		value := strings.TrimSpace(raw)
		if len(value) > field.max {
			return observability.AnalyticsFilter{}, fmt.Errorf("%s is invalid or too long", field.name)
		}
		*field.dest = value
	}
	return filter, nil
}

func validateAnalyticsEndpointQuery(r *http.Request) error {
	path := r.URL.Path
	allowed := map[string]bool{
		"range": true, "status": true, "provider": true, "model": true, "route": true,
		"guardrail_action": true, "user_id": true, "organization_id": true, "project_id": true,
	}
	if strings.HasSuffix(path, "/failures") || strings.HasSuffix(path, "/guardrails") {
		allowed["limit"] = true
		if _, err := parseLimit(r.URL.Query().Get("limit"), 50); err != nil {
			return err
		}
	}
	if path == "/api/observability/traces" {
		allowed["limit"] = true
		allowed["cursor"] = true
		if _, err := parseLimit(r.URL.Query().Get("limit"), observability.DefaultTracePageSize); err != nil {
			return err
		}
		if _, err := parseTraceCursor(r.URL.Query().Get("cursor")); err != nil {
			return err
		}
	}
	for key := range r.URL.Query() {
		if !allowed[key] {
			return fmt.Errorf("query parameter %s is not supported for this endpoint", key)
		}
	}
	return nil
}

func parseLimit(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > observability.MaxTracePageSize {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", observability.MaxTracePageSize)
	}
	return value, nil
}

func parseTraceCursor(raw string) (*observability.TraceCursor, error) {
	if raw == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(data) > 512 {
		return nil, fmt.Errorf("cursor is invalid")
	}
	var cursor observability.TraceCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Timestamp.IsZero() || cursor.TraceID == "" {
		return nil, fmt.Errorf("cursor is invalid")
	}
	return &cursor, nil
}

func serveLegacyTraceCount(w http.ResponseWriter, r *http.Request, store observability.AnalyticsStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAnalyticsError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	if store == nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "observability storage is unavailable")
		return
	}
	for key, values := range r.URL.Query() {
		if key != "hours" || len(values) != 1 {
			writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", "unsupported or repeated query parameter")
			return
		}
	}
	hours := 24
	if value := r.URL.Query().Get("hours"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || (parsed != 1 && parsed != 6 && parsed != 24 && parsed != 168) {
			writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", "hours must be 1, 6, 24, or 168")
			return
		}
		hours = parsed
	}
	now := time.Now().UTC()
	filter := observability.AnalyticsFilter{From: now.Add(-time.Duration(hours) * time.Hour), To: now}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := store.Overview(ctx, filter)
	if err != nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics data is temporarily unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]uint64{"count": result.Requests})
}

func serveUserUsage(w http.ResponseWriter, r *http.Request, store observability.AnalyticsStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAnalyticsError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	if store == nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "observability storage is unavailable")
		return
	}
	filter, err := parseObservabilityFilter(r, time.Now(), "limit")
	if err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), 50)
	if err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	users, err := store.UserUsage(ctx, filter, limit)
	if err != nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics data is temporarily unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
}

func servePromptSearch(w http.ResponseWriter, r *http.Request, store observability.AnalyticsStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAnalyticsError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET is required")
		return
	}
	if store == nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "observability storage is unavailable")
		return
	}
	filter, err := parseObservabilityFilter(r, time.Now(), "user", "q", "limit")
	if err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if strings.ContainsAny(user+q, "\r\n\x00") || len(user) > 128 || len(q) > 512 {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", "user or q is invalid or too long")
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), 200)
	if err != nil {
		writeAnalyticsError(w, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	prompts, err := store.PromptSearch(ctx, filter, user, q, limit)
	if err != nil {
		writeAnalyticsError(w, http.StatusServiceUnavailable, "analytics_unavailable", "analytics data is temporarily unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"prompts": prompts})
}

func parseObservabilityFilter(r *http.Request, now time.Time, extra ...string) (observability.AnalyticsFilter, error) {
	allowed := map[string]bool{"range": true}
	for _, key := range extra {
		allowed[key] = true
	}
	for key, values := range r.URL.Query() {
		if !allowed[key] || len(values) != 1 {
			return observability.AnalyticsFilter{}, fmt.Errorf("unsupported or repeated query parameter")
		}
	}
	rangeValue := r.URL.Query().Get("range")
	if rangeValue == "" {
		rangeValue = string(observability.Range24Hours)
	}
	from, to, err := observability.ParseAnalyticsRange(rangeValue, now)
	if err != nil {
		return observability.AnalyticsFilter{}, err
	}
	return observability.AnalyticsFilter{Range: observability.AnalyticsRange(rangeValue), From: from, To: to}, nil
}

func writeAnalyticsError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	response := analyticsError{}
	response.Error.Code = code
	response.Error.Message = message
	_ = json.NewEncoder(w).Encode(response)
}
