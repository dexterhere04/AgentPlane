package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

var (
	ErrRoutingUnavailable = errors.New("proxy: routing unavailable")
	ErrNoProvider         = errors.New("proxy: no provider available")
)

type routeRequestContextKey struct{}

type UpstreamSelector interface {
	Select([]*Upstream) (*Upstream, error)
}

type Router struct {
	defaultProvider Provider
	rules           *RuleSet
	registry        *Registry
	selector        UpstreamSelector
}

func NewRouter(
	defaultProvider Provider,
	rules *RuleSet,
	registry *Registry,
	selector UpstreamSelector,
) *Router {
	return &Router{
		defaultProvider: defaultProvider,
		rules:           rules,
		registry:        registry,
		selector:        selector,
	}
}

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

func (r *Router) Forward(
	ctx context.Context,
	body []byte,
	requestID string,
) ([]byte, error) {
	if r == nil {
		return nil, ErrRoutingUnavailable
	}

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

	if !matched {
		return r.forwardDefault(ctx, body, requestID)
	}

	if r.registry == nil || r.selector == nil {
		return nil, ErrRoutingUnavailable
	}

	model := routeReq.Model
	if model == "" {
		model = ResolveModel(body)
	}

	candidates := r.registry.Candidates(model, rule.ProviderGroup)
	if len(candidates) == 0 {
		return nil, ErrNoProvider
	}

	resilienceConfig, err := rule.ResilienceConfig()
	if err != nil {
		return nil, err
	}

	return r.forwardWithFailover(
		ctx,
		body,
		requestID,
		candidates,
		resilienceConfig,
	)
}

func (r *Router) forwardWithFailover(
	ctx context.Context,
	body []byte,
	requestID string,
	candidates []*Upstream,
	resilienceConfig ResilienceConfig,
) ([]byte, error) {
	remaining := append([]*Upstream(nil), candidates...)

	var lastErr error

	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		selected, err := r.selector.Select(remaining)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

		if selected == nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ErrNoProvider
		}

		resp, err := selected.ForwardWithConfig(
			ctx,
			body,
			requestID,
			resilienceConfig,
		)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// The caller cancelled or its deadline expired. Do not fail over
		// because the request itself is no longer viable.
		if ctx.Err() != nil {
			return nil, err
		}

		// A client-side/non-retryable provider error should be returned
		// directly. Cross-provider failover is reserved for infrastructure
		// and transient provider failures.
		if !failoverEligible(err) {
			return nil, err
		}

		remaining = removeUpstream(remaining, selected)
		if len(remaining) == 0 {
			return nil, lastErr
		}

		observability.DefaultBus.Publish(
			observability.NewDataEvent(
				requestID,
				observability.StageFailover,
				"started",
				map[string]any{
					"from_upstream": selected.Name,
					"reason":        err.Error(),
					"remaining":     upstreamNames(remaining),
				},
			),
		)
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, ErrNoProvider
}

func failoverEligible(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	retryable, _ := classify(err)
	return retryable
}

func removeUpstream(
	candidates []*Upstream,
	selected *Upstream,
) []*Upstream {
	remaining := make([]*Upstream, 0, len(candidates)-1)

	for _, candidate := range candidates {
		if candidate != selected {
			remaining = append(remaining, candidate)
		}
	}

	return remaining
}

func upstreamNames(candidates []*Upstream) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil {
			names = append(names, candidate.Name)
		}
	}
	return names
}

func (r *Router) forwardDefault(
	ctx context.Context,
	body []byte,
	requestID string,
) ([]byte, error) {
	if r.defaultProvider == nil {
		return nil, fmt.Errorf("%w: default provider", ErrNoProvider)
	}

	return r.defaultProvider.Forward(ctx, body, requestID)
}
