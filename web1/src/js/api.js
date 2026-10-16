// HTTP client for the AgentPlane gateway. Every path and body here maps to the
// capability spec (docs/frontend-spec.md). Auth is either the admin token or a
// user API key, held in the local store.

import { getAdminToken, getUserKey } from "./store.js";

export class ApiError extends Error {
  constructor(status, type, message, body) {
    super(message || `Request failed (${status})`);
    this.name = "ApiError";
    this.status = status;
    this.type = type || "";
    this.body = body;
  }
}

/** True when the failure is a transient fail-closed condition (§13, retryable). */
export function isRetryable(err) {
  return err instanceof ApiError && err.status === 503 &&
    (err.type === "policy_unavailable" || err.type === "guardrail_unavailable");
}

function authHeader(kind) {
  if (kind === "admin") {
    const t = getAdminToken();
    if (!t) throw new ApiError(401, "missing_admin_token", "No admin token configured.");
    return { Authorization: `Bearer ${t}` };
  }
  if (kind === "key") {
    const k = getUserKey();
    if (!k) throw new ApiError(401, "missing_user_key", "No user API key configured.");
    return { Authorization: `Bearer ${k}` };
  }
  return {};
}

function qs(params) {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params || {})) {
    if (v === undefined || v === null || v === "") continue;
    sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}

async function request(path, { method = "GET", body, auth = "admin", signal } = {}) {
  const headers = { Accept: "application/json" };
  Object.assign(headers, authHeader(auth));
  let payload;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }

  let res;
  try {
    res = await fetch(path, { method, headers, body: payload, signal });
  } catch (e) {
    if (e?.name === "AbortError") throw e;
    throw new ApiError(0, "network_error", "Gateway unreachable.", null);
  }

  const text = await res.text();
  let json = null;
  if (text) {
    try { json = JSON.parse(text); } catch { json = null; }
  }

  if (!res.ok) {
    const errObj = json && json.error && typeof json.error === "object" ? json.error : null;
    const type = errObj?.type || errObj?.code || "";
    let message =
      errObj?.message ||
      (typeof json?.error === "string" ? json.error : "") ||
      (json?.message ?? "") ||
      "";

    // The proxying web server (nginx) returns HTML error pages for 502/504 when
    // it cannot reach the gateway. Never surface raw markup as an error message.
    if (!message) {
      const trimmed = (text || "").trim();
      if (trimmed.startsWith("<")) {
        message = (res.status === 502 || res.status === 504)
          ? "The API gateway is unreachable from the web server."
          : `HTTP ${res.status} returned by the web server.`;
      } else {
        message = trimmed.slice(0, 200) || `HTTP ${res.status}`;
      }
    }
    throw new ApiError(res.status, type, message, json);
  }

  return json;
}

/* ── Response unwrapping ────────────────────────────────────────────────
   Endpoints differ in envelope shape across versions. Be tolerant. */

export function asList(json, key) {
  if (Array.isArray(json)) return json;
  if (json && typeof json === "object") {
    if (key && Array.isArray(json[key])) return json[key];
    for (const v of Object.values(json)) if (Array.isArray(v)) return v;
  }
  return [];
}

function pick(json, keys) {
  if (json && typeof json === "object") {
    if (json.data && typeof json.data === "object") json = json.data;
    for (const k of keys) if (json[k] !== undefined) return json[k];
  }
  return json;
}

/* ── Capability: Inference (§3) ───────────────────────────────────────── */

export function sendChat(body, { signal } = {}) {
  return request("/chat", { method: "POST", body, auth: "key", signal });
}

/* ── Capability: API keys (§5) ────────────────────────────────────────── */

export function provisionUser({ username, email, key_name }) {
  const body = { username };
  if (email) body.email = email;
  if (key_name) body.key_name = key_name;
  return request("/provision/user", { method: "POST", body });
}
export function revokeApiKey(key_id) {
  return request("/admin/api-keys/revoke", { method: "POST", body: { key_id } });
}
export function listApiKeys() {
  return request("/admin/api-keys").then((j) => asList(j, "keys"));
}

/* ── Capability: Provider secrets (§6) ────────────────────────────────── */

export function storeProviderSecret(provider, api_key) {
  return request("/admin/secrets/provider", { method: "POST", body: { provider, api_key } });
}

