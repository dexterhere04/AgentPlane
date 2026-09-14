import { useEffect, useState } from 'react';
import { provisionUser, revokeKey, fetchRoles, assignRole, Role } from '../api';
import { Card } from './ui';
import { Icon } from '../icons';

interface ProvisionResult {
  user_id?: string;
  key_id?: string;
  api_key?: string;
}

export default function ApiKeysView() {
  const [username, setUsername] = useState('demo-user');
  const [keyName, setKeyName] = useState('demo key');
  const [provisioned, setProvisioned] = useState<ProvisionResult | null>(null);
  const [provisionErr, setProvisionErr] = useState('');
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  const [roles, setRoles] = useState<Role[]>([]);
  const [role, setRole] = useState('');
  const [roleMsg, setRoleMsg] = useState('');
  const [roleErr, setRoleErr] = useState('');

  const [revokeId, setRevokeId] = useState('');
  const [revokeResult, setRevokeResult] = useState('');
  const [revokeErr, setRevokeErr] = useState('');

  useEffect(() => {
    fetchRoles().then((res) => {
      if (res.ok) setRoles(res.roles);
    });
  }, []);

  const doProvision = async () => {
    setBusy(true);
    setProvisionErr('');
    setProvisioned(null);
    setRoleMsg('');
    setRoleErr('');
    const res = await provisionUser(username, keyName);
    if (res.ok && typeof res.body === 'object') {
      const body = res.body as ProvisionResult;
      setProvisioned(body);

      // Two-step: provision the user + key, then assign the selected role.
      // Access is deny-by-default, so a user with no role cannot use the
      // gateway until one is assigned.
      if (role && body.user_id) {
        const assigned = await assignRole(body.user_id, role);
        if (assigned.ok) setRoleMsg('Assigned role "' + role + '" to ' + username);
        else
          setRoleErr(
            'User and key created, but role assignment failed: HTTP ' +
              assigned.status +
              ' · ' +
              JSON.stringify(assigned.body)
          );
      }
    } else {
      const body = res.body as { error?: string } | string;
      const msg = typeof body === 'object' && body?.error ? body.error : String(body);
      setProvisionErr('HTTP ' + res.status + ' · ' + msg);
    }
    setBusy(false);
  };

  const doRevoke = async () => {
    setRevokeErr('');
    setRevokeResult('');
    const res = await revokeKey(revokeId);
    if (res.ok) setRevokeResult('Revoked ' + revokeId);
    else setRevokeErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
  };

  const doCopy = async () => {
    if (!provisioned?.api_key) return;
    await navigator.clipboard.writeText(provisioned.api_key);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Card
      title="User API keys"
      subtitle="POST /provision/user and POST /admin/api-keys/revoke (admin-gated). The full key is shown once; only its SHA-256 hash is stored."
    >
      <div className="grid-2">
        <div className="form">
          <h4 style={{ margin: 0, fontSize: 13, fontWeight: 700, color: 'var(--ink)' }}>Provision a user + mint a key</h4>
          <div className="field">
            <label>username</label>
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} />
          </div>
          <div className="field">
            <label>key name</label>
            <input className="input" value={keyName} onChange={(e) => setKeyName(e.target.value)} />
          </div>
          <div className="field">
            <label>role (optional)</label>
            <select className="select" value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="">no role — no access</option>
              {roles.map((r) => (
                <option key={r.name} value={r.name}>{r.name}</option>
              ))}
            </select>
          </div>
          <div>
            <button className="btn" disabled={busy} onClick={doProvision}>
              <Icon name="plus" size={15} />
              {busy ? 'Provisioning…' : 'Provision user + key'}
            </button>
          </div>

          {provisionErr && <div className="alert error">{provisionErr}</div>}
          {roleErr && <div className="alert error">{roleErr}</div>}
          {roleMsg && <div className="alert ok">{roleMsg}</div>}
          {provisioned && (
            <div className="result">
              <div className="kv-row" style={{ padding: 0 }}>
                <span className="kv-key">user_id</span>
                <span className="kv-val">{provisioned.user_id}</span>
              </div>
              <div className="kv-row" style={{ padding: 0 }}>
                <span className="kv-key">key_id</span>
                <span className="kv-val">{provisioned.key_id}</span>
              </div>
              <div className="field">
                <label>full key (shown once)</label>
                <textarea className="input mono" readOnly value={provisioned.api_key ?? ''} rows={2} />
              </div>
              <div>
                <button className="btn ghost sm" onClick={doCopy}>
                  <Icon name={copied ? 'check' : 'copy'} size={14} />
                  {copied ? 'Copied' : 'Copy key'}
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="form">
          <h4 style={{ margin: 0, fontSize: 13, fontWeight: 700, color: 'var(--ink)' }}>Revoke a key</h4>
          <div className="field">
            <label>key_id</label>
            <input className="input mono" value={revokeId} onChange={(e) => setRevokeId(e.target.value)} placeholder="key_id (public portion)" />
          </div>
          <div>
            <button className="btn danger" onClick={doRevoke}>
              <Icon name="x" size={15} />
              Revoke
            </button>
          </div>
          {revokeErr && <div className="alert error">{revokeErr}</div>}
          {revokeResult && <div className="alert ok">{revokeResult}</div>}
        </div>
      </div>
    </Card>
  );
}
