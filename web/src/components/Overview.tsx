import { useEffect, useState } from 'react';
import { StreamState } from '../types';
import { fetchAnalytics, fetchMetrics } from '../api';
import { Icon, IconName } from '../icons';
import { Badge, Card, Metric, fmtNum } from './ui';

interface MetricDef {
  label: string;
  value: string;
  icon: IconName;
  tone?: string;
}

interface SystemRow {
  label: string;
  desc: string;
  ok: boolean | null;
  live: string;
  down: string;
  icon: IconName;
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

  const completed = Math.max(ended.length - failed.length, 0);
  const successTone = successRate >= 99.5 ? 'green' : successRate >= 90 ? 'amber' : 'red';

  const systems: SystemRow[] = [
    { label: 'Gateway', desc: 'SSE event stream', ok: stream.connected, live: 'Live', down: 'Offline', icon: 'bolt' },
    { label: 'Provider', desc: 'chat completion', ok: true, live: 'openai', down: 'unreachable', icon: 'message' },
    { label: 'Secret store', desc: 'credentials', ok: true, live: 'vault', down: 'unreachable', icon: 'lock' },
    { label: 'ClickHouse', desc: 'traces & analytics', ok: chOnline, live: 'connected', down: 'unavailable', icon: 'chart' }
  ];

  const degraded = !stream.connected || systems.some((s) => s.ok === false);
  const allOperational = !degraded && systems.every((s) => s.ok !== null);
  const healthLabel = degraded ? 'Degraded' : allOperational ? 'Operational' : 'Checking…';
  const healthTone = degraded ? 'red' : allOperational ? 'green' : 'amber';
  const healthDot = degraded ? 'bad' : allOperational ? 'ok' : 'scan';

  const primary: MetricDef[] = [
    { label: 'Success rate', value: fmtNum(successRate, 1) + '%', icon: 'check', tone: successTone },
    { label: 'In flight', value: fmtNum(stream.active), icon: 'layers', tone: 'amber' },
    { label: 'Blocked', value: fmtNum(stream.blocked), icon: 'shield', tone: 'red' },
    { label: 'Avg latency', value: fmtNum(avgLatency, 0) + ' ms', icon: 'activity' }
  ];

  const secondary: MetricDef[] = [
    { label: 'Requests', value: fmtNum(stream.requests.length), icon: 'bolt', tone: 'accent' },
    { label: 'Completed', value: fmtNum(completed), icon: 'check', tone: 'green' },
    { label: 'Failed', value: fmtNum(failed.length), icon: 'warning', tone: 'red' },
    { label: 'Events', value: fmtNum(events), icon: 'list', tone: 'violet' },
    { label: 'Tokens', value: fmtNum(totalTokens), icon: 'message', tone: 'green' },
    { label: 'Guardrail evals', value: fmtNum(metrics.guardrail_evaluations_total ?? 0), icon: 'gauge', tone: 'accent' }
  ];

  const teleMax = Math.max(1, ...TELEMETRY.map((t) => metrics[t.key] ?? 0));

