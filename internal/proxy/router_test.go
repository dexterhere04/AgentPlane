package proxy

import (
	"context"
	"errors"
	"testing"
)

type routerTestProvider struct {
	name  string
	errs  []error
	calls int
}

func (p *routerTestProvider) Name() string {
	return p.name
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