export const PROVIDER_SLUGS = [
  "openai", "azure_openai", "anthropic", "gemini", "google", "mistral", "groq",
  "together", "openrouter", "deepseek", "xai", "fireworks", "perplexity",
];

/* ── Response normalisation ─────────────────────────────────────────────
   The gateway serialises provider and routing-rule records straight from Go
   structs *without* json tags, so those two endpoints emit PascalCase keys
   (ID, Name, BaseURL, …) while everything else is snake_case/lowercase. Fold
   both spellings into one shape so views never care which side they got. */

function firstOf(obj, ...keys) {
  for (const k of keys) if (obj && obj[k] !== undefined && obj[k] !== null) return obj[k];
  return undefined;
}

export function normalizeProvider(p) {
  if (!p || typeof p !== "object") return p;
  return {
    id: firstOf(p, "id", "ID"),
    name: firstOf(p, "name", "Name") ?? "",
    base_url: firstOf(p, "base_url", "BaseURL") ?? "",
    secret_ref: firstOf(p, "secret_key", "SecretKey", "SecretRef") ?? "",
    provider_group: firstOf(p, "provider_group", "ProviderGroup") ?? "",
    weight: firstOf(p, "weight", "Weight") ?? 1,
    supported_models: firstOf(p, "supported_models", "SupportedModels") ?? [],
    timeout_ms: firstOf(p, "timeout_ms", "TimeoutMS"),
    max_retries: firstOf(p, "max_retries", "MaxRetries"),
    breaker_failure_threshold: firstOf(p, "breaker_failure_threshold", "BreakerFailureThreshold"),
    enabled: firstOf(p, "enabled", "Enabled") !== false,
  };
}

export function normalizeRule(r) {
  if (!r || typeof r !== "object") return r;
  return {
    id: firstOf(r, "id", "ID"),
    name: firstOf(r, "name", "Name") ?? "",
    priority: firstOf(r, "priority", "Priority") ?? 0,
    model_matcher: firstOf(r, "model_matcher", "ModelMatcher") ?? {},
    role_matcher: firstOf(r, "role_matcher", "RoleMatcher") ?? {},
    user_matcher: firstOf(r, "user_matcher", "UserMatcher") ?? {},
    action: firstOf(r, "action", "Action") ?? "",
    provider_group: firstOf(r, "provider_group", "ProviderGroup") ?? "",
    timeout_retry_overrides: firstOf(r, "timeout_retry_overrides", "TimeoutRetryOverrides") ?? {},
    enabled: firstOf(r, "enabled", "Enabled") !== false,
  };
}

export function normalizeUpstream(u) {
  if (!u || typeof u !== "object") return u;
  return {
    name: firstOf(u, "name", "Name") ?? "",
    enabled: firstOf(u, "enabled", "Enabled") !== false,
    weight: firstOf(u, "weight", "Weight") ?? 1,
    models: firstOf(u, "models", "Models", "supported_models") ?? [],
    breaker: firstOf(u, "breaker", "Breaker", "state", "State") ?? "closed",
    breaker_failure_threshold: firstOf(u, "breaker_failure_threshold", "BreakerFailureThreshold"),
  };
}

/* ── Capability: Metrics (§7) ─────────────────────────────────────────── */

export function getMetrics() {
  return request("/metrics");
}

/* ── Capability: IAM (§8) ─────────────────────────────────────────────── */

// 8.1 Policies
export function listPolicies() { return request("/admin/policies").then((j) => asList(j, "policies")); }
export function createPolicy(body) { return request("/admin/policies", { method: "POST", body }); }
export function deletePolicy(name) { return request(`/admin/policies/${encodeURIComponent(name)}`, { method: "DELETE" }); }
export function attachPolicy(name, principal_type, principal_id) {
  return request(`/admin/policies/${encodeURIComponent(name)}/attach`, { method: "POST", body: { principal_type, principal_id } });
}
export function detachPolicy(name, principal_type, principal_id) {
  return request(`/admin/policies/${encodeURIComponent(name)}/detach`, { method: "POST", body: { principal_type, principal_id } });
}
export function getPolicyAttachments(name) {
  return request(`/admin/policies/${encodeURIComponent(name)}/attachments`).then((j) => asList(j, "attachments"));
}

