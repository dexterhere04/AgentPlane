package functions

import (
	"context"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

func TestModelWhitelistGuardrailEvaluate(t *testing.T) {
	configured := &ModelWhitelistGuardrail{
		name:      "model_whitelist",
		whitelist: []string{"gpt-4o", "gpt-4o-mini"},
	}
	unconfigured := &ModelWhitelistGuardrail{
		name:      "model_whitelist",
		whitelist: nil,
	}

	tests := []struct {
		name string
		g    *ModelWhitelistGuardrail
		body string
		want guardrail.Decision
	}{
		{"allowed model", configured, `{"model":"gpt-4o"}`, guardrail.DecisionPass},
		{"allowed model with messages", configured, `{"model":"gpt-4o","messages":[]}`, guardrail.DecisionPass},
		{"denied model", configured, `{"model":"evil-model"}`, guardrail.DecisionBlock},
		{"missing model", configured, `{"messages":[{"role":"user","content":"hi"}]}`, guardrail.DecisionBlock},
		{"invalid json", configured, `not-json`, guardrail.DecisionBlock},
		{"unconfigured missing model", unconfigured, `{"messages":[]}`, guardrail.DecisionPass},
		{"case insensitive", configured, `{"model":"GPT-4O"}`, guardrail.DecisionPass},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.g.Evaluate(context.Background(), guardrail.DirectionInput, []byte(tt.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Decision != tt.want {
				t.Fatalf("decision = %v, want %v", result.Decision, tt.want)
			}
		})
	}
}

func TestModelRulesGuardrailEvaluate(t *testing.T) {
	configured := &ModelRulesGuardrail{
		name:  "model_rules",
		rules: map[string]map[string]any{"evil-model": {"disabled": true}},
	}
	unconfigured := &ModelRulesGuardrail{
		name:  "model_rules",
		rules: nil,
	}

	tests := []struct {
		name string
		g    *ModelRulesGuardrail
		body string
		want guardrail.Decision
	}{
		{"disabled model", configured, `{"model":"evil-model"}`, guardrail.DecisionBlock},
		{"allowed model", configured, `{"model":"gpt-4o"}`, guardrail.DecisionPass},
		{"missing model", configured, `{"messages":[]}`, guardrail.DecisionBlock},
		{"unconfigured missing model", unconfigured, `{"messages":[]}`, guardrail.DecisionPass},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.g.Evaluate(context.Background(), guardrail.DirectionInput, []byte(tt.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Decision != tt.want {
				t.Fatalf("decision = %v, want %v", result.Decision, tt.want)
			}
		})
	}
}
