package proxy

import (
	"context"
	"errors"
)

var (
	ErrRoutingUnavailable = errors.New("proxy: routing unavailable")
	ErrNoProvider         = errors.New("proxy: no provider available")
)

type routeRequestContextKey struct{}

type Router struct {
	defaultProvider Provider
	rules           *RuleSet
	registry        *Registry
	selector        *WeightedSelector
}

func NewRouter(
	defaultProvider Provider,
	rules *RuleSet,
	registry *Registry,
	selector *WeightedSelector,
) *Router {
	return &Router{
		defaultProvider: defaultProvider,
		rules:           rules,
		registry:        registry,
		selector:        selector,
	}
}

// WithRouteRequest attaches the resolved routing context to a request context.
func WithRouteRequest(ctx context.Context, req RouteRequest) context.Context {
	return context.WithValue(ctx, routeRequestContextKey{}, req)
}

func routeRequestFromContext(ctx context.Context) (RouteRequest, bool) {
	req, ok := ctx.Value(routeRequestContextKey{}).(RouteRequest)
	return req, ok
}

func (r *Router) Name() string {
	return "router"
}

// Forward implements Provider.
//
// Task 4 responsibilities:
//  1. resolve the routing request from context
//  2. find the first matching rule
//  3. resolve the provider group
//  4. filter eligible upstreams
//  5. perform weighted selection
//  6. forward through the selected upstream
//
// Cross-upstream failover is intentionally not implemented here yet.
// Upstream.Forward already owns retry, timeout and circuit-breaker behavior.
func (r *Router) Forward(
	ctx context.Context,
	body []byte,
	requestID string,
) ([]byte, error) {
	if r == nil {
		return nil, ErrRoutingUnavailable
	}

	// No routing configuration means use the configured default provider.
	if r.rules == nil {
		return r.forwardDefault(ctx, body, requestID)
	}

	routeReq, ok := routeRequestFromContext(ctx)
	if !ok {
		return r.forwardDefault(ctx, body, requestID)
	}

	rule, matched, err := r.rules.Match(routeReq)
	if err != nil {
		return nil, err
	}

	if !matched || rule.ProviderGroup == "" {
		return r.forwardDefault(ctx, body, requestID)
	}

	if r.registry == nil || r.selector == nil {
		return nil, ErrRoutingUnavailable
	}

	model := routeReq.Model
	if model == "" {
		model = ResolveModel(body)
	}

	candidates := r.registry.Candidates(
		model,
		rule.ProviderGroup,
	)

	if len(candidates) == 0 {
		return nil, ErrNoProvider
	}

	selected, err := r.selector.Select(candidates)
	if err != nil {
		return nil, err
	}

	if selected == nil {
		return nil, ErrNoProvider
	}

	return selected.Forward(ctx, body, requestID)
}

func (r *Router) forwardDefault(
	ctx context.Context,
	body []byte,
	requestID string,
) ([]byte, error) {
	if r.defaultProvider == nil {
		return nil, ErrNoProvider
	}

	return r.defaultProvider.Forward(ctx, body, requestID)
}
