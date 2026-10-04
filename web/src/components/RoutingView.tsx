import { useEffect, useMemo, useState } from 'react';
import {
  fetchRoutingCostByProvider,
  fetchRoutingCostByRoute,
  fetchRoutingErrorSummary,
  fetchRoutingFailovers,
  fetchRoutingLatency,
  fetchRoutingProviderTraffic,
  fetchRoutingTokensByProvider,
  fetchRoutingTokensByRoute,
  RoutingCostByProvider,
  RoutingCostByRoute,
  RoutingErrorSummary,
  RoutingFailoverSummary,
  RoutingLatency,
  RoutingProviderTraffic,
  RoutingTokensByProvider,
  RoutingTokensByRoute,
  RoutingFailoverTransition,
  fetchRoutingFailoverTransitions
} from '../api';
import { ApiEvent, StreamState } from '../types';
import { Icon } from '../icons';
import {
  Alert,
  Badge,
  Card,
  EmptyState,
  LoadingState,
  Section,
  Stat,
  fmtNum,
  fmtTime
} from './ui';

interface RoutingViewProps {
  stream: StreamState;
}

function number(value: unknown): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function percent(value: unknown): string {
  return `${number(value).toFixed(1)}%`;
}

function milliseconds(value: unknown): string {
  return `${Math.round(number(value))} ms`;
}

function cost(value: unknown): string {
  return `$${number(value).toFixed(4)}`;
}

function routingEvents(stream: StreamState): ApiEvent[] {
  return stream.log.filter(
    (event) =>
      event.stage === 'routing_decision' ||
      event.stage === 'routing_failover'
  );
}

function eventData(event: ApiEvent): Record<string, unknown> {
  return event.data && typeof event.data === 'object'
    ? event.data as Record<string, unknown>
    : {};
}

