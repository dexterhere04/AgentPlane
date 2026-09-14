package guardrail

import (
	"bytes"
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
			if spec.Required {
				return ep.failClosed(requestID, spec.Name, dir, fmt.Errorf("%s: %w", ErrGuardrailUnavailable, err))
			}
			continue
		}

		if spec.Required {
			strategy := ep.config.Strategy(g.Name())

			result, err := g.Evaluate(ctx, dir, currentBody)
			if err != nil {
				if ep.metrics != nil {
					ep.metrics.RecordError()
				}
				return ep.failClosed(requestID, g.Name(), dir, err)
			}

			if ep.metrics != nil {
				ep.metrics.RecordEvaluation(result.Decision, time.Duration(0))
			}

			if result.Decision == DecisionPass {
				ep.publishEvent(requestID, g.Name(), dir, result, currentBody)
				continue
			}

			ep.publishEvent(requestID, g.Name(), dir, result, currentBody)

			effective := ep.resolve(strategy, result)

			if effective.Decision == DecisionBlock {
				ep.publishBlocked(requestID, effective)
				return effective, nil
			}

			if effective.Decision == DecisionRedact && effective.Redacted != nil {
				currentBody = effective.Redacted
			}
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
			ep.publishEvent(requestID, g.Name(), dir, &Result{
				Guardrail: g.Name(),
				Decision:  DecisionPass,
				Message:   fmt.Sprintf("error: %v", err),
			}, currentBody)
			continue
		}

		if ep.metrics != nil {
			ep.metrics.RecordEvaluation(result.Decision, latency)
		}

		if result.Decision == DecisionPass {
			ep.publishEvent(requestID, g.Name(), dir, result, currentBody)
			continue
		}

		ep.publishEvent(requestID, g.Name(), dir, result, currentBody)

		effective := ep.resolve(strategy, result)

		if effective.Decision == DecisionBlock {
			ep.publishBlocked(requestID, effective)
			return effective, nil
		}

		if effective.Decision == DecisionRedact && effective.Redacted != nil {
			currentBody = effective.Redacted
		}
	}

	if !bytes.Equal(currentBody, body) {
		return &Result{
			Decision: DecisionRedact,
			Redacted: currentBody,
		}, nil
	}

	return &Result{Decision: DecisionPass}, nil
}

func (ep *EnforcementPoint) failClosed(requestID, name string, dir Direction, err error) (*Result, error) {
	ep.publishEvent(requestID, name, dir, &Result{
		Guardrail: name,
		Decision:  DecisionBlock,
		Message:   fmt.Sprintf("%s: %v", ErrGuardrailUnavailable, err),
	}, nil)
	ep.publishBlocked(requestID, &Result{
		Guardrail: name,
		Decision:  DecisionBlock,
		Message:   ErrGuardrailUnavailable.Error(),
	})
	return &Result{
		Guardrail: name,
		Decision:  DecisionBlock,
		Message:   ErrGuardrailUnavailable.Error(),
	}, err
}

func (ep *EnforcementPoint) ValidateSet(set GuardrailSet) error {
	for _, spec := range set.Guards {
		if !spec.Required {
			continue
		}
		if _, err := ep.registry.Resolve(spec); err != nil {
			return fmt.Errorf("required guardrail %q is not registered: %w", spec.Name, err)
		}
	}
	return nil
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

func (ep *EnforcementPoint) publishEvent(requestID, guardrail string, dir Direction, result *Result, before []byte) {
	stage := observability.StageGuardrailInput
	if dir == DirectionOutput {
		stage = observability.StageGuardrailOutput
	}

	decision := result.Decision.String()
	msg := result.Message
	if result.Details != nil {
		if detailsJSON, err := json.Marshal(result.Details); err == nil {
			msg = msg + " " + string(detailsJSON)
		}
	}

	ep.captureGuardrail(requestID, guardrail, dir, result, msg)

	if ep.bus == nil {
		return
	}

	e := observability.NewMessageEvent(requestID, stage, "info",
		fmt.Sprintf("%s: %s", guardrail, decision))
	if result.Decision != DecisionPass && result.Message != "" {
		e.Message = fmt.Sprintf("%s: %s — %s", guardrail, decision, msg)
	}

	// Structured payload for the UI: decision, findings, and before/after
	// body snapshots so redactions/transformations can be visualized.
	data := map[string]any{
		"guardrail": guardrail,
		"decision":  decision,
		"direction": dir.String(),
	}
	if result.Message != "" {
		data["message"] = result.Message
	}
	if len(result.Findings) > 0 {
		data["findings"] = result.Findings
	}
	if before != nil {
		data["before"] = truncatePayload(before)
	}
	if result.Redacted != nil && !bytes.Equal(result.Redacted, before) {
		data["after"] = truncatePayload(result.Redacted)
	}
	if raw, err := json.Marshal(data); err == nil {
		e.Data = raw
	}

	ep.bus.Publish(e)
}

const maxPayloadSnapshot = 2000

func truncatePayload(b []byte) string {
	s := string(b)
	if len(s) > maxPayloadSnapshot {
		return s[:maxPayloadSnapshot] + "…"
	}
	return s
}

// captureGuardrail persists the outcome to the configured observability store
// (ClickHouse) without blocking the request path. No-op when unset.
func (ep *EnforcementPoint) captureGuardrail(requestID, guardrail string, dir Direction, result *Result, msg string) {
	if observability.DefaultStore == nil {
		return
	}
	evt := observability.GuardrailEvent{
		TraceID:   requestID,
		RequestID: requestID,
		Timestamp: time.Now(),
		Rule:      guardrail,
		Action:    guardrailActionString(result.Decision),
		Details:   msg,
		Phase:     dir.String(),
	}
	go observability.CaptureGuardrail(evt)
}

func guardrailActionString(d Decision) string {
	switch d {
	case DecisionBlock:
		return "blocked"
	case DecisionRedact:
		return "redacted"
	case DecisionWarn:
		return "warn"
	case DecisionLogOnly:
		return "log_only"
	default:
		return "allowed"
	}
}

func (ep *EnforcementPoint) publishBlocked(requestID string, result *Result) {
	if ep.bus == nil {
		return
	}

	e := observability.NewMessageEvent(requestID, observability.StageGuardrailBlocked, "error",
		fmt.Sprintf("%s blocked request: %s", result.Guardrail, result.Message))
	ep.bus.Publish(e)
}
