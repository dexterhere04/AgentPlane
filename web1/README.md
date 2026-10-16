# AgentPlane — Control Plane (Oat UI)

A separate, from-scratch web client for the AgentPlane gateway, built **only** from the
capability spec in [`../docs/frontend-spec.md`](../docs/frontend-spec.md) and the
[Oat UI](https://oat.ink) component library.

- **No framework, no build step.** Static HTML + Oat's CSS/JS + small ES modules.
- **~90% Oat primitives.** Nav, tabs, dialogs, tables, forms, badges, alerts, toasts,
  progress, meters, spinners and skeletons are Oat components used as documented.
- **~10% hand-rolled.** Data-visualisation, the pipeline flow, the event log and the
  floating assistant — things Oat deliberately does not ship.

## Run it

The client calls the gateway on its **own origin**, so it must be served *with a proxy*.
A plain static file server (e.g. `python3 -m http.server`) returns 404 for every API
call, which makes Access Control, Keys, Analytics and Overview look empty.

### Docker (how it is deployed)

This app is the `web` service in the repo's `docker-compose.yml`, published on
**http://localhost:8080**:

```sh
docker compose up -d --build web
```

It is built from `./web1`, runs behind nginx (which proxies the API), is
`restart: unless-stopped`, and has a container healthcheck.

### Local, without Docker

```sh
node serve.mjs                                  # http://localhost:8080 -> gateway :3001
PORT=9001 GATEWAY_URL=http://gateway:3001 node serve.mjs
```

`serve.mjs` serves the static files and proxies `/chat`, `/events`, `/provision`,
`/admin`, `/metrics`, `/api/observability` and `/analytics` (SSE included) — the dev
equivalent of `nginx.conf`. If the gateway is unreachable it returns a JSON
`502 gateway_unreachable` that the client renders as a normal error.

> `nginx.conf` resolves the gateway **per request** via Docker's embedded DNS
> (`resolver 127.0.0.11`), so the container starts even when the gateway is not up yet
> and reconnects on its own. The previous frontend crash-looped on
> `host not found in upstream "gateway"` because nginx resolved upstreams only once at
> startup.

## Structure

```
index.html                 app shell: topnav + sidebar + <main>
serve.mjs                  dev server: static files + API proxy (no deps)
assets/oat.min.css|js      vendored Oat 0.8.3 (pinned; Oat is sub-v1)
assets/logo.svg            app mark + favicon
tests/api-integration.sh   live endpoint test (curl + python3)
src/theme.css              brand: Oat CSS-variable overrides (light + dark)
src/app.css                the ~10% custom layer
src/js/
  api.js                   typed client for every spec endpoint + error contract
  store.js                 localStorage: credentials, theme, setup progress (§2)
  stream.js                SSE consumer + FIFO playback queue + request folding (§4)
  poll.js                  auto-refresh helper (visibility-aware, no overlap)
  format.js                null-safe number/date/latency formatting
  charts.js                inline-SVG bars, latency groups, donut, gantt, sparkline,
                           responsive multi-series time-axis chart
  ui.js                    Oat-composed kit: dialogs, tables, tiles, toasts
  chat.js                  floating assistant
  views/*.js               one module per section
  main.js                  router, nav, theme, sidebar, settings, stream wiring
```

## Sections → spec capability

| Section | Spec | Notes |
|---|---|---|
| Overview | §11 + §7 | setup checklist, operational-health banner, system chips, KPI tiles, session summary, guardrail telemetry, quick-access grid; auto-refresh (metrics 5 s, health 8 s, routing 15 s) |
| Playground | §3 | conversation transcript, Enter-to-send, inline API key, typing indicator, streamed or buffered, full error taxonomy |
| Pipeline | §4 | 13-stage flow, concurrency Gantt, average/min/max latency, staged/realtime playback toggle, fire N in parallel |
| Guardrails | §3, §10 | crafted payloads + live decisions + findings list + before/after diff + action counts |
| Routing | §9, §7 | providers, rules, upstream/breaker status, routing analytics with a time-range selector, live routing feed, admin-token validator + write gate; 5 s upstream polling |
| Analytics | §10.1, §10.2 | vertical, filter-driven dashboard: filter bar + KPI strip + `/series` time charts + breakdowns + guardrails + failures/trace explorer + per-user spend; 5 s auto-refresh, click a user to filter the page |
| Event Log | §4 | filterable raw stream, click a request id to filter, stage-family colour coding, incremental rendering |
| Usage | §2, §10 | verify a key, per-key request attribution, per-user spend and a searchable prompt log |
| Keys & Secrets | §5, §6 | provider secrets (Vault mount-path hint), key provisioning with optional role, one-time reveal |
| Access Control | §8.1–8.4 | policies (document viewer), roles (policy attach picker), groups, users, entity pickers throughout, attachment inspector |

## Backend integration notes

Verified against a live gateway with `tests/api-integration.sh` (62/62). Behaviours
that differ from the spec's idealised shapes, and how the client handles them:

- **PascalCase keys.** Provider and routing-rule records are serialised straight from Go
  structs *without* `json` tags, so those two endpoints emit `ID`, `Name`, `BaseURL`,
  `SupportedModels`, … `src/js/api.js` normalises both spellings into one shape.
  (`UpstreamStatus` *does* have tags, so upstreams are lowercase.)
- **Empty lists are `null`.** `GET /admin/providers` and `/admin/routing-rules` return
  `null`, not `[]`, when empty.
- **Deletes return `204`.** The client only requires a 2xx.
- **Ranges are limited** to `15m, 1h, 6h, 24h, 7d`; anything else is `400 invalid_filter`.
- **Empty overview.** `GET /api/observability/overview` returns **200 with an empty body**
  when the trace table has no rows (ClickHouse `quantileTDigest` produces `NaN`, which Go's
  JSON encoder refuses). Analytics detects this and renders an explicit empty state.
- **Partial writes.** `POST`/`PUT /admin/providers` can return `500` ("created but reload
  failed") while the record *was* persisted; the client reloads the list either way.
- **`secret_key` is a reference.** A provider's `secret_key` is the *name* of a secret in
  the store (e.g. `OPENAI_API_KEY`), not the raw key. §6 writes secrets as
  `<PROVIDER>_API_KEY`.
- **Deny by default.** `POST /chat` returns `403 policy_denied` (with `{action, resource}`)
  until a role or policy grants `ap:InvokeChat`.

## Tests

```sh
BASE_URL=http://127.0.0.1:3001 ADMIN_TOKEN=dev-admin-token-change-me \
  bash tests/api-integration.sh
```

Exercises every endpoint the client calls (reads, writes, error paths, all analytics
query types), then cleans up the data it created. Requires only `curl` and `python3`.

## Design decisions

- **Theme via variables only.** `src/theme.css` redefines Oat's colour tokens (a cool
  indigo accent on a slate base) for both light and dark; components are untouched.
- **Charts are inline SVG** using the same CSS variables, so they theme with the app and
  keep Oat's zero-dependency ethos.
- **Credentials** live under the spec's storage keys (`agentplane.adminToken`,
  `agentplane.userKey`, `agentplane.theme`, `agentplane.setup.*`).
- **Errors follow §13**: 401 → set-token guidance, 403 → deny/guardrail, 503
  `policy_unavailable`/`guardrail_unavailable` → retryable, analytics non-200 → unavailable.
- **Grafana deep-dive (optional).** Opt-in via the `grafana` Compose profile:
  `docker compose --profile grafana up -d`. The Analytics page links to `/grafana/`,
  served same-origin through `nginx.conf`/`serve.mjs` and opened in a new tab.
  Grafana keeps its own login and queries ClickHouse directly — it is not gated by
  the admin token. See `../docs/observability-dashboard.md`.
