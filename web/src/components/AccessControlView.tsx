import { useCallback, useEffect, useState } from 'react';
import {
  AdminUser,
  Group,
  Policy,
  PolicyAttachment,
  PrincipalType,
  Role,
  addGroupMember,
  assignRole,
  attachPolicy,
  createGroup,
  createPolicy,
  createRole,
  deleteGroup,
  deletePolicy,
  deleteRole,
  detachPolicy,
  fetchGroupMembers,
  fetchGroups,
  fetchPolicies,
  fetchPolicyAttachments,
  fetchRoles,
  fetchUsers,
  grantRole,
  removeGroupMember,
  revokeRole,
  setUserBoundary
} from '../api';
import { Card, EmptyState, Tabs } from './ui';
import { Icon } from '../icons';

const DEFAULT_DOCUMENT = `{
  "Version": "2026-01-01",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["ap:InvokeChat", "ap:InvokeModel"],
      "Resource": ["arn:ap:model/*"]
    }
  ]
}`;

export default function AccessControlView() {
  const [tab, setTab] = useState('policies');

  return (
    <div className="stack">
      <Tabs
        items={[
          { id: 'policies', label: 'Policies', icon: <Icon name="list" size={14} /> },
          { id: 'roles', label: 'Roles', icon: <Icon name="shield" size={14} /> },
          { id: 'groups', label: 'Groups', icon: <Icon name="layers" size={14} /> },
          { id: 'users', label: 'Users', icon: <Icon name="users" size={14} /> }
        ]}
        active={tab}
        onChange={setTab}
      />

      {tab === 'policies' && <PoliciesPanel />}
      {tab === 'roles' && <RolesPanel />}
      {tab === 'groups' && <GroupsPanel />}
      {tab === 'users' && <UsersPanel />}
    </div>
  );
}

function RefreshButton({ loading, onClick }: { loading: boolean; onClick: () => void }) {
  return (
    <button className="btn ghost sm" onClick={onClick} disabled={loading}>
      <Icon name="refresh" size={14} />
      {loading ? 'Loading…' : 'Refresh'}
    </button>
  );
}

// loadError renders an actionable message. A 401 almost always means the admin
// token is missing from Settings — surface that instead of a bare status code.
function loadError(status: number, what: string): string {
  if (status === 401) {
    return 'HTTP 401 · unauthorized — open Settings (the ⚙ icon in the top bar) and paste your AGENTPLANE_ADMIN_TOKEN.';
  }
  if (status === 403) {
    return 'HTTP 403 · forbidden — your credentials are valid but not permitted this action.';
  }
  return 'HTTP ' + status + ' · could not load ' + what;
}

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

