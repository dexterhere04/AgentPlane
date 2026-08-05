package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

func chatURL() string {
	return config.OpenAIBaseURL() + "/chat/completions"
}

func ForwardChat(body []byte, requestID string) ([]byte, error) {
	bus := observability.DefaultBus
	url := chatURL()

	apiKey, err := config.OpenAIKey()
	if err != nil {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageLoadingAPIKey, "error", err.Error()))
		return nil, fmt.Errorf("loading OPENAI_API_KEY: %w", err)
	}

	if apiKey == "" {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageLoadingAPIKey, "error", "OPENAI_API_KEY is empty"))
		return nil, fmt.Errorf("OPENAI_API_KEY is empty")
	}

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageLoadingAPIKey, "completed", "API key loaded"))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageBuildingRequest, "started", fmt.Sprintf("POST %s", url)))
	buildStart := time.Now()

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageBuildingRequest, "error", err.Error()))
		return nil, fmt.Errorf("creating request: %w", err)
	}

	bus.Publish(observability.NewDurationEvent(requestID, observability.StageBuildingRequest, "completed", time.Since(buildStart)))

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSettingHeaders, "started", "Content-Type, Authorization"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSettingHeaders, "completed", "Headers set"))

	client := &http.Client{Timeout: 60 * time.Second}

	bus.Publish(observability.NewMessageEvent(requestID, observability.StageSendingRequest, "started", url))
	sendStart := time.Now()

	resp, err := client.Do(req)
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
		bus.Publish(observability.NewMessageEvent(requestID, observability.StageValidatingStatus, "error", fmt.Sprintf("HTTP %d", resp.StatusCode)))
		return nil, fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, string(respBody))
	}
	bus.Publish(observability.NewMessageEvent(requestID, observability.StageValidatingStatus, "completed", "HTTP 200"))

	var respData interface{}
	if json.Valid(respBody) {
		json.Unmarshal(respBody, &respData)
	}
	bus.Publish(observability.NewDataEvent(requestID, observability.StageResponseSent, "completed", respData))

	return respBody, nil
}
