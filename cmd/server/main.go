package main

import (
	"context"
	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/dashboard"
	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/secrets"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/joho/godotenv"

	"log"
	"net/http"
	"os"
	"time"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newVaultStore() secrets.VaultStore {
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
				log.Fatalf("Vault AppRole login: %v", err)
			}
			store.Token = token
		}
	}
	return store
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file loaded: %v", err)
	}

	databaseURL, err := config.DatabaseURL()
	if err != nil {
		log.Fatalf("Database configuration: %v", err)
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Database connection: %v", err)
	}
	defer pool.Close()

	userStore := users.NewStore(pool)
	apiKeyStore := api.NewStore(pool)
	authStore := auth.NewStore(pool)

	pepper, err := config.KeyPepper()
	if err != nil {
		log.Fatalf("API key pepper: %v", err)
	}

	adminToken, err := config.AdminToken()
	if err != nil {
		log.Fatalf("Admin token: %v", err)
	}

	provisioner := provisioning.NewProvisioner(
		userStore,
		apiKeyStore,
		pepper,
	)

	authenticator := auth.NewAuthenticator(
		authStore,
		userStore,
		pepper,
	)

	bus := observability.DefaultBus

	switch os.Getenv("SECRET_STORE") {
	case "env":
		config.SetStore(secrets.EnvStore{})
	case "file":
		config.SetStore(secrets.FileStore{
			BasePath: os.Getenv("SECRET_STORE_PATH"),
		})
	case "aws":
		config.SetStore(&secrets.AWSSecretsManagerStore{
			Region:      os.Getenv("AWS_REGION"),
			EndpointURL: os.Getenv("AWS_ENDPOINT_URL"),
			SecretID:    os.Getenv("AWS_SECRET_ID"),
			JSONKey:     os.Getenv("AWS_SECRET_JSON_KEY"),
		})
	case "azure":
		config.SetStore(&secrets.AzureKeyVaultStore{
			VaultURL: os.Getenv("AZURE_KEY_VAULT_URL"),
		})
	case "chain":
		config.SetStore(secrets.ChainStore{
			Stores: []secrets.SecretStore{
				secrets.FileStore{BasePath: os.Getenv("SECRET_STORE_PATH")},
				secrets.EnvStore{},
			},
		})
	default:
		log.Printf("Secrets backend: Vault (%s)", envOrDefault("VAULT_ADDR", "http://127.0.0.1:8200"))
		config.SetStore(newVaultStore())
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/chat",
		authenticator.Middleware(http.HandlerFunc(handlers.Chat)),
	)
	mux.Handle("/provision/user", handlers.ProvisionUser(provisioner))
	mux.Handle(
		"/admin/api-keys/revoke",
		auth.AdminMiddleware(
			adminToken,
			handlers.RevokeAPIKey(apiKeyStore),
		),
	)
	mux.HandleFunc("/events", observability.SSEHandler(bus))
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboard.HTML))
	})

	log.Printf("AgentPlane Dev Mode")
	log.Printf("  Gateway   → http://localhost:%s/chat", port)
	log.Printf("  Events    → http://localhost:%s/events", port)
	log.Printf("  Dashboard → http://localhost:%s/dashboard", port)
	log.Printf("  Mock API  → http://localhost:%s (set OPENAI_BASE_URL)", port)

	err = http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
