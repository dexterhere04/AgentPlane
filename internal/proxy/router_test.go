package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type routerTestProvider struct {
	name  string
	errs  []error
	calls int
}

func (p *routerTestProvider) Name() string {
	return p.name
}

func TestRouterAppliesResilienceOverridesWhenProviderGroupIsEmpty(t *testing.T) {
	t.Parallel()

	provider := &stubProvider{
		errs: []error{
			&StatusError{Code: 500},
			&StatusError{Code: 500},
			nil,
		},
	}

	upstream := NewUpstream(
		"openai-primary",
		provider,
		WithModels("gpt-*"),
		WithMaxRetries(0),
	)

	rule := RoutingRule{
		ID:            "route-1",
		Name:          "empty-group-resilience",
		Priority:      1,
		Action:        "route",
		ProviderGroup: "",
		ModelMatcher:  json.RawMessage(`{"equals":"gpt-4o"}`),
		TimeoutRetryOverrides: json.RawMessage(`{
			"timeout_ms": 1000,
			"max_retries": 2,
			"backoff_base_ms": 1,
			"backoff_max_ms": 2,
			"jitter_percent": 0
		}`),
		Enabled: true,
	}

	router := newTestRouter(
		t,
		nil,
		[]RoutingRule{rule},
		upstream,
	)

	ctx := routeContext()

	resp, err := router.Forward(
		ctx,
		[]byte(`{"model":"gpt-4o"}`),
		"req-empty-group",
	)
	if err != nil {
		t.Fatalf("expected request to succeed after retries, got %v", err)
	}

	if string(resp) != "ok" {
		t.Fatalf("unexpected response: %s", resp)
	}

	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", provider.calls)
	}
}

func (p *routerTestProvider) Forward(
	_ context.Context,
	_ []byte,
	_ string,
) ([]byte, error) {
	p.calls++

	if len(p.errs) > 0 {
		err := p.errs[0]
		p.errs = p.errs[1:]
		if err != nil {
			return nil, err
		}
	}

	return []byte(`{"provider":"` + p.name + `"}`), nil
}

func routeRule(group string) RoutingRule {
	return RoutingRule{
		ID:            "route-1",
		Name:          "test route",
		Priority:      1,
		Action:        "route",
		ProviderGroup: group,
		Enabled:       true,
	}
}

func newTestRouter(
	t *testing.T,
	defaultProvider Provider,
	rules []RoutingRule,
	upstreams ...*Upstream,
) *Router {
	t.Helper()

	ruleSet, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	for _, upstream := range upstreams {
		if err := registry.Register(upstream); err != nil {
			t.Fatal(err)
		}
	}

	return NewRouter(
		defaultProvider,
		ruleSet,
		registry,
		NewWeightedSelector(42),
	)
}

func newTestRouterWithSelector(
	t *testing.T,
	defaultProvider Provider,
	rules []RoutingRule,
	selector UpstreamSelector,
	upstreams ...*Upstream,
) *Router {
	t.Helper()

	ruleSet, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	for _, upstream := range upstreams {
		if err := registry.Register(upstream); err != nil {
			t.Fatal(err)
		}
	}

	return NewRouter(
		defaultProvider,
		ruleSet,
		registry,
		selector,
	)
}

func routeContext() context.Context {
	return WithRouteRequest(
		context.Background(),
		RouteRequest{
			Model:  "gpt-4o",
			UserID: "user-1",
		},
	)
}

func TestRoutingRuleResilienceConfig(t *testing.T) {
	rule := RoutingRule{
		Name: "test-rule",
		TimeoutRetryOverrides: json.RawMessage(`{
			"timeout_ms": 30000,
			"max_retries": 3,
			"backoff_base_ms": 100,
			"backoff_max_ms": 2000,
			"jitter_percent": 20
		}`),
	}

	config, err := rule.ResilienceConfig()
	if err != nil {
		t.Fatalf("ResilienceConfig() error = %v", err)
	}

	if config.Timeout != 30*time.Second {
		t.Fatalf("timeout = %v, want 30s", config.Timeout)
	}

	if config.MaxRetries != 3 {
		t.Fatalf("max retries = %d, want 3", config.MaxRetries)
	}

	if config.BackoffBase != 100*time.Millisecond {
		t.Fatalf("backoff base = %v, want 100ms", config.BackoffBase)
	}

	if config.BackoffMax != 2*time.Second {
		t.Fatalf("backoff max = %v, want 2s", config.BackoffMax)
	}

	if config.JitterFactor != 0.20 {
		t.Fatalf("jitter = %v, want 0.20", config.JitterFactor)
	}
}

