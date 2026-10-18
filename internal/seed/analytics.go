package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/clickhouse"
	"github.com/jackc/pgx/v5/pgxpool"
)

// demoUser is a seeded identity joined with the traffic attributes that drive
// analytics generation.
type demoUser struct {
	ID       string
	Username string
	Team     string
	Weight   int
	EU       bool
}

// dataset holds every telemetry row a generation run produces.
type dataset struct {
	traces     []clickhouse.TraceEvent
	usage      []clickhouse.UsageEvent
	prompts    []clickhouse.PromptEvent
	responses  []clickhouse.ResponseEvent
	guardrails []clickhouse.GuardrailEvent
	tools      []clickhouse.ToolCallEvent
}

// seedAnalytics writes an observability window into ClickHouse.
func seedAnalytics(ctx context.Context, pool *pgxpool.Pool, opts Options, rep *Report) error {
	if opts.ClickHouseHost == "" {
		return fmt.Errorf("seed: CLICKHOUSE_HOST is required for analytics")
	}
	if opts.Days <= 0 {
		opts.Days = 7
	}
	if opts.TraceCount <= 0 {
		opts.TraceCount = 12000
	}

	client, err := clickhouse.New(opts.ClickHouseHost, opts.ClickHousePort)
	if err != nil {
		return fmt.Errorf("seed: connect clickhouse: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("seed: ping clickhouse: %w", err)
	}

	demoUsers, err := loadDemoUsers(ctx, pool)
	if err != nil {
		return err
	}
	if len(demoUsers) == 0 {
		return fmt.Errorf("seed: no demo users found; run without --analytics-only first")
	}

	if opts.Reset {
		opts.logf("  truncating analytics tables")
		if err := client.TruncateTables(ctx,
			"traces", "prompt_events", "response_events",
			"tool_call_events", "guardrail_events", "usage_events",
		); err != nil {
			return err
		}
	}

	g := newGenerator(opts, demoUsers)
	const chunkSize = 8000

	ds, err := g.generate(time.Now().UTC())
	if err != nil {
		return err
	}
	opts.logf("  generated %d traces, %d prompt captures, %d guardrail events",
		len(ds.traces), len(ds.prompts), len(ds.guardrails))

	for _, part := range chunk(ds.traces, chunkSize) {
		if err := client.InsertTracesBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert traces: %w", err)
		}
	}
	for _, part := range chunk(ds.usage, chunkSize) {
		if err := client.InsertUsageBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert usage events: %w", err)
		}
	}
	for _, part := range chunk(ds.prompts, chunkSize) {
		if err := client.InsertPromptsBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert prompt events: %w", err)
		}
	}
	for _, part := range chunk(ds.responses, chunkSize) {
		if err := client.InsertResponsesBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert response events: %w", err)
		}
	}
	for _, part := range chunk(ds.guardrails, chunkSize) {
		if err := client.InsertGuardrailsBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert guardrail events: %w", err)
		}
	}
	for _, part := range chunk(ds.tools, chunkSize) {
		if err := client.InsertToolCallsBatch(ctx, part); err != nil {
			return fmt.Errorf("seed: insert tool calls: %w", err)
		}
	}

	rep.Traces = len(ds.traces)
	rep.UsageEvents = len(ds.usage)
	rep.PromptEvents = len(ds.prompts)
	rep.ResponseEvents = len(ds.responses)
	rep.GuardrailEvents = len(ds.guardrails)
	rep.ToolCallEvents = len(ds.tools)
	return nil
}

