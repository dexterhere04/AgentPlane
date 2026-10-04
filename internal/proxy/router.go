package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

var (
	ErrRoutingUnavailable = errors.New("proxy: routing unavailable")
	ErrNoProvider         = errors.New("proxy: no provider available")
)

type routingTelemetry struct {
	Route    string
	Upstream string
	Attempt  uint32
	Failover bool
}

type routingTelemetryContextKey struct{}

func withRoutingTelemetry(ctx context.Context, metadata routingTelemetry) context.Context {
	return context.WithValue(ctx, routingTelemetryContextKey{}, metadata)
}

func routingTelemetryFromContext(ctx context.Context) routingTelemetry {
	metadata, _ := ctx.Value(routingTelemetryContextKey{}).(routingTelemetry)
	return metadata
}

type routeRequestContextKey struct{}

type UpstreamSelector interface {
	Select([]*Upstream) (*Upstream, error)
}

type Router struct {
	defaultProvider Provider

	rulesMu sync.RWMutex
	rules   *RuleSet

	registry *Registry
	selector UpstreamSelector
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

func (r *Router) SetRuleSet(rules *RuleSet) {
	r.rulesMu.Lock()
	defer r.rulesMu.Unlock()

	r.rules = rules
}

func (r *Router) ruleSet() *RuleSet {
	r.rulesMu.RLock()
	defer r.rulesMu.RUnlock()

	return r.rules
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

	rules := r.ruleSet()

	if rules == nil {
		return r.forwardDefault(ctx, body, requestID)
	}

	routeReq, ok := routeRequestFromContext(ctx)
	if !ok {
		return r.forwardDefault(ctx, body, requestID)
	}

	rule, matched, err := rules.Match(routeReq)
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
		rule.Name,
		candidates,
		resilienceConfig,
	)
}

func (r *Router) forwardWithFailover(
	ctx context.Context,
	body []byte,
	requestID string,
	route string,
	candidates []*Upstream,
	resilienceConfig ResilienceConfig,
) ([]byte, error) {
	remaining := append([]*Upstream(nil), candidates...)
	var lastErr error
	var attempt uint32 = 1
	var previous *Upstream
	failover := false

	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		selected, err := r.selector.Select(remaining)
		if err != nil {
			return nil, err
		}
		if selected == nil {
			return nil, ErrNoProvider
		}

		telemetryCtx := withRoutingTelemetry(ctx, routingTelemetry{
			Route:    route,
			Upstream: selected.Name,
			Attempt:  attempt,
			Failover: failover,
		})

		if !failover {
			observability.DefaultBus.Publish(
				observability.NewDataEvent(
					requestID,
					observability.StageRoutingDecision,
					"selected",
					map[string]any{
						"route":             route,
						"selected_provider": selected.Name,
						"upstream":          selected.Name,
						"attempt":           attempt,
						"failover":          false,
					},
				),
			)
		} else {
			observability.DefaultBus.Publish(
				observability.NewDataEvent(
					requestID,
					observability.StageRoutingFailover,
					"selected",
					map[string]any{
						"route":    route,
						"from":     previous.Name,
						"to":       selected.Name,
						"upstream": selected.Name,
						"attempt":  attempt,
						"failover": true,
					},
				),
			)
		}

		resp, err := selected.ForwardWithConfig(
			telemetryCtx,
			body,
			requestID,
			resilienceConfig,
		)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return nil, err
		}

		if !failoverEligible(err) {
			return nil, err
		}

		previous = selected
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
					"route":         route,
					"from_upstream": selected.Name,
					"reason":        err.Error(),
					"remaining":     upstreamNames(remaining),
					"attempt":       attempt,
					"failover":      true,
				},
			),
		)

		attempt++
		failover = true
	}

	return nil, lastErr
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
