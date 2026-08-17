package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/secrets"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	config.SetStore(secrets.VaultStore{
		Addr:       envOrDefault("VAULT_ADDR", "http://127.0.0.1:8200"),
		Token:      os.Getenv("VAULT_TOKEN"),
		MountPath:  envOrDefault("VAULT_MOUNT_PATH", "agentplane"),
		KVVersion:  2,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	})

	pepper, err := config.KeyPepper()
	if err != nil {
		log.Fatal(err)
	}

	key, err := api.GenerateAPIKey(pepper)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Generated API Key:")
	fmt.Println(key.FullKey)

	fmt.Println("\nKey ID:")
	fmt.Println(key.KeyID)

	fmt.Println("\nSecret Hash:")
	fmt.Println(key.SecretHash)
}
