import { useEffect, useState } from 'react';
import { AnalyticsRow, fetchAnalytics } from '../api';
import { Icon } from '../icons';
import { Alert, Badge, Card, Metric, EmptyState, fmtNum } from './ui';

type Datasets = Record<string, AnalyticsRow[]>;

const QUERIES = ['traces_count', 'latency', 'token_usage', 'cost', 'top_models', 'provider_usage', 'guardrail_events', 'error_rate'];

function num(v: string | number | undefined): number {
  const n = typeof v === 'string' ? parseFloat(v) : (v ?? 0);
  return Number.isFinite(n) ? n : 0;
}

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
  const [data, setData] = useState<Datasets>({});
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(0);

  const load = async () => {
    const results = await Promise.all(QUERIES.map(async (q) => ({ key: q, res: await fetchAnalytics(q, 24) })));
    const next: Datasets = {};
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
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, []);

  const row = (key: string) => data[key]?.[0] ?? {};
  const topModels = (data.top_models ?? []).map((r) => ({ label: String(r.model ?? ''), value: num(r.request_count) }));
  const providers = (data.provider_usage ?? []).map((r) => ({ label: String(r.provider ?? ''), value: num(r.request_count) }));
  const unavailable = !loading && failed === QUERIES.length;

  return (
    <div className="stack">
      {unavailable && (
        <Alert tone="error">
          ClickHouse analytics unreachable — is the <code>clickhouse</code> service up and the admin token correct?
        </Alert>
      )}

      <div className="metric-grid">
        <Metric emphasis="primary" icon="activity" tone="accent"
          label="Traces (24h)" value={fmtNum(num(row('traces_count').trace_count))} />
        <Metric emphasis="primary" icon="gauge" label="Avg latency"
          value={fmtNum(num(row('latency').avg_latency_ms)) + ' ms'} />
        <Metric emphasis="primary" icon="message" label="Total tokens"
          value={fmtNum(num(row('token_usage').total_tokens_sum))} />
        <Metric emphasis="primary" icon="chart" tone="green" label="Est. cost"
          value={'$' + fmtNum(num(row('cost').total_cost), 4)} />
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
              <div className="kv-row"><span className="kv-key">input tokens</span><span className="kv-val">{fmtNum(num(row('token_usage').total_input_tokens))}</span></div>
              <div className="kv-row"><span className="kv-key">output tokens</span><span className="kv-val">{fmtNum(num(row('token_usage').total_output_tokens))}</span></div>
              <div className="kv-row"><span className="kv-key">avg tokens / req</span><span className="kv-val">{fmtNum(num(row('token_usage').avg_tokens_per_request))}</span></div>
              <div className="kv-row"><span className="kv-key">latency min / max</span><span className="kv-val">{fmtNum(num(row('latency').min_latency_ms))} / {fmtNum(num(row('latency').max_latency_ms))} ms</span></div>
              <div className="kv-row"><span className="kv-key">max cost / req</span><span className="kv-val">${fmtNum(num(row('cost').max_cost_per_request), 4)}</span></div>
            </div>
          </div>
          <div>
            <div className="subhead">Error rate by status</div>
            {(data.error_rate ?? []).length ? (
              <Bars items={(data.error_rate ?? []).map((r) => ({ label: String(r.status ?? ''), value: num(r.count) }))} />
            ) : (
              <EmptyState icon={<Icon name="check" size={20} />} text="no errors" />
            )}
          </div>
        </div>

        <div className="subhead">Guardrail actions (ClickHouse)</div>
        {(data.guardrail_events ?? []).length ? (
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
              {(data.guardrail_events ?? []).map((r, i) => (
                <tr key={i}>
                  <td className="mono">{String(r.guardrail_name)}</td>
                  <td>{String(r.phase)}</td>
                  <td><Badge tone="neutral">{String(r.action)}</Badge></td>
                  <td>{fmtNum(num(r.event_count))}</td>
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