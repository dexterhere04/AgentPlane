package observability

// PricingTable defines token-based pricing for LLM models.
// Pricing is stored per provider and model, with separate rates for input and output tokens.
// This is a configuration mechanism only — prices should not be hardcoded here.
// For production use, load pricing from external configuration (e.g., YAML, environment, database).
type PricingTable struct {
	// Rates maps "provider/model" to (inputRatePerMTok, outputRatePerMTok) in USD
	// Example: "openai/gpt-4o" -> (15.0, 45.0) means $15 per million input tokens, $45 per million output
	Rates map[string][2]float64
}

// DefaultPricingTable returns a basic pricing table with common OpenAI models as examples.
// This is for demonstration only. In production, load pricing from a configuration source.
// IMPORTANT: These prices are examples and may not reflect actual current pricing.
// Always verify pricing with your provider.
func DefaultPricingTable() *PricingTable {
	return &PricingTable{
		Rates: map[string][2]float64{
			// OpenAI pricing (example; verify with actual OpenAI pricing)
			"openai/gpt-4o":      {5.0, 15.0},         // $5/$15 per million tokens (input/output)
			"openai/gpt-4-turbo": {10.0, 30.0},        // $10/$30 per million tokens
			"openai/gpt-3.5":     {0.5, 1.5},          // $0.5/$1.5 per million tokens
			"openai/gpt-4":       {30.0, 60.0},        // $30/$60 per million tokens
		},
	}
}

// EstimateCost calculates estimated cost from token counts using the pricing table.
// If the provider/model is not found in the table, returns 0 (unavailable pricing).
// Usage notes:
// - This is an estimate, not billing data. Actual costs may vary.
// - Pricing should be updated regularly from your provider.
// - Consider loading from external configuration rather than hardcoding.
func (p *PricingTable) EstimateCost(provider, model string, inputTokens, outputTokens uint64) float64 {
	if p == nil || p.Rates == nil {
		return 0
	}
	key := provider + "/" + model
	rates, ok := p.Rates[key]
	if !ok {
		// Model not in pricing table; return 0 rather than guessing
		return 0
	}

	inputRate := rates[0]  // price per million input tokens
	outputRate := rates[1] // price per million output tokens

	inputCost := (float64(inputTokens) / 1_000_000.0) * inputRate
	outputCost := (float64(outputTokens) / 1_000_000.0) * outputRate

	return inputCost + outputCost
}

// Package-level pricing singleton
var defaultPricing = DefaultPricingTable()

// SetPricingTable sets the global pricing table for cost estimation.
func SetPricingTable(p *PricingTable) {
	defaultPricing = p
}

// EstimateRequestCost estimates cost for a single request using the global pricing table.
func EstimateRequestCost(provider, model string, inputTokens, outputTokens uint64) float64 {
	if defaultPricing == nil {
		return 0
	}
	return defaultPricing.EstimateCost(provider, model, inputTokens, outputTokens)
}
