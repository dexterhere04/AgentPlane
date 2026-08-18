package config

import (
	"math"
	"testing"
)

func TestEstimateRequestCostKnownModel(t *testing.T) {
	// gpt-4o defaults: $0.003 per 1k input, $0.006 per 1k output.
	got := EstimateRequestCost("openai", "gpt-4o", 1000, 1000)
	want := 0.003 + 0.006
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("EstimateRequestCost = %v, want %v", got, want)
	}
}

func TestEstimateRequestCostUnknownModel(t *testing.T) {
	if got := EstimateRequestCost("openai", "does-not-exist", 1000, 1000); got != 0.0 {
		t.Fatalf("expected 0.0 for unknown model, got %v", got)
	}
}

func TestGetPricingKnownModel(t *testing.T) {
	if rate := GetPricing("openai", "gpt-4o"); rate == nil {
		t.Fatalf("expected pricing for openai:gpt-4o")
	}
}
