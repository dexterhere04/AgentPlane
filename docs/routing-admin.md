# AgentPlane Routing Administration

## 1. Purpose

Routing configuration is operator-managed, not hardcoded. Providers and
routing rules live in PostgreSQL and are maintained through a small admin REST
API and the in-app management UI. This document is the reference for managing
that configuration — the router's runtime behavior is covered in
`provider-routing-and-router.md` and its metrics in `routing-metrics.md`.

```text
Admin UI / REST API
        |
        v
 PostgreSQL (providers, routing_rules)
        |
        v
 routing_rules / providers stores  -->  reload -->  runtime Registry + Router
```

Every successful mutation is persisted and then triggers a `reload` callback
that regenerates the runtime registry and rule set, so changes take effect
immediately without a gateway restart.

## 2. Data Model

### `providers`

| Column | Type | Meaning |
|--------|------|---------|
| `id` | uuid | Primary key (referenced by rules and UI) |
| `name` | text | Unique provider/upstream name |
| `base_url` | text | Upstream OpenAI-compatible endpoint base URL |
| `secret_key` | text | Secret-store key for the upstream credential |
| `weight` | int | Weight for weighted primary selection (`> 0` eligible) |
| `models` | text[] | Supported models (`*` or empty = any) |
| `timeout_ms` | int | Per-request timeout |
| `max_retries` | int | In-place retries before failover |
| `breaker_failure_threshold` | int | Consecutive failures before the breaker opens |
| `enabled` | bool | Admin on/off switch |

`secret_key` names an entry in the secret store (the same `providerSecretKeys`
style mapping used for the built-in OpenAI key) and is never echoed back from
the API (`secret_key,omitempty`).

### `routing_rules`

| Column | Type | Meaning |
|--------|------|---------|
| `id` | uuid | Primary key |
| `name` | text | Rule label |
| `priority` | int | Evaluation order (lower = evaluated first) |
| `model_matcher` | jsonb | `{equals}` / `{in:[...]}` / `{prefix}` / `{equals:"*"}` |
| `role_matcher` | jsonb | `{equals}` / `{in:[...]}` over the caller's effective roles |
| `user_matcher` | jsonb | `{equals}` / `{in:[...]}` over user attributes |
| `action` | text | `route` (first candidate) or `weighted` (weighted split) or `deny` |
| `provider_group` | jsonb | Ordered/weighted list of provider names (see below) |
| `timeout_retry_overrides` | jsonb | Overrides applied to the matched route |
| `enabled` | bool | Rule on/off switch |

A rule matches when **all** present matchers match. Rules are evaluated in
`priority` order and the first match wins; unmatched requests fall through to
the default provider. A `deny` action is an operator override (e.g. draining a
provider) and is enforced **before** any provider selection.

`provider_group` is a JSON list where each entry names a provider and carries
an optional weight:

```json
[
  { "name": "openai-primary", "weight": 70 },
  { "name": "anthropic-failover", "weight": 30 }
]
```

`timeout_retry_overrides` accepts any subset of:

```json
{ "timeout_ms": 30000, "max_retries": 2, "backoff_base_ms": 200, "backoff_max_ms": 4000, "jitter_percent": 20 }
```

## 3. Provider Admin API

All routes require admin authorization (`Authorization: Bearer <admin-token>`
where the token is the bootstrap admin token or an API key whose user holds
the `admin` role).

| Method | Path | Body | Effect |
|--------|------|------|--------|
| `GET` | `/admin/providers` | — | List configured providers |
| `POST` | `/admin/providers` | `providerRequest` | Create a provider |
| `GET` | `/admin/providers/{id}` | — | Fetch one provider |
| `PUT` | `/admin/providers/{id}` | `providerRequest` | Update a provider |
| `POST` | `/admin/providers/{id}/enable` | — | Enable |
| `POST` | `/admin/providers/{id}/disable` | — | Disable |
| `DELETE` | `/admin/providers/{id}` | — | Delete |
| `GET` | `/admin/upstreams` | — | Live runtime status (see §5) |

