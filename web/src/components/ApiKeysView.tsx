import { useState } from 'react';
import { provisionUser, revokeKey } from '../api';
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

  const [revokeId, setRevokeId] = useState('');
  const [revokeResult, setRevokeResult] = useState('');
  const [revokeErr, setRevokeErr] = useState('');

  const doProvision = async () => {
    setBusy(true);
    setProvisionErr('');
    setProvisioned(null);
    const res = await provisionUser(username, keyName);
    if (res.ok && typeof res.body === 'object') {
      setProvisioned(res.body as ProvisionResult);
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
          <div>
            <button className="btn" disabled={busy} onClick={doProvision}>
              <Icon name="plus" size={15} />
              {busy ? 'Provisioning…' : 'Provision user + key'}
            </button>
          </div>

          {provisionErr && <div className="alert error">{provisionErr}</div>}
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
