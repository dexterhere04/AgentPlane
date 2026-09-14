package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func NewOpenAIProviderFromEnv() *OpenAIProvider {
	apiKey, _ := config.OpenAIKey()
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return NewOpenAIProvider(apiKey, baseURL)
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error) {
	bus := observability.DefaultBus
	url := p.baseURL + "/chat/completions"

	// Attribute this request to the authenticated user so traces, prompts,
	// and usage events can be aggregated per user in ClickHouse.
	userID, username := "", ""
	if user, ok := auth.UserFromContext(ctx); ok {
		userID = user.ID.String()
		username = user.Username
	}

	if p.apiKey == "" {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageLoadingAPIKey, "error", "OPENAI_API_KEY is not set"))
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageLoadingAPIKey, "completed", "API key loaded"))

	if p.isStreaming(body) {
		return p.forwardStreaming(ctx, body, requestID, url, bus, userID, username)
	}

	return p.forwardNonStreaming(ctx, body, requestID, url, bus, userID, username)
}

func (p *OpenAIProvider) isStreaming(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

func (p *OpenAIProvider) forwardNonStreaming(ctx context.Context, body []byte, requestID, url string, bus *observability.EventBus, userID, username string) ([]byte, error) {
	start := time.Now()
	var statusStr string = "success"
	var inputTokens, outputTokens, totalTokens uint64

	model := requestModel(body)

	// Ensure we always record a trace (success or error paths)
	defer func() {
		recordTraceAsync(requestID, model, statusStr, start, inputTokens, outputTokens, totalTokens, userID, username)
	}()

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageBuildingRequest, "started", fmt.Sprintf("POST %s", url)))
	buildStart := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageBuildingRequest, "error", err.Error()))
		return nil, fmt.Errorf("creating request: %w", err)
	}

	bus.Publish(observability.NewDurationEvent(requestID, observability.StageBuildingRequest, "completed", time.Since(buildStart)))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSettingHeaders, "started", "Content-Type, Authorization"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSettingHeaders, "completed", "Headers set"))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSendingRequest, "started", url))
	sendStart := time.Now()

	resp, err := p.httpClient.Do(req)
	if err != nil {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageSendingRequest, "error", err.Error()))
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	bus.Publish(observability.NewDurationEvent(requestID, observability.StageSendingRequest, "completed", time.Since(sendStart)))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageReadingResponse, "started", fmt.Sprintf("HTTP %d", resp.StatusCode)))
	readStart := time.Now()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageReadingResponse, "error", err.Error()))
		return nil, fmt.Errorf("reading response: %w", err)
	}

	bus.Publish(observability.NewDurationEvent(requestID, observability.StageReadingResponse, "completed", time.Since(readStart)))

	if resp.StatusCode != http.StatusOK {
		statusStr = fmt.Sprintf("http_%d", resp.StatusCode)
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageValidatingStatus, "error", fmt.Sprintf("HTTP %d", resp.StatusCode)))
		return nil, fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, string(respBody))
	}
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageValidatingStatus, "completed", "HTTP 200"))

	var respData map[string]interface{}
	if json.Valid(respBody) {
		json.Unmarshal(respBody, &respData)
	}
	bus.Publish(observability.NewDataEvent(requestID, observability.StageResponseSent, "completed", respData))

	// Enforce capture mode for response payload
	captureResponseAsync(requestID, respBody)

	// Extract usage tokens when present (updates variables captured by defer)
	var reasoningTokens uint64
	var cachedInputTokens uint64
	if usage, ok := respData["usage"].(map[string]interface{}); ok {
		if v, ok := usage["prompt_tokens"].(float64); ok {
			inputTokens = uint64(v)
		}
		if v, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = uint64(v)
		}
		if v, ok := usage["total_tokens"].(float64); ok {
			totalTokens = uint64(v)
		}
		// Extract extended token fields if provider supplies them
		if v, ok := usage["reasoning_tokens"].(float64); ok {
			reasoningTokens = uint64(v)
		}
		if v, ok := usage["cached_input_tokens"].(float64); ok {
			cachedInputTokens = uint64(v)
		}
		// Record usage event with provider/model from request
		captureUsageAsync(requestID, model, inputTokens, outputTokens, totalTokens, reasoningTokens, cachedInputTokens)
	}

	return respBody, nil
}

func (p *OpenAIProvider) forwardStreaming(ctx context.Context, body []byte, requestID, url string, bus *observability.EventBus, userID, username string) ([]byte, error) {
	start := time.Now()
	var statusStr string = "success"
	var inputTokens, outputTokens, totalTokens uint64

	model := requestModel(body)

	// Ensure we always record a trace (success or error paths)
	defer func() {
		recordTraceAsync(requestID, model, statusStr, start, inputTokens, outputTokens, totalTokens, userID, username)
	}()

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageBuildingRequest, "started", fmt.Sprintf("POST %s (stream)", url)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		statusStr = "error"
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		statusStr = "error"
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		statusStr = fmt.Sprintf("http_%d", resp.StatusCode)
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var allChunks [][]byte
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(payload) == "[DONE]" {
			break
		}

		allChunks = append(allChunks, []byte(payload))
	}

	if err := scanner.Err(); err != nil {
		statusStr = "error"
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	structuredResponse := p.buildStructuredResponse(allChunks)
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageResponseSent, "completed", fmt.Sprintf("streamed %d chunks", len(allChunks))))

	// Extract usage tokens from the stream (present when stream_options.include_usage is set).
	inputTokens, outputTokens, totalTokens = parseStreamingUsage(allChunks)

	// Enforce capture mode for response payload
	captureResponseAsync(requestID, structuredResponse)

	// Record usage event when the stream included a usage chunk.
	if totalTokens > 0 {
		captureUsageAsync(requestID, model, inputTokens, outputTokens, totalTokens, 0, 0)
	}

	return structuredResponse, nil
}

