# AgentPlane Control Plane — Functional & API Specification

> Scope: the web control plane under `web/`. This document specifies, by *capability*,
> what the client does and the exact HTTP contract it depends on. It intentionally does
> not describe visual layout or component structure.

## 1. How to read this document

Each capability lists:

- **Purpose** — the outcome the client delivers.
- **Endpoints** — the HTTP calls required, with method, path, auth, and purpose.

Auth tokens:

- `ADMIN` — `Authorization: Bearer <AGENTPLANE_ADMIN_TOKEN>` (the bootstrap root token),
  or an API key whose user is granted the required IAM action.
- `KEY` — `Authorization: Bearer ap_live_<key_id>_<secret>` (a user API key).

Base URL: same origin as the served client (the gateway). All bodies are JSON.

## 2. Credentials and sessions (client-side only)

The client holds two credentials in browser storage; no backend endpoint is involved.

| Storage key | Meaning | Used for |
|---|---|---|
| `agentplane.adminToken` | Operator admin token | Every `ADMIN` request; the SSE stream |
| `agentplane.userKey` | A user API key (`ap_live_…`) | Every `KEY` request |

Other client-side state (no backend): theme (`agentplane.theme`), navigation state
(`agentplane.sidebar`), and setup-progress markers in `agentplane.setup.*`.

Capability: **Set/validate credentials.** The admin token is validated opportunistically
by issuing a protected read (`GET /admin/providers`); `401`/`403` marks it invalid.
There is no dedicated token-validation endpoint.

## 3. Capability: Inference (chat completion)

**Purpose.** Send an OpenAI-style chat request through the gateway (auth → input
guardrails → provider routing → provider → output guardrails → response). Used by the
playground, the guardrail playground, key verification, the pipeline demonstration, and
the floating assistant.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/chat` | `KEY` | Forward a chat completion; also exercises guardrails and routing |

Request body:

```json
{ "model": "gpt-4o", "messages": [{ "role": "user", "content": "…" }], "stream": false }
```

Response: the provider's completion (`choices[].message.content`), unchanged.
Error shapes returned by the gateway (all with a JSON `error` object):

| Status | `error.type` | Meaning |
|---|---|---|
| 400 | `model_required` | No resolvable model (no `model` field, no `DEFAULT_MODEL`) |
| 401 | — | Missing/invalid API key |
| 403 | `policy_denied` | IAM denied `ap:InvokeChat` or `ap:InvokeModel` `{action, resource}` |
| 403 | `guardrail_blocked` | A guardrail blocked the content |
| 502 | — | Upstream provider failure |
| 503 | `policy_unavailable` | Authorization could not be evaluated (fail-closed) |
| 503 | `guardrail_unavailable` | A mandatory guardrail could not be evaluated |

## 4. Capability: Live telemetry stream

**Purpose.** Receive the gateway's real-time event stream and derive request state
(stage timings, guardrail decisions, auth identity, routing outcomes) without polling.
Drives the event log, pipeline visualization, guardrail inspection, per-key attribution,
and connection status.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/events?token=<ADMIN>` | `ADMIN` (header or `token` query) | Server-Sent Events stream |

Notes:
- The `token` query parameter exists because `EventSource` cannot set headers.
- Events are JSON objects consumed incrementally; the client de-duplicates by event id
  and caps retained history (~5000).

## 5. Capability: API key provisioning and lifecycle

**Purpose.** Create a user with an API key, optionally grant an access role, list minted
keys, and revoke a key. The full key is returned exactly once; only its hash is stored.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/provision/user` | `ADMIN` / `ap:ProvisionUser` | Create a user and mint an API key |
| POST | `/admin/api-keys/revoke` | `ADMIN` / `ap:RevokeApiKey` | Revoke a key by `key_id` |
| GET | `/admin/api-keys` | `ADMIN` / `ap:ListApiKeys` | List minted keys and owning user |

`POST /provision/user` request:

```json
{ "username": "demo-user", "email": "optional@example.com", "key_name": "demo key" }
```

Response:

```json
{ "user_id": "uuid", "key_id": "public-part", "api_key": "ap_live_<key_id>_<secret>" }
```

`POST /admin/api-keys/revoke` request: `{ "key_id": "public-part" }`.
`GET /admin/api-keys` response: `{ "keys": [ { key_id, username, name, status, created_at, revoked_at? } ] }`.

Post-provisioning role assignment (authorization): a newly created user has no access
until a role or policy is granted (§8).

## 6. Capability: Provider secret storage

**Purpose.** Store an upstream provider credential so the gateway can forward requests.
Credentials are written to the configured secret store (Vault by default) and re-read on
each forward; the client never reads them back.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/admin/secrets/provider` | `ADMIN` / `ap:ManageSecrets` | Store a provider API key |

