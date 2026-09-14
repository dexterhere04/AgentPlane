package secrets

import (
	"fmt"
	"os"
	"strings"
)

type SecretStore interface {
	GetSecret(key string) (string, error)
}

// SecretWriter is implemented by secret stores that support persisting
// secrets (e.g. Vault). Read-only backends (env, file, aws, azure) do not.
type SecretWriter interface {
	SecretStore
	SetSecret(key, value string) error
}

type EnvStore struct{}

func (e EnvStore) GetSecret(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("env var %s is not set", key)
	}
	return val, nil
}

type FileStore struct {
	BasePath string
}

func (f FileStore) GetSecret(key string) (string, error) {
	path := strings.TrimRight(f.BasePath, "/") + "/" + key
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading secret file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

type ChainStore struct {
	Stores []SecretStore
}

func (c ChainStore) GetSecret(key string) (string, error) {
	var errs []string
	for i, s := range c.Stores {
		val, err := s.GetSecret(key)
		if err == nil {
			return val, nil
		}
		errs = append(errs, fmt.Sprintf("store %d: %v", i, err))
	}
	return "", fmt.Errorf("chain: all stores failed for key %q: %s", key, strings.Join(errs, "; "))
}
