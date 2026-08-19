import { useEffect, useState } from 'react';
import { StreamState } from '../types';
import { fetchAnalytics, fetchMetrics } from '../api';
import { Icon, IconName } from '../icons';
import { fmtNum } from './ui';

interface MetricDef {
  label: string;
  value: string;
  icon: IconName;
  tone?: string;
}

function num(v: unknown): number {
  const n = typeof v === 'number' ? v : parseFloat(String(v ?? 0));
  return Number.isFinite(n) ? n : 0;
}

const TELEMETRY = [
  { key: 'guardrail_passed_total', label: 'Passed', color: 'var(--green)' },
  { key: 'guardrail_blocks_total', label: 'Blocked', color: 'var(--red)' },
  { key: 'guardrail_redactions_total', label: 'Redacted', color: 'var(--violet)' },
  { key: 'guardrail_warns_total', label: 'Warned', color: 'var(--amber)' },
  { key: 'guardrail_errors_total', label: 'Errors', color: 'var(--red)' }
];

export default function Overview({ stream }: { stream: StreamState }) {
  const [chOnline, setChOnline] = useState<boolean | null>(null);
  const [metrics, setMetrics] = useState<Record<string, number>>({});

  useEffect(() => {
    let mounted = true;
    const probe = async () => {
      const res = await fetchAnalytics('traces_count', 1);
      if (mounted) setChOnline(res.ok);
    };
    probe();
    const t = setInterval(probe, 8000);
    return () => {
      mounted = false;
      clearInterval(t);
    };
  }, []);

  useEffect(() => {
    let mounted = true;
    const poll = async () => {
      const res = await fetchMetrics();
      if (mounted && res.ok) {
        const m: Record<string, number> = {};
        for (const [k, v] of Object.entries(res.body)) m[k] = num(v);
        setMetrics(m);
      }
    };
    poll();
    const t = setInterval(poll, 3000);
    return () => {
      mounted = false;
      clearInterval(t);
    };
  }, []);

  const ended = stream.requests.filter((r) => r.endedAt);
  const events = stream.requests.reduce((n, r) => n + r.events.length, 0);
  const totalTokens = stream.requests.reduce((n, r) => {
    const u = (r.responseBody as { usage?: { total_tokens?: number } } | undefined)?.usage;
    return n + (typeof u?.total_tokens === 'number' ? u.total_tokens : 0);
  }, 0);
  const avgLatency = ended.length
    ? ended.reduce((n, r) => n + ((r.endedAt ?? r.startedAt) - r.startedAt), 0) / ended.length
    : 0;
  const failed = stream.requests.filter((r) => r.blocked || r.events.some((e) => e.stage === 'error'));
  const successRate = ended.length ? ((ended.length - failed.length) / ended.length) * 100 : 0;

  const metricsList: MetricDef[] = [
    { label: 'Requests', value: fmtNum(stream.requests.length), icon: 'bolt', tone: 'accent' },
    { label: 'In flight', value: fmtNum(stream.active), icon: 'layers', tone: 'amber' },
    { label: 'Blocked', value: fmtNum(stream.blocked), icon: 'shield', tone: 'red' },
    { label: 'Events', value: fmtNum(events), icon: 'list', tone: 'violet' },
    { label: 'Tokens', value: fmtNum(totalTokens), icon: 'message', tone: 'green' },
    { label: 'Avg latency', value: fmtNum(avgLatency, 0) + ' ms', icon: 'activity' },
    { label: 'Success', value: fmtNum(successRate, 0) + '%', icon: 'check', tone: 'green' },
    { label: 'Guardrail evals', value: fmtNum(metrics.guardrail_evaluations_total ?? 0), icon: 'gauge', tone: 'accent' }
  ];

  const teleMax = Math.max(1, ...TELEMETRY.map((t) => metrics[t.key] ?? 0));

  return (
    <div className="stack">
      <div className="metric-grid">
        {metricsList.map((m) => (
          <div key={m.label} className={'metric' + (m.tone ? ' ' + m.tone : '')}>
            <span className="m-icon"><Icon name={m.icon} size={18} /></span>
            <div className="m-body">
              <span className="m-value">{m.value}</span>
              <span className="m-label">{m.label}</span>
            </div>
          </div>
        ))}
      </div>

      <div className="grid-2">
        <div className="card">
          <div className="card-head">
            <h3>Guardrail telemetry</h3>
            <span className="badge accent">/metrics</span>
          </div>
          <div className="card-body">
            <div className="telemetry">
              {TELEMETRY.map((t) => {
                const v = metrics[t.key] ?? 0;
                return (
                  <div className="tele-row" key={t.key}>
                    <span className="t-label">{t.label}</span>
                    <div className="t-track">
                      <div className="t-fill" style={{ width: (v / teleMax) * 100 + '%', background: t.color }} />
                    </div>
                    <span className="t-value">{fmtNum(v)}</span>
                  </div>
                );
              })}
              <div className="hint" style={{ marginTop: 6 }}>
                avg evaluation <b style={{ color: 'var(--ink-2)' }}>{fmtNum(metrics.guardrail_evaluation_duration_avg_ms ?? 0)} ms</b>
              </div>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="card-head">
            <h3>System status</h3>
            <span className="badge neutral">live</span>
          </div>
          <div className="card-body">
            <div className="status-grid" style={{ gridTemplateColumns: '1fr 1fr', gap: 10 }}>
              <div className="status-chip">
                <span className="sc-icon"><Icon name="bolt" size={18} /></span>
                <div className="sc-body">
                  <span className="sc-label">Gateway</span>
                  <span className="sc-value" style={{ color: stream.connected ? 'var(--green)' : 'var(--red)' }}>{stream.connected ? 'Live' : 'Offline'}</span>
                </div>
              </div>
              <div className="status-chip">
                <span className="sc-icon"><Icon name="message" size={18} /></span>
                <div className="sc-body">
                  <span className="sc-label">Provider</span>
                  <span className="sc-value">openai</span>
                </div>
              </div>
              <div className="status-chip">
                <span className="sc-icon"><Icon name="lock" size={18} /></span>
                <div className="sc-body">
                  <span className="sc-label">Secret store</span>
                  <span className="sc-value">vault</span>
                </div>
              </div>
              <div className="status-chip">
                <span className="sc-icon"><Icon name="chart" size={18} /></span>
                <div className="sc-body">
                  <span className="sc-label">ClickHouse</span>
                  <span className="sc-value" style={{ color: chOnline ? 'var(--green)' : chOnline === false ? 'var(--red)' : 'var(--ink-3)' }}>
                    {chOnline === null ? 'checking…' : chOnline ? 'connected' : 'unavailable'}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
