package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

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
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/policy/rbac"
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

	// Policy layer: RBAC is the first (and currently only) policy. Access is
	// deny-by-default — a user with no assigned role is permitted nothing.
	rbacStore := rbac.NewStore(pool)
	policyEP := policy.NewEnforcementPoint(rbac.New(rbacStore))

	routingStore := proxy.NewRoutingRuleStore(pool)

	ruleSet, err := routingStore.LoadRuleSet(ctx)
	if err != nil {
		log.Fatalf("load routing rules: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	provider := proxy.NewOpenAIProviderFromEnv()

	providerRegistry := proxy.NewRegistry()

	providerStore := proxy.NewProviderStore(pool)

	upstreams, err := providerStore.ListEnabled(ctx)
	if err != nil {
		log.Fatalf("load providers: %v", err)
	}

	for _, upstream := range upstreams {
		if err := providerRegistry.Register(upstream); err != nil {
			log.Fatalf("register provider %q: %v", upstream.Name, err)
		}
	}

	selector := proxy.NewWeightedSelector(time.Now().UnixNano())
	router := proxy.NewRouter(
		provider,
		ruleSet,
		providerRegistry,
		selector,
	)

	reloadRouting := func(ctx context.Context) error {
		newRuleSet, err := routingStore.LoadRuleSet(ctx)
		if err != nil {
			return fmt.Errorf("reload routing rules: %w", err)
		}

		newUpstreams, err := providerStore.ListEnabled(ctx)
		if err != nil {
			return fmt.Errorf("reload providers: %w", err)
		}

		if err := providerRegistry.ReplaceAll(newUpstreams); err != nil {
			return fmt.Errorf("replace provider registry: %w", err)
		}

		router.SetRuleSet(newRuleSet)

		return nil
	}

	routingAdmin := handlers.NewRoutingAdmin(
		providerStore,
		routingStore,
		reloadRouting,
	)
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

	// The core, always-available, locally-evaluated guardrails are marked
	// Required: true so they run even when their strategy is disabled and fail
	// closed on any resolution or evaluation error — a guardrail error blocks
	// the request (503) rather than being skipped. Optional/external guardrails
	// remain Required: false and are skipped on error (fail open).
	mandatoryInput := guardrail.GuardrailSet{
		Guards: []guardrail.GuardrailSpec{
			{Name: "prompt_injection", Required: true},
			{Name: "secrets", Required: true},
			{Name: "pii", Required: true},
			{Name: "content_moderation", Required: true},
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
			{Name: "secrets", Required: true},
			{Name: "pii", Required: true},
			{Name: "content_moderation", Required: true},
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

	// Fail fast on misconfiguration: every Required guardrail must resolve in
	// the registry, otherwise the gateway would start unable to enforce a
	// mandatory security control.
	if err := enforcement.ValidateSet(mandatoryInput); err != nil {
		log.Fatalf("mandatory input guardrails: %v", err)
	}
	if err := enforcement.ValidateSet(mandatoryOutput); err != nil {
		log.Fatalf("mandatory output guardrails: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/chat",
		authenticator.Middleware(
			policyEP.Require("chat:invoke")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handlers.ChatWithRouting(
						w, r, enforcement, policyEP, mandatoryInput, mandatoryOutput,
						router, rbacStore, providerRegistry, selector,
					)
				}),
			),
		),
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

	// RBAC role administration. Roles are assigned to users; access is
	// deny-by-default, so a user must be granted a role (e.g. "member")
	// before they can use the gateway.
	mux.Handle("GET /admin/roles", auth.AdminMiddleware(adminToken, handlers.ListRoles(rbacStore)))
	mux.Handle("POST /admin/roles", auth.AdminMiddleware(adminToken, handlers.CreateRole(rbacStore)))
	mux.Handle("POST /admin/roles/{name}/permissions", auth.AdminMiddleware(adminToken, handlers.AddRolePermission(rbacStore)))
	mux.Handle("DELETE /admin/roles/{name}/permissions/{permission}", auth.AdminMiddleware(adminToken, handlers.RemoveRolePermission(rbacStore)))
	mux.Handle("GET /admin/users", auth.AdminMiddleware(adminToken, handlers.ListUsers(userStore, rbacStore)))
	mux.Handle("GET /admin/users/{id}/roles", auth.AdminMiddleware(adminToken, handlers.ListUserRoles(rbacStore)))
	mux.Handle("POST /admin/users/{id}/roles", auth.AdminMiddleware(adminToken, handlers.AssignUserRole(rbacStore)))
	mux.Handle("DELETE /admin/users/{id}/roles/{role}", auth.AdminMiddleware(adminToken, handlers.RevokeUserRole(rbacStore)))
	// Operator-only observability endpoints. All three require the admin
	// token; the browser-facing /events and /dashboard also accept it via the
	// "token" query parameter since EventSource and top-level navigation
	// cannot set request headers.
	mux.Handle("GET /admin/providers",
		auth.AdminMiddleware(adminToken, routingAdmin.ListProviders()))

	mux.Handle("POST /admin/providers",
		auth.AdminMiddleware(adminToken, routingAdmin.CreateProvider()))

	mux.Handle("GET /admin/providers/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.GetProvider()))

	mux.Handle("PUT /admin/providers/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.UpdateProvider()))

	mux.Handle("POST /admin/providers/{id}/enable",
		auth.AdminMiddleware(adminToken, routingAdmin.EnableProvider()))

	mux.Handle("POST /admin/providers/{id}/disable",
		auth.AdminMiddleware(adminToken, routingAdmin.DisableProvider()))

	mux.Handle("DELETE /admin/providers/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.DeleteProvider()))

	mux.Handle("GET /admin/routing-rules",
		auth.AdminMiddleware(adminToken, routingAdmin.ListRules()))

	mux.Handle("POST /admin/routing-rules",
		auth.AdminMiddleware(adminToken, routingAdmin.CreateRule()))

	mux.Handle("GET /admin/routing-rules/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.GetRule()))

	mux.Handle("PUT /admin/routing-rules/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.UpdateRule()))

	mux.Handle("POST /admin/routing-rules/{id}/enable",
		auth.AdminMiddleware(adminToken, routingAdmin.EnableRule()))

	mux.Handle("POST /admin/routing-rules/{id}/disable",
		auth.AdminMiddleware(adminToken, routingAdmin.DisableRule()))

	mux.Handle("DELETE /admin/routing-rules/{id}",
		auth.AdminMiddleware(adminToken, routingAdmin.DeleteRule()))
	mux.Handle("/events", auth.AdminMiddlewareQuery(adminToken, observability.SSEHandler(bus)))
	if chURL != "" {
		mux.Handle("/analytics/traces_count", auth.AdminMiddleware(adminToken, handlers.AnalyticsHandler(chURL, "traces_count")))
		mux.Handle("/admin/analytics", auth.AdminMiddleware(adminToken, handlers.AnalyticsHandler(chURL, "")))
	}
	mux.Handle("/dashboard", auth.AdminMiddlewareQuery(adminToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboard.HTML))
	})))

	mux.Handle("/metrics", auth.AdminMiddleware(adminToken, handlers.MetricsHandler()))

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
