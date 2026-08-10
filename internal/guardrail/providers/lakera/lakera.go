package lakera

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

type LakeraGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *LakeraGuardrail {
	baseURL := os.Getenv("LAKERA_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.lakera.ai"
	}
	return &LakeraGuardrail{
		apiKey:  os.Getenv("LAKERA_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *LakeraGuardrail) Name() string {
	return "lakera"
}

func (g *LakeraGuardrail) Type() guardrail.GuardrailType {
	return guardrail.TypePolicy
}

func (g *LakeraGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"input": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("lakera: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/v1/guard", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("lakera: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lakera: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lakera: read response: %w", err)
	}

	var result struct {
		Flagged bool             `json:"flagged"`
		Results []map[string]any `json:"results,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("lakera: parse response: %w", err)
	}

	if result.Flagged {
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   "Lakera Guard flagged prompt injection or unsafe content",
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
