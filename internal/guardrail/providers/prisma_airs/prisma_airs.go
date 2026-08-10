package prisma_airs

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

type PrismaAIRSGuardrail struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(_ guardrail.Strategy) *PrismaAIRSGuardrail {
	baseURL := os.Getenv("PRISMA_AIRS_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.prisma.paloaltonetworks.com/airs/v1"
	}
	return &PrismaAIRSGuardrail{
		apiKey:  os.Getenv("PRISMA_AIRS_API_KEY"),
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (g *PrismaAIRSGuardrail) Name() string {
	return "prisma_airs"
}

func (g *PrismaAIRSGuardrail) Type() guardrail.GuardrailType {
	return guardrail.TypePolicy
}

func (g *PrismaAIRSGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.apiKey == "" {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	reqBody := map[string]any{"input": string(body)}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("prisma_airs: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/scan", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("prisma_airs: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", g.apiKey)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prisma_airs: send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prisma_airs: read response: %w", err)
	}

	var result struct {
		Findings []struct {
			Category string `json:"category"`
			Severity string `json:"severity"`
		} `json:"findings,omitempty"`
		Blocked bool   `json:"blocked"`
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("prisma_airs: parse response: %w", err)
	}

	if result.Blocked || result.Verdict == "block" {
		var findings []guardrail.Finding
		for _, f := range result.Findings {
			findings = append(findings, guardrail.Finding{
				Guardrail: g.Name(),
				Type:      f.Category,
				Entity:    f.Category,
				Severity:  guardrail.SeverityHigh,
			})
		}
		return &guardrail.Result{
			Guardrail: g.Name(),
			Decision:  guardrail.DecisionBlock,
			Message:   "PANW Prisma AIRS blocked content",
			Findings:  findings,
		}, nil
	}

	return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
}
