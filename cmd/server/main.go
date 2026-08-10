package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/dashboard"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
	guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"github.com/dexterhere04/AgentPlane/internal/secrets"
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

	provider := proxy.NewOpenAIProviderFromEnv()

	cfg := guardrail.LoadConfig()

	registry := guardrail.NewRegistry()
	registry.Register(guardrail.NewPromptInjectionGuardrail(cfg.Strategy("prompt_injection")))
	registry.Register(guardsecrets.New(cfg.Strategy("secrets")))
	registry.Register(guardpii.New(cfg.Strategy("pii")))
	registry.Register(guardrail.NewContentModerationGuardrail(cfg.Strategy("content_moderation")))

	enforcement := guardrail.NewEnforcementPoint(registry, cfg, bus)

	mandatoryInput := guardrail.GuardrailSet{
		Guards: []guardrail.GuardrailSpec{
			{Name: "prompt_injection"},
			{Name: "secrets"},
			{Name: "pii"},
		},
	}
	mandatoryOutput := guardrail.GuardrailSet{
		Guards: []guardrail.GuardrailSpec{
			{Name: "secrets"},
			{Name: "pii"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		handlers.Chat(w, r, enforcement, mandatoryInput, mandatoryOutput, provider)
	})
	mux.HandleFunc("/events", observability.SSEHandler(bus))
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboard.HTML))
	})

	mux.HandleFunc("/metrics", handlers.MetricsHandler())

	log.Printf("AgentPlane Dev Mode")
	log.Printf("  Gateway   → http://localhost:%s/chat (%s)", port, provider.Name())
	log.Printf("  Events    → http://localhost:%s/events", port)
	log.Printf("  Dashboard → http://localhost:%s/dashboard", port)
	log.Printf("  Metrics   → http://localhost:%s/metrics", port)
	log.Printf("  Mock API  → http://localhost:%s (set OPENAI_BASE_URL)", port)

	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