Request: `{ "provider": "openai", "api_key": "sk-…" }`.
Provider slugs offered by the client: `openai, azure_openai, anthropic, gemini, google,
mistral, groq, together, openrouter, deepseek, xai, fireworks, perplexity`.

## 7. Capability: Operational metrics

**Purpose.** Read the gateway's in-process counters (served synchronously, available even
without ClickHouse), including routing decisions, failovers, retries, and circuit-breaker
activity used to render status and routing summaries.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/metrics` | `ADMIN` / `ap:ViewMetrics` | JSON metrics snapshot |

Fields consumed: `routing_decisions_total`, `routing_failovers_total`,
`routing_retries_total`, `routing_breaker_trips_total`,
`routing_breaker_recoveries_total`, `routing_no_provider_total`, `routing_denied_total`,
and the `routing_upstreams[]` array (`name, decisions, failovers, retries,
breaker_trips, breaker_recoveries`).

## 8. Capability: Identity & Access Management (IAM)

**Purpose.** Author AWS-style JSON policies, attach them to principals, compose roles and
groups, grant roles to users, and set permission boundaries. Access is deny-by-default and
an explicit `Deny` overrides any `Allow`.

Every call below requires `ADMIN`. When using an API key, the required IAM action is
listed per endpoint.

### 8.1 Policies

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/policies` | `ap:ListPolicies` | List every policy |
| POST | `/admin/policies` | `ap:CreatePolicy` | Create a validated policy |
| DELETE | `/admin/policies/{name}` | `ap:DeletePolicy` | Delete a policy |
| POST | `/admin/policies/{name}/attach` | `ap:AttachPolicy` | Attach a policy to a principal |
| POST | `/admin/policies/{name}/detach` | `ap:DetachPolicy` | Detach a policy from a principal |
| GET | `/admin/policies/{name}/attachments` | `ap:GetPolicy` | List a policy's attachments |

Create body: `{ "name": "…", "description": "…", "document": <policy-document> }`.
Policy document shape: `{ "Version": "2026-01-01", "Statement": [ { "Sid"?, "Effect":
"Allow|Deny", "Action": [...], "Resource": [...] } ] }`. `Action` uses the `ap:` prefix;
`Resource` uses `arn:ap:{type}/{id}` with `*`/`?` wildcards.
Attach/detach body: `{ "principal_type": "user|group|role", "principal_id": "<uuid>" }`.
List response item: `{ name, description, version, is_builtin, document }`.

### 8.2 Roles

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/roles` | `ap:ListRoles` | List roles with attached policy names |
| POST | `/admin/roles` | `ap:CreateRole` | Create a role |
| DELETE | `/admin/roles/{name}` | `ap:DeleteRole` | Delete a role |
| POST | `/admin/roles/{name}/grant` | `ap:GrantRole` | Grant a role to a user or group |
| POST | `/admin/roles/{name}/revoke` | `ap:RevokeRole` | Revoke a role grant |
| GET | `/admin/roles/{name}/grants` | `ap:ListRoles` | List a role's grants |

Create body: `{ "name": "…", "description": "…", "trust_policy"?, "permissions_boundary"? }`.
Grant/revoke body: `{ "principal_type": "user|group", "principal_id": "<uuid>" }`.
List response item: `{ id, name, description, policies: [<policy-name>] }`.

> Backend-available but not currently invoked by the client: `POST /admin/roles/{name}/revoke`
> and `GET /admin/roles/{name}/grants`.

### 8.3 Groups

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/groups` | `ap:ListGroups` | List groups |
| POST | `/admin/groups` | `ap:CreateGroup` | Create a group |
| DELETE | `/admin/groups/{name}` | `ap:DeleteGroup` | Delete a group |
| GET | `/admin/groups/{name}/members` | `ap:GetGroup` | List member user IDs |
| POST | `/admin/groups/{name}/members` | `ap:AddUserToGroup` | Add a user to a group |
| DELETE | `/admin/groups/{name}/members/{id}` | `ap:RemoveUserFromGroup` | Remove a user |