// loadDemoUsers reads the seeded users' IDs and joins them with catalog
// attributes (team, traffic weight, EU residency).
func loadDemoUsers(ctx context.Context, pool *pgxpool.Pool) ([]demoUser, error) {
	names := demoUsernameList()
	rows, err := pool.Query(ctx, `SELECT id::text, username FROM users WHERE username = ANY($1)`, names)
	if err != nil {
		return nil, fmt.Errorf("seed: load demo users: %w", err)
	}
	defer rows.Close()

	byName := make(map[string]string, len(names))
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err != nil {
			return nil, fmt.Errorf("seed: scan demo user: %w", err)
		}
		byName[username] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("seed: iterate demo users: %w", err)
	}

	out := make([]demoUser, 0, len(names))
	for _, u := range demoUsers() {
		id, ok := byName[u.Username]
		if !ok {
			continue
		}
		out = append(out, demoUser{
			ID:       id,
			Username: u.Username,
			Team:     u.Team,
			Weight:   u.TrafficWeight,
			EU:       u.EU,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Generator
// ---------------------------------------------------------------------------

type generator struct {
	rng        *rand.Rand
	org        string
	days       int
	traceCount int

	users      []demoUser
	userTable  *weightedTable[demoUser]
	models     []modelSpec
	modelTable *weightedTable[modelSpec]
}

func newGenerator(opts Options, users []demoUser) *generator {
	models := demoModels()
	return &generator{
		rng:        rand.New(rand.NewSource(opts.RandSeed)),
		org:        opts.orgID(),
		days:       opts.Days,
		traceCount: opts.TraceCount,
		users:      users,
		userTable:  newWeightedTable(users, func(u demoUser) int { return u.Weight }),
		models:     models,
		modelTable: newWeightedTable(models, func(m modelSpec) int { return m.TrafficWeight }),
	}
}

func (g *generator) generate(now time.Time) (dataset, error) {
	ds := dataset{
		traces:     make([]clickhouse.TraceEvent, 0, g.traceCount),
		usage:      make([]clickhouse.UsageEvent, 0, g.traceCount),
		prompts:    make([]clickhouse.PromptEvent, 0, g.traceCount/3),
		responses:  make([]clickhouse.ResponseEvent, 0, g.traceCount/4),
		guardrails: make([]clickhouse.GuardrailEvent, 0, g.traceCount/3),
		tools:      make([]clickhouse.ToolCallEvent, 0, g.traceCount/12),
	}

	start := now.AddDate(0, 0, -g.days).Truncate(time.Hour)
	buckets := distributeAcrossHours(start, now, g.traceCount)

	for _, bucket := range buckets {
		span := time.Hour
		if remaining := now.Sub(bucket.start); remaining < span {
			span = remaining
		}
		for i := 0; i < bucket.count && span > 0; i++ {
			// Uniform within the hour; the RNG already shapes the hour's weight.
			offset := time.Duration(g.rng.Int63n(int64(span)))
			g.emit(&ds, bucket.start.Add(offset))
		}
	}

	// Guarantee a fresh tail so the dashboard health check reports "fresh"
	// rather than "stale" (the check flags telemetry older than 15 minutes).
	tail := clampInt(g.traceCount/150, 12, 60)
	for i := 0; i < tail; i++ {
		offset := time.Duration(g.rng.Int63n(int64(12 * time.Minute)))
		g.emit(&ds, now.Add(-offset).Add(-20*time.Second))
	}

	return ds, nil
}

// emit generates one trace and its related telemetry rows.
func (g *generator) emit(ds *dataset, ts time.Time) {
	user := g.userTable.pick(g.rng)
	model := g.modelTable.pick(g.rng)

	provider := model.Provider
	route := model.Route
	attempt := uint32(1)
	failover := uint8(0)
	var failoverFrom, failoverTo string

	inTok, outTok, latency, status, guardrailAction, cost := g.outcome(model)

	// Failover: a slice of requests the primary could not serve, retried on a
	// backup upstream. Kept consistent between provider/upstream/failover_*.
	if status == "success" && g.rng.Float64() < 0.025 {
		if backup := g.backupFor(model); backup != "" {
			failoverFrom = provider
			failoverTo = backup
			provider = backup
			attempt = 2
			failover = 1
		}
	}

	cacheHit := status == "success" && g.rng.Float64() < 0.12
	if cacheHit {
		cost *= 0.6
	}

	traceID := "trc_" + g.hex(16)
	requestID := "req_" + g.hex(8)
	project := pickOne(g.rng, Projects)

	ds.traces = append(ds.traces, clickhouse.TraceEvent{
		TraceID:         traceID,
		RequestID:       requestID,
		Timestamp:       ts,
		UserID:          user.ID,
		Username:        user.Username,
		OrgID:           g.org,
		ProjectID:       project,
		Provider:        provider,
		Model:           model.Name,
		LatencyMs:       uint32(latency),
		Status:          status,
		CacheHit:        cacheHit,
		InputTokens:     uint32(inTok),
		OutputTokens:    uint32(outTok),
		TotalTokens:     uint32(inTok + outTok),
		EstimatedCost:   cost,
		GuardrailAction: guardrailAction,
		Route:           route,
		Upstream:        provider,
		Attempt:         attempt,
		Failover:        failover,
		FailoverFrom:    failoverFrom,
		FailoverTo:      failoverTo,
	})

	reasoning := 0
	if model.Name == "o3-mini" || strings.HasPrefix(model.Name, "o1") {
		reasoning = outTok / 3
	}
	cached := 0
	if cacheHit {
		cached = inTok / 2
	}
	ds.usage = append(ds.usage, clickhouse.UsageEvent{
		TraceID:           traceID,
		Provider:          provider,
		Model:             model.Name,
		InputTokens:       uint32(inTok),
		OutputTokens:      uint32(outTok),
		ReasoningTokens:   uint32(reasoning),
		CachedInputTokens: uint32(cached),
		EstimatedCost:     cost,
		Route:             route,
		Upstream:          provider,
		Attempt:           attempt,
		Failover:          failover,
		FailoverFrom:      failoverFrom,
		FailoverTo:        failoverTo,
		CreatedAt:         ts,
	})

	// Prompt capture.
	if g.rng.Float64() < 0.38 {
		g.emitPrompt(ds, traceID, user, model, ts)
	}
	if g.rng.Float64() < 0.24 {
		ds.responses = append(ds.responses, clickhouse.ResponseEvent{
			TraceID:        traceID,
			ResponseHash:   g.hash(fmt.Sprintf("resp:%s:%d", traceID, outTok)),
			ResponseBytes:  uint32(outTok * 4),
			CompressedSize: uint32(outTok * 2),
			CaptureMode:    g.captureMode(),
			CreatedAt:      ts,
		})
	}

	// Guardrail events.
	g.emitGuardrails(ds, traceID, guardrailAction, ts)

	// Tool calls.
	if g.rng.Float64() < 0.09 {
		toolCount := 1 + g.rng.Intn(2)
		for i := 0; i < toolCount; i++ {
			name := pickOne(g.rng, toolNames)
			success := g.rng.Float64() > 0.12
			ds.tools = append(ds.tools, clickhouse.ToolCallEvent{
				TraceID:   traceID,
				ToolName:  name,
				ToolInput: fmt.Sprintf(`{"tool":%q,"call":%d}`, name, i+1),
				Success:   success,
				LatencyMs: uint32(clampInt(int(lognormal(g.rng, 220, 0.6)), 15, 8000)),
				CreatedAt: ts,
			})
		}
	}
}

// outcome rolls the request result and returns tokens/latency/status/cost.
func (g *generator) outcome(m modelSpec) (inTok, outTok, latency int, status, guardrail string, cost float64) {
	roll := g.rng.Float64()
	switch {
	case roll < 0.020: // guardrail block
		return g.blockedOutcome(m)
	case roll < 0.055: // provider error
		return g.errorOutcome(m)
	default: // success
		inTok = clampInt(int(lognormal(g.rng, float64(m.InTokens), 0.55)), 24, m.InTokens*6)
		outTok = clampInt(int(lognormal(g.rng, float64(m.OutTokens), 0.6)), 1, m.OutTokens*6)
		latency = clampInt(int(float64(m.LatencyMS)*(0.55+g.rng.Float64()*1.5))+outTok/3, 45, 110000)
		cost = estimateCost(m, inTok, outTok)
		guardrail = "pass"
		if g.rng.Float64() < 0.035 {
			guardrail = "redact"
		}
		return inTok, outTok, latency, "success", guardrail, cost
	}
}

func (g *generator) blockedOutcome(m modelSpec) (int, int, int, string, string, float64) {
	// Input-phase blocks never reach the provider; output-phase blocks do.
	if g.rng.Float64() < 0.4 {
		latency := clampInt(int(lognormal(g.rng, 70, 0.5)), 12, 1500)
		return 0, 0, latency, "success", "block", 0
	}
	inTok := clampInt(int(lognormal(g.rng, float64(m.InTokens), 0.5)), 24, m.InTokens*6)
	outTok := clampInt(int(lognormal(g.rng, float64(m.OutTokens)*0.5, 0.6)), 1, m.OutTokens*4)
	latency := clampInt(int(float64(m.LatencyMS)*(0.6+g.rng.Float64()*1.2)), 45, 110000)
	return inTok, outTok, latency, "success", "block", estimateCost(m, inTok, outTok)
}

func (g *generator) errorOutcome(m modelSpec) (int, int, int, string, string, float64) {
	inTok := clampInt(int(lognormal(g.rng, float64(m.InTokens)*0.7, 0.5)), 12, m.InTokens*4)
	status := weightedString(g.rng,
		[]string{"error", "timeout", "rate_limited", "upstream_error"},
		[]int{40, 20, 25, 15})

	var latency int
	switch status {
	case "timeout":
		latency = 28000 + g.rng.Intn(32000)
	case "rate_limited":
		latency = 90 + g.rng.Intn(400)
	default:
		latency = clampInt(int(float64(m.LatencyMS)*(1.2+g.rng.Float64()*2.2)), 60, 90000)
	}
	return inTok, 0, latency, status, "", 0
}

func (g *generator) emitPrompt(ds *dataset, traceID string, user demoUser, m modelSpec, ts time.Time) {
	mode := g.captureMode()
	prompt := g.promptText(user, m, traceID)
	event := clickhouse.PromptEvent{
		TraceID:     traceID,
		UserID:      user.ID,
		Username:    user.Username,
		PromptHash:  g.hash(prompt),
		CaptureMode: mode,
		CreatedAt:   ts,
	}
	if mode != "hash_only" {
		event.PromptText = prompt
		event.PromptBytes = uint32(len(prompt))
		event.CompressedSize = uint32(len(prompt) / 3)
	} else {
		event.PromptBytes = uint32(len(prompt))
		event.CompressedSize = 0
	}
	ds.prompts = append(ds.prompts, event)
}

func (g *generator) emitGuardrails(ds *dataset, traceID, guardrailAction string, ts time.Time) {
	appendEvent := func(name, phase, action, reason string) {
		ds.guardrails = append(ds.guardrails, clickhouse.GuardrailEvent{
			TraceID:       traceID,
			GuardrailName: name,
			Phase:         phase,
			Action:        action,
			Reason:        reason,
			CreatedAt:     ts,
		})
	}

	switch guardrailAction {
	case "block":
		name := pickOne(g.rng, guardrailNames)
		phase := "input"
		if g.rng.Float64() < 0.4 {
			phase = "output"
		}
		appendEvent(name, phase, "block", pickOne(g.rng, blockReasons))
		return
	case "redact":
		appendEvent(pickOne(g.rng, guardrailNames), "output", "redact", "sensitive content redacted")
		return
	case "pass":
		// Safe traffic: a fraction records the positive decisions that ran.
		if g.rng.Float64() >= 0.12 {
			return
		}
		appendEvent(pickOne(g.rng, guardrailNames), "input", "pass", "no issues detected")
		if g.rng.Float64() < 0.5 {
			appendEvent(pickOne(g.rng, guardrailNames), "output", "pass", "no issues detected")
		}
	}
}

func (g *generator) backupFor(m modelSpec) string {
	if m.Backup != "" {
		return m.Backup
	}
	if m.Provider == "openrouter-pool" {
		return ""
	}
	return "openrouter-pool"
}

func (g *generator) captureMode() string {
	return weightedString(g.rng, []string{"full", "sample", "hash_only"}, []int{60, 25, 15})
}

func (g *generator) promptText(user demoUser, m modelSpec, traceID string) string {
	template := pickOne(g.rng, promptTemplates)
	context := pickOne(g.rng, promptContexts)
	return fmt.Sprintf("%s\n\nContext: team=%s, project=%s, model=%s, ref=%s",
		fmt.Sprintf(template, context), user.Team, pickOne(g.rng, Projects), m.Name, traceID[len(traceID)-6:])
}

// hex returns n random bytes hex-encoded from the deterministic RNG.
func (g *generator) hex(n int) string {
	buf := make([]byte, n)
	g.rng.Read(buf)
	return hex.EncodeToString(buf)
}

func (g *generator) hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Distribution helpers
// ---------------------------------------------------------------------------

type hourBucket struct {
	start time.Time
	count int
}

// hourWeights shapes a working-day diurnal curve (UTC).
var hourWeights = [24]float64{
	0.15, 0.12, 0.10, 0.10, 0.12, 0.20,
	0.35, 0.60, 1.00, 1.30, 1.45, 1.45,
	1.10, 1.35, 1.45, 1.40, 1.20, 1.00,
	0.80, 0.55, 0.45, 0.35, 0.25, 0.18,
}

func diurnalWeight(t time.Time) float64 {
	w := hourWeights[t.Hour()]
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		w *= 0.45
	}
	return w
}

// distributeAcrossHours allocates total requests across hourly buckets in
// [start, end) proportionally to the diurnal curve, then corrects rounding
// drift so the counts sum to total.
func distributeAcrossHours(start, end time.Time, total int) []hourBucket {
	var buckets []hourBucket
	var weights []float64
	sum := 0.0
	for t := start; t.Before(end); t = t.Add(time.Hour) {
		w := diurnalWeight(t)
		buckets = append(buckets, hourBucket{start: t})
		weights = append(weights, w)
		sum += w
	}
	if len(buckets) == 0 || sum == 0 {
		return nil
	}

	assigned := 0
	for i := range buckets {
		c := int(math.Round(float64(total) * weights[i] / sum))
		buckets[i].count = c
		assigned += c
	}
	// Fix any rounding drift on a per-bucket round-robin.
	for diff := total - assigned; diff != 0; {
		for i := range buckets {
			if diff == 0 {
				break
			}
			if diff > 0 {
				buckets[i].count++
				diff--
			} else if buckets[i].count > 0 {
				buckets[i].count--
				diff++
			}
		}
	}
	return buckets
}

func estimateCost(m modelSpec, in, out int) float64 {
	cost := float64(in)*m.InPrice/1e6 + float64(out)*m.OutPrice/1e6
	return math.Round(cost*1e6) / 1e6
}

func weightedString(rng *rand.Rand, values []string, weights []int) string {
	total := 0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return pickOne(rng, values)
	}
	roll := rng.Intn(total)
	running := 0
	for i, v := range values {
		if i < len(weights) && weights[i] > 0 {
			running += weights[i]
		}
		if roll < running {
			return v
		}
	}
	return values[len(values)-1]
}

// ---------------------------------------------------------------------------
// Insert chunking
// ---------------------------------------------------------------------------

// chunk splits items into contiguous slices of at most size elements, so large
// datasets are sent to ClickHouse in bounded batches.
func chunk[T any](items []T, size int) [][]T {
	if size < 1 {
		size = 1
	}
	var out [][]T
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[start:end])
	}
	return out
}