export default function RoutingView({ stream }: RoutingViewProps) {
  const [hours, setHours] = useState(24);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [providerTraffic, setProviderTraffic] = useState<RoutingProviderTraffic[]>([]);
  const [latency, setLatency] = useState<RoutingLatency[]>([]);
  const [errorSummary, setErrorSummary] = useState<RoutingErrorSummary[]>([]);
  const [failovers, setFailovers] = useState<RoutingFailoverSummary | null>(null);
  const [tokensByRoute, setTokensByRoute] = useState<RoutingTokensByRoute[]>([]);
  const [tokensByProvider, setTokensByProvider] = useState<RoutingTokensByProvider[]>([]);
  const [costByRoute, setCostByRoute] = useState<RoutingCostByRoute[]>([]);
  const [costByProvider, setCostByProvider] = useState<RoutingCostByProvider[]>([]);
  const [failoverTransitions, setFailoverTransitions] =
  useState<RoutingFailoverTransition[]>([]);
  useState<RoutingFailoverTransition[]>([]);
  async function load() {
    setLoading(true);
    setError('');

    const results = await Promise.all([
      fetchRoutingProviderTraffic(hours),
      fetchRoutingLatency(hours),
      fetchRoutingErrorSummary(hours),
      fetchRoutingFailovers(hours),
      fetchRoutingTokensByRoute(hours),
      fetchRoutingTokensByProvider(hours),
      fetchRoutingCostByRoute(hours),
      fetchRoutingCostByProvider(hours),
      fetchRoutingFailoverTransitions(hours),
    ]);

    const failed = results.some((result) => !result.ok);

    if (failed) {
      setError('One or more routing analytics queries could not be loaded.');
    }
    setProviderTraffic(results[0].rows);
    setLatency(results[1].rows);
    setErrorSummary(results[2].rows);
    setFailovers(results[3].rows[0] ?? null);
    setTokensByRoute(results[4].rows);
    setTokensByProvider(results[5].rows);
    setCostByRoute(results[6].rows);
    setCostByProvider(results[7].rows);
    setFailoverTransitions(results[8].rows);
  }

  useEffect(() => {
    void load();

    const timer = window.setInterval(() => {
      void load();
    }, 15000);

    return () => window.clearInterval(timer);
  }, [hours]);

  const liveEvents = useMemo(
    () => routingEvents(stream).slice(0, 30),
    [stream.log]
  );

  const routingDecisions = liveEvents.filter(
    (event) => event.stage === 'routing_decision'
  ).length;

  const liveFailovers = liveEvents.filter(
    (event) => event.stage === 'routing_failover'
  ).length;

  return (
    <>
      <Section
        id="routing"
        index="03"
        eyebrow="Observe"
        title="Routing"
        description="Provider traffic, routing performance, failover activity, token usage, and cost."
        right={
          <select
            className="select"
            value={hours}
            onChange={(event) => setHours(Number(event.target.value))}
          >
            <option value={1}>Last hour</option>
            <option value={6}>Last 6 hours</option>
            <option value={24}>Last 24 hours</option>
            <option value={168}>Last 7 days</option>
            <option value={720}>Last 30 days</option>
          </select>
        }
      >
        {error && <Alert tone="warn">{error}</Alert>}

        <div className="stat-grid">
          <Stat
            label="Providers"
            value={providerTraffic.length}
            note="with recorded traffic"
            tone="accent"
          />
          <Stat
            label="Traffic"
            value={providerTraffic.length ? fmtNum(providerTraffic[0].request_count) : '—'}
            note={providerTraffic[0]?.provider || 'No traffic'}
          />
          <Stat
            label="Failovers"
            value={failovers ? fmtNum(failovers.failover_count) : '—'}
            note={failovers ? percent(failovers.failover_percentage) + ' of requests' : 'No data'}
            tone={failovers?.failover_count ? 'amber' : 'green'}
          />
        <Stat
  label="Error rate"
  value={
    errorSummary.length
      ? percent(
          errorSummary.reduce((sum, row) => sum + number(row.error_count), 0) /
            Math.max(
              1,
              errorSummary.reduce(
                (sum, row) => sum + number(row.total_requests),
                0
              )
            ) *
            100
        )
      : '—'
  }
  note={
    errorSummary.length
      ? `${fmtNum(
          errorSummary.reduce((sum, row) => sum + number(row.error_count), 0)
        )} errors`
      : 'No data'
  }
  tone={
    errorSummary.some((row) => number(row.error_count) > 0)
      ? 'red'
      : 'green'
  }
/>
        </div>

        <div className="grid-2" style={{ marginTop: 16 }}>
          <Card
            title="Provider traffic"
            subtitle={`Last ${hours} hour${hours === 1 ? '' : 's'}`}
          >
            {loading && providerTraffic.length === 0 ? (
              <LoadingState text="Loading provider traffic…" />
            ) : providerTraffic.length === 0 ? (
              <EmptyState
                icon={<Icon name="chart" size={20} />}
                text="No provider traffic recorded."
              />
            ) : (
              <div className="stack">
                {providerTraffic.map((provider) => (
                  <div className="bar-row" key={provider.provider}>
                    <span>{provider.provider || 'unknown'}</span>
                    <div className="bar-track">
                      <div
                        className="bar-fill"
                        style={{
                          width: `${Math.min(100, Math.max(0, provider.traffic_percentage))}%`
                        }}
                      />
                    </div>
                    <span>{percent(provider.traffic_percentage)}</span>
                  </div>
                ))}
              </div>
            )}
          </Card>
            <Card
  title="Latency by provider"
  subtitle={`Last ${hours} hour${hours === 1 ? '' : 's'}`}
>
  {latency.length === 0 ? (
    <EmptyState
      icon={<Icon name="chart" size={20} />}
      text="No latency data available."
    />
  ) : (
    <table className="table">
      <thead>
        <tr>
          <th>Provider</th>
          <th>P50</th>
          <th>P95</th>
          <th>P99</th>
        </tr>
      </thead>
      <tbody>
        {latency.map((row) => (
          <tr key={row.provider}>
            <td>{row.provider || 'unknown'}</td>
            <td>{milliseconds(row.p50_latency_ms)}</td>
            <td>{milliseconds(row.p95_latency_ms)}</td>
            <td>{milliseconds(row.p99_latency_ms)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )}
</Card>
        </div>

        <div className="grid-2" style={{ marginTop: 16 }}>
          <Card title="Tokens by route" subtitle="Consumption by routing rule">
            {tokensByRoute.length === 0 ? (
              <EmptyState
                icon={<Icon name="chart" size={20} />}
                text="No route token data available."
              />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Route</th>
                    <th>Requests</th>
                    <th>Input</th>
                    <th>Output</th>
                    <th>Total</th>
                  </tr>
                </thead>
                <tbody>
                  {tokensByRoute.map((row) => (
                    <tr key={row.route}>
                      <td>{row.route || 'default'}</td>
                      <td>{fmtNum(row.request_count)}</td>
                      <td>{fmtNum(row.input_tokens)}</td>
                      <td>{fmtNum(row.output_tokens)}</td>
                      <td>{fmtNum(row.total_tokens)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <Card title="Cost by route" subtitle="Estimated spend by routing rule">
            {costByRoute.length === 0 ? (
              <EmptyState
                icon={<Icon name="chart" size={20} />}
                text="No route cost data available."
              />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Route</th>
                    <th>Requests</th>
                    <th>Total</th>
                    <th>Avg/request</th>
                  </tr>
                </thead>
                <tbody>
                  {costByRoute.map((row) => (
                    <tr key={row.route}>
                      <td>{row.route || 'default'}</td>
                      <td>{fmtNum(row.request_count)}</td>
                      <td>{cost(row.total_cost)}</td>
                      <td>{cost(row.avg_cost_per_request)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>
        </div>

        <div className="grid-2" style={{ marginTop: 16 }}>
          <Card title="Provider consumption" subtitle="Tokens and estimated cost">
            {tokensByProvider.length === 0 ? (
              <EmptyState
                icon={<Icon name="chart" size={20} />}
                text="No provider usage data available."
              />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Provider</th>
                    <th>Requests</th>
                    <th>Tokens</th>
                    <th>Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {tokensByProvider.map((row) => {
                    const providerCost = costByProvider.find(
                      (item) => item.provider === row.provider
                    );

                    return (
                      <tr key={row.provider}>
                        <td>{row.provider || 'unknown'}</td>
                        <td>{fmtNum(row.request_count)}</td>
                        <td>{fmtNum(row.total_tokens)}</td>
                        <td>{cost(providerCost?.total_cost ?? 0)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </Card>

          <Card title="Routing activity" subtitle="Current SSE session">
            <div className="stat-grid">
              <Stat
                label="Decisions"
                value={routingDecisions}
                note="recent events"
                tone="accent"
              />
              <Stat
                label="Failovers"
                value={liveFailovers}
                note="recent events"
                tone={liveFailovers ? 'amber' : 'green'}
              />
              <Stat
                label="Stream"
                value={stream.connected ? 'Live' : 'Offline'}
                tone={stream.connected ? 'green' : 'red'}
              />
            </div>
          </Card>
        </div>
            <div style={{ marginTop: 16 }}>
  <Card
    title="Failover transitions"
    subtitle={`Historical provider-to-provider failovers — last ${hours} hour${hours === 1 ? '' : 's'}`}
  >
    {failoverTransitions.length === 0 ? (
      <EmptyState
        icon={<Icon name="route" size={20} />}
        text="No provider failover transitions recorded."
      />
    ) : (
      <table className="table">
        <thead>
          <tr>
            <th>From</th>
            <th>To</th>
            <th>Failovers</th>
          </tr>
        </thead>
        <tbody>
          {failoverTransitions.map((row) => (
            <tr
              key={`${row.failover_from}-${row.failover_to}`}
            >
              <td>{row.failover_from}</td>
              <td>{row.failover_to}</td>
              <td>{fmtNum(row.failover_count)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    )}
  </Card>
</div>
        <div style={{ marginTop: 16 }}>
          <Card
            title="Live routing feed"
            subtitle="Routing decisions and failovers from the existing event stream"
            right={
              <Badge
                tone={stream.connected ? 'pass' : 'neutral'}
                dot
              >
                {stream.connected ? 'LIVE' : 'OFFLINE'}
              </Badge>
            }
          >
            {liveEvents.length === 0 ? (
              <EmptyState
                icon={<Icon name="bolt" size={20} />}
                text="No routing events have been received in this session."
              />
            ) : (
              <div className="event-table">
                <div className="event-head">
                  <span>Time</span>
                  <span>Stage</span>
                  <span>Route</span>
                  <span>Provider</span>
                  <span>Attempt</span>
                  <span>Details</span>
                </div>

                <div className="event-body">
                  {liveEvents.map((event) => {
                    const data = eventData(event);
                    const route = String(data.route ?? '');
                    const provider = String(
                      data.selected_provider ??
                      data.upstream ??
                      data.to ??
                      ''
                    );
                    const from = String(data.from ?? '');
                    const to = String(data.to ?? '');
                    const attempt = number(data.attempt);

                    return (
                      <div className="event-row" key={event.id}>
                        <span>{fmtTime(event.timestamp)}</span>
                        <span className="e-status">
                          <Badge
                            tone={
                              event.stage === 'routing_failover'
                                ? 'warn'
                                : 'accent'
                            }
                          >
                            {event.stage === 'routing_failover'
                              ? 'FAILOVER'
                              : 'DECISION'}
                          </Badge>
                        </span>
                        <span>{route || 'default'}</span>
                        <span>{provider || 'unknown'}</span>
                        <span>{attempt || '—'}</span>
                        <span>
                          {event.stage === 'routing_failover' && from && to
                            ? `${from} → ${to}`
                            : event.message || 'Provider selected'}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </Card>
        </div>
      </Section>
    </>
  );
}