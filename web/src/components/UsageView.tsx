import { useEffect, useMemo, useState } from 'react';
import { fetchAnalytics, sendChat } from '../api';
import { StreamState } from '../types';
import { Icon } from '../icons';
import UserAnalytics from './UserAnalytics';
import { Card, DecisionBadge, EmptyState, Metric, fmtNum, shortId } from './ui';

const QUERY_KEYS = ['token_usage', 'cost', 'traces_count', 'latency', 'top_models', 'provider_usage'];

type AnalyticsRow = Record<string, string | number>;
type AnalyticsData = Record<string, AnalyticsRow[]>;

function num(v: string | number | undefined): number {
  const n = typeof v === 'string' ? parseFloat(v) : (v ?? 0);
  return Number.isFinite(n) ? n : 0;
}

function fmtMoney(n: number): string {
  if (!n) return '$0';
  if (n < 0.001) return '$' + n.toExponential(2);
  return '$' + n.toFixed(4);
}

function Bars({ items }: { items: { label: string; value: number }[] }) {
  const max = Math.max(...items.map((i) => i.value), 1);
  return (
    <div className="bars">
      {items.map((i) => (
        <div className="bar-row" key={i.label}>
          <span className="bar-label" title={i.label}>{i.label}</span>
          <div className="bar-track" aria-hidden="true">
            <div className="bar-fill" style={{ width: `${(i.value / max) * 100}%` }} />
          </div>
          <span className="bar-value">{fmtNum(i.value)}</span>
        </div>
      ))}
    </div>
  );
}

