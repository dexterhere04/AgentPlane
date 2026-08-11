package zscaler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type ZscalerGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *ZscalerGuardrail {
	baseURL := os.Getenv("ZSCALER_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.zscaler.com/ai/v1"
	}
	return &ZscalerGuardrail{
		apiKey:  os.Getenv("ZSCALER_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *ZscalerGuardrail) Name() string {
	return "zscaler"
}

func (g *ZscalerGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"input": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("zscaler: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/guard", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("zscaler: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zscaler: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zscaler: read response: %w", err)
	}

	var result struct {
		Blocked  bool           `json:"blocked"`
		Risk     string         `json:"risk,omitempty"`
		Category string         `json:"category,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("zscaler: parse response: %w", err)
	}

	if result.Blocked {
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("Zscaler blocked: %s (%s)", result.Risk, result.Category),
			Details:   map[string]any{"risk": result.Risk, "category": result.Category},
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
