package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/dashboard"
	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	guardaim "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/aim"
	guardfunctions "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/functions"
	guardlakera "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/lakera"
	guardlumigator "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/lumigator"
	guardnvidia "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/nvidia_content"
	guardopenaimod "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/openai_moderation"
	guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
	guardprisma "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/prisma_airs"
	guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
	guardzscaler "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/zscaler"
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
	registry.Register(guardaim.New(cfg.Strategy("aim")))
	registry.Register(guardlakera.New(cfg.Strategy("lakera")))
	registry.Register(guardlumigator.New(cfg.Strategy("lumigator")))
	registry.Register(guardprisma.New(cfg.Strategy("prisma_airs")))
	registry.Register(guardnvidia.New(cfg.Strategy("nvidia_content")))
	registry.Register(guardopenaimod.New(cfg.Strategy("openai_moderation")))
	registry.Register(guardzscaler.New(cfg.Strategy("zscaler")))
	registry.Register(guardfunctions.NewRegexMatch(cfg.Strategy("regex_match")))
	registry.Register(guardfunctions.NewContains(cfg.Strategy("contains")))
	registry.Register(guardfunctions.NewContainsCode(cfg.Strategy("contains_code")))
	registry.Register(guardfunctions.NewEndsWith(cfg.Strategy("ends_with")))
	registry.Register(guardfunctions.NewValidUrls(cfg.Strategy("valid_urls")))
	registry.Register(guardfunctions.NewJsonSchema(cfg.Strategy("json_schema")))
	registry.Register(guardfunctions.NewJsonKeys(cfg.Strategy("json_keys")))
	registry.Register(guardfunctions.NewJWT(cfg.Strategy("jwt")))
	registry.Register(guardfunctions.NewWordCount(cfg.Strategy("word_count")))
	registry.Register(guardfunctions.NewSentenceCount(cfg.Strategy("sentence_count")))
	registry.Register(guardfunctions.NewCharacterCount(cfg.Strategy("character_count")))
	registry.Register(guardfunctions.NewAllUppercase(cfg.Strategy("all_uppercase")))
	registry.Register(guardfunctions.NewAllLowercase(cfg.Strategy("all_lowercase")))
	registry.Register(guardfunctions.NewModelWhitelist(cfg.Strategy("model_whitelist")))
	registry.Register(guardfunctions.NewModelRules(cfg.Strategy("model_rules")))
	registry.Register(guardfunctions.NewRequiredMetadataKeys(cfg.Strategy("required_metadata_keys")))
	registry.Register(guardfunctions.NewAllowedRequestTypes(cfg.Strategy("allowed_request_types")))
	registry.Register(guardfunctions.NewNotNull(cfg.Strategy("not_null")))
	registry.Register(guardfunctions.NewWebhook(cfg.Strategy("webhook")))
	registry.Register(guardfunctions.NewLog(cfg.Strategy("log")))
	registry.Register(guardfunctions.NewAddPrefix(cfg.Strategy("add_prefix")))
	registry.Register(guardfunctions.NewRegexReplace(cfg.Strategy("regex_replace")))

	enforcement := guardrail.NewEnforcementPoint(registry, cfg, bus)

	mandatoryInput := guardrail.GuardrailSet{
		Guards: []guardrail.GuardrailSpec{
			{Name: "prompt_injection"},
			{Name: "secrets"},
			{Name: "pii"},
			{Name: "aim"},
			{Name: "lakera"},
			{Name: "lumigator"},
			{Name: "prisma_airs"},
			{Name: "nvidia_content"},
			{Name: "openai_moderation"},
			{Name: "zscaler"},
			{Name: "regex_match"},
			{Name: "contains"},
			{Name: "contains_code"},
			{Name: "ends_with"},
			{Name: "valid_urls"},
			{Name: "json_schema"},
			{Name: "json_keys"},
			{Name: "jwt"},
			{Name: "word_count"},
			{Name: "sentence_count"},
			{Name: "character_count"},
			{Name: "all_uppercase"},
			{Name: "all_lowercase"},
			{Name: "model_whitelist"},
			{Name: "model_rules"},
			{Name: "required_metadata_keys"},
			{Name: "allowed_request_types"},
			{Name: "not_null"},
			{Name: "webhook"},
			{Name: "log"},
			{Name: "add_prefix"},
			{Name: "regex_replace"},
		},
	}
	mandatoryOutput := guardrail.GuardrailSet{
		Guards: []guardrail.GuardrailSpec{
			{Name: "secrets"},
			{Name: "pii"},
			{Name: "aim"},
			{Name: "lakera"},
			{Name: "nvidia_content"},
			{Name: "openai_moderation"},
			{Name: "zscaler"},
			{Name: "regex_match"},
			{Name: "contains"},
			{Name: "contains_code"},
			{Name: "valid_urls"},
			{Name: "word_count"},
			{Name: "sentence_count"},
			{Name: "character_count"},
			{Name: "all_uppercase"},
			{Name: "all_lowercase"},
			{Name: "webhook"},
			{Name: "log"},
			{Name: "add_prefix"},
			{Name: "regex_replace"},
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