func TestRouterUsesDefaultProviderWhenNoRuleMatches(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}
	groupProvider := &routerTestProvider{name: "group"}

	upstream := NewUpstream(
		"group-provider",
		groupProvider,
		WithProviderGroup("production"),
	)

	router := newTestRouter(
		t,
		defaultProvider,
		nil,
		upstream,
	)

	ctx := WithRouteRequest(
		context.Background(),
		RouteRequest{
			Model: "gpt-3.5",
		},
	)

	resp, err := router.Forward(ctx, []byte(`{"model":"gpt-3.5"}`), "req-1")
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"default"}` {
		t.Fatalf("expected default provider, got %s", resp)
	}

	if defaultProvider.calls != 1 {
		t.Fatalf("expected default provider to be called once, got %d", defaultProvider.calls)
	}

	if groupProvider.calls != 0 {
		t.Fatalf("expected group provider not to be called, got %d", groupProvider.calls)
	}
}

func TestRouterSelectsProviderFromMatchingGroup(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}
	groupProvider := &routerTestProvider{name: "group"}

	upstream := NewUpstream(
		"group-provider",
		groupProvider,
		WithProviderGroup("production"),
	)

	router := newTestRouter(
		t,
		defaultProvider,
		[]RoutingRule{routeRule("production")},
		upstream,
	)

	resp, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-2",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"group"}` {
		t.Fatalf("expected group provider, got %s", resp)
	}
}

type sequenceSelector struct {
	indexes []int
	pos     int
}

func (s *sequenceSelector) Select(candidates []*Upstream) (*Upstream, error) {
	if len(candidates) == 0 {
		return nil, ErrNoWeightedUpstream
	}

	if s.pos >= len(s.indexes) {
		return candidates[0], nil
	}

	index := s.indexes[s.pos]
	s.pos++

	if index < 0 || index >= len(candidates) {
		return nil, errors.New("test selector index out of range")
	}

	return candidates[index], nil
}
func TestRouterReturnsNoProviderWhenGroupHasNoCandidates(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}

	router := newTestRouter(
		t,
		defaultProvider,
		[]RoutingRule{routeRule("missing")},
	)

	_, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-3",
	)

	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func TestRouterUsesDefaultWithoutRouteContext(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}
	groupProvider := &routerTestProvider{name: "group"}

	upstream := NewUpstream(
		"group-provider",
		groupProvider,
		WithProviderGroup("production"),
	)

	router := newTestRouter(
		t,
		defaultProvider,
		[]RoutingRule{routeRule("production")},
		upstream,
	)

	resp, err := router.Forward(
		context.Background(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-4",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"default"}` {
		t.Fatalf("expected default provider, got %s", resp)
	}
}

func TestRouterFailsOverAfterPrimaryRetriesAreExhausted(t *testing.T) {
	primary := &routerTestProvider{
		name: "primary",
		errs: []error{
			&StatusError{Code: 503, Body: "unavailable"},
			&StatusError{Code: 503, Body: "unavailable"},
		},
	}
	fallback := &routerTestProvider{name: "fallback"}

	primaryUpstream := NewUpstream(
		"primary",
		primary,
		WithProviderGroup("production"),
		WithWeight(100),
		WithMaxRetries(1),
	)

	fallbackUpstream := NewUpstream(
		"fallback",
		fallback,
		WithProviderGroup("production"),
		WithWeight(1),
	)

	router := newTestRouter(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		primaryUpstream,
		fallbackUpstream,
	)

	resp, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-failover-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"fallback"}` {
		t.Fatalf("expected fallback provider, got %s", resp)
	}

	// The primary must consume its configured retry before failover.
	if primary.calls != 2 {
		t.Fatalf("expected 2 primary calls, got %d", primary.calls)
	}

	if fallback.calls != 1 {
		t.Fatalf("expected 1 fallback call, got %d", fallback.calls)
	}
}

func TestRouterDoesNotFailoverClientError(t *testing.T) {
	primary := &routerTestProvider{
		name: "primary",
		errs: []error{
			&StatusError{Code: 400, Body: "bad request"},
		},
	}
	fallback := &routerTestProvider{name: "fallback"}

	primaryUpstream := NewUpstream(
		"primary",
		primary,
		WithProviderGroup("production"),
		WithWeight(100),
		WithMaxRetries(3),
	)

	fallbackUpstream := NewUpstream(
		"fallback",
		fallback,
		WithProviderGroup("production"),
		WithWeight(1),
	)

	router := newTestRouter(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		primaryUpstream,
		fallbackUpstream,
	)

	_, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-failover-2",
	)
	if err == nil {
		t.Fatal("expected client error")
	}

	if primary.calls != 1 {
		t.Fatalf("expected one primary call, got %d", primary.calls)
	}

	if fallback.calls != 0 {
		t.Fatalf("client error must not fail over, got %d fallback calls", fallback.calls)
	}
}

func TestRouterFailsOverAcrossMultipleUpstreams(t *testing.T) {
	first := &routerTestProvider{
		name: "first",
		errs: []error{
			&StatusError{Code: 503, Body: "down"},
		},
	}
	second := &routerTestProvider{
		name: "second",
		errs: []error{
			&StatusError{Code: 503, Body: "down"},
		},
	}
	third := &routerTestProvider{name: "third"}

	upstreams := []*Upstream{
		NewUpstream(
			"first",
			first,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
		NewUpstream(
			"second",
			second,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
		NewUpstream(
			"third",
			third,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
	}

	router := newTestRouterWithSelector(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		&sequenceSelector{
			indexes: []int{0, 0, 0},
		},
		upstreams...,
	)

	resp, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-failover-3",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"third"}` {
		t.Fatalf("expected third provider, got %s", resp)
	}

	if first.calls != 1 || second.calls != 1 || third.calls != 1 {
		t.Fatalf(
			"expected one call each, got first=%d second=%d third=%d",
			first.calls,
			second.calls,
			third.calls,
		)
	}
}

