// Command seed populates AgentPlane with a realistic demo organization:
// IAM policies/roles/groups, users with API keys, providers, routing rules,
// and a matching window of observability telemetry in ClickHouse.
//
// Typical use (see seed.sh for a ready-made wrapper):
//
//	go run ./cmd/seed --reset
//
// Environment: DATABASE_URL, CLICKHOUSE_HOST/PORT/USER/PASSWORD, API_KEY_PEPPER,
// AGENTPLANE_ADMIN_TOKEN (optional), GATEWAY_URL (optional).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/dexterhere04/AgentPlane/internal/seed"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("seed: no .env file loaded (%v)", err)
	}

	var (
		reset         = flag.Bool("reset", false, "delete previously seeded demo data before writing")
		days          = flag.Int("days", 7, "analytics window in days")
		traces        = flag.Int("traces", 12000, "approximate number of traces to generate")
		randSeed      = flag.Int64("seed", 42, "RNG seed for reproducible generation")
		baseURL       = flag.String("provider-base-url", env("SEED_PROVIDER_BASE_URL", "http://mock:3002/v1"), "base URL stored on seeded providers")
		gatewayURL    = flag.String("gateway-url", env("GATEWAY_URL", "http://localhost:3001"), "gateway base URL used to trigger a reload")
		orgID         = flag.String("org", seed.OrgID, "organization identifier recorded in telemetry")
		outPath       = flag.String("out", "", "optional path to write the run report as JSON")
		postgresOnly  = flag.Bool("postgres-only", false, "seed only Postgres (skip ClickHouse analytics)")
		analyticsOnly = flag.Bool("analytics-only", false, "seed only ClickHouse analytics (skip Postgres)")
	)
	flag.Parse()

	opts := seed.Options{
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		ClickHouseHost:  env("CLICKHOUSE_HOST", "localhost"),
		ClickHousePort:  envInt("CLICKHOUSE_PORT", 9000),
		Pepper:          resolvePepper(),
		GatewayURL:      strings.TrimSpace(*gatewayURL),
		AdminToken:      strings.TrimSpace(os.Getenv("AGENTPLANE_ADMIN_TOKEN")),
		ProviderBaseURL: strings.TrimSpace(*baseURL),
		OrgID:           strings.TrimSpace(*orgID),
		Reset:           *reset,
		Days:            *days,
		TraceCount:      *traces,
		RandSeed:        *randSeed,
		SkipPostgres:    *analyticsOnly,
		SkipAnalytics:   *postgresOnly,
	}

	if opts.SkipPostgres && opts.SkipAnalytics {
		log.Fatal("seed: --postgres-only and --analytics-only are mutually exclusive")
	}
	if opts.ProviderBaseURL == "" {
		log.Fatal("seed: --provider-base-url must not be empty")
	}

	if err := run(opts, *outPath); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
}

func run(opts seed.Options, outPath string) error {
	if !opts.SkipPostgres && opts.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	if !opts.SkipPostgres && opts.Pepper == "" {
		return fmt.Errorf("API_KEY_PEPPER is not set (needed to mint API keys)")
	}

	log.Printf("seeding %s (reset=%t, days=%d, traces=%d, seed=%d)",
		opts.OrgID, opts.Reset, opts.Days, opts.TraceCount, opts.RandSeed)

	report, err := seed.Run(context.Background(), opts)
	if err != nil {
		return err
	}

	printReport(report)

	if outPath != "" {
		if err := writeReportJSON(outPath, report); err != nil {
			return err
		}
		log.Printf("wrote run report to %s", outPath)
	}
	return nil
}

func printReport(r *seed.Report) {
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println(" Seed complete")
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Printf("  Organization   : %s\n", r.Org)
	fmt.Printf("  Policies       : %d\n", r.Policies)
	fmt.Printf("  Roles          : %d\n", r.Roles)
	fmt.Printf("  Groups (teams) : %d\n", r.Groups)
	fmt.Printf("  Users          : %d\n", r.Users)
	fmt.Printf("  Providers      : %d\n", r.Providers)
	fmt.Printf("  Routing rules  : %d\n", r.RoutingRules)
	fmt.Printf("  Traces         : %d\n", r.Traces)
	fmt.Printf("  Usage events   : %d\n", r.UsageEvents)
	fmt.Printf("  Prompt events  : %d\n", r.PromptEvents)
	fmt.Printf("  Guardrail evts : %d\n", r.GuardrailEvents)
	fmt.Printf("  Tool calls     : %d\n", r.ToolCallEvents)
	fmt.Println("──────────────────────────────────────────────────────────────")

	if len(r.Credentials) == 0 {
		return
	}
	fmt.Println(" Demo API keys (shown once — copy them now):")
	fmt.Println()
	for _, c := range r.Credentials {
		roles := strings.Join(c.Roles, ",")
		if roles == "" {
			roles = "-"
		}
		fmt.Printf("  %-22s %-16s roles=%-28s\n", c.Username, c.Team, roles)
		fmt.Printf("      %s\n", c.APIKey)
	}
	fmt.Println()
	fmt.Println(" Use with:  curl -H \"Authorization: Bearer <key>\" http://localhost:3001/chat ...")
	fmt.Println()
}

func writeReportJSON(path string, r *seed.Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// resolvePepper reads API_KEY_PEPPER, falling back to the configured secret
// store (e.g. Vault) so the seed mints keys the gateway will accept.
func resolvePepper() string {
	if p := strings.TrimSpace(os.Getenv("API_KEY_PEPPER")); p != "" {
		return p
	}
	if err := config.ConfigureSecretStore(); err != nil {
		return ""
	}
	if p, err := config.KeyPepper(); err == nil {
		return p
	}
	return ""
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
