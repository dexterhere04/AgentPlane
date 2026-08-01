package config

import "os"

func OpenAIKey() string {
	return os.Getenv("OPENAI_API_KEY")
}

func OpenAIBaseURL() string {
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		return url
	}
	return "https://api.openai.com/v1"
}
