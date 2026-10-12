import { useEffect, useState } from 'react';
import { StreamState } from '../types';
import {
  fetchAnalytics,
  fetchMetrics,
  fetchRoutingErrorSummary,
  fetchRoutingFailovers,
  fetchRoutingMetrics,
  fetchRoutingProviderTraffic,
  RoutingErrorSummary,
  RoutingFailoverSummary,
  RoutingMetrics,
  RoutingProviderTraffic
} from '../api';
import { Icon, IconName } from '../icons';
import { Badge, Card, EmptyState, Metric, Stat, fmtNum } from './ui';
import SetupChecklist from './SetupChecklist';

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

const SERVICES: { id: string; label: string; desc: string; icon: IconName }[] = [
  { id: 'chat', label: 'Playground', desc: 'Send a test prompt through the gateway', icon: 'play' },
  { id: 'pipeline', label: 'Pipeline', desc: 'Watch every stage of a request live', icon: 'route' },
  { id: 'routing', label: 'Routing', desc: 'Configure routing rules and provider selection', icon: 'route' },
  { id: 'guardrails', label: 'Guardrails', desc: 'Test PII, secret and injection checks', icon: 'shield' },
  { id: 'events', label: 'Event Log', desc: 'Raw SSE events for every request', icon: 'list' },
  { id: 'keys', label: 'Keys & Vault', desc: 'Provider secrets and user API keys', icon: 'key' },
  { id: 'usage', label: 'Usage', desc: 'Spend and tokens attributed to keys', icon: 'activity' },
  { id: 'observability', label: 'Analytics', desc: 'Traces, cost and models from ClickHouse', icon: 'chart' },
  { id: 'access', label: 'Access Control', desc: 'Roles and permissions for your users', icon: 'users' }
];