func (p *OpenAIProvider) buildStructuredResponse(chunks [][]byte) []byte {
	type delta struct {
		Content string `json:"content"`
		Role    string `json:"role,omitempty"`
	}

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type choice struct {
		Index   int     `json:"index"`
		Message message `json:"message"`
	}

	type response struct {
		ID      string   `json:"id"`
		Object  string   `json:"object"`
		Created int64    `json:"created"`
		Model   string   `json:"model"`
		Choices []choice `json:"choices"`
	}

	resp := response{
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []choice{
			{
				Index: 0,
				Message: message{
					Role: "assistant",
				},
			},
		},
	}

	var fullContent strings.Builder

	for _, chunk := range chunks {
		var c struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			Choices []struct {
				Delta delta `json:"delta"`
			} `json:"choices"`
		}

		if err := json.Unmarshal(chunk, &c); err != nil {
			continue
		}

		if resp.ID == "" {
			resp.ID = c.ID
		}
		if resp.Model == "" {
			resp.Model = c.Model
		}

		for _, ch := range c.Choices {
			if ch.Delta.Role != "" && resp.Choices[0].Message.Role == "" {
				resp.Choices[0].Message.Role = ch.Delta.Role
			}
			fullContent.WriteString(ch.Delta.Content)
		}
	}

	resp.Choices[0].Message.Content = fullContent.String()

	result, _ := json.Marshal(resp)
	return result
}

// ParseUsageTokens extracts prompt/completion/total tokens from an OpenAI-style response body.
func ParseUsageTokens(respBody []byte) (uint64, uint64, uint64) {
	var in, out, total uint64
	var respData map[string]interface{}
	if json.Valid(respBody) {
		if err := json.Unmarshal(respBody, &respData); err == nil {
			if usage, ok := respData["usage"].(map[string]interface{}); ok {
				if v, ok := usage["prompt_tokens"].(float64); ok {
					in = uint64(v)
				}
				if v, ok := usage["completion_tokens"].(float64); ok {
					out = uint64(v)
				}
				if v, ok := usage["total_tokens"].(float64); ok {
					total = uint64(v)
				}
			}
		}
	}
	return in, out, total
}

// requestModel extracts the model name from an OpenAI-style request body.
func requestModel(body []byte) string {
	var reqObj map[string]interface{}
	if json.Valid(body) {
		if err := json.Unmarshal(body, &reqObj); err == nil {
			if m, ok := reqObj["model"].(string); ok {
				return m
			}
		}
	}
	return ""
}

// recordTraceAsync records a request trace without blocking the caller.
func recordTraceAsync(requestID, model, status string, start time.Time, in, out, total uint64, userID, username string) {
	latency := time.Since(start).Milliseconds()
	estimatedCost := observability.EstimateRequestCost("openai", model, in, out)
	go observability.RecordTrace(observability.Trace{
		TraceID:       requestID,
		RequestID:     requestID,
		Timestamp:     start,
		UserID:        userID,
		Username:      username,
		Provider:      "openai",
		Model:         model,
		LatencyMS:     latency,
		Status:        status,
		CacheHit:      false,
		InputTokens:   in,
		OutputTokens:  out,
		TotalTokens:   total,
		EstimatedCost: estimatedCost,
		Route:         "/chat",
	})
}

// captureResponseAsync stores a response payload according to the configured
// capture mode, without blocking the caller.
func captureResponseAsync(requestID string, respBody []byte) {
	cfg := observability.GetCaptureConfig()
	decision := observability.MakeCaptureDecision(cfg.ResponseMode, respBody, cfg.SampleRate)
	if !decision.ShouldCapture {
		return
	}
	go func() {
		var payload []byte
		if decision.StorePayload {
			if compressed, err := observability.CompressPayload(respBody); err == nil {
				payload = compressed
			} else {
				payload = respBody
			}
		} else if decision.ComputeHash {
			payload = respBody
		}
		_, _ = observability.CaptureResponsePayload(observability.Payload{
			TraceID:     requestID,
			RequestID:   requestID,
			Timestamp:   time.Now(),
			Payload:     payload,
			CaptureMode: decision.FinalMode,
		})
	}()
}

// captureUsageAsync records a usage event without blocking the caller.
func captureUsageAsync(requestID, model string, in, out, total, reasoning, cached uint64) {
	estimatedCost := observability.EstimateRequestCost("openai", model, in, out)
	go func() {
		_ = observability.CaptureUsage(observability.UsageEvent{
			TraceID:           requestID,
			RequestID:         requestID,
			Timestamp:         time.Now(),
			Provider:          "openai",
			Model:             model,
			InputTokens:       in,
			OutputTokens:      out,
			TotalTokens:       total,
			ReasoningTokens:   reasoning,
			CachedInputTokens: cached,
			Cost:              estimatedCost,
		})
	}()
}

// parseStreamingUsage scans stream chunks for a usage object and returns the
// most recent token counts. OpenAI emits usage in a final chunk when
// stream_options.include_usage is set; other providers may omit it entirely.
func parseStreamingUsage(chunks [][]byte) (in, out, total uint64) {
	for _, c := range chunks {
		i, o, t := ParseUsageTokens(c)
		if i == 0 && o == 0 && t == 0 {
			continue
		}
		in, out, total = i, o, t
	}
	return in, out, total
}
