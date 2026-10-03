package proxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubProvider struct {
	calls int
	errs  []error // errs[i] is returned on call i; nil / past the end = success
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Forward(ctx context.Context, body []byte, id string) ([]byte, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	return []byte("ok"), nil
}

func TestBreakerLifecycle(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewCircuitBreaker(BreakerConfig{FailureThreshold: 2, Cooldown: time.Minute})
	b.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatal("closed breaker must allow")
		}
		b.RecordFailure()
	}
	if b.State() != BreakerOpen || b.Allow() || b.Ready() {
		t.Fatal("expected open breaker to reject")
	}

	now = now.Add(time.Minute)
	if !b.Ready() {
		t.Fatal("Ready should be true after cooldown")
	}
	if !b.Allow() || b.State() != BreakerHalfOpen {
		t.Fatal("expected half-open probe to be admitted")
	}
	if b.Allow() {
		t.Fatal("only one probe allowed while half-open")
	}
	b.RecordFailure()
	if b.State() != BreakerOpen {
		t.Fatal("failed probe should reopen")
	}

	now = now.Add(time.Minute)
	b.Allow()
	b.RecordSuccess()
	if b.State() != BreakerClosed {
		t.Fatal("successful probe should close")
	}
}

func TestSupportsModel(t *testing.T) {
	u := NewUpstream("x", &stubProvider{}, WithModels("gpt-*", "o1"))
	for model, want := range map[string]bool{
		"gpt-4o": true, "GPT-4o-mini": true, "o1": true,
		"o1-mini": false, "claude-3": false,
	} {
		if got := u.SupportsModel(model); got != want {
			t.Errorf("SupportsModel(%q)=%v want %v", model, got, want)
		}
	}
	if !NewUpstream("y", &stubProvider{}).SupportsModel("anything") {
		t.Error("empty Models should match everything")
	}
}

func TestForwardRetriesRetryableErrors(t *testing.T) {
	p := &stubProvider{errs: []error{&StatusError{Code: 500}, errors.New("conn reset")}}
	u := NewUpstream("x", p, WithMaxRetries(2))
	u.backoffBase = time.Millisecond

	if _, err := u.Forward(context.Background(), nil, "r1"); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if p.calls != 3 {
		t.Fatalf("calls=%d want 3", p.calls)
	}
}

func TestForwardDoesNotRetryClientErrorOrTripBreaker(t *testing.T) {
	bad := &StatusError{Code: 400}
	p := &stubProvider{errs: []error{bad, bad, bad, bad, bad, bad}}
	u := NewUpstream("x", p, WithMaxRetries(3), WithBreaker(BreakerConfig{FailureThreshold: 1}))

	for i := 0; i < 5; i++ {
		if _, err := u.Forward(context.Background(), nil, "r"); err == nil {
			t.Fatal("expected error")
		}
	}
	if p.calls != 5 {
		t.Fatalf("calls=%d want 5 (no retries on 400)", p.calls)
	}
	if u.Breaker.State() != BreakerClosed {
		t.Fatal("400s must not trip the breaker")
	}
}

func TestBackoffWithJitterIsBounded(t *testing.T) {
	base := 100 * time.Millisecond
	max := 2 * time.Second

	for attempt := 1; attempt <= 10; attempt++ {
		for i := 0; i < 100; i++ {
			got := backoffWithJitter(attempt, base, max, 0.20)

			if got <= 0 {
				t.Fatalf("attempt %d: expected positive delay, got %v", attempt, got)
			}

			if got > max {
				t.Fatalf("attempt %d: delay %v exceeds max %v", attempt, got, max)
			}
		}
	}
}

func TestBackoffWithoutJitter(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
		{6, 2 * time.Second},
	}

	for _, tt := range tests {
		got := backoffWithJitter(
			tt.attempt,
			100*time.Millisecond,
			2*time.Second,
			0,
		)

		if got != tt.want {
			t.Errorf(
				"attempt %d: got %v, want %v",
				tt.attempt,
				got,
				tt.want,
			)
		}
	}
}

func TestForwardOpensBreakerAndDisable(t *testing.T) {
	p := &stubProvider{errs: []error{&StatusError{Code: 503}, &StatusError{Code: 503}}}
	u := NewUpstream("x", p, WithMaxRetries(0), WithBreaker(BreakerConfig{FailureThreshold: 2, Cooldown: time.Hour}))

	u.Forward(context.Background(), nil, "r")
	u.Forward(context.Background(), nil, "r")
	if u.Available() {
		t.Fatal("upstream should be unavailable with open breaker")
	}
	if _, err := u.Forward(context.Background(), nil, "r"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen, got %v", err)
	}

	u2 := NewUpstream("y", &stubProvider{})
	u2.SetEnabled(false)
	if _, err := u2.Forward(context.Background(), nil, "r"); !errors.Is(err, ErrUpstreamDisabled) {
		t.Fatalf("want ErrUpstreamDisabled, got %v", err)
	}
}
