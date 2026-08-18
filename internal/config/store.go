package config

import (
	"fmt"
	"net/http"
	"os"
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newVaultStore() (secrets.VaultStore, error) {
	store := secrets.VaultStore{
		Addr:       envOrDefault("VAULT_ADDR", "http://127.0.0.1:8200"),
		Token:      os.Getenv("VAULT_TOKEN"),
		MountPath:  envOrDefault("VAULT_MOUNT_PATH", "agentplane"),
		KVVersion:  2,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	if os.Getenv("VAULT_KV_VERSION") == "1" {
		store.KVVersion = 1
	}
	if store.Token == "" {
		roleID := os.Getenv("VAULT_ROLE_ID")
		secretID := os.Getenv("VAULT_SECRET_ID")
		if roleID != "" && secretID != "" {
			token, err := secrets.VaultAppRoleLogin(store.Addr, roleID, secretID)
			if err != nil {
				return secrets.VaultStore{}, fmt.Errorf("Vault AppRole login: %w", err)
			}
			store.Token = token
		}
	}
	return store, nil
}