Create body: `{ "name": "…", "description": "…" }`. Add member body: `{ "user_id": "<uuid>" }`.
List response item: `{ id, name, description }`.

### 8.4 Users, role grants, and permission boundaries

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/users` | `ap:ListUsers` | List users with their role names |
| POST | `/admin/users/{id}/roles` | `ap:GrantRole` | Grant a role to a user |
| DELETE | `/admin/users/{id}/roles/{role}` | `ap:RevokeRole` | Revoke a role from a user |
| POST | `/admin/users/{id}/permissions-boundary` | `ap:SetPermissionsBoundary` | Set or clear a boundary |
| DELETE | `/admin/users/{id}` | `ap:DeleteUser` | Delete a user and their keys/bindings |

Assign body: `{ "role": "…" }`. Boundary body: `{ "policy": "…" }` (empty string clears).
User list item: `{ id, username, email?, status, created_at, roles: [<role-name>] }`.

## 9. Capability: Provider and routing configuration

**Purpose.** Register upstream providers, control their availability and weights, tune
timeouts/retries/circuit-breaker thresholds, and define routing rules that select a
provider group based on model, role, and user matchers.

### 9.1 Providers

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/providers` | `ap:ListProviders` | List configured providers |
| POST | `/admin/providers` | `ap:CreateProvider` | Create a provider |
| PUT | `/admin/providers/{id}` | `ap:UpdateProvider` | Update a provider |
| DELETE | `/admin/providers/{id}` | `ap:DeleteProvider` | Delete a provider |
| POST | `/admin/providers/{id}/enable` | `ap:EnableProvider` | Enable a provider |
| POST | `/admin/providers/{id}/disable` | `ap:DisableProvider` | Disable a provider |
| GET | `/admin/upstreams` | `ap:ListProviders` | Live upstream/circuit-breaker status |

Provider write body:

```json
{ "name": "…", "base_url": "https://…", "secret_key": "…", "provider_group": "…",
  "weight": 1, "supported_models": ["gpt-4o"], "timeout_ms": 60000, "max_retries": 1,
  "breaker_failure_threshold": 5, "enabled": true }
```

Provider list item adds `id`. Upstream status item: `{ name, enabled, weight, models,
breaker, breaker_failure_threshold }`.

### 9.2 Routing rules

| Method | Path | Action | Purpose |
|---|---|---|---|
| GET | `/admin/routing-rules` | `ap:ListRoutingRules` | List routing rules |
| POST | `/admin/routing-rules` | `ap:CreateRoutingRule` | Create a rule |
| PUT | `/admin/routing-rules/{id}` | `ap:UpdateRoutingRule` | Update a rule |
| DELETE | `/admin/routing-rules/{id}` | `ap:DeleteRoutingRule` | Delete a rule |
| POST | `/admin/routing-rules/{id}/enable` | `ap:EnableRoutingRule` | Enable a rule |
| POST | `/admin/routing-rules/{id}/disable` | `ap:DisableRoutingRule` | Disable a rule |

Rule write body:

```json
{ "name": "…", "priority": 10, "model_matcher": {}, "role_matcher": {},
  "user_matcher": {}, "action": "…", "provider_group": "…",
  "timeout_retry_overrides": {}, "enabled": true }
```

Rule list item adds `id`.

> Note: the admin token is validated by issuing `GET /admin/providers` and treating
> `401`/`403` as invalid.

## 10. Capability: Observability and analytics

**Purpose.** Surface ClickHouse-backed traces, tokens, cost, latency, per-provider and
per-model breakdowns, guardrail actions, per-user spend, and a searchable prompt log.

