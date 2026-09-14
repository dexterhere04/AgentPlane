import { useCallback, useEffect, useState } from 'react';
import {
  AdminUser,
  Role,
  addRolePermission,
  assignRole,
  createRole,
  fetchRoles,
  fetchUsers,
  removeRolePermission,
  revokeRole
} from '../api';
import { Card, EmptyState, Tabs } from './ui';
import { Icon } from '../icons';

export default function AccessControlView() {
  const [tab, setTab] = useState('roles');

  return (
    <div className="stack">
      <Tabs
        items={[
          { id: 'roles', label: 'Roles', icon: <Icon name="shield" size={14} /> },
          { id: 'users', label: 'Users', icon: <Icon name="users" size={14} /> }
        ]}
        active={tab}
        onChange={setTab}
      />

      {tab === 'roles' ? <RolesPanel /> : <UsersPanel />}
    </div>
  );
}

function RolesPanel() {
  const [roles, setRoles] = useState<Role[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [permissions, setPermissions] = useState('');
  const [creating, setCreating] = useState(false);
  const [createErr, setCreateErr] = useState('');
  const [createOk, setCreateOk] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const res = await fetchRoles();
    if (res.ok) setRoles(res.roles);
    else setErr('HTTP ' + res.status + ' · could not load roles');
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const doCreate = async () => {
    setCreating(true);
    setCreateErr('');
    setCreateOk('');
    const perms = permissions
      .split(',')
      .map((p) => p.trim())
      .filter(Boolean);
    const res = await createRole(name.trim(), description.trim(), perms);
    if (res.ok) {
      setCreateOk('Created role "' + name.trim() + '"');
      setName('');
      setDescription('');
      setPermissions('');
      await load();
    } else {
      setCreateErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setCreating(false);
  };

  return (
    <>
      <Card
        title="Roles"
        subtitle="GET /admin/roles — a role is a named set of permissions. Assign roles to users in the Users tab."
        right={
          <button className="btn ghost sm" onClick={load} disabled={loading}>
            <Icon name="refresh" size={14} />
            {loading ? 'Loading…' : 'Refresh'}
          </button>
        }
      >
        {err && <div className="alert error">{err}</div>}

        {roles.length === 0 ? (
          <EmptyState icon={<Icon name="shield" size={20} />} text="No roles defined yet." />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Role</th>
                <th>Permissions</th>
              </tr>
            </thead>
            <tbody>
              {roles.map((role) => (
                <tr key={role.name}>
                  <td>
                    <span className="key-name">{role.name}</span>
                    {role.description && <div className="hint" style={{ marginTop: 2 }}>{role.description}</div>}
                  </td>
                  <td>
                    <PermissionEditor role={role} onChanged={load} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card title="Create role" subtitle="POST /admin/roles — permissions are comma-separated (e.g. chat:invoke, model:gpt-4o).">
        <div className="row-form">
          <div className="field">
            <label>name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="gpt4o-only" />
          </div>
          <div className="field grow">
            <label>description</label>
            <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="optional" />
          </div>
          <div className="field grow">
            <label>permissions</label>
            <input
              className="input mono"
              value={permissions}
              onChange={(e) => setPermissions(e.target.value)}
              placeholder="chat:invoke, model:gpt-4o"
            />
          </div>
          <button className="btn" disabled={creating || !name.trim()} onClick={doCreate}>
            <Icon name="plus" size={15} />
            {creating ? 'Creating…' : 'Create'}
          </button>
        </div>
        {createErr && <div className="alert error">{createErr}</div>}
        {createOk && <div className="alert ok">{createOk}</div>}
      </Card>
    </>
  );
}

function PermissionEditor({ role, onChanged }: { role: Role; onChanged: () => void }) {
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  const add = async () => {
    const permission = value.trim();
    if (!permission) return;
    setBusy(true);
    setErr('');
    const res = await addRolePermission(role.name, permission);
    if (res.ok) {
      setValue('');
      onChanged();
    } else {
      setErr('HTTP ' + res.status);
    }
    setBusy(false);
  };

  const remove = async (permission: string) => {
    setBusy(true);
    setErr('');
    const res = await removeRolePermission(role.name, permission);
    if (res.ok) onChanged();
    else setErr('HTTP ' + res.status);
    setBusy(false);
  };

  return (
    <div>
      <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
        {role.permissions.length === 0 && <span className="hint">No permissions — this role grants nothing.</span>}
        {role.permissions.map((p) => (
          <span className="badge accent" key={p}>
            {p}
            <button
              title={'Remove ' + p}
              disabled={busy}
              onClick={() => remove(p)}
              style={{
                border: 'none',
                background: 'transparent',
                color: 'inherit',
                cursor: 'pointer',
                padding: '0 0 0 5px',
                font: 'inherit',
                lineHeight: 1
              }}
            >
              ×
            </button>
          </span>
        ))}
      </div>
      <div className="row-form" style={{ marginTop: 8 }}>
        <div className="field grow">
          <input
            className="input mono"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && add()}
            placeholder="model:gpt-4o-mini"
          />
        </div>
        <button className="btn ghost sm" disabled={busy || !value.trim()} onClick={add}>
          <Icon name="plus" size={14} />
          Add
        </button>
      </div>
      {err && <div className="alert error">{err}</div>}
    </div>
  );
}

function UsersPanel() {
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [busyId, setBusyId] = useState('');
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const [u, r] = await Promise.all([fetchUsers(), fetchRoles()]);
    if (u.ok) setUsers(u.users);
    else setErr('HTTP ' + u.status + ' · could not load users');
    if (r.ok) setRoles(r.roles);
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

  return (
    <Card
      title="Users"
      subtitle="GET /admin/users — every user and the roles assigned to them. Access is deny-by-default: a user with no roles can do nothing."
      right={
        <button className="btn ghost sm" onClick={load} disabled={loading}>
          <Icon name="refresh" size={14} />
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      }
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
            </tr>
          </thead>
          <tbody>
            {users.map((u) => {
              const available = roles.filter((r) => !u.roles.includes(r.name));
              return (
                <tr key={u.id}>
                  <td><span className="key-user">{u.username}</span></td>
                  <td><span className="hint">{u.email || '—'}</span></td>
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
                            style={{
                              border: 'none',
                              background: 'transparent',
                              color: 'inherit',
                              cursor: 'pointer',
                              padding: '0 0 0 5px',
                              font: 'inherit',
                              lineHeight: 1
                            }}
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
                          <option key={r.name} value={r.name}>{r.name}</option>
                        ))}
                      </select>
                      <button
                        className="btn ghost sm"
                        disabled={busyId === u.id || !selected[u.id]}
                        onClick={() => assign(u.id)}
                      >
                        Assign
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Card>
  );
}
