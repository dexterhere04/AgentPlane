package guardrail

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type EnforcementPoint struct {
	registry *Registry
	metrics  *Metrics
	bus      *observability.EventBus
	config   Config
}

func NewEnforcementPoint(registry *Registry, cfg Config, bus *observability.EventBus) *EnforcementPoint {
	return &EnforcementPoint{
		registry: registry,
		config:   cfg,
		bus:      bus,
		metrics:  DefaultMetrics,
	}
}

func (ep *EnforcementPoint) SetMetrics(m *Metrics) {
	ep.metrics = m
}

func (ep *EnforcementPoint) Evaluate(
	ctx context.Context,
	requestID string,
	dir Direction,
	body []byte,
	set GuardrailSet,
) (*Result, error) {
	currentBody := body

	for _, spec := range set.Guards {
		g, err := ep.registry.Resolve(spec)
		if err != nil {
			continue
		}

		strategy := ep.config.Strategy(g.Name())
		if !strategy.Enabled {
			continue
		}

		startTime := time.Now()

		result, err := g.Evaluate(ctx, dir, currentBody)

		latency := time.Since(startTime)

		if err != nil {
			if ep.metrics != nil {
				ep.metrics.RecordError()
			}
			if g.Type() == TypeMandatory {
				ep.publishEvent(requestID, g.Name(), dir, &Result{
					Guardrail: g.Name(),
					Decision:  DecisionBlock,
					Message:   fmt.Sprintf("%s: %v", ErrGuardrailUnavailable, err),
				})
				ep.publishBlocked(requestID, &Result{
					Guardrail: g.Name(),
					Decision:  DecisionBlock,
					Message:   ErrGuardrailUnavailable.Error(),
				})
				return &Result{
					Guardrail: g.Name(),
					Decision:  DecisionBlock,
					Message:   ErrGuardrailUnavailable.Error(),
				}, nil
			}
			ep.publishEvent(requestID, g.Name(), dir, &Result{
				Guardrail: g.Name(),
				Decision:  DecisionPass,
				Message:   fmt.Sprintf("error: %v", err),
			})
			continue
		}

		if ep.metrics != nil {
			ep.metrics.RecordEvaluation(result.Decision, latency)
		}

		if result.Decision == DecisionPass {
			ep.publishEvent(requestID, g.Name(), dir, result)
			continue
		}

		ep.publishEvent(requestID, g.Name(), dir, result)

		effective := ep.resolve(strategy, result)

		if effective.Decision == DecisionBlock {
			ep.publishBlocked(requestID, effective)
			return effective, nil
		}

		if effective.Decision == DecisionRedact && effective.Redacted != nil {
			currentBody = effective.Redacted
		}
	}

	if len(currentBody) != len(body) {
		return &Result{
			Decision: DecisionRedact,
			Redacted: currentBody,
		}, nil
	}

	return &Result{Decision: DecisionPass}, nil
}

func (ep *EnforcementPoint) resolve(strategy Strategy, result *Result) *Result {
	resolved := *result

	switch strategy.Mode {
	case ModeEnforce:
		return &resolved
	case ModeLogOnly:
		resolved.Decision = DecisionLogOnly
		resolved.Redacted = nil
		return &resolved
	case ModeWarn:
		if resolved.Decision == DecisionBlock {
			resolved.Decision = DecisionWarn
			resolved.Redacted = nil
		}
		if resolved.Decision == DecisionRedact {
			resolved.Decision = DecisionLogOnly
			resolved.Redacted = nil
		}
		return &resolved
	default:
		return &resolved
	}
}

func (ep *EnforcementPoint) publishEvent(requestID, guardrail string, dir Direction, result *Result) {
	if ep.bus == nil {
		return
	}

	var stage observability.Stage
	switch dir {
	case DirectionInput:
		stage = observability.StageGuardrailInput
	case DirectionOutput:
		stage = observability.StageGuardrailOutput
	default:
		stage = observability.StageGuardrailInput
	}

	decision := result.Decision.String()
	msg := result.Message
	if result.Details != nil {
		if detailsJSON, err := json.Marshal(result.Details); err == nil {
			msg = msg + " " + string(detailsJSON)
		}
	}

	e := observability.NewMessageEvent(requestID, stage, "info",
		fmt.Sprintf("%s: %s", guardrail, decision))
	if result.Decision != DecisionPass && result.Message != "" {
		e.Message = fmt.Sprintf("%s: %s — %s", guardrail, decision, msg)
	}
	ep.bus.Publish(e)
}

func (ep *EnforcementPoint) publishBlocked(requestID string, result *Result) {
	if ep.bus == nil {
		return
	}

	e := observability.NewMessageEvent(requestID, observability.StageGuardrailBlocked, "error",
		fmt.Sprintf("%s blocked request: %s", result.Guardrail, result.Message))
	ep.bus.Publish(e)
}
