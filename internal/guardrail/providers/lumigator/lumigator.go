package lumigator

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

type LumigatorGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *LumigatorGuardrail {
	baseURL := os.Getenv("LUMIGATOR_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.lumigator.ai/v1"
	}
	return &LumigatorGuardrail{
		apiKey:  os.Getenv("LUMIGATOR_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *LumigatorGuardrail) Name() string {
	return "lumigator"
}

func (g *LumigatorGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"prompt": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("lumigator: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/evaluate", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("lumigator: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lumigator: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lumigator: read response: %w", err)
	}

	var result struct {
		Blocked bool           `json:"blocked"`
		Reason  string         `json:"reason,omitempty"`
		Score   float64        `json:"score,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("lumigator: parse response: %w", err)
	}

	if result.Blocked {
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("Lumigator blocked: %s", result.Reason),
			Details:   map[string]any{"score": result.Score, "reason": result.Reason},
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
