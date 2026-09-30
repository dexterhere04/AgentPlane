package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/google/uuid"
)

type fakeAnalyticsStore struct {
	filter observability.AnalyticsFilter
	page   observability.TracePageRequest
	err    error
}

type analyticsIsolationProvider struct{}

func (analyticsIsolationProvider) Name() string { return "test" }
func (analyticsIsolationProvider) Forward(context.Context, []byte, string) ([]byte, error) {
	return []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
}

type errorDetailsProvider struct{}

func (errorDetailsProvider) Name() string { return "test" }
func (errorDetailsProvider) Forward(context.Context, []byte, string) ([]byte, error) {
	return nil, errors.New("https://user:private-password@example.test failed with credential private-token")
}

type correlationProvider struct {
	requestID string
	startedAt time.Time
	startSet  bool
}

func (*correlationProvider) Name() string { return "test" }
func (p *correlationProvider) Forward(ctx context.Context, _ []byte, requestID string) ([]byte, error) {
	p.requestID = requestID
	p.startedAt, p.startSet = observability.RequestStart(ctx)
	return []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
}

func (s *fakeAnalyticsStore) Health(_ context.Context, filter observability.AnalyticsFilter) (observability.ObservabilityHealth, error) {
	s.filter = filter
	return observability.ObservabilityHealth{Status: "healthy", ClickHouseReachable: true, DataState: "empty"}, s.err
}
func (s *fakeAnalyticsStore) Overview(_ context.Context, filter observability.AnalyticsFilter) (observability.AnalyticsOverview, error) {
	s.filter = filter
	return observability.AnalyticsOverview{}, s.err
}
func (s *fakeAnalyticsStore) Series(_ context.Context, filter observability.AnalyticsFilter) (observability.AnalyticsSeries, error) {
	s.filter = filter
	return observability.AnalyticsSeries{Buckets: []observability.AnalyticsBucket{}}, s.err
}
func (s *fakeAnalyticsStore) Breakdowns(_ context.Context, filter observability.AnalyticsFilter) (map[string][]observability.AnalyticsBreakdown, error) {
	s.filter = filter
	return map[string][]observability.AnalyticsBreakdown{}, s.err
}
func (s *fakeAnalyticsStore) Failures(_ context.Context, filter observability.AnalyticsFilter, _ int) ([]observability.TraceSummary, error) {
	s.filter = filter
	return []observability.TraceSummary{}, s.err
}
func (s *fakeAnalyticsStore) Guardrails(_ context.Context, filter observability.AnalyticsFilter, _ int) (observability.GuardrailAnalytics, error) {
	s.filter = filter
	return observability.GuardrailAnalytics{Breakdown: []observability.GuardrailBreakdown{}, Recent: []observability.GuardrailMetadata{}}, s.err
}
func (s *fakeAnalyticsStore) Traces(_ context.Context, request observability.TracePageRequest) (observability.TracePage, error) {
	s.filter, s.page = request.Filter, request
	return observability.TracePage{Items: []observability.TraceSummary{}}, s.err
}
func (s *fakeAnalyticsStore) Trace(_ context.Context, filter observability.AnalyticsFilter, _ string) (observability.TraceDetail, error) {
	s.filter = filter
	return observability.TraceDetail{}, s.err
}

func TestParseAnalyticsFilterAllSupportedRanges(t *testing.T) {
	for _, value := range []string{"15m", "1h", "6h", "24h", "7d"} {
		filter, err := parseAnalyticsFilter(url.Values{"range": {value}}, time.Now())
		if err != nil {
			t.Errorf("range %q rejected: %v", value, err)
			continue
		}
		if string(filter.Range) != value || !filter.To.After(filter.From) {
			t.Errorf("range %q produced invalid filter: %+v", value, filter)
		}
	}
}

func TestParseAnalyticsFilterRejectsUnsafeOrUnsupportedInput(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
	}{
		{"unsupported range", url.Values{"range": {"30d"}}},
		{"unsupported status", url.Values{"status": {"pending"}}},
		{"unknown parameter", url.Values{"sql": {"DROP TABLE traces"}}},
		{"repeated parameter", url.Values{"model": {"one", "two"}}},
		{"control character", url.Values{"route": {"/x\n"}}},
		{"oversize field", url.Values{"model": {strings.Repeat("m", 129)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAnalyticsFilter(test.query, time.Now()); err == nil {
				t.Fatal("invalid query accepted")
			}
		})
	}
}

func TestObservabilityHandlerAppliesAllFiltersTogether(t *testing.T) {
	store := &fakeAnalyticsStore{}
	handler := NewObservabilityHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/api/observability/overview?range=6h&status=error&provider=prov&model=mod&route=%2Fchat&guardrail_action=block&user_id=u1&organization_id=o1&project_id=p1", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	got := store.filter
	if got.Range != observability.Range6Hours || got.Status != "error" || got.Provider != "prov" || got.Model != "mod" ||
		got.Route != "/chat" || got.GuardrailAction != "block" || got.UserID != "u1" || got.OrganizationID != "o1" || got.ProjectID != "p1" {
		t.Fatalf("filters not propagated together: %+v", got)
	}
}

