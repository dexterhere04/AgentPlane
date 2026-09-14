package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/auth"
	"github.com/dexterhere04/AgentPlane/internal/clickhouse"
	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/dashboard"
	"github.com/dexterhere04/AgentPlane/internal/db"
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
	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/dexterhere04/AgentPlane/migrations"
	"github.com/joho/godotenv"
)

// Analytics queries are forwarded to ClickHouse's HTTP interface (see
// handlers.AnalyticsHandler). The window bounds and HTTP client live there.

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

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Fatalf("Database migrations: %v", err)
	}

	userStore := users.NewStore(pool)
	apiKeyStore := api.NewStore(pool)
	authStore := auth.NewStore(pool)

	adminToken, err := config.AdminToken()
	if err != nil {
		log.Fatalf("Admin token: %v", err)
	}

	bus := observability.DefaultBus

	// ClickHouse observability store initialization (optional).
	chCfg := clickhouse.LoadFromEnv()
	var chURL string
	if chCfg.Enabled {
		// HTTP interface used by the analytics endpoint (default 8123).
		chHTTPPort := 8123
		if v := os.Getenv("CLICKHOUSE_HTTP_PORT"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				chHTTPPort = n
			}
		}
		chURL = fmt.Sprintf("http://%s:%d", chCfg.Host, chHTTPPort)
		// initialize native ClickHouse client
		if client, err := clickhouse.New(chCfg.Host, chCfg.Port); err != nil {
			log.Printf("clickhouse native init error: %v", err)
		} else {
			adapter := observability.NewClickHouseAdapter(client)
			if err := adapter.Init(); err != nil {
				log.Printf("clickhouse adapter init error: %v", err)
			} else {
				observability.SetStore(adapter)
				log.Printf("ClickHouse observability enabled (host=%s port=%d)", chCfg.Host, chCfg.Port)
			}
		}
	}

	if err := config.ConfigureSecretStore(); err != nil {
		log.Fatalf("Secret store: %v", err)
	}

	pepper, err := config.KeyPepper()
	if err != nil {
		log.Fatalf("API key pepper: %v", err)
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
			{Name: "content_moderation"},
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
			{Name: "content_moderation"},
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
	mux.Handle(
		"/chat",
		authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.Chat(w, r, enforcement, mandatoryInput, mandatoryOutput, provider)
		})),
	)

	// SECURITY: /provision/user creates users and mints API keys, so it is
	// gated behind the same admin authentication as /admin/api-keys/revoke.
	// It must never be reachable without adminToken — this is what mints
	// the credentials that everything else in the gateway trusts.
	mux.Handle(
		"/provision/user",
		auth.AdminMiddleware(
			adminToken,
			handlers.ProvisionUser(provisioner),
		),
	)

	mux.Handle(
		"/admin/api-keys/revoke",
		auth.AdminMiddleware(
			adminToken,
			handlers.RevokeAPIKey(apiKeyStore),
		),
	)
	mux.Handle(
		"/admin/api-keys",
		auth.AdminMiddleware(
			adminToken,
			handlers.ListAPIKeys(apiKeyStore),
		),
	)
	mux.Handle(
		"/admin/analytics/users",
		auth.AdminMiddleware(
			adminToken,
			handlers.UserAnalyticsHandler(),
		),
	)
	mux.Handle(
		"/admin/secrets/provider",
		auth.AdminMiddleware(
			adminToken,
			handlers.StoreProviderSecret(),
		),
	)
	mux.HandleFunc("/events", observability.SSEHandler(bus))
	if chURL != "" {
		mux.Handle("/analytics/traces_count", auth.AdminMiddleware(adminToken, handlers.AnalyticsHandler(chURL, "traces_count")))
		mux.Handle("/admin/analytics", auth.AdminMiddleware(adminToken, handlers.AnalyticsHandler(chURL, "")))
	}
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

	err = http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
