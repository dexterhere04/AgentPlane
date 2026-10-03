package proxy

import (
	"context"
	"errors"
	"testing"
)

type routerTestProvider struct {
	name  string
	calls int
}

func (p *routerTestProvider) Name() string {
	return p.name
}

func (p *routerTestProvider) Forward(
	ctx context.Context,
	body []byte,
	requestID string,
) ([]byte, error) {
	p.calls++
	return []byte(p.name), nil
}

func TestRouterUsesDefaultProviderWhenNoRuleMatches(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}

	rules, err := NewRuleSet([]RoutingRule{
		{
			ID:            "rule-1",
			Name:          "gpt",
			Priority:      1,
			Action:        "route",
			ModelMatcher:  []byte(`{"equals":"gpt-5"}`),
			ProviderGroup: "premium",
			Enabled:       true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(
		defaultProvider,
		rules,
		NewRegistry(),
		NewWeightedSelector(1),
	)

	ctx := WithRouteRequest(
		context.Background(),
		RouteRequest{
			Model:    "gpt-4",
			UserID:   "user-1",
			Username: "alice",
		},
	)

	got, err := router.Forward(ctx, []byte(`{"model":"gpt-4"}`), "req-1")
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "default" {
		t.Fatalf("expected default provider, got %q", got)
	}

	if defaultProvider.calls != 1 {
		t.Fatalf("expected one default provider call, got %d", defaultProvider.calls)
	}
}

func TestRouterSelectsProviderFromMatchingGroup(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}
	groupProvider := &routerTestProvider{name: "group-a"}

	upstream := NewUpstream(
		"group-a-upstream",
		groupProvider,
		WithProviderGroup("premium"),
		WithWeight(1),
	)

	registry := NewRegistry()
	if err := registry.Register(upstream); err != nil {
		t.Fatal(err)
	}

	rules, err := NewRuleSet([]RoutingRule{
		{
			ID:            "rule-1",
			Name:          "premium-gpt",
			Action:        "route",
			Priority:      1,
			ModelMatcher:  []byte(`{"equals":"gpt-5"}`),
			ProviderGroup: "premium",
			Enabled:       true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(
		defaultProvider,
		rules,
		registry,
		NewWeightedSelector(1),
	)

	ctx := WithRouteRequest(
		context.Background(),
		RouteRequest{
			Model:    "gpt-5",
			UserID:   "user-1",
			Username: "alice",
		},
	)

	got, err := router.Forward(
		ctx,
		[]byte(`{"model":"gpt-5"}`),
		"req-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "group-a" {
		t.Fatalf("expected group provider, got %q", got)
	}

	if groupProvider.calls != 1 {
		t.Fatalf("expected one group provider call, got %d", groupProvider.calls)
	}

	if defaultProvider.calls != 0 {
		t.Fatalf("expected default provider to be unused, got %d calls", defaultProvider.calls)
	}
}

func TestRouterReturnsNoProviderWhenGroupHasNoCandidates(t *testing.T) {
	rules, err := NewRuleSet([]RoutingRule{
		{
			ID:            "rule-1",
			Name:          "premium",
			Action:        "route",
			Priority:      1,
			ModelMatcher:  []byte(`{"equals":"gpt-5"}`),
			ProviderGroup: "premium",
			Enabled:       true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(
		&routerTestProvider{name: "default"},
		rules,
		NewRegistry(),
		NewWeightedSelector(1),
	)

	ctx := WithRouteRequest(
		context.Background(),
		RouteRequest{Model: "gpt-5"},
	)

	_, err = router.Forward(
		ctx,
		[]byte(`{"model":"gpt-5"}`),
		"req-1",
	)

	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func TestRouterUsesDefaultWithoutRouteContext(t *testing.T) {
	defaultProvider := &routerTestProvider{name: "default"}

	router := NewRouter(
		defaultProvider,
		nil,
		nil,
		nil,
	)

	got, err := router.Forward(
		context.Background(),
		[]byte(`{"model":"gpt-5"}`),
		"req-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "default" {
		t.Fatalf("expected default provider, got %q", got)
	}
}
