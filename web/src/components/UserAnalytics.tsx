import { useEffect, useMemo, useState } from 'react';
import { fetchUserAnalytics, UserPromptRow, UserUsageRow } from '../api';
import { Alert, Card, EmptyState, fmtNum } from './ui';
import { Icon } from '../icons';

function fmtMoney(n: number): string {
  if (!n) return '$0';
  if (n < 0.001) return '$' + n.toExponential(2);
  return '$' + n.toFixed(4);
}

export default function UserAnalytics({ hours }: { hours: number }) {
  const [users, setUsers] = useState<UserUsageRow[]>([]);
  const [prompts, setPrompts] = useState<UserPromptRow[]>([]);
  const [user, setUser] = useState('');
  const [q, setQ] = useState('');
  const [debouncedQ, setDebouncedQ] = useState('');
  const [reload, setReload] = useState(0);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q.trim()), 300);
    return () => clearTimeout(t);
  }, [q]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setErr('');
      const res = await fetchUserAnalytics({ hours, user, q: debouncedQ });
      if (cancelled) return;
      if (res.ok) {
        setUsers(res.users);
        setPrompts(res.prompts);
      } else {
        setErr('HTTP ' + res.status + ' · user analytics unavailable');
      }
      setLoading(false);
    })();
    return () => { cancelled = true; };
  }, [hours, user, debouncedQ, reload]);

  const totalSpend = useMemo(() => users.reduce((s, u) => s + u.total_cost, 0), [users]);
  const totalRequests = useMemo(() => users.reduce((s, u) => s + u.request_count, 0), [users]);
  const totalTokens = useMemo(() => users.reduce((s, u) => s + u.total_tokens, 0), [users]);

  return (
    <Card
      title="Usage by user"
      subtitle="Attributed requests, tokens, estimated spend, and searchable prompts."
      right={
        <button className="btn ghost sm" onClick={() => setReload((n) => n + 1)} disabled={loading}>
          <Icon name="refresh" size={14} />
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      }
    >
      {err && <Alert tone="error">{err}</Alert>}

      <div className="ua-totals">
        <div className="ua-total">
          <span className="ua-total-label">Attributed spend</span>
          <span className="ua-total-value">{fmtMoney(totalSpend)}</span>
          <span className="ua-total-note">estimated, not billing data</span>
        </div>
        <div className="ua-total">
          <span className="ua-total-label">Requests</span>
          <span className="ua-total-value">{fmtNum(totalRequests)}</span>
        </div>
        <div className="ua-total">
          <span className="ua-total-label">Tokens</span>
          <span className="ua-total-value">{fmtNum(totalTokens)}</span>
        </div>
      </div>

      <h4 className="ua-h">Users</h4>
      {users.length === 0 ? (
        <EmptyState icon={<Icon name="activity" size={20} />} text="No attributed usage in this window." />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>User</th>
                <th>Requests</th>
                <th>Tokens</th>
                <th>Spend</th>
                <th>Avg latency</th>
                <th>Last seen</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr
                  key={u.user_id}
                  className={'ua-user-row' + (user === u.username ? ' selected' : '')}
                  onClick={() => setUser((cur) => (cur === u.username ? '' : u.username))}
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      setUser((cur) => (cur === u.username ? '' : u.username));
                    }
                  }}
                  title="Select user to filter prompts"
                >
                  <td><span className="ua-username">{u.username}</span></td>
                  <td>{fmtNum(u.request_count)}</td>
                  <td>{fmtNum(u.total_tokens)}</td>
                  <td><span className="ua-cost">{fmtMoney(u.total_cost)}</span></td>
                  <td>{Math.round(u.avg_latency_ms)} ms</td>
                  <td><span className="ua-dim">{u.last_seen}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="ua-search">
        <span className="ua-search-icon"><Icon name="search" size={15} /></span>
        <input
          className="input ua-search-input"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search captured prompts…"
          aria-label="Search captured prompts"
        />
        {user && (
          <button className="ua-filter" onClick={() => setUser('')} aria-label={`Clear user filter ${user}`}>
            user: {user} <Icon name="x" size={13} />
          </button>
        )}
      </div>

      {prompts.length === 0 ? (
        <EmptyState icon={<Icon name="message" size={20} />} text={q || user ? 'No prompts match the current filters.' : 'No prompts captured in this window.'} />
      ) : (
        <div className="ua-prompts">
          {prompts.map((p) => (
            <article className="ua-prompt" key={p.trace_id}>
              <div className="ua-prompt-head">
                <span className="ua-username">{p.username || 'Unknown user'}</span>
                <span className="ua-meta">{p.model || 'Unknown model'}</span>
                <span className="ua-meta">{p.created_at}</span>
                <span className="ua-meta">{fmtNum(p.total_tokens)} tok</span>
                <span className="ua-cost">{fmtMoney(p.estimated_cost)}</span>
              </div>
              <div className="ua-prompt-text">{p.prompt}</div>
            </article>
          ))}
        </div>
      )}
    </Card>
  );
}