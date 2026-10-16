const ADMIN_TOKEN_KEY = 'agentplane.adminToken';
const USER_KEY_KEY = 'agentplane.userKey';

export function getAdminToken(): string {
  return localStorage.getItem(ADMIN_TOKEN_KEY) || '';
}

export function setAdminToken(token: string): void {
  localStorage.setItem(ADMIN_TOKEN_KEY, token);
}

export function getUserKey(): string {
  return localStorage.getItem(USER_KEY_KEY) || '';
}

export function setUserKey(key: string): void {
  localStorage.setItem(USER_KEY_KEY, key);
}

async function parseJSON(res: Response): Promise<unknown> {
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export interface ChatResult {
  ok: boolean;
  status: number;
  body: unknown;
}

export async function sendChat(
  prompt: string,
  opts: { model?: string; stream?: boolean; userKey?: string } = {}
): Promise<ChatResult> {
  const key = opts.userKey ?? getUserKey();
  const res = await fetch('/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + key
    },
    body: JSON.stringify({
      model: opts.model || 'gpt-4o',
      messages: [{ role: 'user', content: prompt }],
      stream: opts.stream ?? false
    })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export interface ProvisionResult {
  ok: boolean;
  status: number;
  body: { user_id?: string; key_id?: string; api_key?: string; error?: string } | string;
}

export async function provisionUser(username: string, keyName: string): Promise<ProvisionResult> {
  const res = await fetch('/provision/user', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + getAdminToken()
    },
    body: JSON.stringify({ username, key_name: keyName })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) as ProvisionResult['body'] };
}