### 10.1 Typed observability API

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/api/observability/health` | `ADMIN` / `ap:ViewMetrics` | ClickHouse reachability and data state |
| GET | `/api/observability/overview?range=<24h\|7d\|30d>` | `ADMIN` / `ap:ViewMetrics` | Totals: requests, successes, errors, error rate, latency percentiles, tokens, cost |
| GET | `/api/observability/breakdowns?range=<…>` | `ADMIN` / `ap:ViewMetrics` | Grouped breakdowns keyed by `model`, `provider`, … |
| GET | `/api/observability/guardrails?range=<…>&limit=<n>` | `ADMIN` / `ap:ViewMetrics` | Guardrail action counts |
| GET | `/api/observability/users?range=<…>&limit=<n>` | `ADMIN` / `ap:ViewMetrics` | Per-user usage: requests, tokens, cost, avg latency, last seen |
| GET | `/api/observability/prompts?range=<…>&user=<u>&q=<q>&limit=<n>` | `ADMIN` / `ap:ViewMetrics` | Searchable captured prompt log |

Response fields consumed:

- Overview: `requests, successes, errors, error_rate, latency_ms{p50_ms,p95_ms,p99_ms},
  tokens{input,output,total}, estimated_cost`.
- Breakdown item: `key, requests, successes, errors, error_rate, tokens{…},
  estimated_cost, latency_ms{…}` grouped under `model`, `provider`, etc.
- Guardrail item: `{ name, phase, action, count }` (under `breakdown`).
- User item: `{ user_id, username, request_count, input_tokens, output_tokens,
  total_tokens, total_cost, avg_latency_ms, last_seen }` (under `users`).
- Prompt item: `{ trace_id, prompt, created_at, username, model, total_tokens,
  estimated_cost }` (under `prompts`).

### 10.2 Routing analytics (pre-built queries)

**Purpose.** Routing dashboards: provider traffic share, latency percentiles, error
summary, failover counts and transitions, tokens and cost by route/provider.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/admin/analytics?type=<type>&hours=<n>` | `ADMIN` / `ap:ViewMetrics` | Run one whitelisted query |

`type` values used by the client (with consumed fields):

| `type` | Fields |
|---|---|
| `provider_traffic` | `provider, request_count, traffic_percentage` |
| `latency_percentiles` | `provider, p50_latency_ms, p95_latency_ms, p99_latency_ms` |
| `error_rate_summary` | `provider, total_requests, error_count, error_rate_percentage` |
| `failover_count` | `failover_count, total_requests, failover_percentage` |
| `failover_transitions` | `failover_from, failover_to, failover_count` |
| `tokens_by_route` | `route, input_tokens, output_tokens, total_tokens, request_count` |
| `tokens_by_provider` | `provider, input_tokens, output_tokens, total_tokens, request_count` |
| `cost_by_route` | `route, total_cost, avg_cost_per_request, request_count` |
| `cost_by_provider` | `provider, total_cost, avg_cost_per_request, request_count` |

`hours` is an integer window (backend accepts `1`–`720`, default `24`). Response:
`{ "data": [ …rows… ] }`.

> Backend-available but unused by the client: `GET /admin/analytics/users` and
> `GET /analytics/traces_count`; also `GET /api/observability/series`,
> `/api/observability/traces`, `/api/observability/failures`.

## 11. Capability: Setup guidance (client-side only)

The client derives a getting-started checklist from local state and already-observed
activity; it requires no endpoint. Completion signals: an admin token is set, a provider
secret was stored (§6), a user was provisioned (§5), and at least one request appears on
the stream (§4) or was sent (§3).

## 12. Complete endpoint reference

Auth column: `KEY` = user API key; `ADMIN` = root admin token or an API key holding the
listed action.

