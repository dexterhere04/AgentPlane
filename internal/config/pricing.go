package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// PricingRate holds input and output token rates (dollars per 1000 tokens)
type PricingRate struct {
	InputCost  float64 // $ per 1k input tokens
	OutputCost float64 // $ per 1k output tokens
}

// PricingTable maps provider:model -> rates
type PricingTable map[string]PricingRate

var (
	pricingMu sync.RWMutex
	pricing   PricingTable = make(PricingTable)
)

func init() {
	// Load default OpenAI pricing (2024 estimates).
	// These are examples and should be updated based on actual OpenAI pricing documentation.
	// https://openai.com/pricing/
	SetDefaultPricing(PricingTable{
		"openai:gpt-4o":                 {InputCost: 0.003, OutputCost: 0.006},
		"openai:gpt-4-turbo":            {InputCost: 0.01, OutputCost: 0.03},
		"openai:gpt-4":                  {InputCost: 0.03, OutputCost: 0.06},
		"openai:gpt-3.5-turbo":          {InputCost: 0.0005, OutputCost: 0.0015},
		"openai:text-embedding-3-small": {InputCost: 0.00002, OutputCost: 0.00002},
		"openai:text-embedding-3-large": {InputCost: 0.00013, OutputCost: 0.00013},
	})

	// Allow environment override for all provider:model combos.
	// Format: PRICING_<PROVIDER>_<MODEL>=input_rate:output_rate
	// E.g.: PRICING_OPENAI_GPT4O=0.003:0.006
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "PRICING_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0][len("PRICING_"):] // remove "PRICING_" prefix
			key = strings.ToLower(key)
			val := parts[1]
			rates := strings.Split(val, ":")
			if len(rates) != 2 {
				continue
			}
			inRate, _ := strconv.ParseFloat(rates[0], 64)
			outRate, _ := strconv.ParseFloat(rates[1], 64)
			// Convert env key like "OPENAI_GPT4O" to "openai:gpt-4o"
			provider := strings.Split(key, "_")[0]
			model := strings.ToLower(strings.Join(strings.Split(key, "_")[1:], "-"))
			pricingMu.Lock()
			pricing[provider+":"+model] = PricingRate{InputCost: inRate, OutputCost: outRate}
			pricingMu.Unlock()
		}
	}
}

// SetDefaultPricing sets the pricing table (e.g., from config file or code)
func SetDefaultPricing(table PricingTable) {
	pricingMu.Lock()
	defer pricingMu.Unlock()
	for k, v := range table {
		pricing[k] = v
	}
}

// GetPricing returns the pricing rate for a provider:model combination.
// If not found, returns a zero-cost rate (nil cost = no pricing available).
func GetPricing(provider, model string) *PricingRate {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	if rate, ok := pricing[provider+":"+model]; ok {
		return &rate
	}
	return nil
}

// EstimateRequestCost calculates the estimated cost for a request.
// If pricing is not available for the model, returns 0.0.
// Inputs are token counts (not per-1k); output is in dollars.
// This is an estimate and should NOT be used for billing.
func EstimateRequestCost(provider, model string, inputTokens, outputTokens uint64) float64 {
	rate := GetPricing(provider, model)
	if rate == nil {
		return 0.0 // Pricing not available
	}
	// Convert tokens to cost: (tokens / 1000) * rate
	inputCost := (float64(inputTokens) / 1000.0) * rate.InputCost
	outputCost := (float64(outputTokens) / 1000.0) * rate.OutputCost
	return inputCost + outputCost
}
