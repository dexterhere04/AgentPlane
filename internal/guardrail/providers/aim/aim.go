package aim

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

type AIMGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *AIMGuardrail {
	baseURL := os.Getenv("AIM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.aim.security/v1"
	}
	return &AIMGuardrail{
		apiKey:  os.Getenv("AIM_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *AIMGuardrail) Name() string {
	return "aim"
}

func (g *AIMGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"content": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("aim: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/analyze", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("aim: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aim: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("aim: read response: %w", err)
	}

	var result struct {
		Blocked bool              `json:"blocked"`
		Score   float64           `json:"score"`
		Reasons []string          `json:"reasons,omitempty"`
		Details map[string]any    `json:"details,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("aim: parse response: %w", err)
	}

	if result.Blocked {
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("AIM flagged content (score: %.2f)", result.Score),
			Details:   result.Details,
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