func TestRouterReturnsLastFailoverError(t *testing.T) {
	first := &routerTestProvider{
		name: "first",
		errs: []error{
			&StatusError{Code: 503, Body: "first down"},
		},
	}
	second := &routerTestProvider{
		name: "second",
		errs: []error{
			&StatusError{Code: 503, Body: "second down"},
		},
	}

	router := newTestRouterWithSelector(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		&sequenceSelector{
			indexes: []int{0, 0},
		},
		NewUpstream(
			"first",
			first,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
		NewUpstream(
			"second",
			second,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
	)

	_, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-failover-4",
	)
	if err == nil {
		t.Fatal("expected final provider error")
	}

	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected StatusError, got %T: %v", err, err)
	}

	if statusErr.Code != 503 || statusErr.Body != "second down" {
		t.Fatalf("expected second upstream error, got %+v", statusErr)
	}
}

func TestRouterDoesNotReuseFailedUpstreamDuringFailover(t *testing.T) {
	first := &routerTestProvider{
		name: "first",
		errs: []error{
			&StatusError{Code: 503, Body: "down"},
			nil,
		},
	}
	second := &routerTestProvider{name: "second"}

	router := newTestRouterWithSelector(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		&sequenceSelector{
			indexes: []int{0, 0},
		},
		NewUpstream(
			"first",
			first,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
		NewUpstream(
			"second",
			second,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
	)

	resp, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		"req-failover-5",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"second"}` {
		t.Fatalf("expected second provider, got %s", resp)
	}

	if first.calls != 1 {
		t.Fatalf("failed upstream must not be selected again, got %d calls", first.calls)
	}
}

func TestRouterEmitsRoutingTelemetry(t *testing.T) {
	first := &routerTestProvider{
		name: "first",
		errs: []error{
			&StatusError{Code: 503, Body: "first down"},
		},
	}

	second := &routerTestProvider{
		name: "second",
	}

	router := newTestRouterWithSelector(
		t,
		nil,
		[]RoutingRule{routeRule("production")},
		&sequenceSelector{
			indexes: []int{0, 0},
		},
		NewUpstream(
			"first",
			first,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
		NewUpstream(
			"second",
			second,
			WithProviderGroup("production"),
			WithWeight(100),
			WithMaxRetries(0),
		),
	)

	const requestID = "req-routing-telemetry"

	resp, err := router.Forward(
		routeContext(),
		[]byte(`{"model":"gpt-4o"}`),
		requestID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp) != `{"provider":"second"}` {
		t.Fatalf("expected second provider, got %s", resp)
	}

	var routingDecision bool
	var routingFailover bool
	var providerFailover bool

	for _, event := range observability.DefaultBus.History() {
		if event.RequestID != requestID {
			continue
		}

		var data map[string]any
		if len(event.Data) > 0 {
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatalf("decode event %s: %v", event.Stage, err)
			}
		}

		switch event.Stage {
		case observability.StageRoutingDecision:
			routingDecision = true

			if data["route"] != "test route" {
				t.Fatalf("expected route test route, got %v", data["route"])
			}
			if data["selected_provider"] != "first" {
				t.Fatalf("expected selected provider first, got %v", data["selected_provider"])
			}
			if data["upstream"] != "first" {
				t.Fatalf("expected upstream first, got %v", data["upstream"])
			}
			if data["attempt"] != float64(1) {
				t.Fatalf("expected attempt 1, got %v", data["attempt"])
			}
			if data["failover"] != false {
				t.Fatalf("expected initial decision failover=false, got %v", data["failover"])
			}

		case observability.StageRoutingFailover:
			routingFailover = true

			if data["route"] != "test route" {
				t.Fatalf("expected failover route test route, got %v", data["route"])
			}
			if data["from"] != "first" {
				t.Fatalf("expected failover from first, got %v", data["from"])
			}
			if data["to"] != "second" {
				t.Fatalf("expected failover to second, got %v", data["to"])
			}
			if data["upstream"] != "second" {
				t.Fatalf("expected failover upstream second, got %v", data["upstream"])
			}
			if data["attempt"] != float64(2) {
				t.Fatalf("expected failover attempt 2, got %v", data["attempt"])
			}
			if data["failover"] != true {
				t.Fatalf("expected failover=true, got %v", data["failover"])
			}

		case observability.StageFailover:
			providerFailover = true
		}
	}

	if !routingDecision {
		t.Fatal("expected routing_decision event")
	}
	if !routingFailover {
		t.Fatal("expected routing_failover event")
	}
	if !providerFailover {
		t.Fatal("expected existing provider_failover event")
	}
}
