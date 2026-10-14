package guardrail

import (
	"context"
	"testing"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type telemetryTestStore struct {
	guardrails chan observability.GuardrailEvent
}

func (s *telemetryTestStore) Init() error                          { return nil }
func (s *telemetryTestStore) Close() error                         { return nil }
func (s *telemetryTestStore) StoreTrace(observability.Trace) error { return nil }
func (s *telemetryTestStore) StorePromptPayload(observability.Payload) (string, error) {
	return "", nil
}
func (s *telemetryTestStore) StoreResponsePayload(observability.Payload) (string, error) {
	return "", nil
}
func (s *telemetryTestStore) StoreToolCall(observability.ToolCall) error { return nil }
func (s *telemetryTestStore) StoreGuardrail(event observability.GuardrailEvent) error {
	s.guardrails <- event
	return nil
}
func (s *telemetryTestStore) StoreUsage(observability.UsageEvent) error { return nil }

type telemetryTestGuardrail struct{}

func (telemetryTestGuardrail) Name() string { return "test_rule" }
func (telemetryTestGuardrail) Evaluate(context.Context, Direction, []byte) (*Result, error) {
	return &Result{Guardrail: "test_rule", Decision: DecisionBlock, Message: "sensitive finding must not persist"}, nil
}

func TestEnforcementPersistsSafePhaseAwareGuardrailTelemetry(t *testing.T) {
	oldStore := observability.DefaultStore
	store := &telemetryTestStore{guardrails: make(chan observability.GuardrailEvent, 1)}
	observability.SetStore(store)
	t.Cleanup(func() { observability.SetStore(oldStore) })

	registry := NewRegistry()
	registry.Register(telemetryTestGuardrail{})
	point := NewEnforcementPoint(registry, Config{Strategies: map[string]Strategy{
		"test_rule": {Name: "test_rule", Mode: ModeEnforce, Enabled: true},
	}}, nil)
	result, err := point.Evaluate(context.Background(), "req-test", DirectionOutput, []byte("content"), GuardrailSet{
		Guards: []GuardrailSpec{{Name: "test_rule"}},
	})
	if err != nil || result.Decision != DecisionBlock {
		t.Fatalf("Evaluate result=%+v err=%v", result, err)
	}
	select {
	case event := <-store.guardrails:
		if event.TraceID != "req-test" || event.RequestID != "req-test" || event.Phase != "output" || event.Action != "block" {
			t.Fatalf("persisted event = %+v", event)
		}
		if event.Details != "" || event.Timestamp.IsZero() {
			t.Fatalf("unsafe or incomplete event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("guardrail telemetry was not persisted")
	}
}

func TestEnforcementPersistsEffectiveGuardrailAction(t *testing.T) {
	oldStore := observability.DefaultStore
	store := &telemetryTestStore{guardrails: make(chan observability.GuardrailEvent, 3)}
	observability.SetStore(store)
	t.Cleanup(func() { observability.SetStore(oldStore) })

	registry := NewRegistry()
	registry.Register(telemetryTestGuardrail{})
	metrics := &Metrics{}
	tests := []struct {
		mode   Mode
		result Decision
		action string
	}{
		{ModeEnforce, DecisionBlock, "block"},
		{ModeLogOnly, DecisionPass, "log_only"},
		{ModeWarn, DecisionPass, "warn"},
	}
	for _, test := range tests {
		point := NewEnforcementPoint(registry, Config{Strategies: map[string]Strategy{
			"test_rule": {Name: "test_rule", Mode: test.mode, Enabled: true},
		}}, nil)
		point.SetMetrics(metrics)
		result, err := point.Evaluate(context.Background(), "req-policy", DirectionInput, []byte("content"), GuardrailSet{
			Guards: []GuardrailSpec{{Name: "test_rule"}},
		})
		if err != nil || result.Decision != test.result {
			t.Fatalf("mode %s result=%+v err=%v", test.mode, result, err)
		}
		select {
		case event := <-store.guardrails:
			if event.Action != test.action || event.Phase != "input" || event.TraceID != "req-policy" {
				t.Fatalf("mode %s persisted event = %+v", test.mode, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("mode %s did not persist telemetry", test.mode)
		}
	}
	snapshot := metrics.Snapshot()
	if snapshot.BlocksTotal != 1 || snapshot.WarnsTotal != 1 || snapshot.PassedTotal != 1 || snapshot.EvaluationsTotal != 3 {
		t.Fatalf("effective metrics = %+v", snapshot)
	}
}

func TestFailClosedUpdatesGuardrailMetricsOnce(t *testing.T) {
	metrics := &Metrics{}
	point := NewEnforcementPoint(NewRegistry(), Config{}, nil)
	point.SetMetrics(metrics)
	result, err := point.Evaluate(context.Background(), "req-missing-rule", DirectionInput, []byte("content"), GuardrailSet{
		Guards: []GuardrailSpec{{Name: "required-rule", Required: true}},
	})
	if err == nil || result.Decision != DecisionBlock {
		t.Fatalf("fail-closed result=%+v err=%v", result, err)
	}
	snapshot := metrics.Snapshot()
	if snapshot.EvaluationsTotal != 1 || snapshot.BlocksTotal != 1 || snapshot.ErrorsTotal != 1 {
		t.Fatalf("fail-closed metrics = %+v", snapshot)
	}
}
