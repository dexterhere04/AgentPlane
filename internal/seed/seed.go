package seed

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/db"
	"github.com/dexterhere04/AgentPlane/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options configures a seeding run.
type Options struct {
	// DatabaseURL is the Postgres DSN. Required unless SkipPostgres.
	DatabaseURL string

	// ClickHouse connection. Required unless SkipAnalytics.
	ClickHouseHost string
	ClickHousePort int

	// Pepper must match the running gateway's API_KEY_PEPPER so provisioned
	// keys authenticate.
	Pepper string

	// GatewayURL + AdminToken are used to poke the running gateway into
	// reloading providers/routing rules after seeding. Best-effort.
	GatewayURL string
	AdminToken string

	// ProviderBaseURL is stored on every seeded provider. It defaults (in the
	// command) to the in-network mock so a seeded stack routes end to end.
	ProviderBaseURL string

	// OrgID overrides the organization identifier recorded in telemetry.
	OrgID string

	// Reset deletes previously seeded demo entities before writing.
	Reset bool

	// Analytics window and volume.
	Days       int
	TraceCount int

	// RandSeed makes generation reproducible.
	RandSeed int64

	SkipPostgres  bool
	SkipAnalytics bool

	// Logf receives progress messages; defaults to the standard logger.
	Logf func(format string, args ...any)
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (o Options) orgID() string {
	if strings.TrimSpace(o.OrgID) != "" {
		return o.OrgID
	}
	return OrgID
}

// Credential is a provisioned demo API key. The plaintext key is only ever
// available at creation time.
type Credential struct {
	Username       string   `json:"username"`
	Email          string   `json:"email,omitempty"`
	Team           string   `json:"team"`
	Roles          []string `json:"roles,omitempty"`
	BoundaryPolicy string   `json:"permissions_boundary,omitempty"`
	APIKey         string   `json:"api_key"`
}

// Report summarizes a seeding run.
type Report struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Org        string    `json:"org"`

	Policies     int `json:"policies"`
	Roles        int `json:"roles"`
	Groups       int `json:"groups"`
	Users        int `json:"users"`
	Providers    int `json:"providers"`
	RoutingRules int `json:"routing_rules"`

	Traces          int `json:"traces"`
	UsageEvents     int `json:"usage_events"`
	PromptEvents    int `json:"prompt_events"`
	ResponseEvents  int `json:"response_events"`
	GuardrailEvents int `json:"guardrail_events"`
	ToolCallEvents  int `json:"tool_call_events"`

	Credentials []Credential `json:"credentials"`
}

// Run seeds Postgres and ClickHouse according to opts.
func Run(ctx context.Context, opts Options) (*Report, error) {
	rep := &Report{StartedAt: time.Now().UTC(), Org: opts.orgID()}

	needPool := !opts.SkipPostgres || !opts.SkipAnalytics
	var pool *pgxpool.Pool
	if needPool {
		if opts.DatabaseURL == "" {
			return nil, fmt.Errorf("seed: DATABASE_URL is required")
		}
		p, err := db.NewPool(ctx, opts.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("seed: connect postgres: %w", err)
		}
		defer p.Close()
		pool = p
	}

	var providerIDs []uuid.UUID

	if !opts.SkipPostgres {
		opts.logf("applying migrations")
		if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
			return nil, fmt.Errorf("seed: migrate: %w", err)
		}

		opts.logf("seeding Postgres (policies, roles, groups, users, providers, routing rules)")
		ids, err := seedPostgres(ctx, pool, opts, rep)
		if err != nil {
			return nil, err
		}
		providerIDs = ids
	}

	if !opts.SkipAnalytics {
		opts.logf("seeding ClickHouse analytics (%d days, ~%d traces)", opts.Days, opts.TraceCount)
		if err := seedAnalytics(ctx, pool, opts, rep); err != nil {
			return nil, err
		}
	}

	if !opts.SkipPostgres && opts.GatewayURL != "" && opts.AdminToken != "" {
		opts.logf("asking the gateway to reload providers and routing rules")
		if err := triggerGatewayReload(ctx, opts, providerIDs); err != nil {
			opts.logf("gateway reload skipped: %v (restart the gateway to pick up providers/rules)", err)
		}
	}

	rep.FinishedAt = time.Now().UTC()
	return rep, nil
}

// triggerGatewayReload performs a no-op provider enable via the admin API,
// which makes the gateway reload providers and routing rules from Postgres.
func triggerGatewayReload(ctx context.Context, opts Options, providerIDs []uuid.UUID) error {
	if len(providerIDs) == 0 {
		return fmt.Errorf("no seeded providers to reload")
	}
	url := strings.TrimRight(opts.GatewayURL, "/") + "/admin/providers/" + providerIDs[0].String() + "/enable"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+opts.AdminToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %s", resp.Status)
	}
	return nil
}
