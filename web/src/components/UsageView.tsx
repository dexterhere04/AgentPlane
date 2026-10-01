import { useState } from 'react';
import { StreamState } from '../types';
import { sendChat } from '../api';
import { Card, DecisionBadge, fmtTime, shortId } from './ui';
import { Icon } from '../icons';
import UserAnalytics from './UserAnalytics';

export default function UsageView({
  userKey,
  onUserKeyChange,
  stream
}: {
  userKey: string;
  onUserKeyChange: (v: string) => void;
  stream: StreamState;
}) {
  const [testResult, setTestResult] = useState('');
  const [busy, setBusy] = useState(false);

  const doTest = async () => {
    setBusy(true);
    setTestResult('');
    const res = await sendChat('Ping — confirm you are reachable.', { userKey, model: 'gpt-4o' });
    if (res.ok) {
      const b = res.body as { choices?: { message?: { content?: string } }[] };
      setTestResult('HTTP ' + res.status + ' OK · ' + (b?.choices?.[0]?.message?.content ?? ''));
    } else {
      setTestResult('HTTP ' + res.status + ' · ' + JSON.stringify(res.body));
    }
    setBusy(false);
  };

  const authed = stream.requests.filter((r) => r.authenticated);

  return (
    <div className="stack">
      <Card
        title="Verify a key"
        subtitle="Paste a user's API key and confirm the identity it resolves to on the gateway."
      >
        <div className="row-form">
          <div className="field grow">
            <label>user api key</label>
            <input className="input mono" type="password" value={userKey} onChange={(e) => onUserKeyChange(e.target.value)} placeholder="ap_live_..." />
          </div>
          <button className="btn" disabled={busy || !userKey} onClick={doTest}>
            <Icon name="key" size={15} />
            {busy ? 'Testing…' : 'Verify & test'}
          </button>
        </div>

        {testResult && <div className="alert info">{testResult}</div>}

        <p className="hint">
          The gateway parses <code>ap_live_&lt;key_id&gt;_&lt;secret&gt;</code>, hashes the secret with the server pepper,
          compares in constant time, then resolves and marks the key used.
        </p>
      </Card>

      <Card
        title="Requests attributed to keys"
        subtitle="Authenticated requests observed on the SSE stream this session."
      >
        {authed.length === 0 ? (
          <div className="empty">
            <span className="empty-icon"><Icon name="activity" size={20} /></span>
            <span>No authenticated requests yet — send one from Chat or verify above.</span>
          </div>
        ) : (
          <div className="usage-list">
            {[...authed].reverse().map((r) => (
              <div className="usage-row" key={r.id}>
                <span className="mono" style={{ color: 'var(--ink-4)', fontSize: 12 }}>#{shortId(r.id)}</span>
                <span className="u-user">
                  {r.authenticated?.username}
                  <span className="u-id">{r.authenticated?.user_id}</span>
                </span>
                <span className="u-prompt" title={r.userPrompt}>{r.userPrompt ?? ''}</span>
                <span className="u-time">{fmtTime(r.startedAt)}</span>
                <span className="u-status">
                  {r.blocked ? <DecisionBadge decision="block" /> : r.endedAt ? <DecisionBadge decision="pass" /> : <DecisionBadge decision="warn" />}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <UserAnalytics />
    </div>
  );
}
