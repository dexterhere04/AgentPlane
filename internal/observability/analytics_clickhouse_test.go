package observability

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type analyticsTestRows struct {
	rows   [][]any
	index  int
	closed bool
	err    error
}

type contextCheckingRows struct {
	driver.Rows
	ctx                context.Context
	canceledDuringScan bool
}

func (r *contextCheckingRows) Next() bool {
	if r.ctx.Err() != nil {
		r.canceledDuringScan = true
	}
	return r.Rows.Next()
}

func (r *analyticsTestRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}
func (r *analyticsTestRows) Scan(dest ...any) error {
	values := r.rows[r.index-1]
	if len(values) != len(dest) {
		return errors.New("fake scan destination mismatch")
	}
	for i, value := range values {
		target := reflect.ValueOf(dest[i]).Elem()
		if value == nil {
			continue
		}
		source := reflect.ValueOf(value)
		if source.Type().AssignableTo(target.Type()) {
			target.Set(source)
		} else if source.Type().ConvertibleTo(target.Type()) {
			target.Set(source.Convert(target.Type()))
		} else {
			return errors.New("fake scan type mismatch")
		}
	}
	return nil
}
func (r *analyticsTestRows) ScanStruct(any) error             { return errors.New("unused") }
func (r *analyticsTestRows) ColumnTypes() []driver.ColumnType { return nil }
func (r *analyticsTestRows) Totals(...any) error              { return nil }
func (r *analyticsTestRows) Columns() []string                { return nil }
func (r *analyticsTestRows) Close() error                     { r.closed = true; return nil }
func (r *analyticsTestRows) Err() error                       { return r.err }
func (r *analyticsTestRows) HasData() bool                    { return len(r.rows) > 0 }

type analyticsTestClient struct {
	queries []string
	args    [][]any
	pingErr error
	queryFn func(context.Context, string, ...any) (driver.Rows, error)
}

func (c *analyticsTestClient) Ping(context.Context) error { return c.pingErr }
func (c *analyticsTestClient) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	c.queries = append(c.queries, query)
	c.args = append(c.args, append([]any(nil), args...))
	if c.queryFn != nil {
		return c.queryFn(ctx, query, args...)
	}
	return &analyticsTestRows{}, nil
}

