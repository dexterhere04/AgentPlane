import { useEffect, useState } from 'react';
import {
  fetchObservabilityBreakdowns,
  fetchObservabilityGuardrails,
  fetchObservabilityOverview,
  GuardrailBreakdown,
  ObservabilityBreakdown,
  ObservabilityOverview
} from '../api';
import { Card, Stat, EmptyState, fmtNum } from './ui';
import { Icon } from '../icons';

function Bars({ items }: { items: { label: string; value: number }[] }) {
  const max = Math.max(...items.map((i) => i.value), 1);
  return (
    <div className="bars">
      {items.map((i) => (
        <div className="bar-row" key={i.label}>
          <span className="bar-label">{i.label}</span>
          <div className="bar-track">
            <div className="bar-fill" style={{ width: (i.value / max) * 100 + '%' }} />
          </div>
          <span className="bar-value">{fmtNum(i.value)}</span>
        </div>
      ))}
    </div>
  );
}

export default function ObservabilityView() {
  const [overview, setOverview] = useState<ObservabilityOverview | null>(null);
  const [breakdowns, setBreakdowns] = useState<Record<string, ObservabilityBreakdown[]>>({});
  const [guardrails, setGuardrails] = useState<GuardrailBreakdown[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(0);

  const load = async () => {
    const [o, b, g] = await Promise.all([
      fetchObservabilityOverview('24h'),
      fetchObservabilityBreakdowns('24h'),
      fetchObservabilityGuardrails('24h')
    ]);
    let failures = 0;
    if (o.ok && o.overview) setOverview(o.overview);
    else failures++;
    if (b.ok) setBreakdowns(b.breakdowns);
    else failures++;
    if (g.ok) setGuardrails(g.guardrails);
    else failures++;
    setFailed(failures);
    setLoading(false);
  };

  useEffect(() => {
    load();
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, []);

  const topModels = (breakdowns.model ?? []).map((r) => ({ label: r.key, value: r.requests }));
  const providers = (breakdowns.provider ?? []).map((r) => ({ label: r.key, value: r.requests }));
  const unavailable = !loading && failed === 3;

  const tokens = overview?.tokens ?? { input: 0, output: 0, total: 0 };
  const latency = overview?.latency_ms ?? { p50_ms: 0, p95_ms: 0, p99_ms: 0 };
  const avgTokensPerReq = overview && overview.requests > 0 ? tokens.total / overview.requests : 0;

  return (
    <div className="stack">
      {unavailable && (
        <div className="alert error">
          ClickHouse analytics unreachable — is the <code>clickhouse</code> service up and the admin token correct?
        </div>
      )}

      <div className="stat-grid">
        <Stat label="Traces (24h)" value={fmtNum(overview?.requests ?? 0)} tone="accent" />
        <Stat label="Avg latency" value={fmtNum(latency.p50_ms) + ' ms'} />
        <Stat label="Total tokens" value={fmtNum(tokens.total)} />
        <Stat label="Est. cost" value={'$' + fmtNum(overview?.estimated_cost ?? 0, 4)} tone="green" small />
      </div>

      <div className="grid-2">
        <Card title="Top models" subtitle="by request count (24h)">
          {topModels.length ? (
            <Bars items={topModels} />
          ) : (
            <EmptyState icon={<Icon name="chart" size={20} />} text="no data" />
          )}
        </Card>
        <Card title="Provider usage" subtitle="by request count (24h)">
          {providers.length ? (
            <Bars items={providers} />
          ) : (
            <EmptyState icon={<Icon name="chart" size={20} />} text="no data" />
          )}
        </Card>
      </div>

      <Card
        title="ClickHouse observability"
        subtitle="Traces, token usage, cost, guardrail actions and error rate — 24h window, auto-refreshes every 5s."
        right={<button className="btn ghost sm" onClick={load}><Icon name="refresh" size={14} />{loading ? 'Loading…' : 'Refresh'}</button>}
      >
        <div className="grid-2">
          <div>
            <div className="subhead">Token &amp; cost detail</div>
            <div className="kv">
              <div className="kv-row"><span className="kv-key">input tokens</span><span className="kv-val">{fmtNum(tokens.input)}</span></div>
              <div className="kv-row"><span className="kv-key">output tokens</span><span className="kv-val">{fmtNum(tokens.output)}</span></div>
              <div className="kv-row"><span className="kv-key">avg tokens / req</span><span className="kv-val">{fmtNum(avgTokensPerReq)}</span></div>
              <div className="kv-row"><span className="kv-key">latency p50 / p95 / p99</span><span className="kv-val">{fmtNum(latency.p50_ms)} / {fmtNum(latency.p95_ms)} / {fmtNum(latency.p99_ms)} ms</span></div>
              <div className="kv-row"><span className="kv-key">error rate</span><span className="kv-val">{fmtNum((overview?.error_rate ?? 0) * 100, 2)}%</span></div>
            </div>
          </div>
          <div>
            <div className="subhead">Request outcomes</div>
            <div className="kv">
              <div className="kv-row"><span className="kv-key">successes</span><span className="kv-val">{fmtNum(overview?.successes ?? 0)}</span></div>
              <div className="kv-row"><span className="kv-key">errors</span><span className="kv-val">{fmtNum(overview?.errors ?? 0)}</span></div>
            </div>
          </div>
        </div>

        <div className="subhead">Guardrail actions (ClickHouse)</div>
        {guardrails.length ? (
          <table className="table">
            <thead>
              <tr>
                <th>guardrail</th>
                <th>phase</th>
                <th>action</th>
                <th>events</th>
              </tr>
            </thead>
            <tbody>
              {guardrails.map((r, i) => (
                <tr key={i}>
                  <td className="mono">{r.name}</td>
                  <td>{r.phase}</td>
                  <td><span className="badge" style={{ background: 'var(--surface-3)', color: 'var(--ink-2)' }}>{r.action}</span></td>
                  <td>{fmtNum(r.count)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <EmptyState icon={<Icon name="shield" size={20} />} text="No guardrail events persisted yet — run the Guardrails section." />
        )}
      </Card>
    </div>
  );
}
