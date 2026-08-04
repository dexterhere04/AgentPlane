package config

import (
	"os"

	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

var Store secrets.SecretStore = secrets.EnvStore{}

func SetStore(s secrets.SecretStore) {
	if s == nil {
		Store = secrets.EnvStore{}
		return
	}
	Store = s
}

func OpenAIKey() (string, error) {
	return Store.GetSecret("OPENAI_API_KEY")
}

func OpenAIBaseURL() string {
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		return url
	}
	return "https://api.openai.com/v1"
}
