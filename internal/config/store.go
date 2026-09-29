package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

// ConfigureSecretStore selects the secret backend based on the SECRET_STORE
// environment variable and installs it into Store. It is shared by the
// server and cmd/keygeneration so both read secrets the same way.
//
// Backends: env, file, aws, azure, chain. When SECRET_STORE is unset or
// unrecognized, it falls back to HashiCorp Vault.
func ConfigureSecretStore() error {
	switch os.Getenv("SECRET_STORE") {
	case "env":
		SetStore(secrets.EnvStore{})
	case "file":
		SetStore(secrets.FileStore{BasePath: os.Getenv("SECRET_STORE_PATH")})
	case "aws":
		SetStore(&secrets.AWSSecretsManagerStore{
			Region:      os.Getenv("AWS_REGION"),
			EndpointURL: os.Getenv("AWS_ENDPOINT_URL"),
			SecretID:    os.Getenv("AWS_SECRET_ID"),
			JSONKey:     os.Getenv("AWS_SECRET_JSON_KEY"),
		})
	case "azure":
		SetStore(&secrets.AzureKeyVaultStore{VaultURL: os.Getenv("AZURE_KEY_VAULT_URL")})
	case "chain":
		SetStore(secrets.ChainStore{
			Stores: []secrets.SecretStore{
				secrets.FileStore{BasePath: os.Getenv("SECRET_STORE_PATH")},
				secrets.EnvStore{},
			},
		})
	default:
		store, err := newVaultStore()
		if err != nil {
			return err
		}
		SetStore(store)
	}
	return nil
}

// readAppRoleFile parses a KEY=VALUE env file (as written by the compose
// vault-init service to /vault-creds/creds.env) for AppRole credentials.
func readAppRoleFile(path string) (roleID, secretID string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "VAULT_ROLE_ID="):
			roleID = strings.TrimSpace(strings.TrimPrefix(line, "VAULT_ROLE_ID="))
		case strings.HasPrefix(line, "VAULT_SECRET_ID="):
			secretID = strings.TrimSpace(strings.TrimPrefix(line, "VAULT_SECRET_ID="))
		}
	}
	return roleID, secretID
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newVaultStore() (*secrets.VaultStore, error) {
	store := &secrets.VaultStore{
		Addr:       envOrDefault("VAULT_ADDR", "http://127.0.0.1:8200"),
		Token:      os.Getenv("VAULT_TOKEN"),
		MountPath:  envOrDefault("VAULT_MOUNT_PATH", "agentplane"),
		KVVersion:  2,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	if os.Getenv("VAULT_KV_VERSION") == "1" {
		store.KVVersion = 1
	}

	// AppRole credentials let the store re-issue tokens when the cached one
	// expires. They come from the environment (exported by docker-entrypoint.sh)
	// or, as a fallback, straight from the file vault-init writes.
	roleID := os.Getenv("VAULT_ROLE_ID")
	secretID := os.Getenv("VAULT_SECRET_ID")
	if roleID == "" || secretID == "" {
		fileRole, fileSecret := readAppRoleFile(envOrDefault("VAULT_APPROLE_FILE", "/vault-creds/creds.env"))
		if roleID == "" {
			roleID = fileRole
		}
		if secretID == "" {
			secretID = fileSecret
		}
	}
	store.RoleID = roleID
	store.SecretID = secretID

	if store.Token == "" && store.RoleID != "" && store.SecretID != "" {
		token, err := secrets.VaultAppRoleLogin(store.Addr, store.RoleID, store.SecretID)
		if err != nil {
			return nil, fmt.Errorf("Vault AppRole login: %w", err)
		}
		store.Token = token
	}
	return store, nil
}