### `providerRequest`

```json
{
  "name": "openai-primary",
  "base_url": "https://api.openai.com/v1",
  "secret_key": "openai",
  "provider_group": "primary",
  "weight": 70,
  "supported_models": ["gpt-4o", "gpt-4*"],
  "timeout_ms": 60000,
  "max_retries": 1,
  "breaker_failure_threshold": 5,
  "enabled": true
}
```

### Example

```bash
curl -X POST http://localhost:3001/admin/providers \
  -H "Authorization: Bearer dev-admin-token-change-me" \
  -H "Content-Type: application/json" \
  -d '{"name":"openai-primary","base_url":"https://api.openai.com/v1","weight":70,"supported_models":["gpt-4o"],"breaker_failure_threshold":5,"enabled":true}'
```

## 4. Routing Rule Admin API

| Method | Path | Body | Effect |
|--------|------|------|--------|
| `GET` | `/admin/routing-rules` | — | List rules |
| `POST` | `/admin/routing-rules` | `routingRuleRequest` | Create a rule |
| `GET` | `/admin/routing-rules/{id}` | — | Fetch one rule |
| `PUT` | `/admin/routing-rules/{id}` | `routingRuleRequest` | Update a rule |
| `POST` | `/admin/routing-rules/{id}/enable` | — | Enable |
| `POST` | `/admin/routing-rules/{id}/disable` | — | Disable |
| `DELETE` | `/admin/routing-rules/{id}` | — | Delete |

### `routingRuleRequest`

```json
{
  "name": "premium-gpt",
  "priority": 10,
  "model_matcher": { "equals": "gpt-4o" },
  "role_matcher": { "in": ["premium", "admin"] },
  "action": "weighted",
  "provider_group": [
    { "name": "openai-primary", "weight": 80 },
    { "name": "openai-backup", "weight": 20 }
  ],
  "timeout_retry_overrides": { "max_retries": 1 },
  "enabled": true
}
```

Matcher forms:

```text
model_matcher:  { "equals": "gpt-4o" } | { "prefix": "gpt-" } | { "equals": "*" }
role_matcher:   { "equals": "member" } | { "in": ["premium", "admin"] }
user_matcher:   { "equals": "alice" }  | { "in": ["alice", "bob"] }
```

Matchers, `provider_group`, and `timeout_retry_overrides` are optional; a rule
with no matchers matches every request (a catch-all), so order it by priority
last.

## 5. Live Upstream Status — `GET /admin/upstreams`

Read-only, poll-safe snapshot of every registered runtime upstream:

```json
{
  "name": "openai-primary",
  "enabled": true,
  "weight": 70,
  "models": ["gpt-4o"],
  "breaker": "closed",
  "breaker_failure_threshold": 5
}
```

`breaker` is the passive circuit-breaker state on open source:
`closed` → `open` → `half_open` → `closed`, per `resilience-and-retry.md`.

## 6. In-App Management UI

* **Routing tab** (`RoutingView`) — the live dashboard: per-provider traffic
  split, latency percentiles, error/failover counts and transitions, and
  tokens/cost by route and provider. The in-process counters card pulls from
  `/metrics` and needs no ClickHouse; the analytics cards poll `/admin/analytics`
  on the selected window.
* **Routing Admin card** (`RoutingAdminView`) — a token validator (writes the
  admin token into the shared app state, flipping the header badge live) plus
  full provider and routing-rule CRUD against the endpoints above, including
  enable/disable toggles.
* **Overview** — a routing widget with live counters and 24h provider traffic,
  failover, and error-rate summaries.

## 7. Authentication & Reboot Notes

* Provider secrets are resolved from the secret store at reload time; adding a
  provider whose `secret_key` has no entry will leave that upstream unable to
  authenticate. Provision the secret first, or create the provider with an
  existing key name.
* Route/user matchers reference the same permission vocabulary
  (`model:*`, role/policy names) used by the policy engine, but routing is
  never an authorization decision — the policy check happens upstream.
--