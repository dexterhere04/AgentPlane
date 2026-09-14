const ADMIN_TOKEN_KEY = 'agentplane.adminToken';
const USER_KEY_KEY = 'agentplane.userKey';

export function getAdminToken(): string {
  return localStorage.getItem(ADMIN_TOKEN_KEY) || 'dev-admin-token-change-me';
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

export async function fetchUserAnalytics(
  opts: { hours?: number; user?: string; q?: string } = {}
): Promise<{ ok: boolean; status: number; users: UserUsageRow[]; prompts: UserPromptRow[] }> {
  const params = new URLSearchParams();
  if (opts.hours) params.set('hours', String(opts.hours));
  if (opts.user) params.set('user', opts.user);
  if (opts.q) params.set('q', opts.q);
  const qs = params.toString();
  const res = await fetch('/admin/analytics/users' + (qs ? '?' + qs : ''), {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, users: [], prompts: [] };
  const body = (await parseJSON(res)) as { users?: UserUsageRow[]; prompts?: UserPromptRow[] };
  return { ok: true, status: res.status, users: body?.users ?? [], prompts: body?.prompts ?? [] };
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

export interface AnalyticsRow {
  [key: string]: string | number;
}

export async function fetchAnalytics(type: string, hours = 24): Promise<{ ok: boolean; status: number; rows: AnalyticsRow[] }> {
  const res = await fetch(`/admin/analytics?type=${type}&hours=${hours}`, {
    headers: { Authorization: 'Bearer ' + getAdminToken() }
  });
  if (!res.ok) return { ok: false, status: res.status, rows: [] };
  const body = (await parseJSON(res)) as { data?: AnalyticsRow[] };
  return { ok: true, status: res.status, rows: body?.data ?? [] };
}

export async function fetchMetrics(): Promise<{ ok: boolean; body: Record<string, unknown> }> {
  const res = await fetch('/metrics');
  const body = (await parseJSON(res)) as Record<string, unknown>;
  return { ok: res.ok, body };
}

export interface Role {
  name: string;
  description: string;
  permissions: string[];
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

export async function fetchRoles(): Promise<{ ok: boolean; status: number; roles: Role[] }> {
  const res = await fetch('/admin/roles', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, roles: [] };
  const body = (await parseJSON(res)) as { roles?: Role[] };
  return { ok: true, status: res.status, roles: body?.roles ?? [] };
}

export async function createRole(
  name: string,
  description: string,
  permissions: string[]
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ name, description, permissions })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function addRolePermission(
  role: string,
  permission: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch('/admin/roles/' + encodeURIComponent(role) + '/permissions', {
    method: 'POST',
    headers: adminHeaders(true),
    body: JSON.stringify({ permission })
  });
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

export async function removeRolePermission(
  role: string,
  permission: string
): Promise<{ ok: boolean; status: number; body: unknown }> {
  const res = await fetch(
    '/admin/roles/' + encodeURIComponent(role) + '/permissions/' + encodeURIComponent(permission),
    { method: 'DELETE', headers: adminHeaders() }
  );
  return { ok: res.ok, status: res.status, body: await parseJSON(res) };
}

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

export async function fetchUsers(): Promise<{ ok: boolean; status: number; users: AdminUser[] }> {
  const res = await fetch('/admin/users', { headers: adminHeaders() });
  if (!res.ok) return { ok: false, status: res.status, users: [] };
  const body = (await parseJSON(res)) as { users?: AdminUser[] };
  return { ok: true, status: res.status, users: body?.users ?? [] };
}