// ---------------------------------------------------------------------------
// Static vocabularies
// ---------------------------------------------------------------------------

var toolNames = []string{
	"web_search", "sql_query", "code_interpreter", "http_request",
	"vector_search", "crm_lookup", "calendar_lookup",
}

var guardrailNames = []string{
	"pii", "secrets", "prompt_injection", "content_moderation",
	"jailbreak", "toxicity", "pii_redaction",
}

var blockReasons = []string{
	"prompt injection pattern detected",
	"PII detected in prompt",
	"secret/credential detected",
	"policy violation: violence",
	"blocked by customer DLP rule",
}

var promptTemplates = []string{
	"Summarize the following customer message and draft a concise reply:\n%s",
	"Draft a SQL query to investigate the following issue for the data team:\n%s",
	"Explain the following error to a non-technical stakeholder:\n%s",
	"Generate unit tests for the function described below:\n%s",
	"Classify the following support ticket by urgency and product area:\n%s",
	"Rewrite the following documentation section for clarity and tone:\n%s",
	"Produce a release note summarizing the following changes:\n%s",
	"Analyze the following log excerpt and identify the root cause:\n%s",
}

var promptContexts = []string{
	"\"Checkout latency spiked after the 14:00 deploy; error budget is almost exhausted.\"",
	"\"The nightly ETL job failed with a schema mismatch on the orders table.\"",
	"\"Customer reports duplicate charges on invoices from last week.\"",
	"\"We need a per-region breakdown of token spend for the quarterly review.\"",
	"\"The recommendation service returns stale results for new users.\"",
	"\"Onboarding documents don't mention the new SSO flow.\"",
	"\"Alert: circuit breaker opened for the embeddings upstream.\"",
	"\"Please confirm the data-retention window for EU customer prompts.\"",
}
