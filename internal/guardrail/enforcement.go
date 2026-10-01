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

// Evaluate runs the guardrails in set in order for the given direction.
//
// The behavior of each guardrail is governed by GuardrailSpec.Required:
//
//   - Required guards always run — even when their strategy is disabled — and
//     fail closed: a resolution error (guardrail not registered) or an
//     evaluation error returns DecisionBlock along with the error, so the
//     caller blocks the request instead of proceeding.
//   - Non-Required guards run only when their strategy is enabled, and are
//     skipped on error (fail open): the error is recorded and published, then
//     evaluation continues with the next guardrail.
//
// The pipeline short-circuits on DecisionBlock. A DecisionRedact replaces the
// body for subsequent guards and is returned as the result's Redacted body.
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
				return ep.failClosed(requestID, g.Name(), dir, err)
			}

			if result.Decision == DecisionPass {
				if ep.metrics != nil {
					ep.metrics.RecordEvaluation(result.Decision, time.Duration(0))
				}
				ep.publishEvent(requestID, g.Name(), dir, result)
				continue
			}

			effective := ep.resolve(strategy, result)
			if ep.metrics != nil {
				ep.metrics.RecordEvaluation(effective.Decision, time.Duration(0))
			}
			ep.publishEvent(requestID, g.Name(), dir, effective)

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
			}, "error")
			continue
		}

		if result.Decision == DecisionPass {
			if ep.metrics != nil {
				ep.metrics.RecordEvaluation(result.Decision, latency)
			}
			ep.publishEvent(requestID, g.Name(), dir, result)
			continue
		}

		effective := ep.resolve(strategy, result)
		if ep.metrics != nil {
			ep.metrics.RecordEvaluation(effective.Decision, latency)
		}
		ep.publishEvent(requestID, g.Name(), dir, effective)

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
	if ep.metrics != nil {
		ep.metrics.RecordError()
		ep.metrics.RecordEvaluation(DecisionBlock, 0)
	}
	ep.publishEvent(requestID, name, dir, &Result{
		Guardrail: name,
		Decision:  DecisionBlock,
		Message:   fmt.Sprintf("%s: %v", ErrGuardrailUnavailable, err),
	})
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

// ValidateSet verifies that every Required guardrail in set resolves in the
// registry. Non-Required specs are ignored: they are optional, run only when
// enabled, and are skipped on error, so a missing optional guardrail is not a
// misconfiguration. It is intended to be called at startup so a Required guard
// that cannot be resolved fails fast rather than at request time.
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

func (ep *EnforcementPoint) publishEvent(requestID, guardrail string, dir Direction, result *Result, actionOverride ...string) {
	stage := observability.StageGuardrailInput
	if dir == DirectionOutput {
		stage = observability.StageGuardrailOutput
	}

	phase := dir.String()
	if phase != DirectionInput.String() && phase != DirectionOutput.String() {
		phase = DirectionInput.String()
	}
	action := result.Decision.String()
	if len(actionOverride) > 0 {
		action = actionOverride[0]
	}
	ep.captureGuardrail(requestID, guardrail, phase, action)

	if ep.bus == nil {
		return
	}

	decision := result.Decision.String()

	// The event bus is streamed unauthenticated: publish only a capped,
	// details-free message and a sanitized projection of findings (no matched
	// secret/PII values).
	msg := capPublishedMessage(result.Message)

	e := observability.NewMessageEvent(requestID, stage, "info",
		fmt.Sprintf("%s: %s", guardrail, decision))
	if result.Decision != DecisionPass && msg != "" {
		e.Message = fmt.Sprintf("%s: %s — %s", guardrail, decision, msg)
	}

	data := map[string]any{
		"guardrail": guardrail,
		"decision":  decision,
		"direction": dir.String(),
	}
	if msg != "" {
		data["message"] = msg
	}
	if len(result.Findings) > 0 {
		data["findings"] = sanitizeFindings(result.Findings)
	}
	if raw, err := json.Marshal(data); err == nil {
		e.Data = raw
	}

	ep.bus.Publish(e)
}

const maxPublishedMessage = 500

func capPublishedMessage(s string) string {
	if len(s) > maxPublishedMessage {
		return s[:maxPublishedMessage]
	}
	return s
}

// sanitizeFindings projects findings for bus publication, omitting the
// matched Value which may contain a raw secret or PII snippet.
func sanitizeFindings(findings []Finding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, f := range findings {
		out = append(out, map[string]any{
			"guardrail": f.Guardrail,
			"type":      f.Type,
			"severity":  string(f.Severity),
			"entity":    f.Entity,
			"start":     f.Start,
			"end":       f.End,
		})
	}
	return out
}

// captureGuardrail persists the outcome to the configured observability store
// (ClickHouse) without blocking the request path. No-op when unset.
func (ep *EnforcementPoint) captureGuardrail(requestID, guardrail, phase, action string) {
	if observability.DefaultStore == nil {
		return
	}
	evt := observability.GuardrailEvent{
		TraceID:   requestID,
		RequestID: requestID,
		Timestamp: time.Now(),
		Rule:      guardrail,
		Phase:     phase,
		Action:    action,
	}
	go observability.CaptureGuardrail(evt)
}

func (ep *EnforcementPoint) publishBlocked(requestID string, result *Result) {
	if ep.bus == nil {
		return
	}

	e := observability.NewMessageEvent(requestID, observability.StageGuardrailBlocked, "error",
		fmt.Sprintf("%s blocked request: %s", result.Guardrail, result.Message))
	ep.bus.Publish(e)
}
