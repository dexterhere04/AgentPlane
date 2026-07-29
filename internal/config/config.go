package config

import "os"

// OpenAIKey returns the OpenAI API key loaded from the environment.
// For V0 this is read from the OPENAI_API_KEY environment variable.
func OpenAIKey() string {
	return os.Getenv("OPENAI_API_KEY")
}
