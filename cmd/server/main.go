package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	"github.com/dexterhere04/AgentPlane/internal/iam"
	"github.com/dexterhere04/AgentPlane/internal/observability"
	"github.com/dexterhere04/AgentPlane/internal/policy"
	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/dexterhere04/AgentPlane/migrations"
	"github.com/joho/godotenv"
)

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
	// HTTP interface used by the admin analytics endpoint (default 8123).
	var chURL string
	// Typed analytics store backing the /api/observability/* dashboard API.
	var analyticsStore observability.AnalyticsStore
	if chCfg.Enabled {
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
				analyticsStore = observability.NewClickHouseAnalyticsStore(client)
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

	// Policy layer: IAM is the authorization implementation. Access is
	// deny-by-default and an explicit Deny overrides any Allow. The built-in
	// policies and the admin/member roles are seeded idempotently here so a
	// fresh install and an upgraded install converge on the same state.
	iamStore := iam.NewStore(pool)
	policyEP := policy.NewEnforcementPoint(iam.New(iamStore))

	if err := iamStore.SeedBuiltins(ctx); err != nil {
		log.Fatalf("seed IAM builtins: %v", err)
	}

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
		providerRegistry,
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
	// authorize wraps an admin handler so it is reachable by either the static
	// root admin token (AGENTPLANE_ADMIN_TOKEN) or a database-backed API key
	// whose user is permitted the given IAM action. The root token is the
	// bootstrap superuser and always bypasses policy evaluation.
	//
	// allowQuery additionally accepts the token via the "token" query
	// parameter, for browser clients (EventSource, top-level navigation) that
	// cannot set an Authorization header.
	authorize := func(action string, allowQuery bool, next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Preserve root admin-token access.
			const bearerPrefix = "Bearer "
			header := r.Header.Get("Authorization")
			token := ""
			if strings.HasPrefix(header, bearerPrefix) {
				token = strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
			}
			if token == "" && allowQuery {
				token = strings.TrimSpace(r.URL.Query().Get("token"))
			}

			if token != "" && subtle.ConstantTimeCompare(
				[]byte(token), []byte(adminToken),
			) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			// Otherwise, authenticate the submitted database-backed API key.
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			user, err := authenticator.Authenticate(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			decision, err := policyEP.Authorize(r.Context(), policy.Request{
				UserID:   user.ID,
				Action:   action,
				Resource: "arn:ap:*",
			})
			if err != nil {
				// Fail closed on an authorization outage (503, not 403).
				http.Error(w, "authorization failed", http.StatusServiceUnavailable)
				return
			}
			if decision != policy.DecisionAllow {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	adminOrRBAC := func(action string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler { return authorize(action, false, next) }
	}
	adminOrRBACQuery := func(action string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler { return authorize(action, true, next) }
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/chat",
		authenticator.Middleware(
			policyEP.Require(iam.ActionInvokeChat, "arn:ap:*")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handlers.ChatWithRouting(
						w, r, enforcement, policyEP, mandatoryInput, mandatoryOutput,
						router, iamStore, providerRegistry, selector,
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
		adminOrRBAC(iam.ActionProvisionUser)(
			handlers.ProvisionUser(provisioner),
		),
	)

	mux.Handle(
		"/admin/api-keys/revoke",
		adminOrRBAC(iam.ActionRevokeApiKey)(
			handlers.RevokeAPIKey(apiKeyStore),
		),
	)
	mux.Handle(
		"/admin/api-keys",
		adminOrRBAC(iam.ActionListApiKeys)(handlers.ListAPIKeys(apiKeyStore)),
	)
	mux.Handle(
		"/admin/analytics/users",
		adminOrRBAC(iam.ActionViewMetrics)(handlers.UserAnalyticsHandler()),
	)
	mux.Handle(
		"/admin/secrets/provider",
		adminOrRBAC(iam.ActionManageSecrets)(
			handlers.StoreProviderSecret(),
		),
	)

	// IAM administration. Access is deny-by-default and an explicit Deny
	// overrides any Allow; a user must be permitted the relevant action (via a
	// policy granting e.g. ap:ManageIAM, or the APAdmin policy) to operate
	// these endpoints. The built-in admin role carries APAdmin.
	mux.Handle("GET /admin/policies", adminOrRBAC(iam.ActionListPolicies)(handlers.ListPolicies(iamStore)))
	mux.Handle("POST /admin/policies", adminOrRBAC(iam.ActionCreatePolicy)(handlers.CreatePolicy(iamStore)))
	mux.Handle("GET /admin/policies/{name}", adminOrRBAC(iam.ActionGetPolicy)(handlers.GetPolicy(iamStore)))
	mux.Handle("PUT /admin/policies/{name}", adminOrRBAC(iam.ActionUpdatePolicy)(handlers.UpdatePolicy(iamStore)))
	mux.Handle("DELETE /admin/policies/{name}", adminOrRBAC(iam.ActionDeletePolicy)(handlers.DeletePolicy(iamStore)))
	mux.Handle("POST /admin/policies/{name}/attach", adminOrRBAC(iam.ActionAttachPolicy)(handlers.AttachPolicy(iamStore)))
	mux.Handle("POST /admin/policies/{name}/detach", adminOrRBAC(iam.ActionDetachPolicy)(handlers.DetachPolicy(iamStore)))
	mux.Handle("GET /admin/policies/{name}/attachments", adminOrRBAC(iam.ActionGetPolicy)(handlers.PolicyAttachments(iamStore)))

	mux.Handle("GET /admin/groups", adminOrRBAC(iam.ActionListGroups)(handlers.ListGroups(iamStore)))
	mux.Handle("POST /admin/groups", adminOrRBAC(iam.ActionCreateGroup)(handlers.CreateGroup(iamStore)))
	mux.Handle("DELETE /admin/groups/{name}", adminOrRBAC(iam.ActionDeleteGroup)(handlers.DeleteGroup(iamStore)))
	mux.Handle("GET /admin/groups/{name}/members", adminOrRBAC(iam.ActionGetGroup)(handlers.ListGroupMembers(iamStore)))
	mux.Handle("POST /admin/groups/{name}/members", adminOrRBAC(iam.ActionAddUserToGroup)(handlers.AddGroupMember(iamStore)))
	mux.Handle("DELETE /admin/groups/{name}/members/{id}", adminOrRBAC(iam.ActionRemoveUserFromGroup)(handlers.RemoveGroupMember(iamStore)))

	mux.Handle("GET /admin/roles", adminOrRBAC(iam.ActionListRoles)(handlers.ListRoles(iamStore)))
	mux.Handle("POST /admin/roles", adminOrRBAC(iam.ActionCreateRole)(handlers.CreateRole(iamStore)))
	mux.Handle("DELETE /admin/roles/{name}", adminOrRBAC(iam.ActionDeleteRole)(handlers.DeleteRole(iamStore)))
	mux.Handle("GET /admin/roles/{name}/grants", adminOrRBAC(iam.ActionListRoles)(handlers.RoleGrants(iamStore)))
	mux.Handle("POST /admin/roles/{name}/grant", adminOrRBAC(iam.ActionGrantRole)(handlers.GrantRole(iamStore)))
	mux.Handle("POST /admin/roles/{name}/revoke", adminOrRBAC(iam.ActionRevokeRole)(handlers.RevokeRole(iamStore)))

	mux.Handle("GET /admin/users", adminOrRBAC(iam.ActionListUsers)(handlers.ListUsers(userStore, iamStore)))
	mux.Handle("GET /admin/users/{id}/roles", adminOrRBAC(iam.ActionListRoles)(handlers.ListUserRoles(iamStore)))
	mux.Handle("POST /admin/users/{id}/roles", adminOrRBAC(iam.ActionGrantRole)(handlers.AssignUserRole(iamStore)))
	mux.Handle("DELETE /admin/users/{id}/roles/{role}", adminOrRBAC(iam.ActionRevokeRole)(handlers.RevokeUserRole(iamStore)))
	mux.Handle("POST /admin/users/{id}/permissions-boundary", adminOrRBAC(iam.ActionSetPermissionsBoundary)(handlers.SetUserPermissionsBoundary(iamStore)))
	mux.Handle("DELETE /admin/users/{id}", adminOrRBAC(iam.ActionDeleteUser)(handlers.DeleteUser(userStore)))

	// Operator-only observability endpoints. These require the root admin
	// token or an API key whose user is permitted ap:ViewMetrics. The
	// browser-facing /events, /dashboard, and /observability also accept the
	// admin token via the "token" query parameter since EventSource and
	// top-level navigation cannot set request headers.

	// Provider administration.
	mux.Handle("GET /admin/providers",
		adminOrRBAC(iam.ActionListProviders)(routingAdmin.ListProviders()))
	mux.Handle("POST /admin/providers",
		adminOrRBAC(iam.ActionCreateProvider)(routingAdmin.CreateProvider()))
	mux.Handle("GET /admin/providers/{id}",
		adminOrRBAC(iam.ActionListProviders)(routingAdmin.GetProvider()))
	mux.Handle("PUT /admin/providers/{id}",
		adminOrRBAC(iam.ActionUpdateProvider)(routingAdmin.UpdateProvider()))
	mux.Handle("POST /admin/providers/{id}/enable",
		adminOrRBAC(iam.ActionEnableProvider)(routingAdmin.EnableProvider()))
	mux.Handle("POST /admin/providers/{id}/disable",
		adminOrRBAC(iam.ActionDisableProvider)(routingAdmin.DisableProvider()))
	mux.Handle("DELETE /admin/providers/{id}",
		adminOrRBAC(iam.ActionDeleteProvider)(routingAdmin.DeleteProvider()))

	// Live upstream / circuit-breaker status. Read-only snapshot of the
	// runtime registry, separate from the persisted provider records above.
	mux.Handle("GET /admin/upstreams",
		adminOrRBAC(iam.ActionListProviders)(routingAdmin.ListUpstreams()))

	// Routing-rule administration.
	mux.Handle("GET /admin/routing-rules",
		adminOrRBAC(iam.ActionListRoutingRules)(routingAdmin.ListRules()))
	mux.Handle("POST /admin/routing-rules",
		adminOrRBAC(iam.ActionCreateRoutingRule)(routingAdmin.CreateRule()))
	mux.Handle("GET /admin/routing-rules/{id}",
		adminOrRBAC(iam.ActionListRoutingRules)(routingAdmin.GetRule()))
	mux.Handle("PUT /admin/routing-rules/{id}",
		adminOrRBAC(iam.ActionUpdateRoutingRule)(routingAdmin.UpdateRule()))
	mux.Handle("POST /admin/routing-rules/{id}/enable",
		adminOrRBAC(iam.ActionEnableRoutingRule)(routingAdmin.EnableRule()))
	mux.Handle("POST /admin/routing-rules/{id}/disable",
		adminOrRBAC(iam.ActionDisableRoutingRule)(routingAdmin.DisableRule()))
	mux.Handle("DELETE /admin/routing-rules/{id}",
		adminOrRBAC(iam.ActionDeleteRoutingRule)(routingAdmin.DeleteRule()))
	mux.Handle("/events", adminOrRBACQuery(iam.ActionViewMetrics)(observability.SSEHandler(bus)))
	if chURL != "" {
		mux.Handle("/analytics/traces_count",
			adminOrRBAC(iam.ActionViewMetrics)(handlers.AnalyticsHandler(chURL, "traces_count")))
		mux.Handle("/admin/analytics",
			adminOrRBAC(iam.ActionViewMetrics)(handlers.AnalyticsHandler(chURL, "")))
	}

	// Typed observability dashboard API (health, overview, series,
	// breakdowns, traces, guardrails, user usage, prompt search).
	analyticsHandler := handlers.NewObservabilityHandler(analyticsStore)
	mux.Handle("/api/observability/", adminOrRBAC(iam.ActionViewMetrics)(analyticsHandler))

	mux.Handle("/dashboard", adminOrRBACQuery(iam.ActionViewMetrics)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboard.HTML))
	})))
	mux.Handle("/observability", adminOrRBACQuery(iam.ActionViewMetrics)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(dashboard.ObservabilityHTML))
	})))

	mux.Handle("/metrics", adminOrRBAC(iam.ActionViewMetrics)(handlers.MetricsHandler()))

	log.Printf("AgentPlane Dev Mode")
	log.Printf("  Gateway   → http://localhost:%s/chat (%s)", port, provider.Name())
	log.Printf("  Events    → http://localhost:%s/events", port)
	log.Printf("  Dashboard → http://localhost:%s/dashboard", port)
	log.Printf("  Observe   → http://localhost:%s/observability", port)
	log.Printf("  Metrics   → http://localhost:%s/metrics", port)
	log.Printf("  Mock API  → http://localhost:%s (set OPENAI_BASE_URL)", port)

	err = http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
