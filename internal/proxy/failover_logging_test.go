package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

// failoverCaptureStore records what the observability layer would persist.
// It embeds the no-op store so only the methods under test are overridden.
type failoverCaptureStore struct {
	observability.Store

	mu     sync.Mutex
	traces []observability.Trace
	usages []observability.UsageEvent
}

func (s *failoverCaptureStore) StoreTrace(t observability.Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces = append(s.traces, t)
	return nil
}

func (s *failoverCaptureStore) StoreUsage(u observability.UsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usages = append(s.usages, u)
	return nil
}

// forRequest returns the traces and usage events recorded for one request ID.
// The store is process-global, so filtering keeps other tests out of the way.
func (s *failoverCaptureStore) forRequest(id string) ([]observability.Trace, []observability.UsageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ts []observability.Trace
	for _, t := range s.traces {
		if t.TraceID == id {
			ts = append(ts, t)
		}
	}
	var us []observability.UsageEvent
	for _, u := range s.usages {
		if u.TraceID == id {
			us = append(us, u)
		}
	}
	return ts, us
}

// failoverFirstSelector always picks the first remaining candidate, so the
// test is deterministic regardless of weights.
type failoverFirstSelector struct{}

func (failoverFirstSelector) Select(c []*Upstream) (*Upstream, error) {
	if len(c) == 0 {
		return nil, ErrNoProvider
	}
	return c[0], nil
}

const failoverOKResponse = `{
  "id": "chatcmpl-test",
  "object": "chat.completion",
  "model": "gpt-4o",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}
}`

// runFailover sends one request through a two-upstream router where "primary"
// always returns HTTP 500 and "secondary" succeeds, so exactly one failover
// primary -> secondary happens. Telemetry is recorded with requestID as the
// trace ID.
func runFailover(t *testing.T, requestID string) {
	t.Helper()

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(primary.Close)

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(failoverOKResponse))
	}))
	t.Cleanup(secondary.Close)

	// MaxRetries 0: a 500 must fail over to the next upstream immediately
	// instead of being retried on the same one.
	up1 := NewUpstream("primary", NewOpenAIProvider("test-key", primary.URL), WithMaxRetries(0))
	up2 := NewUpstream("secondary", NewOpenAIProvider("test-key", secondary.URL), WithMaxRetries(0))

	router := &Router{selector: failoverFirstSelector{}}

	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	resp, err := router.forwardWithFailover(
		context.Background(),
		body,
		requestID,
		"test-route",
		"gpt-4o",
		[]*Upstream{up1, up2},
		ResilienceConfig{},
	)
	if err != nil {
		t.Fatalf("forwardWithFailover returned error, expected failover to succeed: %v", err)
	}
	if len(resp) == 0 {
		t.Fatal("expected a response body from the secondary upstream")
	}
}

func TestFailoverTelemetryIsLogged(t *testing.T) {
	store := &failoverCaptureStore{Store: observability.NewNoopStore()}
	prev := observability.DefaultStore
	observability.SetStore(store)
	t.Cleanup(func() { observability.SetStore(prev) })

	requestID := fmt.Sprintf("req-failover-test-%d", time.Now().UnixNano())
	runFailover(t, requestID)

	// Traces and usage are recorded from goroutines; wait for both attempts'
	// traces and the one usage event (only the successful attempt has usage).
	var traces []observability.Trace
	var usages []observability.UsageEvent
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		traces, usages = store.forRequest(requestID)
		if len(traces) >= 2 && len(usages) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces (failed attempt + failover attempt), got %d: %+v", len(traces), traces)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage event, got %d: %+v", len(usages), usages)
	}

	var failed, recovered *observability.Trace
	for i := range traces {
		switch traces[i].Upstream {
		case "primary":
			failed = &traces[i]
		case "secondary":
			recovered = &traces[i]
		}
	}
	if failed == nil || recovered == nil {
		t.Fatalf("expected one trace per upstream, got %+v", traces)
	}

	// The attempt that failed is the first attempt: no failover yet.
	if failed.Failover || failed.FailoverFrom != "" || failed.FailoverTo != "" {
		t.Errorf("failed attempt should not be marked as failover: %+v", *failed)
	}
	if failed.Attempt != 1 || failed.Status != "http_500" {
		t.Errorf("failed attempt: want attempt=1 status=http_500, got attempt=%d status=%q", failed.Attempt, failed.Status)
	}

	// The attempt after the switch carries the transition.
	if !recovered.Failover {
		t.Errorf("recovered trace: Failover = false, want true")
	}
	if recovered.FailoverFrom != "primary" || recovered.FailoverTo != "secondary" {
		t.Errorf("recovered trace: failover transition = %q -> %q, want primary -> secondary",
			recovered.FailoverFrom, recovered.FailoverTo)
	}
	if recovered.Attempt != 2 || recovered.Status != "success" || recovered.Route != "test-route" {
		t.Errorf("recovered trace: want attempt=2 status=success route=test-route, got attempt=%d status=%q route=%q",
			recovered.Attempt, recovered.Status, recovered.Route)
	}

	u := usages[0]
	if !u.Failover || u.FailoverFrom != "primary" || u.FailoverTo != "secondary" {
		t.Errorf("usage event: failover=%v %q -> %q, want true primary -> secondary",
			u.Failover, u.FailoverFrom, u.FailoverTo)
	}
}