func TestClickHouseAnalyticsOverviewMapsAggregates(t *testing.T) {
	client := &analyticsTestClient{queryFn: func(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
		return &analyticsTestRows{rows: [][]any{{
			uint64(10), uint64(8), uint64(2), float64(12), float64(95), float64(180),
			uint64(1000), uint64(500), uint64(1500), float64(0.75), time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
		}}}, nil
	}}
	store := &clickhouseAnalyticsStore{client: client}
	result, err := store.Overview(context.Background(), AnalyticsFilter{From: time.Now().Add(-time.Hour), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requests != 10 || result.Successes != 8 || result.Errors != 2 || result.ErrorRate != 0.2 {
		t.Fatalf("overview counts = %+v", result)
	}
	if result.Latency != (LatencyPercentiles{P50: 12, P95: 95, P99: 180}) {
		t.Fatalf("percentiles = %+v", result.Latency)
	}
	if result.Tokens != (TokenTotals{Input: 1000, Output: 500, Total: 1500}) || result.EstimatedCost != 0.75 || result.LatestTelemetryAt == nil {
		t.Fatalf("overview totals = %+v", result)
	}
	if len(client.queries) != 1 || !strings.Contains(client.queries[0], "quantileTDigest(0.99)") {
		t.Fatalf("percentile query missing: %v", client.queries)
	}
}

func TestAnalyticsQueryContextLivesUntilRowsClose(t *testing.T) {
	var returned *contextCheckingRows
	client := &analyticsTestClient{queryFn: func(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
		returned = &contextCheckingRows{
			ctx: ctx,
			Rows: &analyticsTestRows{rows: [][]any{{
				uint64(1), uint64(1), uint64(0), float64(1), float64(1), float64(1),
				uint64(2), uint64(3), uint64(5), float64(0.01), time.Now().UTC(),
			}}},
		}
		return returned, nil
	}}
	_, err := (&clickhouseAnalyticsStore{client: client}).Overview(context.Background(), AnalyticsFilter{
		From: time.Now().Add(-time.Hour), To: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if returned.canceledDuringScan {
		t.Fatal("query context was canceled before row scanning completed")
	}
	if returned.ctx.Err() == nil {
		t.Fatal("query context was not canceled after rows closed")
	}
}

func TestAnalyticsQueryUsesBoundedDeadline(t *testing.T) {
	var remaining time.Duration
	client := &analyticsTestClient{queryFn: func(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("analytics query has no deadline")
		}
		remaining = time.Until(deadline)
		return &analyticsTestRows{}, nil
	}}
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := (&clickhouseAnalyticsStore{client: client}).Overview(parent, AnalyticsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if remaining <= 0 || remaining > analyticsQueryTimeout {
		t.Fatalf("query deadline remaining = %s, want <= %s", remaining, analyticsQueryTimeout)
	}
}

func TestClickHouseAnalyticsOverviewHandlesNoRows(t *testing.T) {
	client := &analyticsTestClient{queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
		return &analyticsTestRows{}, nil
	}}
	result, err := (&clickhouseAnalyticsStore{client: client}).Overview(context.Background(), AnalyticsFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requests != 0 || result.Errors != 0 || result.Successes != 0 || result.ErrorRate != 0 || result.LatestTelemetryAt != nil {
		t.Fatalf("no-row aggregate = %+v", result)
	}
}

func TestClickHouseAnalyticsSeriesFillsOnlyWindowBuckets(t *testing.T) {
	from := time.Date(2026, 9, 29, 12, 0, 30, 0, time.UTC)
	to := from.Add(2 * time.Minute)
	client := &analyticsTestClient{queryFn: func(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
		return &analyticsTestRows{rows: [][]any{{
			time.Date(2026, 9, 29, 12, 1, 0, 0, time.UTC),
			uint64(1), uint64(0), uint64(1), float64(10), float64(20), float64(30),
			uint64(4), uint64(6), uint64(10), float64(0.02),
		}}}, nil
	}}
	series, err := (&clickhouseAnalyticsStore{client: client}).Series(context.Background(), AnalyticsFilter{
		Range: Range1Hour, From: from, To: to,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(series.Buckets) != 3 {
		t.Fatalf("got %d buckets, want 3", len(series.Buckets))
	}
	if !series.Buckets[0].Timestamp.Equal(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)) || series.Buckets[0].Requests != 0 {
		t.Fatalf("first empty bucket = %+v", series.Buckets[0])
	}
	if !series.Buckets[1].Timestamp.Equal(time.Date(2026, 9, 29, 12, 1, 0, 0, time.UTC)) || series.Buckets[1].Requests != 1 || series.Buckets[1].ErrorRate != 1 {
		t.Fatalf("populated bucket = %+v", series.Buckets[1])
	}
	if !series.Buckets[2].Timestamp.Equal(time.Date(2026, 9, 29, 12, 2, 0, 0, time.UTC)) || series.Buckets[2].Requests != 0 {
		t.Fatalf("trailing empty bucket = %+v", series.Buckets[2])
	}
	if !series.Buckets[len(series.Buckets)-1].Timestamp.Before(to) {
		t.Fatalf("future or end-exclusive bucket returned: %+v", series.Buckets[len(series.Buckets)-1])
	}
}

func TestAnalyticsQueriesShareCommonFilterPopulation(t *testing.T) {
	filter := AnalyticsFilter{
		Range: Range24Hours, From: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		To: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Status: "error",
		Provider: "provider-x", Model: "model-y", Route: "/chat", GuardrailAction: "block",
	}
	client := &analyticsTestClient{queryFn: func(context.Context, string, ...any) (driver.Rows, error) {
		return &analyticsTestRows{}, nil
	}}
	store := &clickhouseAnalyticsStore{client: client}
	ctx := context.Background()
	if _, err := store.Health(ctx, filter); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Overview(ctx, filter); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Series(ctx, filter); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Breakdowns(ctx, filter); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Failures(ctx, filter, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Traces(ctx, TracePageRequest{Filter: filter, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Trace(ctx, filter, "trace-id"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing trace lookup error = %v, want sql.ErrNoRows", err)
	}
	if _, err := store.Guardrails(ctx, filter, 10); err != nil {
		t.Fatal(err)
	}
	_, expectedPrefix := filteredTraceSource(filter)
	for index, query := range client.queries {
		if !strings.Contains(query, "t.timestamp >= ?") || !strings.Contains(query, "t.timestamp < ?") ||
			!strings.Contains(query, "t.is_error = 1") || !strings.Contains(query, "t.provider = ?") ||
			!strings.Contains(query, "t.model = ?") || !strings.Contains(query, "t.route = ?") ||
			!strings.Contains(query, "t.effective_guardrail_action = ?") {
			t.Errorf("query %d omitted a common filter: %s", index, query)
		}
		if len(client.args[index]) < len(expectedPrefix) || !reflect.DeepEqual(client.args[index][:len(expectedPrefix)], expectedPrefix) {
			t.Errorf("query %d has inconsistent common bound values: %#v", index, client.args[index])
		}
	}
}

func TestClickHouseAnalyticsOverviewEmptyAndErrors(t *testing.T) {
	emptyClient := &analyticsTestClient{queryFn: func(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
		return &analyticsTestRows{rows: [][]any{{uint64(0), uint64(0), uint64(0), float64(0), float64(0), float64(0), uint64(0), uint64(0), uint64(0), float64(0), time.Time{}}}}, nil
	}}
	result, err := (&clickhouseAnalyticsStore{client: emptyClient}).Overview(context.Background(), AnalyticsFilter{})
	if err != nil || result.Requests != 0 || result.ErrorRate != 0 || result.LatestTelemetryAt != nil {
		t.Fatalf("empty overview = %+v, err=%v", result, err)
	}
	wantErr := errors.New("driver detail must not escape the API")
	failedClient := &analyticsTestClient{queryFn: func(context.Context, string, ...any) (driver.Rows, error) { return nil, wantErr }}
	if _, err := (&clickhouseAnalyticsStore{client: failedClient}).Overview(context.Background(), AnalyticsFilter{}); !errors.Is(err, wantErr) {
		t.Fatalf("query error = %v, want %v", err, wantErr)
	}
}

func TestClickHouseAnalyticsTraceDetailMetadataOnly(t *testing.T) {
	timestamp := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	client := &analyticsTestClient{queryFn: func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM agentplane.traces"):
			return &analyticsTestRows{rows: [][]any{{timestamp, "trace-1", "req-1", "user-1", "", "", "openai", "gpt-x", "/chat", "success", uint32(45), uint32(10), uint32(5), uint32(15), float64(0.01), ""}}}, nil
		case strings.Contains(query, "FROM agentplane.prompt_events"):
			if strings.Contains(query, "prompt_blob") {
				t.Fatal("raw prompt blob selected")
			}
			return &analyticsTestRows{rows: [][]any{{"hash_only", "prompt-hash", uint32(120), uint32(0)}}}, nil
		case strings.Contains(query, "FROM agentplane.response_events"):
			if strings.Contains(query, "response_blob") {
				t.Fatal("raw response blob selected")
			}
			return &analyticsTestRows{}, nil
		case strings.Contains(query, "FROM agentplane.usage_events"):
			return &analyticsTestRows{rows: [][]any{{"openai", "gpt-x", uint32(2), uint32(3), float64(0.01), timestamp}}}, nil
		case strings.Contains(query, "FROM agentplane.guardrail_events"):
			return &analyticsTestRows{rows: [][]any{{"secrets", "input", "pass", timestamp}}}, nil
		case strings.Contains(query, "FROM agentplane.tool_call_events"):
			return &analyticsTestRows{}, nil
		default:
			return nil, errors.New("unexpected query")
		}
	}}
	store := &clickhouseAnalyticsStore{client: client}
	filter := AnalyticsFilter{From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour)}
	detail, err := store.Trace(context.Background(), filter, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TraceID != "trace-1" || detail.Prompt.Mode != "hash_only" || detail.Prompt.Hash != "prompt-hash" || !detail.Usage.Available {
		t.Fatalf("trace detail mapping = %+v", detail)
	}
	if len(detail.Guardrails) != 1 || detail.Guardrails[0].Phase != "input" || len(detail.Tools) != 0 {
		t.Fatalf("related metadata = %+v", detail)
	}
	for _, query := range client.queries[1:] {
		if !strings.Contains(query, "created_at >= ?") || !strings.Contains(query, "created_at < ?") {
			t.Errorf("detail query is not time bounded: %s", query)
		}
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "blob") || strings.Contains(string(encoded), "credential") {
		t.Fatalf("sensitive fields returned: %s", encoded)
	}
}

func TestClickHouseAnalyticsTraceMissingAndCanceled(t *testing.T) {
	timestamp := time.Now().UTC()
	emptyClient := &analyticsTestClient{queryFn: func(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
		return &analyticsTestRows{}, nil
	}}
	_, err := (&clickhouseAnalyticsStore{client: emptyClient}).Trace(context.Background(), AnalyticsFilter{From: timestamp.Add(-time.Hour), To: timestamp}, "missing")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing trace error = %v, want sql.ErrNoRows", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledClient := &analyticsTestClient{queryFn: func(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
		return nil, ctx.Err()
	}}
	if _, err := (&clickhouseAnalyticsStore{client: canceledClient}).Overview(canceled, AnalyticsFilter{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query error = %v", err)
	}
}

func TestClickHouseAnalyticsTracePagination(t *testing.T) {
	timestamp := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	traceRow := func(id string) []any {
		return []any{timestamp, id, "request-" + id, "", "", "", "openai", "model", "/chat", "success",
			uint32(5), uint32(1), uint32(2), uint32(3), float64(0.01), ""}
	}
	client := &analyticsTestClient{queryFn: func(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
		return &analyticsTestRows{rows: [][]any{traceRow("trace-3"), traceRow("trace-2"), traceRow("trace-1")}}, nil
	}}
	store := &clickhouseAnalyticsStore{client: client}
	filter := AnalyticsFilter{From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour)}
	page, err := store.Traces(context.Background(), TracePageRequest{Filter: filter, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" || page.Items[1].TraceID != "trace-2" {
		t.Fatalf("page = %+v", page)
	}
	if got := client.args[0][len(client.args[0])-1]; got != 3 {
		t.Fatalf("query limit arg = %v, want page size + 1", got)
	}
	cursor, err := decodeCursor(page.NextCursor)
	if err != nil || cursor.TraceID != "trace-2" {
		t.Fatalf("cursor = %+v, err=%v", cursor, err)
	}
	_, err = store.Traces(context.Background(), TracePageRequest{Filter: filter, Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(client.queries[1], "timestamp < ? OR (timestamp = ? AND trace_id < ?)") {
		t.Fatalf("keyset cursor missing from query: %s", client.queries[1])
	}
	if got := client.args[1][len(client.args[1])-2]; got != cursor.TraceID {
		t.Fatalf("cursor trace ID not bound: %#v", client.args[1])
	}
}

func decodeCursor(value string) (*TraceCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor TraceCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}

func TestClickHouseAnalyticsGuardrailResults(t *testing.T) {
	timestamp := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	client := &analyticsTestClient{queryFn: func(_ context.Context, query string, _ ...any) (driver.Rows, error) {
		if strings.Contains(query, "count()") {
			return &analyticsTestRows{rows: [][]any{{"secrets", "output", "block", uint64(2)}}}, nil
		}
		return &analyticsTestRows{rows: [][]any{{"secrets", "output", "block", timestamp}}}, nil
	}}
	filter := AnalyticsFilter{From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour), GuardrailAction: "block"}
	result, err := (&clickhouseAnalyticsStore{client: client}).Guardrails(context.Background(), filter, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Breakdown) != 1 || result.Breakdown[0].Count != 2 || len(result.Recent) != 1 || result.Recent[0].Phase != "output" {
		t.Fatalf("guardrail result = %+v", result)
	}
	for i, query := range client.queries {
		if !strings.Contains(query, "g.action = ?") || !strings.Contains(query, "g.created_at >= ?") {
			t.Errorf("guardrail filter/time bound missing from query %d: %s", i, query)
		}
		if got := client.args[i][len(client.args[i])-1]; got != "block" && got != 3 {
			t.Errorf("unexpected final bound parameter %v", got)
		}
	}
}

func TestClickHouseAnalyticsHealthUnavailable(t *testing.T) {
	client := &analyticsTestClient{pingErr: errors.New("private connection detail")}
	health, err := (&clickhouseAnalyticsStore{client: client}).Health(context.Background(), AnalyticsFilter{})
	if err != nil || health.Status != "unavailable" || health.ClickHouseReachable || health.DataState != "unavailable" {
		t.Fatalf("health = %+v, err=%v", health, err)
	}
	if len(client.queries) != 0 {
		t.Fatalf("health queried after failed ping: %v", client.queries)
	}
}