export async function revokeKey(keyId: string): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/api-keys/revoke', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + getAdminToken()
    },
    body: JSON.stringify({ key_id: keyId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export interface APIKeyItem {
  key_id: string;
  username: string;
  name: string;
  status: string;
  created_at: string;
  revoked_at?: string;
}

export async function fetchAPIKeys(): Promise<{ ok: boolean; status: number; keys: APIKeyItem[] }> {
  const res = await fetch('/admin/api-keys', {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, keys: [] };
  const body = (await parseJSON(res)) as { keys?: APIKeyItem[] };
  return { ok: true, status: res.status, keys: body?.keys ?? [] };
}

export interface UserUsageRow {
  user_id: string;
  username: string;
  request_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  total_cost: number;
  avg_latency_ms: number;
  last_seen: string;
}

export interface UserPromptRow {
  trace_id: string;
  prompt: string;
  created_at: string;
  username: string;
  model: string;
  total_tokens: number;
  estimated_cost: number;
}

export async function fetchObservabilityUsers(
  range = '24h',
  limit = 50
): Promise<{ ok: boolean; status: number; users: UserUsageRow[] }> {
  const params = new URLSearchParams({ range, limit: String(limit) });
  const res = await fetch('/api/observability/users?' + params.toString(), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, users: [] };
  const body = (await parseJSON(res)) as { users?: UserUsageRow[] };
  return { ok: true, status: res.status, users: body?.users ?? [] };
}

export async function fetchObservabilityPrompts(
  opts: { range?: string; user?: string; q?: string; limit?: number } = {}
): Promise<{ ok: boolean; status: number; prompts: UserPromptRow[] }> {
  const params = new URLSearchParams();
  if (opts.range) params.set('range', opts.range);
  if (opts.user) params.set('user', opts.user);
  if (opts.q) params.set('q', opts.q);
  if (opts.limit) params.set('limit', String(opts.limit));
  const qs = params.toString();
  const res = await fetch('/api/observability/prompts' + (qs ? '?' + qs : ''), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, prompts: [] };
  const body = (await parseJSON(res)) as { prompts?: UserPromptRow[] };
  return { ok: true, status: res.status, prompts: body?.prompts ?? [] };
}

export async function storeProviderKey(provider: string, apiKey: string): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/secrets/provider', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: 'Bearer ' + getAdminToken()
    },
    body: JSON.stringify({ provider, api_key: apiKey })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export interface ObservabilityOverview {
  requests: number;
  successes: number;
  errors: number;
  error_rate: number;
  latency_ms: { p50_ms: number; p95_ms: number; p99_ms: number };
  tokens: { input: number; output: number; total: number };
  estimated_cost: number;
  latest_telemetry_at?: string;
}

export interface ObservabilityBreakdown {
  key: string;
  requests: number;
  successes: number;
  errors: number;
  error_rate: number;
  tokens: { input: number; output: number; total: number };
  estimated_cost: number;
  latency_ms: { p50_ms: number; p95_ms: number; p99_ms: number };
}

export interface GuardrailBreakdown {
  name: string;
  phase: string;
  action: string;
  count: number;
}

export interface ObservabilityHealth {
  status: string;
  clickhouse_reachable: boolean;
  data_state: string;
  latest_telemetry_at?: string;
  checked_at: string;
}

export async function fetchObservabilityHealth(): Promise<{ ok: boolean; status: number; health: ObservabilityHealth | null }> {
  const res = await fetch('/api/observability/health', {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, health: null };
  return { ok: true, status: res.status, health: (await parseJSON(res)) as ObservabilityHealth };
}

export async function fetchObservabilityOverview(
  range = '24h'
): Promise<{ ok: boolean; status: number; overview: ObservabilityOverview | null }> {
  const res = await fetch('/api/observability/overview?range=' + encodeURIComponent(range), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, overview: null };
  return { ok: true, status: res.status, overview: (await parseJSON(res)) as ObservabilityOverview };
}

export async function fetchObservabilityBreakdowns(
  range = '24h'
): Promise<{ ok: boolean; status: number; breakdowns: Record<string, ObservabilityBreakdown[]> }> {
  const res = await fetch('/api/observability/breakdowns?range=' + encodeURIComponent(range), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, breakdowns: {} };
  const body = (await parseJSON(res)) as Record<string, ObservabilityBreakdown[]>;
  return { ok: true, status: res.status, breakdowns: body ?? {} };
}

export async function fetchObservabilityGuardrails(
  range = '24h'
): Promise<{ ok: boolean; status: number; guardrails: GuardrailBreakdown[] }> {
  const res = await fetch('/api/observability/guardrails?range=' + encodeURIComponent(range), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, guardrails: [] };
  const body = (await parseJSON(res)) as { breakdown?: GuardrailBreakdown[] };
  return { ok: true, status: res.status, guardrails: body?.breakdown ?? [] };
}

export async function fetchMetrics(): Promise<{ ok: boolean; body: Record<string, unknown> }> {
  const res = await fetch('/metrics', {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  const body = (await parseJSON(res)) as Record<string, unknown>;
  return { ok: res.ok, body };
}

export interface UpstreamRoutingMetrics {
  name: string;
  decisions: number;
  failovers: number;
  retries: number;
  breaker_trips: number;
  breaker_recoveries: number;
}

export interface RoutingMetrics {
  decisions: number;
  failovers: number;
  retries: number;
  breaker_trips: number;
  breaker_recoveries: number;
  no_provider: number;
  denied: number;
  upstreams: UpstreamRoutingMetrics[];
}

// fetchRoutingMetrics pulls the in-process routing counters from /metrics.
// These are served synchronously by the gateway, so they are available even
// when ClickHouse tracing is disabled.
export async function fetchRoutingMetrics(): Promise<{ ok: boolean; metrics: RoutingMetrics }> {
  const res = await fetchMetrics();
  const body = res.body as Record<string, unknown>;

  const rows = Array.isArray(body.routing_upstreams)
    ? (body.routing_upstreams as Record<string, unknown>[])
    : [];

  const upstreams: UpstreamRoutingMetrics[] = rows.map((row) => ({
    name: String(row.name ?? ''),
    decisions: Number(row.decisions ?? 0),
    failovers: Number(row.failovers ?? 0),
    retries: Number(row.retries ?? 0),
    breaker_trips: Number(row.breaker_trips ?? 0),
    breaker_recoveries: Number(row.breaker_recoveries ?? 0)
  }));

  return {
    ok: res.ok,
    metrics: {
      decisions: Number(body.routing_decisions_total ?? 0),
      failovers: Number(body.routing_failovers_total ?? 0),
      retries: Number(body.routing_retries_total ?? 0),
      breaker_trips: Number(body.routing_breaker_trips_total ?? 0),
      breaker_recoveries: Number(body.routing_breaker_recoveries_total ?? 0),
      no_provider: Number(body.routing_no_provider_total ?? 0),
      denied: Number(body.routing_denied_total ?? 0),
      upstreams
    }
  };
}

export interface Role {
  id: string;
  name: string;
  description: string;
  policies: string[];
}

export interface Policy {
  name: string;
  description: string;
  version: number;
  is_builtin: boolean;
  document: unknown;
}

export type PrincipalType = 'user' | 'group' | 'role';

export interface PolicyAttachment {
  principal_type: PrincipalType;
  principal_id: string;
}

export interface Group {
  id: string;
  name: string;
  description: string;
}

export interface AdminUser {
  id: string;
  username: string;
  email?: string;
  status: string;
  created_at: string;
  roles: string[];
}

function adminHeaders(json = false): HeadersInit {
  const h: Record<string, string> = { Authorization: 'Bearer ' + getAdminToken() };
  if (json) h['Content-Type'] = 'application/json';
  return h;
}

// --- Roles -----------------------------------------------------------------

export async function fetchRoles(): Promise<{ ok: boolean; status: number; roles: Role[] }> {
  const res = await fetch('/admin/roles', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, roles: [] };
  const body = (await parseJSON(res)) as { roles?: Role[] };
  return { ok: true, status: res.status, roles: body?.roles ?? [] };
}

export async function createRole(
  name: string,
  description: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ name, description })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function deleteRole(role: string): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles/' + encodeURIComponent(role), {
    method: 'DELETE',
    headers: adminHeaders()
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function grantRole(
  role: string,
  principalType: PrincipalType,
  principalId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles/' + encodeURIComponent(role) + '/grant', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ principal_type: principalType, principal_id: principalId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function revokeRoleGrant(
  role: string,
  principalType: PrincipalType,
  principalId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles/' + encodeURIComponent(role) + '/revoke', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ principal_type: principalType, principal_id: principalId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function fetchRoleGrants(
  role: string
): Promise<{ ok: boolean; status: number; grants: PolicyAttachment[] }> {
  const res = await fetch('/admin/roles/' + encodeURIComponent(role) + '/grants', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, grants: [] };
  const body = (await parseJSON(res)) as { grants?: PolicyAttachment[] };
  return { ok: true, status: res.status, grants: body?.grants ?? [] };
}

// --- Policies --------------------------------------------------------------

export async function fetchPolicies(): Promise<{ ok: boolean; status: number; policies: Policy[] }> {
  const res = await fetch('/admin/policies', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, policies: [] };
  const body = (await parseJSON(res)) as { policies?: Policy[] };
  return { ok: true, status: res.status, policies: body?.policies ?? [] };
}

export async function createPolicy(
  name: string,
  description: string,
  document: unknown
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/policies', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ name, description, document })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function deletePolicy(policy: string): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/policies/' + encodeURIComponent(policy), {
    method: 'DELETE',
    headers: adminHeaders()
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function attachPolicy(
  policy: string,
  principalType: PrincipalType,
  principalId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/policies/' + encodeURIComponent(policy) + '/attach', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ principal_type: principalType, principal_id: principalId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function detachPolicy(
  policy: string,
  principalType: PrincipalType,
  principalId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/policies/' + encodeURIComponent(policy) + '/detach', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ principal_type: principalType, principal_id: principalId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function fetchPolicyAttachments(
  policy: string
): Promise<{ ok: boolean; status: number; attachments: PolicyAttachment[] }> {
  const res = await fetch('/admin/policies/' + encodeURIComponent(policy) + '/attachments', {
    headers: adminHeaders()
  });
  if (!res.ok) return { ok: false, status: res.status, attachments: [] };
  const body = (await parseJSON(res)) as { attachments?: PolicyAttachment[] };
  return { ok: true, status: res.status, attachments: body?.attachments ?? [] };
}

// --- Groups ----------------------------------------------------------------

export async function fetchGroups(): Promise<{ ok: boolean; status: number; groups: Group[] }> {
  const res = await fetch('/admin/groups', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, groups: [] };
  const body = (await parseJSON(res)) as { groups?: Group[] };
  return { ok: true, status: res.status, groups: body?.groups ?? [] };
}

export async function createGroup(
  name: string,
  description: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/groups', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ name, description })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function deleteGroup(group: string): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/groups/' + encodeURIComponent(group), {
    method: 'DELETE',
    headers: adminHeaders()
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function fetchGroupMembers(
  group: string
): Promise<{ ok: boolean; status: number; members: string[] }> {
  const res = await fetch('/admin/groups/' + encodeURIComponent(group) + '/members', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, members: [] };
  const body = (await parseJSON(res)) as { members?: string[] };
  return { ok: true, status: res.status, members: body?.members ?? [] };
}

export async function addGroupMember(
  group: string,
  userId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/groups/' + encodeURIComponent(group) + '/members', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ user_id: userId })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function removeGroupMember(
  group: string,
  userId: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch(
    '/admin/groups/' + encodeURIComponent(group) + '/members/' + encodeURIComponent(userId),
    { method: 'DELETE', headers: adminHeaders() }
  );
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

// --- User roles & permission boundary --------------------------------------

export async function assignRole(
  userId: string,
  role: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/users/' + encodeURIComponent(userId) + '/roles', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ role })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function revokeRole(
  userId: string,
  role: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch(
    '/admin/users/' + encodeURIComponent(userId) + '/roles/' + encodeURIComponent(role),
    { method: 'DELETE', headers: adminHeaders() }
  );
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function setUserBoundary(
  userId: string,
  policy: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/users/' + encodeURIComponent(userId) + '/permissions-boundary', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ policy })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function fetchUsers(): Promise<{ ok: boolean; status: number; users: AdminUser[] }> {
  const res = await fetch('/admin/users', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, users: [] };
  const body = (await parseJSON(res)) as { users?: AdminUser[] };
  return { ok: true, status: res.status, users: body?.users ?? [] };
}

export interface RoutingProviderTraffic {
  provider: string;
  request_count: number;
  traffic_percentage: number;
}

export interface RoutingLatency {
  provider: string;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
}

export interface RoutingErrorSummary {
  provider: string;
  total_requests: number;
  error_count: number;
  error_rate_percentage: number;
}

export interface RoutingFailoverTransition {
  failover_from: string;
  failover_to: string;
  failover_count: number;
}

export interface RoutingFailoverSummary {
  failover_count: number;
  total_requests: number;
  failover_percentage: number;
}

export interface RoutingTokensByRoute {
  route: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  request_count: number;
}

export interface RoutingTokensByProvider {
  provider: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  request_count: number;
}

export interface RoutingCostByRoute {
  route: string;
  total_cost: number;
  avg_cost_per_request: number;
  request_count: number;
}

export interface RoutingCostByProvider {
  provider: string;
  total_cost: number;
  avg_cost_per_request: number;
  request_count: number;
}

export interface AnalyticsRow {
  [key: string]: string | number;
}

export async function fetchAnalytics(
  type: string,
  hours = 24
): Promise<{ ok: boolean; status: number; rows: AnalyticsRow[] }> {
  const res = await fetch(`/admin/analytics?type=${type}&hours=${hours}`, {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, rows: [] };
  const body = (await parseJSON(res)) as { data?: AnalyticsRow[] };
  return { ok: true, status: res.status, rows: body?.data ?? [] };
}

export function fetchRoutingFailoverTransitions(hours = 24) {
  return fetchAnalyticsRows<RoutingFailoverTransition>(
    'failover_transitions',
    hours
  );
}

async function fetchAnalyticsRows<T>(type: string, hours = 24): Promise<{
  ok: boolean;
  status: number;
  rows: T[];
}> {
  const result = await fetchAnalytics(type, hours);
  return {
    ok: result.ok,
    status: result.status,
    rows: result.rows as T[]
  };
}

export function fetchRoutingProviderTraffic(hours = 24) {
  return fetchAnalyticsRows<RoutingProviderTraffic>('provider_traffic', hours);
}

export function fetchRoutingLatency(hours = 24) {
  return fetchAnalyticsRows<RoutingLatency>('latency_percentiles', hours);
}

export function fetchRoutingErrorSummary(hours = 24) {
  return fetchAnalyticsRows<RoutingErrorSummary>('error_rate_summary', hours);
}

export function fetchRoutingFailovers(hours = 24) {
  return fetchAnalyticsRows<RoutingFailoverSummary>('failover_count', hours);
}

export function fetchRoutingTokensByRoute(hours = 24) {
  return fetchAnalyticsRows<RoutingTokensByRoute>('tokens_by_route', hours);
}

export function fetchRoutingTokensByProvider(hours = 24) {
  return fetchAnalyticsRows<RoutingTokensByProvider>('tokens_by_provider', hours);
}

export function fetchRoutingCostByRoute(hours = 24) {
  return fetchAnalyticsRows<RoutingCostByRoute>('cost_by_route', hours);
}

export function fetchRoutingCostByProvider(hours = 24) {
  return fetchAnalyticsRows<RoutingCostByProvider>('cost_by_provider', hours);
}

export interface ProviderRecord {
  id: string;
  name: string;
  base_url: string;
  secret_ref?: string;
  provider_group: string;
  weight: number;
  supported_models: string[];
  timeout_ms: number;
  max_retries: number;
  breaker_failure_threshold: number;
  enabled: boolean;
}

export interface UpstreamStatus {
  name: string;
  enabled: boolean;
  weight: number;
  models: string[];
  breaker: string;
  breaker_failure_threshold: number;
}

export async function fetchUpstreamStatuses(): Promise<{
  ok: boolean;
  status: number;
  upstreams: UpstreamStatus[];
}> {
  const result = await adminRequest('/admin/upstreams');

  if (!result.ok) {
    return { ok: false, status: result.status, upstreams: [] };
  }

  const body = result.body as
    | Record<string, unknown>[]
    | { upstreams?: Record<string, unknown>[] }
    | null;

  const rows = Array.isArray(body)
    ? body
    : body?.upstreams ?? [];

  const upstreams: UpstreamStatus[] = rows.map((row) => ({
    name: String(row.name ?? row.Name ?? ''),
    enabled: Boolean(row.enabled ?? row.Enabled ?? false),
    weight: Number(row.weight ?? row.Weight ?? 1),
    models: (row.models ?? row.Models ?? []) as string[],
    breaker: String(row.breaker ?? row.State ?? row.State ?? 'unknown'),
    breaker_failure_threshold: Number(
      row.breaker_failure_threshold ?? row.BreakerFailureThreshold ?? 5
    )
  }));

  return {
    ok: true,
    status: result.status,
    upstreams
  };
}

export interface RoutingRuleRecord {
  id: string;
  name: string;
  priority: number;
  model_matcher?: unknown;
  role_matcher?: unknown;
  user_matcher?: unknown;
  action: string;
  provider_group?: string;
  timeout_retry_overrides?: unknown;
  enabled: boolean;
}

async function adminRequest(
  path: string,
  options: RequestInit = {}
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch(path, {
    ...options,
    headers: {
      ...adminHeaders(Boolean(options.body)),
      ...(options.headers || {})
    }
  });

  return {
    ok: res.ok,
    status: res.status,
    body: await parseJSON(res)
  };
}

export async function fetchProviders(): Promise<{
  ok: boolean;
  status: number;
  providers: ProviderRecord[];
}> {
  const result = await adminRequest('/admin/providers');

  if (!result.ok) {
    return {
      ok: false,
      status: result.status,
      providers: []
    };
  }

  const body = result.body as
    | Record<string, unknown>[]
    | { providers?: Record<string, unknown>[] }
    | null;

  const rows = Array.isArray(body)
    ? body
    : body?.providers ?? [];

  const providers: ProviderRecord[] = rows.map((row) => ({
    id: String(row.id ?? row.ID ?? ''),
    name: String(row.name ?? row.Name ?? ''),
    base_url: String(row.base_url ?? row.BaseURL ?? ''),
    secret_ref: String(row.secret_ref ?? row.SecretRef ?? ''),
    provider_group: String(row.provider_group ?? row.ProviderGroup ?? ''),
    weight: Number(row.weight ?? row.Weight ?? 1),
    supported_models: (
      row.supported_models ?? row.SupportedModels ?? []
    ) as string[],
    timeout_ms: Number(row.timeout_ms ?? row.TimeoutMS ?? 60000),
    max_retries: Number(row.max_retries ?? row.MaxRetries ?? 1),
    breaker_failure_threshold: Number(
      row.breaker_failure_threshold ?? row.BreakerFailureThreshold ?? 5
    ),
    enabled: Boolean(row.enabled ?? row.Enabled ?? false)
  }));

  return {
    ok: true,
    status: result.status,
    providers
  };
}

export async function createProvider(
  provider: Omit<ProviderRecord, 'id'>
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/providers', {
    method: 'POST',
    body: JSON.stringify({
      name: provider.name,
      base_url: provider.base_url,
      secret_key: provider.secret_ref || '',
      provider_group: provider.provider_group,
      weight: provider.weight,
      supported_models: provider.supported_models,
      timeout_ms: provider.timeout_ms,
      max_retries: provider.max_retries,
      breaker_failure_threshold: provider.breaker_failure_threshold,
      enabled: provider.enabled
    })
  });
}

export async function updateProvider(
  id: string,
  provider: Omit<ProviderRecord, 'id'>
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/providers/' + encodeURIComponent(id), {
    method: 'PUT',
    body: JSON.stringify({
      name: provider.name,
      base_url: provider.base_url,
      secret_key: provider.secret_ref || '',
      provider_group: provider.provider_group,
      weight: provider.weight,
      supported_models: provider.supported_models,
      timeout_ms: provider.timeout_ms,
      max_retries: provider.max_retries,
      breaker_failure_threshold: provider.breaker_failure_threshold,
      enabled: provider.enabled
    })
  });
}

export async function enableProvider(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/providers/' + encodeURIComponent(id) + '/enable', {
    method: 'POST'
  });
}

export async function disableProvider(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/providers/' + encodeURIComponent(id) + '/disable', {
    method: 'POST'
  });
}

export async function deleteProvider(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/providers/' + encodeURIComponent(id), {
    method: 'DELETE'
  });
}


export async function fetchRoutingRules(): Promise<{
  ok: boolean;
  status: number;
  rules: RoutingRuleRecord[];
}> {
  const result = await adminRequest('/admin/routing-rules');

  if (!result.ok) {
    return { ok: false, status: result.status, rules: [] };
  }

  const body = result.body as
    | Record<string, unknown>[]
    | {
        rules?: Record<string, unknown>[];
        routing_rules?: Record<string, unknown>[];
      }
    | null;

  const rows = Array.isArray(body)
    ? body
    : body?.rules ?? body?.routing_rules ?? [];

  const rules: RoutingRuleRecord[] = rows.map((row) => ({
    id: String(row.id ?? row.ID ?? ''),
    name: String(row.name ?? row.Name ?? ''),
    priority: Number(row.priority ?? row.Priority ?? 0),
    model_matcher: row.model_matcher ?? row.ModelMatcher ?? {},
    role_matcher: row.role_matcher ?? row.RoleMatcher ?? {},
    user_matcher: row.user_matcher ?? row.UserMatcher ?? {},
    action: String(row.action ?? row.Action ?? ''),
    provider_group: String(
      row.provider_group ?? row.ProviderGroup ?? ''
    ),
    timeout_retry_overrides:
      row.timeout_retry_overrides ?? row.TimeoutRetryOverrides ?? {},
    enabled: Boolean(row.enabled ?? row.Enabled ?? false)
  })) as RoutingRuleRecord[];

  return {
    ok: true,
    status: result.status,
    rules
  };
}

function routingRulePayload(rule: Omit<RoutingRuleRecord, 'id'>) {
  return {
    name: rule.name,
    priority: rule.priority,
    model_matcher: rule.model_matcher ?? {},
    role_matcher: rule.role_matcher ?? {},
    user_matcher: rule.user_matcher ?? {},
    action: rule.action,
    provider_group: rule.provider_group || '',
    timeout_retry_overrides: rule.timeout_retry_overrides ?? {},
    enabled: rule.enabled
  };
}

export async function createRoutingRule(
  rule: Omit<RoutingRuleRecord, 'id'>
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/routing-rules', {
    method: 'POST',
    body: JSON.stringify(routingRulePayload(rule))
  });
}

export async function updateRoutingRule(
  id: string,
  rule: Omit<RoutingRuleRecord, 'id'>
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/routing-rules/' + encodeURIComponent(id), {
    method: 'PUT',
    body: JSON.stringify(routingRulePayload(rule))
  });
}

export async function enableRoutingRule(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/routing-rules/' + encodeURIComponent(id) + '/enable', {
    method: 'POST'
  });
}

export async function disableRoutingRule(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/routing-rules/' + encodeURIComponent(id) + '/disable', {
    method: 'POST'
  });
}

export async function deleteRoutingRule(
  id: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  return adminRequest('/admin/routing-rules/' + encodeURIComponent(id), {
    method: 'DELETE'
  });
}