| Method | Path | Auth | Required action |
|---|---|---|---|
| POST | `/chat` | `KEY` | `ap:InvokeChat`, `ap:InvokeModel` |
| GET | `/events` | `ADMIN` | `ap:ViewMetrics` |
| POST | `/provision/user` | `ADMIN` | `ap:ProvisionUser` |
| POST | `/admin/api-keys/revoke` | `ADMIN` | `ap:RevokeApiKey` |
| GET | `/admin/api-keys` | `ADMIN` | `ap:ListApiKeys` |
| POST | `/admin/secrets/provider` | `ADMIN` | `ap:ManageSecrets` |
| GET | `/metrics` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/admin/policies` | `ADMIN` | `ap:ListPolicies` |
| POST | `/admin/policies` | `ADMIN` | `ap:CreatePolicy` |
| DELETE | `/admin/policies/{name}` | `ADMIN` | `ap:DeletePolicy` |
| POST | `/admin/policies/{name}/attach` | `ADMIN` | `ap:AttachPolicy` |
| POST | `/admin/policies/{name}/detach` | `ADMIN` | `ap:DetachPolicy` |
| GET | `/admin/policies/{name}/attachments` | `ADMIN` | `ap:GetPolicy` |
| GET | `/admin/roles` | `ADMIN` | `ap:ListRoles` |
| POST | `/admin/roles` | `ADMIN` | `ap:CreateRole` |
| DELETE | `/admin/roles/{name}` | `ADMIN` | `ap:DeleteRole` |
| POST | `/admin/roles/{name}/grant` | `ADMIN` | `ap:GrantRole` |
| POST | `/admin/roles/{name}/revoke` | `ADMIN` | `ap:RevokeRole` |
| GET | `/admin/roles/{name}/grants` | `ADMIN` | `ap:ListRoles` |
| GET | `/admin/groups` | `ADMIN` | `ap:ListGroups` |
| POST | `/admin/groups` | `ADMIN` | `ap:CreateGroup` |
| DELETE | `/admin/groups/{name}` | `ADMIN` | `ap:DeleteGroup` |
| GET | `/admin/groups/{name}/members` | `ADMIN` | `ap:GetGroup` |
| POST | `/admin/groups/{name}/members` | `ADMIN` | `ap:AddUserToGroup` |
| DELETE | `/admin/groups/{name}/members/{id}` | `ADMIN` | `ap:RemoveUserFromGroup` |
| GET | `/admin/users` | `ADMIN` | `ap:ListUsers` |
| POST | `/admin/users/{id}/roles` | `ADMIN` | `ap:GrantRole` |
| DELETE | `/admin/users/{id}/roles/{role}` | `ADMIN` | `ap:RevokeRole` |
| POST | `/admin/users/{id}/permissions-boundary` | `ADMIN` | `ap:SetPermissionsBoundary` |
| DELETE | `/admin/users/{id}` | `ADMIN` | `ap:DeleteUser` |
| GET | `/admin/providers` | `ADMIN` | `ap:ListProviders` |
| POST | `/admin/providers` | `ADMIN` | `ap:CreateProvider` |
| PUT | `/admin/providers/{id}` | `ADMIN` | `ap:UpdateProvider` |
| DELETE | `/admin/providers/{id}` | `ADMIN` | `ap:DeleteProvider` |
| POST | `/admin/providers/{id}/enable` | `ADMIN` | `ap:EnableProvider` |
| POST | `/admin/providers/{id}/disable` | `ADMIN` | `ap:DisableProvider` |
| GET | `/admin/upstreams` | `ADMIN` | `ap:ListProviders` |
| GET | `/admin/routing-rules` | `ADMIN` | `ap:ListRoutingRules` |
| POST | `/admin/routing-rules` | `ADMIN` | `ap:CreateRoutingRule` |
| PUT | `/admin/routing-rules/{id}` | `ADMIN` | `ap:UpdateRoutingRule` |
| DELETE | `/admin/routing-rules/{id}` | `ADMIN` | `ap:DeleteRoutingRule` |
| POST | `/admin/routing-rules/{id}/enable` | `ADMIN` | `ap:EnableRoutingRule` |
| POST | `/admin/routing-rules/{id}/disable` | `ADMIN` | `ap:DisableRoutingRule` |
| GET | `/api/observability/health` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/api/observability/overview` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/api/observability/breakdowns` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/api/observability/guardrails` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/api/observability/users` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/api/observability/prompts` | `ADMIN` | `ap:ViewMetrics` |
| GET | `/admin/analytics` | `ADMIN` | `ap:ViewMetrics` |

### Backend endpoints not currently invoked by the client

| Method | Path | Intended use |
|---|---|---|
| PUT | `/admin/policies/{name}` | Update a policy document (new version) |
| GET | `/admin/users/{id}/roles` | List a single user's roles |
| GET | `/admin/analytics/users` | Alternate per-user analytics route |
| GET | `/analytics/traces_count` | Single pinned trace-count query |
| GET | `/api/observability/series` | Time-series traces |
| GET | `/api/observability/traces` | Paginated trace list / detail |
| GET | `/api/observability/failures` | Failure breakdown |
| GET | `/dashboard`, `/observability` | Server-rendered legacy HTML views |

## 13. Error-handling contract

- `401` — missing/invalid credential; the client surfaces guidance to set the admin token.
- `403` — authenticated but not permitted (IAM deny or `ap:InvokeChat`/`ap:InvokeModel`).
- `503` with `policy_unavailable` / `guardrail_unavailable` — fail-closed; the operation
  could not be evaluated (retryable).
- Analytics endpoints return `200` with possibly empty `data`/`rows`; a non-`200` is a
  hard failure and the client flags the analytics layer as unavailable.
