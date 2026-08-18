package openai_moderation

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

type OpenAIModerationGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *OpenAIModerationGuardrail {
	baseURL := os.Getenv("OPENAI_MODERATION_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := os.Getenv("OPENAI_MODERATION_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	return &OpenAIModerationGuardrail{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *OpenAIModerationGuardrail) Name() string {
	return "openai_moderation"
}

func (g *OpenAIModerationGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"input": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/moderations", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai_moderation: read response: %w", err)
	}

	var result struct {
		Results []struct {
			Flagged    bool               `json:"flagged"`
			Categories map[string]bool    `json:"categories"`
			Scores     map[string]float64 `json:"category_scores"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("openai_moderation: parse response: %w", err)
	}

	if len(result.Results) > 0 && result.Results[0].Flagged {
		var flaggedCats []string
		for cat, matched := range result.Results[0].Categories {
			if matched {
				flaggedCats = append(flaggedCats, cat)
			}
		}
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("OpenAI moderation flagged categories: %v", flaggedCats),
			Details:   map[string]any{"categories": flaggedCats, "scores": result.Results[0].Scores},
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