export default function UsageView({
  userKey,
  onUserKeyChange,
  stream
}: {
  userKey: string;
  onUserKeyChange: (v: string) => void;
  stream: StreamState;
}) {
  const [hours, setHours] = useState(24);
  const [data, setData] = useState<AnalyticsData>({});
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(0);
  const [testResult, setTestResult] = useState('');
  const [busy, setBusy] = useState(false);

  const load = async () => {
    setLoading(true);
    const results = await Promise.all(QUERY_KEYS.map(async (q) => ({ key: q, res: await fetchAnalytics(q, hours) })));
    const next: AnalyticsData = {};
    let failures = 0;
    for (const { key, res } of results) {
      if (res.ok) next[key] = res.rows;
      else failures++;
    }
    setData(next);
    setFailed(failures);
    setLoading(false);
  };

  useEffect(() => {
    load();
  }, [hours]);

  const row = (key: string) => data[key]?.[0] ?? {};
  const topModels = useMemo(() => (data.top_models ?? []).slice(0, 5).map((r) => ({ label: String(r.model ?? 'unknown'), value: num(r.request_count) })), [data]);
  const providers = useMemo(() => (data.provider_usage ?? []).slice(0, 5).map((r) => ({ label: String(r.provider ?? 'unknown'), value: num(r.request_count) })), [data]);
  const authed = stream.requests.filter((r) => r.authenticated);
  const unavailable = !loading && failed === QUERY_KEYS.length;
  const partial = !loading && failed > 0 && failed < QUERY_KEYS.length;

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

  return (
    <div className="stack">
      <Card
        title="Usage overview"
        subtitle="Token consumption and estimated request cost from ClickHouse. Costs are analytics estimates, not provider billing data."
        right={
          <div className="usage-toolbar">
            <label className="field-inline">
              <span>Window</span>
              <select className="select" value={hours} onChange={(e) => setHours(Number(e.target.value))}>
                <option value={24}>24 hours</option>
                <option value={168}>7 days</option>
                <option value={720}>30 days</option>
              </select>
            </label>
            <button className="btn ghost sm" onClick={load} disabled={loading}>
              <Icon name="refresh" size={14} />{loading ? 'Loading…' : 'Refresh'}
            </button>
          </div>
        }
      >
        {unavailable && <div className="alert error">Usage analytics are unavailable. Check the ClickHouse service and admin credentials.</div>}
        {partial && <div className="alert warn">Some usage datasets could not be loaded. Showing the metrics that are currently available.</div>}

        <div className="metric-grid">
          <Metric emphasis="primary" icon="bolt" tone="accent" label="Requests"
            value={fmtNum(num(row('traces_count').trace_count))} />
          <Metric emphasis="primary" icon="message" label="Total tokens"
            value={fmtNum(num(row('token_usage').total_tokens_sum))} />
          <Metric emphasis="primary" icon="chart" tone="green" label="Estimated cost"
            value={fmtMoney(num(row('cost').total_cost))} />
          <Metric emphasis="primary" icon="gauge" tone="green" label="Avg cost / request"
            value={fmtMoney(num(row('cost').avg_cost_per_request))} />
        </div>

        <div className="usage-detail-grid">
          <div className="usage-detail">
            <span className="usage-detail-label">Input tokens</span>
            <strong>{fmtNum(num(row('token_usage').total_input_tokens))}</strong>
            <span>{fmtNum(num(row('token_usage').avg_tokens_per_request))} avg tokens/request</span>
          </div>
          <div className="usage-detail">
            <span className="usage-detail-label">Output tokens</span>
            <strong>{fmtNum(num(row('token_usage').total_output_tokens))}</strong>
            <span>Estimated cost excludes unavailable provider pricing</span>
          </div>
          <div className="usage-detail">
            <span className="usage-detail-label">Max request cost</span>
            <strong>{fmtMoney(num(row('cost').max_cost_per_request))}</strong>
            <span>Highest estimated single request</span>
          </div>
          <div className="usage-detail">
            <span className="usage-detail-label">Average latency</span>
            <strong>{fmtNum(num(row('latency').avg_latency_ms))} ms</strong>
            <span>Across traced requests</span>
          </div>
        </div>
      </Card>

      <div className="grid-2">
        <Card title="Top models" subtitle="Request volume in the selected window.">
          {topModels.length ? <Bars items={topModels} /> : <EmptyState icon={<Icon name="chart" size={20} />} text={loading ? 'Loading usage…' : 'No model usage in this window.'} />}
        </Card>
        <Card title="Provider usage" subtitle="Request volume by upstream provider.">
          {providers.length ? <Bars items={providers} /> : <EmptyState icon={<Icon name="chart" size={20} />} text={loading ? 'Loading usage…' : 'No provider usage in this window.'} />}
        </Card>
      </div>

      <Card
        title="Verify a key"
        subtitle="Optionally verify a user's API key and confirm the identity resolved by the gateway."
      >
        <div className="row-form">
          <div className="field grow">
            <label htmlFor="usage-user-key">User API key</label>
            <input id="usage-user-key" className="input mono" type="password" value={userKey} onChange={(e) => onUserKeyChange(e.target.value)} placeholder="ap_live_…" />
          </div>
          <button className="btn" disabled={busy || !userKey} onClick={doTest}>
            <Icon name="key" size={15} />
            {busy ? 'Testing…' : 'Verify & test'}
          </button>
        </div>
        {testResult && <div className="alert info">{testResult}</div>}
      </Card>

      <Card title="Authenticated request activity" subtitle="Recent key-attributed requests observed on the live event stream.">
        {authed.length === 0 ? (
          <EmptyState icon={<Icon name="activity" size={20} />} text="No authenticated requests yet." />
        ) : (
          <div className="usage-list">
            {[...authed].reverse().slice(0, 20).map((r) => (
              <div className="usage-row" key={r.id}>
                <span className="mono usage-request-id">#{shortId(r.id)}</span>
                <span className="u-user">{r.authenticated?.username}<span className="u-id">{r.authenticated?.user_id}</span></span>
                <span className="u-prompt" title={r.userPrompt}>{r.userPrompt ?? ''}</span>
                <span className="u-time">{r.startedAt}</span>
                <span className="u-status">{r.blocked ? <DecisionBadge decision="block" /> : r.endedAt ? <DecisionBadge decision="pass" /> : <DecisionBadge decision="warn" />}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <UserAnalytics hours={hours} />
    </div>
  );
}
