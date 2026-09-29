package guardrail

import (
	"context"
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type redactingTestGuardrail struct {
	name string
}

func (g *redactingTestGuardrail) Name() string { return g.name }

func (g *redactingTestGuardrail) Evaluate(_ context.Context, _ Direction, _ []byte) (*Result, error) {
	return &Result{
		Guardrail: g.name,
		Decision:  DecisionRedact,
		Message:   "redacted sensitive value",
		Redacted:  []byte(`{"model":"gpt-4o","redacted":true}`),
		Findings: []Finding{{
			Guardrail: g.name,
			Type:      "aws_key",
			Severity:  SeverityCritical,
			Entity:    "secret",
			Start:     10,
			End:       32,
			Value:     "sk-live-SECRET-123",
		}},
	}, nil
}

func TestPublishEventDoesNotLeakPayload(t *testing.T) {
	bus := observability.NewEventBus(100)
	g := &redactingTestGuardrail{name: "secrets"}

	registry := NewRegistry()
	registry.Register(g)

	cfg := Config{Strategies: map[string]Strategy{
		g.Name(): {Name: g.Name(), Mode: ModeEnforce, Enabled: true},
	}}
	ep := NewEnforcementPoint(registry, cfg, bus)

	set := GuardrailSet{Guards: []GuardrailSpec{{Name: g.Name()}}}
	input := []byte(`{"model":"gpt-4o"}`)

	if _, err := ep.Evaluate(context.Background(), "req-1", DirectionInput, input, set); err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	var evt *observability.Event
	history := bus.History()
	for i := range history {
		if history[i].Stage == observability.StageGuardrailInput {
			evt = &history[i]
			break
		}
	}
	if evt == nil {
		t.Fatal("no guardrail input event was published to the bus")
	}

	data := string(evt.Data)
	for _, leaked := range []string{"before", "after", "SECRET-123", "sk-live-SECRET-123", "gpt-4o", `{"model"`} {
		if strings.Contains(data, leaked) {
			t.Errorf("published guardrail event leaked %q: %s", leaked, data)
		}
	}

	if !strings.Contains(data, `"type"`) {
		t.Errorf("sanitized findings missing type field: %s", data)
	}
	if !strings.Contains(data, `"severity"`) {
		t.Errorf("sanitized findings missing severity field: %s", data)
	}
	if strings.Contains(data, `"value"`) {
		t.Errorf("sanitized findings must omit value field: %s", data)
	}
}