func TestTracePaginationValidationAndBounds(t *testing.T) {
	for _, value := range []string{"0", "101", "not-a-number"} {
		if _, err := parseLimit(value, observability.DefaultTracePageSize); err == nil {
			t.Errorf("invalid page limit %q accepted", value)
		}
	}
	if got, err := parseLimit("100", observability.DefaultTracePageSize); err != nil || got != observability.MaxTracePageSize {
		t.Fatalf("maximum page limit: got %d, err %v", got, err)
	}
	if _, err := parseTraceCursor("not-base64"); err == nil {
		t.Fatal("malformed cursor accepted")
	}
	handler := NewObservabilityHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/observability/traces?limit=101", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized page status = %d, want 400", response.Code)
	}
}

func TestObservabilityEndpointsAreGetOnly(t *testing.T) {
	handler := NewObservabilityHandler(&fakeAnalyticsStore{})
	for _, path := range []string{"/api/observability/overview", "/api/observability/traces/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want 405", path, response.Code)
		}
	}
}

func TestObservabilityEndpointsRequireAdminToken(t *testing.T) {
	secured := auth.AdminMiddleware("admin-secret", NewObservabilityHandler(&fakeAnalyticsStore{}))
	for _, token := range []string{"", "wrong", "ap_live_not-an-admin-api-key"} {
		request := httptest.NewRequest(http.MethodGet, "/api/observability/overview", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		secured.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("token %q status = %d, want 401", token, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/observability/overview", nil)
	request.Header.Set("Authorization", "Bearer admin-secret")
	response := httptest.NewRecorder()
	secured.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("valid admin status = %d", response.Code)
	}
}

func TestAnalyticsFailureDoesNotLeakRawError(t *testing.T) {
	store := &fakeAnalyticsStore{err: errors.New("clickhouse secret-host password=do-not-show")}
	handler := NewObservabilityHandler(store)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/observability/overview", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if strings.Contains(response.Body.String(), "secret-host") || strings.Contains(response.Body.String(), "do-not-show") {
		t.Fatalf("raw storage error leaked: %s", response.Body.String())
	}
}

func TestAnalyticsFailureDoesNotBreakChat(t *testing.T) {
	oldCaptureConfig := observability.GetCaptureConfig()
	observability.SetCaptureConfig(observability.CaptureConfig{
		PromptMode: observability.CaptureModeDisabled, ResponseMode: observability.CaptureModeDisabled,
	})
	t.Cleanup(func() { observability.SetCaptureConfig(oldCaptureConfig) })

	mux := http.NewServeMux()
	mux.Handle("/api/observability/", auth.AdminMiddleware("admin", NewObservabilityHandler(&fakeAnalyticsStore{err: errors.New("ClickHouse offline")})))
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		Chat(w, r, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, analyticsIsolationProvider{})
	})
	analyticsRequest := httptest.NewRequest(http.MethodGet, "/api/observability/overview", nil)
	analyticsRequest.Header.Set("Authorization", "Bearer admin")
	analyticsResponse := httptest.NewRecorder()
	mux.ServeHTTP(analyticsResponse, analyticsRequest)
	if analyticsResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("analytics status = %d", analyticsResponse.Code)
	}
	chatRequest := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"model":"test","messages":[]}`))
	chatResponse := httptest.NewRecorder()
	mux.ServeHTTP(chatResponse, chatRequest)
	if chatResponse.Code != http.StatusOK {
		t.Fatalf("chat status = %d, body = %s", chatResponse.Code, chatResponse.Body.String())
	}
}

func TestChatDoesNotExposeUpstreamErrorDetails(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"model":"test","messages":[]}`))
	response := httptest.NewRecorder()
	Chat(response, request, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, errorDetailsProvider{})
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", response.Code)
	}
	for _, secret := range []string{"private-password", "private-token", "example.test"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("upstream detail %q leaked in response: %s", secret, response.Body.String())
		}
	}
}

func TestChatUsesUniqueTraceIDAndPropagatesStartTime(t *testing.T) {
	provider := &correlationProvider{}
	request := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"model":"test","messages":[]}`))
	response := httptest.NewRecorder()
	Chat(response, request, nil, guardrail.GuardrailSet{}, guardrail.GuardrailSet{}, provider)
	if response.Code != http.StatusOK {
		t.Fatalf("chat status = %d, body = %s", response.Code, response.Body.String())
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(provider.requestID, "req-"))
	if err != nil || parsed == uuid.Nil {
		t.Fatalf("request ID is not a UUID: %q", provider.requestID)
	}
	if !provider.startSet || provider.startedAt.IsZero() || provider.startedAt.After(time.Now()) {
		t.Fatalf("request start was not propagated: %s", provider.startedAt)
	}
}

func TestHealthAndTraceDetailResponsesContainMetadataOnly(t *testing.T) {
	store := &fakeAnalyticsStore{}
	handler := NewObservabilityHandler(store)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/observability/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unconfigured health status = %d", response.Code)
	}
	var health observability.ObservabilityHealth
	if err := json.Unmarshal(response.Body.Bytes(), &health); err != nil || health.DataState != "empty" {
		t.Fatalf("health state = %+v, err=%v", health, err)
	}
	detail := observability.TraceDetail{
		Prompt:   observability.CaptureMetadata{Available: true, Mode: "hash_only", Hash: "safe-hash"},
		Response: observability.CaptureMetadata{Available: false},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "prompt_blob") || strings.Contains(string(body), "response_blob") || strings.Contains(string(body), "credential") {
		t.Fatalf("sensitive payload fields present: %s", body)
	}
}
