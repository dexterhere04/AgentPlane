package nvidia_content

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

type NvidiaContentGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *NvidiaContentGuardrail {
	baseURL := os.Getenv("NVIDIA_CONTENT_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.nvidia.com/v1/content-safety"
	}
	return &NvidiaContentGuardrail{
		apiKey:  os.Getenv("NVIDIA_CONTENT_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *NvidiaContentGuardrail) Name() string {
	return "nvidia_content"
}

func (g *NvidiaContentGuardrail) Type() guardrail.GuardrailType {
	return guardrail.TypePolicy
}

func (g *NvidiaContentGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"prompt": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("nvidia_content: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/evaluate", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("nvidia_content: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nvidia_content: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nvidia_content: read response: %w", err)
	}

	var result struct {
		Categories []struct {
			Name    string  `json:"name"`
			Score   float64 `json:"score"`
			Unsafe  bool    `json:"unsafe"`
		} `json:"categories"`
		OverallUnsafe bool `json:"overall_unsafe"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("nvidia_content: parse response: %w", err)
	}

	if result.OverallUnsafe {
		var findings []guardrail.Finding
		for _, c := range result.Categories {
			if c.Unsafe {
				findings = append(findings, guardrail.Finding{
					Guardrail: g.Name(),
					Type:      c.Name,
					Entity:    c.Name,
					Severity:  guardrail.SeverityHigh,
				})
			}
		}
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   "NVIDIA content safety guard flagged unsafe content",
			Findings:  findings,
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
