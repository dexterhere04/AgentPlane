import { useState } from 'react';
import { storeProviderKey } from '../api';
import { Card } from './ui';
import { Icon } from '../icons';
import ApiKeysView from './ApiKeysView';
import ProvisionedKeysView from './ProvisionedKeysView';

export default function VaultView() {
  const [provider, setProvider] = useState('openai');
  const [apiKey, setApiKey] = useState('sk-demo-provider-key-1234567890');
  const [result, setResult] = useState('');
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);

  const doStore = async () => {
    setBusy(true);
    setErr('');
    setResult('');
    const res = await storeProviderKey(provider, apiKey);
    if (res.ok) {
      setResult('Stored ' + provider + ' key under ' + JSON.stringify(res.body));
    } else {
      setErr('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  return (
    <div className="stack">
      <Card
        title="Provider secrets"
        subtitle="POST /admin/secrets/provider writes the key to the configured secret store (HashiCorp Vault)."
        right={<span className="badge accent"><Icon name="lock" size={12} /> vault</span>}
      >
        <div className="row-form">
          <div className="field">
            <label>provider</label>
            <select className="select" value={provider} onChange={(e) => setProvider(e.target.value)}>
              <option value="openai">openai</option>
            </select>
          </div>
          <div className="field grow">
            <label>api key</label>
            <input className="input mono" value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
          </div>
          <button className="btn" disabled={busy} onClick={doStore}>
            <Icon name="lock" size={15} />
            {busy ? 'Storing…' : 'Store in Vault'}
          </button>
        </div>

        {err && <div className="alert error">{err}</div>}
        {result && <div className="alert ok">{result}</div>}

        <p className="hint">
          The gateway runs with <code>SECRET_STORE=vault</code> and a KV-v2 mount at <code>agentplane/</code>. Writing a key
          updates <code>agentplane/OPENAI_API_KEY</code>; the proxy re-reads it through the secret store on every forward.
        </p>
      </Card>

      <ApiKeysView />
      <ProvisionedKeysView />
    </div>
  );
}