function PoliciesPanel() {
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const [p, r, g, u] = await Promise.all([fetchPolicies(), fetchRoles(), fetchGroups(), fetchUsers()]);
    if (p.ok) setPolicies(p.policies);
    else setErr(loadError(p.status, 'policies'));
    if (r.ok) setRoles(r.roles);
    if (g.ok) setGroups(g.groups);
    if (u.ok) setUsers(u.users);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Card
        title="Policies"
        subtitle="GET /admin/policies — AWS-style JSON policy documents. An explicit Deny overrides any Allow, and access is deny-by-default."
        right={<RefreshButton loading={loading} onClick={load} />}
      >
        {err && <div className="alert error">{err}</div>}

        {policies.length === 0 ? (
          <EmptyState icon={<Icon name="list" size={20} />} text="No policies defined yet." />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Policy</th>
                <th>Document</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {policies.map((p) => (
                <PolicyRow key={p.name} policy={p} onChanged={load} />
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <CreatePolicyCard onCreated={load} />
      <AttachPolicyCard policies={policies} roles={roles} groups={groups} users={users} onChanged={load} />
    </>
  );
}

function PolicyRow({ policy, onChanged }: { policy: Policy; onChanged: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const remove = async () => {
    setBusy(true);
    setErr('');
    const res = await deletePolicy(policy.name);
    if (!res.ok) setErr('HTTP ' + res.status);
    setBusy(false);
    onChanged();
  };

  return (
    <tr>
      <td style={{ verticalAlign: 'top' }}>
        <span className="key-name">{policy.name}</span>
        {policy.is_builtin && <span className="badge accent" style={{ marginLeft: 6 }}>builtin</span>}
        {policy.description && (
          <div className="hint" style={{ marginTop: 2 }}>
            {policy.description}
          </div>
        )}
      </td>
      <td style={{ maxWidth: 520 }}>
        <details>
          <summary className="hint" style={{ cursor: 'pointer' }}>
            v{policy.version} · view document
          </summary>
          <pre className="mono" style={{ margin: '6px 0 0', whiteSpace: 'pre-wrap', fontSize: 12 }}>
            {JSON.stringify(policy.document, null, 2)}
          </pre>
        </details>
        {err && <div className="alert error">{err}</div>}
      </td>
      <td style={{ verticalAlign: 'top', textAlign: 'right' }}>
        <button className="btn ghost sm" disabled={busy || policy.is_builtin} title={policy.is_builtin ? 'Built-in policies cannot be deleted' : 'Delete policy'} onClick={remove}>
          <Icon name="x" size={14} />
          Delete
        </button>
      </td>
    </tr>
  );
}

function CreatePolicyCard({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [document, setDocument] = useState(DEFAULT_DOCUMENT);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');

  const submit = async () => {
    setBusy(true);
    setErr('');
    setOk('');
    let parsed: unknown;
    try {
      parsed = JSON.parse(document);
    } catch {
      setErr('Document is not valid JSON');
      setBusy(false);
      return;
    }
    const res = await createPolicy(name.trim(), description.trim(), parsed);
    if (res.ok) {
      setOk('Created policy "' + name.trim() + '"');
      setName('');
      setDescription('');
      setDocument(DEFAULT_DOCUMENT);
      onCreated();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  return (
    <Card
      title="Create policy"
      subtitle="POST /admin/policies — provide an AWS-style JSON document. Actions use the ap: prefix (e.g. ap:InvokeModel); resources are arn:ap:{type}/{id}."
    >
      <div className="row-form">
        <div className="field">
          <label>name</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="gpt4o-only" />
        </div>
        <div className="field grow">
          <label>description</label>
          <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" />
        </div>
      </div>
      <div className="field" style={{ marginTop: 10 }}>
        <label>document</label>
        <textarea
          className="input mono"
          rows={9}
          value={document}
          onChange={(e) => setDocument(e.target.value)}
          spellCheck={false}
        />
      </div>
      <div style={{ marginTop: 10 }}>
        <button className="btn" disabled={busy || !name.trim()} onClick={submit}>
          <Icon name="plus" size={15} />
          {busy ? 'Creating…' : 'Create policy'}
        </button>
      </div>
      {err && <div className="alert error">{err}</div>}
      {ok && <div className="alert ok">{ok}</div>}
    </Card>
  );
}

function principalOptions(
  ptype: PrincipalType,
  roles: Role[],
  groups: Group[],
  users: AdminUser[]
): { id: string; label: string }[] {
  if (ptype === 'role') return roles.map((r) => ({ id: r.id, label: r.name }));
  if (ptype === 'group') return groups.map((g) => ({ id: g.id, label: g.name }));
  return users.map((u) => ({ id: u.id, label: u.username }));
}

function AttachPolicyCard({
  policies,
  roles,
  groups,
  users,
  onChanged
}: {
  policies: Policy[];
  roles: Role[];
  groups: Group[];
  users: AdminUser[];
  onChanged: () => void;
}) {
  const [policy, setPolicy] = useState('');
  const [ptype, setPtype] = useState<PrincipalType>('role');
  const [pid, setPid] = useState('');
  const [attachments, setAttachments] = useState<PolicyAttachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');

  const loadAttachments = useCallback(async (name: string) => {
    if (!name) {
      setAttachments([]);
      return;
    }
    const res = await fetchPolicyAttachments(name);
    setAttachments(res.ok ? res.attachments : []);
  }, []);

  useEffect(() => {
    loadAttachments(policy);
  }, [policy, loadAttachments]);

  const options = principalOptions(ptype, roles, groups, users);

  const attach = async () => {
    if (!policy || !pid) return;
    setBusy(true);
    setErr('');
    setOk('');
    const res = await attachPolicy(policy, ptype, pid);
    if (res.ok) {
      setOk('Attached "' + policy + '"');
      setPid('');
      await loadAttachments(policy);
      onChanged();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  const detach = async (a: PolicyAttachment) => {
    setBusy(true);
    setErr('');
    setOk('');
    const res = await detachPolicy(policy, a.principal_type, a.principal_id);
    if (res.ok) {
      await loadAttachments(policy);
      onChanged();
    } else {
      setErr('HTTP ' + res.status);
    }
    setBusy(false);
  };

  const labelFor = (a: PolicyAttachment) => {
    const found = principalOptions(a.principal_type, roles, groups, users).find((o) => o.id === a.principal_id);
    return a.principal_type + ': ' + (found?.label ?? a.principal_id);
  };

  return (
    <Card
      title="Attach policy"
      subtitle="POST /admin/policies/{name}/attach — attach a policy to a user, group, or role. Effective permissions are the union, capped by any permission boundary."
    >
      <div className="row-form">
        <div className="field grow">
          <label>policy</label>
          <select className="select" value={policy} onChange={(e) => setPolicy(e.target.value)}>
            <option value="">select policy…</option>
            {policies.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
          <label>principal type</label>
          <select
            className="select"
            value={ptype}
            onChange={(e) => {
              setPtype(e.target.value as PrincipalType);
              setPid('');
            }}
          >
            <option value="role">role</option>
            <option value="group">group</option>
            <option value="user">user</option>
          </select>
        </div>
        <div className="field grow">
          <label>principal</label>
          <select className="select" value={pid} onChange={(e) => setPid(e.target.value)}>
            <option value="">select {ptype}…</option>
            {options.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <button className="btn" disabled={busy || !policy || !pid} onClick={attach}>
          <Icon name="plus" size={15} />
          Attach
        </button>
      </div>

      {policy && (
        <div style={{ marginTop: 12 }}>
          <div className="hint" style={{ marginBottom: 4 }}>
            Attached to
          </div>
          <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
            {attachments.length === 0 && <span className="hint">Not attached to any principal.</span>}
            {attachments.map((a) => (
              <span className="badge accent" key={a.principal_type + a.principal_id}>
                {labelFor(a)}
                <button
                  title="Detach"
                  disabled={busy}
                  onClick={() => detach(a)}
                  style={{ border: 'none', background: 'transparent', color: 'inherit', cursor: 'pointer', padding: '0 0 0 5px', font: 'inherit', lineHeight: 1 }}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        </div>
      )}

      {err && <div className="alert error">{err}</div>}
      {ok && <div className="alert ok">{ok}</div>}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

function RolesPanel() {
  const [roles, setRoles] = useState<Role[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const [r, p, u] = await Promise.all([fetchRoles(), fetchPolicies(), fetchUsers()]);
    if (r.ok) setRoles(r.roles);
    else setErr(loadError(r.status, 'roles'));
    if (p.ok) setPolicies(p.policies);
    if (u.ok) setUsers(u.users);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Card
        title="Roles"
        subtitle="GET /admin/roles — a role is an assumable identity carrying policies. Grant a role to a user to give them its permissions."
        right={<RefreshButton loading={loading} onClick={load} />}
      >
        {err && <div className="alert error">{err}</div>}

        {roles.length === 0 ? (
          <EmptyState icon={<Icon name="shield" size={20} />} text="No roles defined yet." />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Role</th>
                <th>Attached policies</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {roles.map((role) => (
                <RoleRow key={role.name} role={role} policies={policies} onChanged={load} />
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <CreateRoleCard onCreated={load} />
      <GrantRoleCard roles={roles} users={users} onChanged={load} />
    </>
  );
}

function RoleRow({ role, policies, onChanged }: { role: Role; policies: Policy[]; onChanged: () => void }) {
  const [choice, setChoice] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const available = policies.filter((p) => !role.policies.includes(p.name));

  const attach = async () => {
    if (!choice) return;
    setBusy(true);
    setErr('');
    const res = await attachPolicy(choice, 'role', role.id);
    if (!res.ok) setErr('HTTP ' + res.status);
    setChoice('');
    setBusy(false);
    onChanged();
  };

  const detach = async (policy: string) => {
    setBusy(true);
    setErr('');
    const res = await detachPolicy(policy, 'role', role.id);
    if (!res.ok) setErr('HTTP ' + res.status);
    setBusy(false);
    onChanged();
  };

  const remove = async () => {
    setBusy(true);
    setErr('');
    const res = await deleteRole(role.name);
    if (!res.ok) setErr('HTTP ' + res.status);
    setBusy(false);
    onChanged();
  };

  return (
    <tr>
      <td>
        <span className="key-name">{role.name}</span>
        {role.description && (
          <div className="hint" style={{ marginTop: 2 }}>
            {role.description}
          </div>
        )}
      </td>
      <td>
        <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
          {role.policies.length === 0 && <span className="hint">No policies — this role grants nothing.</span>}
          {role.policies.map((p) => (
            <span className="badge accent" key={p}>
              {p}
              <button
                title={'Detach ' + p}
                disabled={busy}
                onClick={() => detach(p)}
                style={{ border: 'none', background: 'transparent', color: 'inherit', cursor: 'pointer', padding: '0 0 0 5px', font: 'inherit', lineHeight: 1 }}
              >
                ×
              </button>
            </span>
          ))}
        </div>
        <div className="row-form" style={{ marginTop: 8 }}>
          <select className="select" value={choice} onChange={(e) => setChoice(e.target.value)}>
            <option value="">attach policy…</option>
            {available.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
          <button className="btn ghost sm" disabled={busy || !choice} onClick={attach}>
            <Icon name="plus" size={14} />
            Attach
          </button>
        </div>
        {err && <div className="alert error">{err}</div>}
      </td>
      <td style={{ textAlign: 'right' }}>
        <button className="btn ghost sm" disabled={busy} onClick={remove}>
          <Icon name="x" size={14} />
          Delete
        </button>
      </td>
    </tr>
  );
}

function CreateRoleCard({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');

  const submit = async () => {
    setBusy(true);
    setErr('');
    setOk('');
    const res = await createRole(name.trim(), description.trim());
    if (res.ok) {
      setOk('Created role "' + name.trim() + '"');
      setName('');
      setDescription('');
      onCreated();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  return (
    <Card title="Create role" subtitle="POST /admin/roles — create a role, then attach policies to it.">
      <div className="row-form">
        <div className="field">
          <label>name</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="gpt4o-user" />
        </div>
        <div className="field grow">
          <label>description</label>
          <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" />
        </div>
        <button className="btn" disabled={busy || !name.trim()} onClick={submit}>
          <Icon name="plus" size={15} />
          {busy ? 'Creating…' : 'Create'}
        </button>
      </div>
      {err && <div className="alert error">{err}</div>}
      {ok && <div className="alert ok">{ok}</div>}
    </Card>
  );
}

function GrantRoleCard({ roles, users, onChanged }: { roles: Role[]; users: AdminUser[]; onChanged: () => void }) {
  const [role, setRole] = useState('');
  const [userId, setUserId] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');

  const submit = async () => {
    if (!role || !userId) return;
    setBusy(true);
    setErr('');
    setOk('');
    const res = await grantRole(role, 'user', userId);
    if (res.ok) {
      setOk('Granted "' + role + '"');
      setUserId('');
      onChanged();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  return (
    <Card title="Grant role" subtitle="POST /admin/roles/{name}/grant — grant a role to a user. Access is deny-by-default.">
      <div className="row-form">
        <div className="field grow">
          <label>role</label>
          <select className="select" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="">select role…</option>
            {roles.map((r) => (
              <option key={r.name} value={r.name}>
                {r.name}
              </option>
            ))}
          </select>
        </div>
        <div className="field grow">
          <label>user</label>
          <select className="select" value={userId} onChange={(e) => setUserId(e.target.value)}>
            <option value="">select user…</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.username}
              </option>
            ))}
          </select>
        </div>
        <button className="btn" disabled={busy || !role || !userId} onClick={submit}>
          <Icon name="plus" size={15} />
          Grant
        </button>
      </div>
      {err && <div className="alert error">{err}</div>}
      {ok && <div className="alert ok">{ok}</div>}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

function GroupsPanel() {
  const [groups, setGroups] = useState<Group[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const [g, u] = await Promise.all([fetchGroups(), fetchUsers()]);
    if (g.ok) setGroups(g.groups);
    else setErr(loadError(g.status, 'groups'));
    if (u.ok) setUsers(u.users);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <>
      <Card
        title="Groups"
        subtitle="GET /admin/groups — a group is a collection of users. Policies attached to a group apply to every member."
        right={<RefreshButton loading={loading} onClick={load} />}
      >
        {err && <div className="alert error">{err}</div>}

        {groups.length === 0 ? (
          <EmptyState icon={<Icon name="layers" size={20} />} text="No groups defined yet." />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Group</th>
                <th>Members</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {groups.map((g) => (
                <GroupRow key={g.name} group={g} users={users} onChanged={load} />
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <CreateGroupCard onCreated={load} />
    </>
  );
}

function GroupRow({ group, users, onChanged }: { group: Group; users: AdminUser[]; onChanged: () => void }) {
  const [members, setMembers] = useState<string[]>([]);
  const [pick, setPick] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const loadMembers = useCallback(async () => {
    const res = await fetchGroupMembers(group.name);
    setMembers(res.ok ? res.members : []);
  }, [group.name]);

  useEffect(() => {
    loadMembers();
  }, [loadMembers]);

  const username = (id: string) => users.find((u) => u.id === id)?.username ?? id;

  const add = async () => {
    if (!pick) return;
    setBusy(true);
    setErr('');
    const res = await addGroupMember(group.name, pick);
    if (!res.ok) setErr('HTTP ' + res.status);
    setPick('');
    setBusy(false);
    await loadMembers();
    onChanged();
  };

  const remove = async (userId: string) => {
    setBusy(true);
    setErr('');
    const res = await removeGroupMember(group.name, userId);
    if (!res.ok) setErr('HTTP ' + res.status);
    setBusy(false);
    await loadMembers();
    onChanged();
  };

  const removeGroup = async () => {
    setBusy(true);
    setErr('');
    const res = await deleteGroup(group.name);
    if (!res.ok) setErr('HTTP ' + res.status);
    setBusy(false);
    onChanged();
  };

  const available = users.filter((u) => !members.includes(u.id));

  return (
    <tr>
      <td>
        <span className="key-name">{group.name}</span>
        {group.description && (
          <div className="hint" style={{ marginTop: 2 }}>
            {group.description}
          </div>
        )}
      </td>
      <td>
        <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
          {members.length === 0 && <span className="hint">No members.</span>}
          {members.map((id) => (
            <span className="badge accent" key={id}>
              {username(id)}
              <button
                title="Remove member"
                disabled={busy}
                onClick={() => remove(id)}
                style={{ border: 'none', background: 'transparent', color: 'inherit', cursor: 'pointer', padding: '0 0 0 5px', font: 'inherit', lineHeight: 1 }}
              >
                ×
              </button>
            </span>
          ))}
        </div>
        <div className="row-form" style={{ marginTop: 8 }}>
          <select className="select" value={pick} onChange={(e) => setPick(e.target.value)}>
            <option value="">add member…</option>
            {available.map((u) => (
              <option key={u.id} value={u.id}>
                {u.username}
              </option>
            ))}
          </select>
          <button className="btn ghost sm" disabled={busy || !pick} onClick={add}>
            <Icon name="plus" size={14} />
            Add
          </button>
        </div>
        {err && <div className="alert error">{err}</div>}
      </td>
      <td style={{ textAlign: 'right' }}>
        <button className="btn ghost sm" disabled={busy} onClick={removeGroup}>
          <Icon name="x" size={14} />
          Delete
        </button>
      </td>
    </tr>
  );
}

function CreateGroupCard({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const [ok, setOk] = useState('');

  const submit = async () => {
    setBusy(true);
    setErr('');
    setOk('');
    const res = await createGroup(name.trim(), description.trim());
    if (res.ok) {
      setOk('Created group "' + name.trim() + '"');
      setName('');
      setDescription('');
      onCreated();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  return (
    <Card title="Create group" subtitle="POST /admin/groups">
      <div className="row-form">
        <div className="field">
          <label>name</label>
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="engineering" />
        </div>
        <div className="field grow">
          <label>description</label>
          <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" />
        </div>
        <button className="btn" disabled={busy || !name.trim()} onClick={submit}>
          <Icon name="plus" size={15} />
          {busy ? 'Creating…' : 'Create'}
        </button>
      </div>
      {err && <div className="alert error">{err}</div>}
      {ok && <div className="alert ok">{ok}</div>}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

function UsersPanel() {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [busyId, setBusyId] = useState('');
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const [u, r, p] = await Promise.all([fetchUsers(), fetchRoles(), fetchPolicies()]);
    if (u.ok) setUsers(u.users);
    else setErr(loadError(u.status, 'users'));
    if (r.ok) setRoles(r.roles);
    if (p.ok) setPolicies(p.policies);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const assign = async (userId: string) => {
    const role = selected[userId];
    if (!role) return;
    setBusyId(userId);
    setErr('');
    const res = await assignRole(userId, role);
    if (res.ok) {
      setSelected((s) => ({ ...s, [userId]: '' }));
      await load();
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusyId('');
  };

  const unassign = async (userId: string, role: string) => {
    setBusyId(userId);
    setErr('');
    const res = await revokeRole(userId, role);
    if (res.ok) await load();
    else setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    setBusyId('');
  };

  const setBoundary = async (userId: string, policy: string) => {
    setBusyId(userId);
    setErr('');
    const res = await setUserBoundary(userId, policy);
    if (!res.ok) setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    setBusyId('');
  };

  return (
    <>
      <Card
        title="Users"
        subtitle="GET /admin/users — every user and the roles granted to them. Access is deny-by-default: a user with no policies or roles can do nothing."
        right={<RefreshButton loading={loading} onClick={load} />}
      >
        {err && <div className="alert error">{err}</div>}

        {users.length === 0 ? (
          <EmptyState icon={<Icon name="users" size={20} />} text="No users yet. Provision one in Keys & Vault." />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>User</th>
                <th>Email</th>
                <th>Status</th>
                <th>Roles</th>
                <th>Assign role</th>
                <th>Permission boundary</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => {
                const available = roles.filter((r) => !u.roles.includes(r.name));
                return (
                  <tr key={u.id}>
                    <td>
                      <span className="key-user">{u.username}</span>
                    </td>
                    <td>
                      <span className="hint">{u.email || '—'}</span>
                    </td>
                    <td>
                      <span className={'badge ' + (u.status === 'active' ? 'pass' : 'neutral')}>{u.status}</span>
                    </td>
                    <td>
                      <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
                        {u.roles.length === 0 && <span className="badge warn">no role</span>}
                        {u.roles.map((r) => (
                          <span className="badge accent" key={r}>
                            {r}
                            <button
                              title={'Revoke ' + r}
                              disabled={busyId === u.id}
                              onClick={() => unassign(u.id, r)}
                              style={{ border: 'none', background: 'transparent', color: 'inherit', cursor: 'pointer', padding: '0 0 0 5px', font: 'inherit', lineHeight: 1 }}
                            >
                              ×
                            </button>
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>
                      <div className="row-form">
                        <select
                          className="select"
                          value={selected[u.id] ?? ''}
                          onChange={(e) => setSelected((s) => ({ ...s, [u.id]: e.target.value }))}
                        >
                          <option value="">select role…</option>
                          {available.map((r) => (
                            <option key={r.name} value={r.name}>
                              {r.name}
                            </option>
                          ))}
                        </select>
                        <button className="btn ghost sm" disabled={busyId === u.id || !selected[u.id]} onClick={() => assign(u.id)}>
                          Assign
                        </button>
                      </div>
                    </td>
                    <td>
                      <select
                        className="select"
                        value=""
                        disabled={busyId === u.id}
                        onChange={(e) => setBoundary(u.id, e.target.value)}
                      >
                        <option value="">no boundary</option>
                        {policies.map((p) => (
                          <option key={p.name} value={p.name}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </Card>
    </>
  );
}
