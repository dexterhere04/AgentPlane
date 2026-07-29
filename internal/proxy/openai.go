package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
)

const openAIURL = "https://api.openai.com/v1/chat/completions"

// ForwardChat forwards a chat completion request to OpenAI.
// It takes the incoming request body, attaches the API key, sends the
// request to OpenAI, and returns the response body and any error.
func ForwardChat(body []byte) ([]byte, error) {
	apiKey := config.OpenAIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	req, err := http.NewRequest(http.MethodPost, openAIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