function ServiceGrid({ onNavigate }: { onNavigate: (tab: string) => void }) {
  return (
    <div className="svc-grid">
      {SERVICES.map((s) => (
        <button key={s.id} className="svc-card" onClick={() => onNavigate(s.id)}>
          <span className="svc-icon"><Icon name={s.icon} size={17} /></span>
          <span className="svc-body">
            <span className="svc-title">
              {s.label}
              <Icon name="chevron" size={13} />
            </span>
            <span className="svc-desc">{s.desc}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

export default function Overview({
  stream,
  adminToken,
  onNavigate,
  onOpenSettings
}: {
  stream: StreamState;
  adminToken: string;
  onNavigate: (tab: string) => void;
  onOpenSettings: () => void;
}) {
  const [chOnline, setChOnline] = useState<boolean | null>(null);
  const [metrics, setMetrics] = useState<Record<string, number>>({});
  const [routing, setRouting] = useState<RoutingMetrics | null>(null);
  const [providerTraffic, setProviderTraffic] = useState<RoutingProviderTraffic[]>([]);
  const [failover, setFailover] = useState<RoutingFailoverSummary | null>(null);
  const [errorSummary, setErrorSummary] = useState<RoutingErrorSummary[]>([]);

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

  // Live in-process routing counters from /metrics — DB-independent, so they
  // refresh quickly and work even when ClickHouse tracing is disabled.
  useEffect(() => {
    let mounted = true;
    const tick = async () => {
      const result = await fetchRoutingMetrics();
      if (mounted && result.ok) setRouting(result.metrics);
    };
    void tick();
    const t = setInterval(tick, 5000);
    return () => {
      mounted = false;
      clearInterval(t);
    };
  }, []);

  // Historical routing analytics (ClickHouse-backed): provider traffic and
  // failover / error summaries for the last 24 hours.
  useEffect(() => {
    let mounted = true;
    const load = async () => {
      const [traffic, fo, errs] = await Promise.all([
        fetchRoutingProviderTraffic(24),
        fetchRoutingFailovers(24),
        fetchRoutingErrorSummary(24)
      ]);
      if (!mounted) return;
      if (traffic.ok) setProviderTraffic(traffic.rows);
      if (fo.ok) setFailover(fo.rows[0] ?? null);
      if (errs.ok) setErrorSummary(errs.rows);
    };
    void load();
    const t = setInterval(load, 15000);
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
  const healthLabel = degraded ? 'Degraded' : allOperational ? 'Operational' : 'Scanning…';
  const healthTone = degraded ? 'red' : allOperational ? 'green' : 'amber';
  const healthDot = degraded ? 'bad' : allOperational ? 'ok' : 'scan';

  const metricsList: MetricDef[] = [
    { label: 'Requests', value: fmtNum(stream.requests.length), icon: 'bolt', tone: 'accent' },
    { label: 'Success rate', value: fmtNum(successRate, 1) + '%', icon: 'check', tone: successTone },
    { label: 'In flight', value: fmtNum(stream.active), icon: 'layers', tone: 'amber' },
    { label: 'Blocked', value: fmtNum(stream.blocked), icon: 'shield', tone: 'red' },
    { label: 'Events', value: fmtNum(events), icon: 'list', tone: 'violet' },
    { label: 'Tokens', value: fmtNum(totalTokens), icon: 'message', tone: 'green' },
    { label: 'Avg latency', value: fmtNum(avgLatency, 0) + ' ms', icon: 'activity' },
    { label: 'Guardrail evals', value: fmtNum(metrics.guardrail_evaluations_total ?? 0), icon: 'gauge', tone: 'accent' }
  ];

  const teleMax = Math.max(1, ...TELEMETRY.map((t) => metrics[t.key] ?? 0));
  const sessionRows: { label: string; icon: IconName; value: string; pct: number; color: string }[] = [
    { label: 'Completed', icon: 'check', value: fmtNum(completed), pct: ended.length ? (completed / ended.length) * 100 : 0, color: 'var(--green)' },
    { label: 'Blocked / failed', icon: 'shield', value: fmtNum(failed.length), pct: ended.length ? (failed.length / ended.length) * 100 : 0, color: 'var(--red)' },
    { label: 'In flight', icon: 'layers', value: fmtNum(stream.active), pct: stream.requests.length ? (stream.active / stream.requests.length) * 100 : 0, color: 'var(--amber)' },
    { label: 'Events emitted', icon: 'list', value: fmtNum(events), pct: 0, color: 'var(--accent)' }
  ];

  const trafficTotal = providerTraffic.reduce((n, p) => n + num(p.request_count), 0);
  const routingErrors = errorSummary.reduce(
    (acc, row) => ({
      errors: acc.errors + num(row.error_count),
      total: acc.total + num(row.total_requests)
    }),
    { errors: 0, total: 0 }
  );
  const routingErrorRate = routingErrors.total
    ? (routingErrors.errors / routingErrors.total) * 100
    : 0;

  return (
    <div className="stack">
      <SetupChecklist
        adminToken={adminToken}
        stream={stream}
        onNavigate={onNavigate}
        onOpenSettings={onOpenSettings}
      />

      <section className="ov-health reveal">
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
                {s.ok === null ? 'checking…' : s.ok ? s.live : s.down}
              </span>
            </div>
          ))}
        </div>
      </section>

      <div className="metric-grid">
        {metricsList.map((m) => (
          <Metric key={m.label} label={m.label} value={m.value} icon={m.icon} tone={m.tone} />
        ))}
      </div>

      <Card title="Quick access" subtitle="Jump straight to any part of the gateway.">
        <ServiceGrid onNavigate={onNavigate} />
      </Card>

      <div className="grid-2">
        <Card
          title="Guardrail telemetry"
          subtitle="cumulative counters from /metrics"
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
              avg evaluation <b style={{ color: 'var(--ink-2)' }}>{fmtNum(metrics.guardrail_evaluation_duration_avg_ms ?? 0)} ms</b>
            </div>
          </div>
        </Card>

        <Card
          title="Session summary"
          subtitle="live breakdown of streamed requests"
          right={
            <Badge tone={healthTone === 'green' ? 'pass' : healthTone === 'red' ? 'error' : 'warn'} dot>
              {healthLabel}
            </Badge>
          }
        >
          <div className="ov-sess">
            {sessionRows.map((s) => (
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
                  {s.label === 'Events emitted' ? 'across all streamed requests' : s.pct.toFixed(0) + '% of completed workload'}
                </span>
              </div>
            ))}
          </div>
        </Card>
      </div>

      <Card
        title="Routing"
        subtitle="Provider selection, failover and traffic"
        right={
          <button className="btn ghost sm" onClick={() => onNavigate('routing')}>
            View all <Icon name="chevron" size={13} />
          </button>
        }
      >
        <div className="stat-grid">
          <Stat
            label="Decisions"
            value={fmtNum(routing?.decisions ?? 0)}
            note="primary selections"
            tone="accent"
          />
          <Stat
            label="Failovers"
            value={fmtNum(routing?.failovers ?? 0)}
            note="provider switches"
            tone={routing?.failovers ? 'amber' : 'green'}
          />
          <Stat
            label="Retries"
            value={fmtNum(routing?.retries ?? 0)}
            note="same-provider"
          />
          <Stat
            label="Breaker trips"
            value={fmtNum(routing?.breaker_trips ?? 0)}
            note={`${fmtNum(routing?.breaker_recoveries ?? 0)} recoveries`}
            tone={routing?.breaker_trips ? 'red' : 'green'}
          />
          <Stat
            label="No provider"
            value={fmtNum(routing?.no_provider ?? 0)}
            note="unroutable"
            tone={routing?.no_provider ? 'amber' : undefined}
          />
          <Stat
            label="Denied"
            value={fmtNum(routing?.denied ?? 0)}
            note="deny-rule requests"
            tone={routing?.denied ? 'red' : undefined}
          />
        </div>

        <div className="grid-2" style={{ marginTop: 16 }}>
          <div>
            <div className="card-sub" style={{ marginBottom: 8 }}>
              Provider traffic · last 24h
            </div>
            {providerTraffic.length === 0 ? (
              <EmptyState
                icon={<Icon name="route" size={20} />}
                text="No provider traffic recorded yet."
              />
            ) : (
              <div className="stack">
                {providerTraffic.slice(0, 6).map((p) => (
                  <div className="bar-row" key={p.provider}>
                    <span>{p.provider || 'unknown'}</span>
                    <div className="bar-track">
                      <div
                        className="bar-fill"
                        style={{
                          width: `${Math.min(100, Math.max(0, num(p.traffic_percentage)))}%`
                        }}
                      />
                    </div>
                    <span>{fmtNum(num(p.traffic_percentage), 1)}%</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div>
            <div className="card-sub" style={{ marginBottom: 8 }}>
              Failover &amp; errors · last 24h
            </div>
            <div className="stat-grid">
              <Stat
                label="Failover rate"
                value={failover ? fmtNum(num(failover.failover_percentage), 1) + '%' : '—'}
                note={
                  failover
                    ? `${fmtNum(num(failover.failover_count))} of ${fmtNum(num(failover.total_requests))}`
                    : 'No data'
                }
                tone={num(failover?.failover_count) ? 'amber' : 'green'}
              />
              <Stat
                label="Error rate"
                value={routingErrors.total ? fmtNum(routingErrorRate, 1) + '%' : '—'}
                note={routingErrors.total ? `${fmtNum(routingErrors.errors)} errors` : 'No data'}
                tone={routingErrors.errors ? 'red' : 'green'}
              />
              <Stat
                label="Traffic"
                value={trafficTotal ? fmtNum(trafficTotal) : '—'}
                note="requests (24h)"
              />
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}