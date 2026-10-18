# Demo Data Seeding

`cmd/seed` populates AgentPlane with a realistic, self-consistent organization so
the control plane and the observability dashboard have something to show. It writes
to both stores the running gateway uses:

- **PostgreSQL** — IAM policies, roles, groups, users + API keys, providers, and
  routing rules.
- **ClickHouse** — a window of `traces`, `usage_events`, `prompt_events`,
  `response_events`, `guardrail_events`, and `tool_call_events`.

The fictional org is **Northwind AI**: 8 teams, 26 users (including 4 service
accounts), 10 providers across 9 groups, 9 routing rules, and 5 custom IAM
policies layered on the built-in ones.

## Quick start

With the docker-compose stack running (`docker compose up -d`):

```sh
./seed.sh --reset
```

`seed.sh` fills in sensible defaults for the compose stack (Postgres on `5433`,
ClickHouse native on `9000`, the dev pepper/admin token, the in-network mock as the
provider upstream) and runs `go run ./cmd/seed`.

Then open the control plane at <http://localhost:8080> and sign in with the admin
token (`dev-admin-token-change-me`), or query the gateway directly:

```sh
curl -H "Authorization: Bearer dev-admin-token-change-me" \
  "http://localhost:3001/api/observability/overview?range=7d"
```

The command prints every provisioned API key once at the end. Copy any of them to
exercise `/chat` with real routing.

## Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--reset` | `false` | Delete previously seeded demo entities (and truncate the analytics tables) before writing. Recommended. |
| `--days` | `7` | Analytics window length. |
| `--traces` | `12000` | Approximate number of traces to generate. |
| `--seed` | `42` | RNG seed; the same seed reproduces the same data. |
| `--provider-base-url` | `http://mock:3002/v1` | Base URL stored on every seeded provider. Point it at the mock so the demo routes end to end; change it to reach real providers. |
| `--gateway-url` | `http://localhost:3001` | Gateway base URL used to trigger a provider reload after seeding. |
| `--org` | `northwind-ai` | Organization identifier recorded in telemetry. |
| `--out` | *(stdout only)* | Also write the run report (counts + credentials) as JSON. |
| `--postgres-only` | `false` | Skip ClickHouse. |
| `--analytics-only` | `false` | Skip Postgres (regenerate telemetry for existing users). |

## Environment

| Variable | Required | Notes |
|----------|----------|-------|
| `DATABASE_URL` | yes (unless `--analytics-only`) | Postgres DSN. Compose publishes `5433`. |
| `API_KEY_PEPPER` | yes (unless `--analytics-only`) | **Must equal the gateway's pepper** or the minted keys will not authenticate. Falls back to the configured secret store if unset. |
| `CLICKHOUSE_HOST` / `CLICKHOUSE_PORT` | yes (unless `--postgres-only`) | Native protocol host/port. |
| `CLICKHOUSE_USER` / `CLICKHOUSE_PASSWORD` | for compose | Compose uses `default` / `agentplane`. |
| `AGENTPLANE_ADMIN_TOKEN` | for reload | Used to poke the gateway into reloading providers/rules. |
| `GATEWAY_URL` | for reload | Defaults to `http://localhost:3001`. |
| `SEED_PROVIDER_BASE_URL` | no | Overrides `--provider-base-url`. |

## What gets seeded

### IAM

- **Policies (custom):** `APTeamLead`, `APServiceAccount`, `APAuditor`,
  `APNoAdminBoundary` (a permissions boundary), `APDenyExperimental` (an explicit
  deny attached to the built-in `member` role to demonstrate deny-wins).
- **Roles (custom):** `team-lead`, `service-account`, `readonly`,
  `security-auditor`, `eu-member`.
- **Groups (teams):** `platform` (admin), `ml-research`, `app-eng`, `support`
  (member), `analytics`, `finance` (readonly), `security` (security-auditor),
  `service-accounts` (service-account).
- **Users:** 26 identities, each with one or more API keys. Team leads get
  `team-lead`; EU members get `eu-member`; service accounts carry the
  `APNoAdminBoundary` permission boundary.

### Providers & routing

Ten providers grouped by purpose (`primary`, `eu`, `anthropic`, `google`, `fast`,
`oss`, `onprem`, `fallback`, `dr`) with realistic weights, models, timeouts, retry
budgets, and breaker thresholds. `bedrock-titan-dr` is disabled to show the
off state. Nine routing rules cover force-deny, EU data residency, per-model
fan-out, weighted primary selection, on-prem batch inference, and a catch-all
fallback.

Every seeded provider points at `--provider-base-url` (the mock) and resolves its
upstream secret to `OPENAI_API_KEY`, so the whole stack routes without extra
configuration.

### Analytics

Traces are spread over `--days` with a working-day diurnal curve (weekends dip),
a realistic model/provider mix, ~5% failures, ~2% guardrail blocks, ~3% redactions,
occasional failovers, prompt/response captures, guardrail events, and tool calls.
Per-user spend is skewed toward the service accounts. A small tail is always
written into the last few minutes so `/api/observability/health` reports `fresh`
rather than `stale`.

Telemetry uses the gateway's own vocabulary — guardrail actions are `pass`,
`block`, and `redact` — so filters and the dashboard behave identically to live
traffic.

## Idempotency

Without `--reset` the Postgres side reconciles: existing demo entities are reused
rather than duplicated, and no new keys are minted for users that already exist.
Telemetry is appended (and regenerated only on `--reset`). For a clean demo always
use `--reset`.

## Verifying

```sh
curl -H "Authorization: Bearer dev-admin-token-change-me" \
  "http://localhost:3001/api/observability/overview?range=7d"
curl -H "Authorization: Bearer dev-admin-token-change-me" \
  "http://localhost:3001/admin/providers"
curl -H "Authorization: Bearer dev-admin-token-change-me" \
  "http://localhost:3001/admin/policies"
```

Unit tests for the generator (distribution, determinism, invariants) live in
`internal/seed/seed_test.go`:

```sh
go test ./internal/seed/...
```