// 8.2 Roles
export function listRoles() { return request("/admin/roles").then((j) => asList(j, "roles")); }
export function createRole(body) { return request("/admin/roles", { method: "POST", body }); }
export function deleteRole(name) { return request(`/admin/roles/${encodeURIComponent(name)}`, { method: "DELETE" }); }
export function grantRole(name, principal_type, principal_id) {
  return request(`/admin/roles/${encodeURIComponent(name)}/grant`, { method: "POST", body: { principal_type, principal_id } });
}
export function revokeRole(name, principal_type, principal_id) {
  return request(`/admin/roles/${encodeURIComponent(name)}/revoke`, { method: "POST", body: { principal_type, principal_id } });
}
export function listRoleGrants(name) {
  return request(`/admin/roles/${encodeURIComponent(name)}/grants`).then((j) => asList(j, "grants"));
}

// 8.3 Groups
export function listGroups() { return request("/admin/groups").then((j) => asList(j, "groups")); }
export function createGroup(body) { return request("/admin/groups", { method: "POST", body }); }
export function deleteGroup(name) { return request(`/admin/groups/${encodeURIComponent(name)}`, { method: "DELETE" }); }
export function listGroupMembers(name) {
  return request(`/admin/groups/${encodeURIComponent(name)}/members`).then((j) => asList(j, "members"));
}
export function addUserToGroup(name, user_id) {
  return request(`/admin/groups/${encodeURIComponent(name)}/members`, { method: "POST", body: { user_id } });
}
export function removeUserFromGroup(name, id) {
  return request(`/admin/groups/${encodeURIComponent(name)}/members/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// 8.4 Users
export function listUsers() { return request("/admin/users").then((j) => asList(j, "users")); }
export function grantUserRole(id, role) {
  return request(`/admin/users/${encodeURIComponent(id)}/roles`, { method: "POST", body: { role } });
}
export function revokeUserRole(id, role) {
  return request(`/admin/users/${encodeURIComponent(id)}/roles/${encodeURIComponent(role)}`, { method: "DELETE" });
}
export function setPermissionsBoundary(id, policy) {
  return request(`/admin/users/${encodeURIComponent(id)}/permissions-boundary`, { method: "POST", body: { policy } });
}
/** Deletes the user and the identities bound to them (keys, grants, memberships). */
export function deleteUser(id) {
  return request(`/admin/users/${encodeURIComponent(id)}`, { method: "DELETE" });
}

/* ── Capability: Providers & routing (§9) ─────────────────────────────── */

export function listProviders() { return request("/admin/providers").then((j) => asList(j, "providers").map(normalizeProvider)); }
export function createProvider(body) { return request("/admin/providers", { method: "POST", body }).then(normalizeProvider); }
export function updateProvider(id, body) { return request(`/admin/providers/${encodeURIComponent(id)}`, { method: "PUT", body }).then(normalizeProvider); }
export function deleteProvider(id) { return request(`/admin/providers/${encodeURIComponent(id)}`, { method: "DELETE" }); }
export function enableProvider(id) { return request(`/admin/providers/${encodeURIComponent(id)}/enable`, { method: "POST" }); }
export function disableProvider(id) { return request(`/admin/providers/${encodeURIComponent(id)}/disable`, { method: "POST" }); }
export function listUpstreams() { return request("/admin/upstreams").then((j) => asList(j, "upstreams").map(normalizeUpstream)); }

export function listRoutingRules() { return request("/admin/routing-rules").then((j) => asList(j, "rules").map(normalizeRule)); }
export function createRoutingRule(body) { return request("/admin/routing-rules", { method: "POST", body }).then(normalizeRule); }
export function updateRoutingRule(id, body) { return request(`/admin/routing-rules/${encodeURIComponent(id)}`, { method: "PUT", body }).then(normalizeRule); }
export function deleteRoutingRule(id) { return request(`/admin/routing-rules/${encodeURIComponent(id)}`, { method: "DELETE" }); }
export function enableRoutingRule(id) { return request(`/admin/routing-rules/${encodeURIComponent(id)}/enable`, { method: "POST" }); }
export function disableRoutingRule(id) { return request(`/admin/routing-rules/${encodeURIComponent(id)}/disable`, { method: "POST" }); }

/* ── Capability: Observability (§10.1) ────────────────────────────────── */

export function obsHealth() { return request("/api/observability/health"); }

/**
 * Fold an optional filter object into the query params the observability API
 * accepts on aggregate/series/breakdown/failure/guardrail/trace endpoints.
 * Empty values are dropped by `qs`.
 */
function obsFilterQS(filters = {}) {
  return {
    status: filters.status,
    provider: filters.provider,
    model: filters.model,
    route: filters.route,
    guardrail_action: filters.guardrail_action,
    user_id: filters.user_id,
    organization_id: filters.organization_id,
    project_id: filters.project_id,
  };
}

export function obsOverview(range = "24h", filters = {}) {
  return request(`/api/observability/overview${qs({ range, ...obsFilterQS(filters) })}`)
    .then((j) => (j && typeof j === "object" ? (j.overview && typeof j.overview === "object" ? j.overview : j) : null));
}

/** Time-bucketed series: { bucket_size, buckets: [...] } (empty when no data). */
export function obsSeries(range = "24h", filters = {}) {
  return request(`/api/observability/series${qs({ range, ...obsFilterQS(filters) })}`)
    .then((j) => (j && typeof j === "object" ? j : { bucket_size: "", buckets: [] }));
}

export function obsBreakdowns(range = "24h", filters = {}) {
  return request(`/api/observability/breakdowns${qs({ range, ...obsFilterQS(filters) })}`).then((j) => {
    if (j && typeof j === "object") {
      const bd = j.breakdowns || j.breakdown || j.data || j;
      return bd && typeof bd === "object" ? bd : {};
    }
    return {};
  });
}

/** Guardrail aggregates (the `breakdown` array) — used by the Analytics view. */
export function obsGuardrails(range = "24h", limit = 50, filters = {}) {
  return request(`/api/observability/guardrails${qs({ range, limit, ...obsFilterQS(filters) })}`).then((j) => asList(j, "breakdown"));
}

/** Guardrail aggregates plus the newest safe guardrail metadata events. */
export function obsGuardrailsFull(range = "24h", limit = 50, filters = {}) {
  return request(`/api/observability/guardrails${qs({ range, limit, ...obsFilterQS(filters) })}`)
    .then((j) => ({ breakdown: asList(j, "breakdown"), recent: asList(j, "recent") }));
}

/** Newest failed traces (≤100 rows). */
export function obsFailures(range = "24h", limit = 25, filters = {}) {
  return request(`/api/observability/failures${qs({ range, limit, ...obsFilterQS(filters) })}`)
    .then((j) => asList(j, "items"));
}

/** One page of trace summaries with keyset pagination: { items, next_cursor }. */
export function obsTraces({ range = "24h", limit = 25, cursor, ...filters } = {}) {
  return request(`/api/observability/traces${qs({ range, limit, cursor, ...obsFilterQS(filters) })}`)
    .then((j) => (j && typeof j === "object" ? { items: asList(j, "items"), next_cursor: j.next_cursor || "" } : { items: [], next_cursor: "" }));
}

/** A single trace's detail (summary + usage + capture + guardrails + tools). */
export function obsTrace(id, range = "24h") {
  return request(`/api/observability/traces/${encodeURIComponent(id)}${qs({ range })}`);
}

/** Per-user usage. The endpoint accepts only range + limit. */
export function obsUsers(range = "24h", limit = 100) {
  return request(`/api/observability/users${qs({ range, limit })}`).then((j) => asList(j, "users"));
}

/** Searchable prompt log. The endpoint accepts only range, user, q, limit. */
export function obsPrompts({ range = "24h", user, q, limit = 50 } = {}) {
  return request(`/api/observability/prompts${qs({ range, user, q, limit })}`).then((j) => asList(j, "prompts"));
}

/* ── Capability: Routing analytics (§10.2) ────────────────────────────── */

export function analytics(type, hours = 24) {
  return request(`/admin/analytics${qs({ type, hours })}`).then((j) => asList(j, "data"));
}

/* ── Admin token validation (§9, opportunistic) ───────────────────────── */

/** Validates the admin token by issuing a protected read. */
export async function validateAdminToken() {
  const t = getAdminToken();
  if (!t) return { ok: false, reason: "No admin token set." };
  try {
    await request("/admin/providers");
    return { ok: true };
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      return { ok: false, reason: "Token was rejected by the gateway (401/403)." };
    }
    return { ok: false, reason: e.message || "Could not reach the gateway." };
  }
}