  return (
    <div className="stack">
      <section className={'ov-health tone-' + healthTone + ' reveal'}>
        <div className="ov-health-main">
          <span className={'ov-h-dot ' + healthDot} />
          <div>
            <span className="ov-h-kicker">Operational health</span>
            <span className={'ov-h-title ' + healthTone}>{healthLabel}</span>
          </div>
        </div>

        <div className="ov-h-stats">
          <div className="ov-h-stat">
            <span className="ov-h-lab">Success rate</span>
            <span className={'ov-h-val ' + successTone}>{fmtNum(successRate, 1)}%</span>
          </div>
          <div className="ov-h-stat">
            <span className="ov-h-lab">In flight</span>
            <span className="ov-h-val amber">{fmtNum(stream.active)}</span>
          </div>
          <div className="ov-h-stat">
            <span className="ov-h-lab">Blocked</span>
            <span className={'ov-h-val' + (stream.blocked ? ' red' : '')}>{fmtNum(stream.blocked)}</span>
          </div>
          <div className="ov-h-stat">
            <span className="ov-h-lab">Completed</span>
            <span className="ov-h-val green">{fmtNum(completed)}</span>
          </div>
        </div>

        <div className="ov-sys">
          {systems.map((s) => (
            <div className="ov-sys-chip" key={s.label} title={s.desc}>
              <span className="ov-sys-icon"><Icon name={s.icon} size={14} /></span>
              <span className={'ov-sys-dot ' + (s.ok ? 'ok' : s.ok === null ? 'scan' : 'bad')} />
              <span className="ov-sys-name">{s.label}</span>
              <span className={'ov-sys-val ' + (s.ok ? 'ok' : s.ok === null ? 'scan' : 'bad')}>
                {s.ok === null ? 'checking' : s.ok ? s.live : s.down}
              </span>
            </div>
          ))}
        </div>
      </section>

      <div className="metric-grid primary">
        {primary.map((m) => (
          <Metric key={m.label} label={m.label} value={m.value} icon={m.icon} tone={m.tone} emphasis="primary" />
        ))}
      </div>

      <div className="metric-strip">
        {secondary.map((m) => (
          <Metric key={m.label} label={m.label} value={m.value} icon={m.icon} tone={m.tone} emphasis="secondary" />
        ))}
      </div>

      <div className="grid-2">
        <Card
          variant="quiet"
          title="Guardrail telemetry"
          subtitle="Cumulative counters from /metrics"
          right={<Badge tone="accent">/metrics</Badge>}
        >
          <div className="telemetry">
            {TELEMETRY.map((t) => {
              const v = metrics[t.key] ?? 0;
              return (
                <div className="tele-row" key={t.key}>
                  <span className="t-label">
                    <i className="t-dot" style={{ background: t.color }} />
                    {t.label}
                  </span>
                  <div className="t-track">
                    <div className="t-fill" style={{ width: (v / teleMax) * 100 + '%', background: t.color }} />
                  </div>
                  <span className="t-value">{fmtNum(v)}</span>
                </div>
              );
            })}
            <div className="hint" style={{ marginTop: 6 }}>
              Average evaluation <b style={{ color: 'var(--ink-2)' }}>{fmtNum(metrics.guardrail_evaluation_duration_avg_ms ?? 0)} ms</b>
            </div>
          </div>
        </Card>

        <Card
          variant="quiet"
          title="Session summary"
          subtitle="Live breakdown of streamed requests"
          right={
            <Badge tone={healthTone === 'green' ? 'pass' : healthTone === 'red' ? 'error' : 'warn'} dot>
              {healthLabel}
            </Badge>
          }
        >
          <div className="ov-sess">
            {[
              { label: 'Completed', icon: 'check' as IconName, value: fmtNum(completed), pct: ended.length ? (completed / ended.length) * 100 : 0, color: 'var(--green)' },
              { label: 'Blocked / failed', icon: 'shield' as IconName, value: fmtNum(failed.length), pct: ended.length ? (failed.length / ended.length) * 100 : 0, color: 'var(--red)' },
              { label: 'In flight', icon: 'layers' as IconName, value: fmtNum(stream.active), pct: stream.requests.length ? (stream.active / stream.requests.length) * 100 : 0, color: 'var(--amber)' },
              { label: 'Events emitted', icon: 'list' as IconName, value: fmtNum(events), pct: 0, color: 'var(--accent)' }
            ].map((s) => (
              <div className="ov-sess-row" key={s.label}>
                <div className="ov-sess-head">
                  <span className="ov-sess-icon"><Icon name={s.icon} size={15} /></span>
                  <span className="ov-sess-label">{s.label}</span>
                  <span className="ov-sess-val">{s.value}</span>
                </div>
                <div className="ov-sess-track">
                  <div className="ov-sess-fill" style={{ width: s.pct + '%', background: s.color }} />
                </div>
                <span className="ov-sess-note">
                  {s.label === 'Events emitted' ? 'Across all streamed requests' : s.pct.toFixed(0) + '% of completed workload'}
                </span>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}