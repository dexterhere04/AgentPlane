import { useMemo, useState } from 'react';
import { RequestState, StreamState } from '../types';
import { FlowMode } from '../queue';
import { sendChat } from '../api';
import { Icon, IconName } from '../icons';
import {
  Alert, Card, EmptyState, Metric, Segmented, StatusDot,
  shortId, fmtTime, fmtNum
} from './ui';

const STAGES: { id: string; label: string; icon: IconName }[] = [
  { id: 'request_received', label: 'Received', icon: 'bolt' },
  { id: 'body_read', label: 'Body read', icon: 'message' },
  { id: 'json_validated', label: 'Validated', icon: 'check' },
  { id: 'guardrail_input', label: 'Input guards', icon: 'shield' },
  { id: 'loading_api_key', label: 'API key', icon: 'key' },
  { id: 'building_request', label: 'Build request', icon: 'route' },
  { id: 'setting_headers', label: 'Headers', icon: 'list' },
  { id: 'sending_request', label: 'Send', icon: 'arrow' },
  { id: 'reading_response', label: 'Read response', icon: 'message' },
  { id: 'validating_status', label: 'Status', icon: 'check' },
  { id: 'response_sent', label: 'Provider response', icon: 'bolt' },
  { id: 'guardrail_output', label: 'Output guards', icon: 'shield' },
  { id: 'info', label: 'Responded', icon: 'check' }
];

type StageState = 'pending' | 'active' | 'done' | 'error' | 'blocked';

function blockStage(req: RequestState): 'guardrail_input' | 'guardrail_output' {
  const reachedProvider = req.events.some(
    (e) => e.stage === 'response_sent' || e.stage === 'reading_response' || e.stage === 'loading_api_key'
  );
  return reachedProvider ? 'guardrail_output' : 'guardrail_input';
}

function stageState(req: RequestState, stageId: string): StageState {
  const blockedAt = req.blocked ? blockStage(req) : null;
  const evts = req.events.filter((e) => e.stage === stageId);

  if (stageId === 'guardrail_input' || stageId === 'guardrail_output') {
    if (blockedAt === stageId) return 'blocked';
    return evts.length ? 'done' : 'pending';
  }

  if (!evts.length) return 'pending';
  if (evts.some((e) => e.status === 'error')) return 'error';

  if (stageId === 'info') return req.blocked ? 'pending' : 'done';

  const last = evts[evts.length - 1];
  if (last.status === 'completed') return 'done';
  if (last.status === 'started' || last.status === 'authenticated') return 'active';
  return 'done';
}

function pillLabel(state: StageState): string {
  switch (state) {
    case 'active': return 'running';
    case 'pending': return 'waiting';
    case 'done': return 'done';
    case 'error': return 'error';
    case 'blocked': return 'blocked';
  }
}

function laneStatus(r: RequestState): 'active' | 'error' | 'blocked' | 'done' {
  if (r.blocked) return 'blocked';
  if (!r.endedAt) return 'active';
  if (r.events.some((e) => e.stage === 'error')) return 'error';
  return 'done';
}

export default function PipelineView({
  stream,
  flowMode,
  onToggleFlow,
  queueDepth,
  userKey
}: {
  stream: StreamState;
  flowMode: FlowMode;
  onToggleFlow: () => void;
  queueDepth: number;
  userKey: string;
}) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [count, setCount] = useState(5);
  const [firing, setFiring] = useState(false);
  const [laneFilter, setLaneFilter] = useState<'all' | 'active' | 'blocked' | 'error'>('all');

  const requests = stream.requests;
  const selected: RequestState | undefined = useMemo(() => {
    if (selectedId) return requests.find((r) => r.id === selectedId);
    return requests.length ? requests[requests.length - 1] : undefined;
  }, [requests, selectedId]);

  const ended = requests.filter((r) => r.endedAt);
  const latencies = ended.map((r) => (r.endedAt ?? r.startedAt) - r.startedAt);
  const avgLatency = latencies.length ? latencies.reduce((a, b) => a + b, 0) / latencies.length : 0;
  const minLatency = latencies.length ? Math.min(...latencies) : 0;
  const maxLatency = latencies.length ? Math.max(...latencies) : 0;

  const stageStates = useMemo(
    () => (selected ? STAGES.map((s) => stageState(selected, s.id)) : []),
    [selected]
  );

  const doneCount = stageStates.filter((s) => s === 'done' || s === 'blocked').length;
  const progress = selected && STAGES.length ? (doneCount / STAGES.length) * 100 : 0;

  const activeStageIdx = stageStates.findIndex((s) => s === 'active' || s === 'blocked' || s === 'error');
  const tickLeftPct = activeStageIdx >= 0
    ? ((activeStageIdx + 0.5) / STAGES.length) * 100
    : progress;

  const selectedState: StageState = selected
    ? selected.blocked
      ? 'blocked'
      : selected.events.some((e) => e.stage === 'error')
        ? 'error'
        : selected.endedAt
          ? 'done'
          : 'active'
    : 'pending';

  const visible = useMemo(() => {
    const sorted = [...requests].sort((a, b) => a.startedAt - b.startedAt);
    return sorted.slice(-24);
  }, [requests]);

  const filteredVisible = useMemo(() => {
    if (laneFilter === 'all') return visible;
    return visible.filter((r) => laneStatus(r) === laneFilter);
  }, [visible, laneFilter]);

  const now = Date.now();
  const t0 = visible.length ? Math.min(...visible.map((r) => r.startedAt)) : now;
  const t1 = visible.length ? Math.max(...visible.map((r) => r.endedAt ?? now), now) : now + 1;
  const span = Math.max(t1 - t0, 1);

  const fire = async () => {
    setFiring(true);
    const jobs = Array.from({ length: count }, (_, i) =>
      sendChat('Parallel request #' + (i + 1) + ' — tell me a short fact.', {
        userKey,
        model: 'gpt-4o'
      })
    );
    await Promise.allSettled(jobs);
    setFiring(false);
  };

  return (
    <div className="stack">
      <div className="metric-grid">
        <Metric label="Requests" value={fmtNum(requests.length)} icon="bolt" tone="accent" />
        <Metric label="In flight" value={fmtNum(stream.active)} icon="layers" tone="amber" />
        <Metric label="Blocked" value={fmtNum(stream.blocked)} icon="shield" tone="red" />
        <Metric label="Avg latency" value={fmtNum(avgLatency, 0) + ' ms'} icon="activity" />
        <Metric label="Min latency" value={fmtNum(minLatency, 0) + ' ms'} icon="gauge" />
        <Metric label="Max latency" value={fmtNum(maxLatency, 0) + ' ms'} icon="chart" tone="green" />
      </div>

      <Card
        title="Gateway pipeline"
        subtitle={selected ? '#' + shortId(selected.id) + (selected.authenticated ? ' · ' + selected.authenticated.username : '') : 'Send a request to watch it flow through each stage'}
        right={
          <div className="row">
            {queueDepth > 0 && <span className="buffer-badge">buffer {queueDepth}</span>}
            <Segmented
              ariaLabel="Flow mode"
              active={flowMode}
              onChange={(id) => { if (id !== flowMode) onToggleFlow(); }}
              items={[
                { id: 'stepped', label: 'Stepped' },
                { id: 'realtime', label: 'Realtime' }
              ]}
            />
            <select className="select" value={selected?.id ?? ''} onChange={(e) => setSelectedId(e.target.value || null)}>
              <option value="">latest request</option>
              {[...requests].reverse().map((r) => (
                <option key={r.id} value={r.id}>#{shortId(r.id)}{r.blocked ? ' (blocked)' : ''}</option>
              ))}
            </select>
          </div>
        }
      >
        {!selected ? (
          <EmptyState icon={<Icon name="route" size={20} />} text="No request yet — send one from the chat overlay or fire one below." />
        ) : (
          <>
            <div className="pipeline-summary">
              <div className="flow-progress">
                <div className="fp-track">
                  <div className="fp-fill" style={{ width: progress + '%' }} />
                  {!selected.endedAt && (
                    <div
                      className={'fp-tick' + (selectedState === 'active' ? ' pulse' : '')}
                      style={{ left: tickLeftPct + '%' }}
                    />
                  )}
                </div>
                <div className="fp-meta">
                  <span className="fp-count">{doneCount} / {STAGES.length} stages complete</span>
                  <span className="fp-count">{selected.endedAt ? fmtNum(selected.endedAt - selected.startedAt, 0) + ' ms total' : 'in progress'}</span>
                </div>
              </div>
              <span className={'p-pill big ' + selectedState}>{pillLabel(selectedState)}</span>
            </div>

            {selected.blocked && (
              <Alert tone="error" icon={<Icon name="warning" size={15} />}>
                <b>Blocked by guardrails</b> — {selected.blockedMessage || 'request was denied before reaching the provider'}
              </Alert>
            )}

            <div className="pipeline">
              {STAGES.map((stage, i) => {
                const state = stageStates[i];
                const info = selected.stages[stage.id];
                const isBlockedStage = state === 'blocked';
                return (
                  <div className="stage-wrap" key={stage.id}>
                    <div className={'stage ' + state + (isBlockedStage ? ' block-reason' : '')}>
                      <span className="stage-icon"><Icon name={stage.icon} size={15} /></span>
                      <span className="stage-label">{stage.label}</span>
                      <span className={'stage-pill p-pill ' + state}>{pillLabel(state)}</span>
                      <span className="stage-meta">
                        {isBlockedStage
                          ? (selected.blockedMessage || info?.message || 'blocked')
                          : info?.duration
                            ? info.duration + ' ms'
                            : info?.message
                              ? info.message
                              : ''}
                      </span>
                      {isBlockedStage && selected.blockedMessage && (
                        <span className="stage-block-msg" title={selected.blockedMessage}>
                          {selected.blockedMessage}
                        </span>
                      )}
                    </div>
                    {i < STAGES.length - 1 && (
                      <span className={'stage-conn' + (state === 'done' || state === 'blocked' ? ' flow' : '')}>
                        <Icon name="chevron" size={13} />
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          </>
        )}
      </Card>

      <Card
        title="Concurrent requests"
        subtitle="Each request's stages advance independently on the shared event bus — overlap below is real."
        right={
          <div className="row">
            <input className="input num" type="number" min={1} max={50} value={count} onChange={(e) => setCount(Number(e.target.value))} />
            <button className="btn" disabled={firing} onClick={fire}>
              <Icon name="bolt" size={15} />
              {firing ? 'Firing…' : 'Fire ' + count + ' in parallel'}
            </button>
          </div>
        }
      >
        <div className="lane-filter-row">
          <div className="legend">
            <span className="lg"><i className="lg-running" /> in-flight</span>
            <span className="lg"><i className="lg-done" /> completed</span>
            <span className="lg"><i className="lg-blocked" /> blocked</span>
            <span className="lg"><i className="lg-error" /> error</span>
          </div>
          <Segmented
            ariaLabel="Filter lanes"
            active={laneFilter}
            onChange={(id) => setLaneFilter(id as typeof laneFilter)}
            items={[
              { id: 'all', label: 'All' },
              { id: 'active', label: 'In flight' },
              { id: 'blocked', label: 'Blocked' },
              { id: 'error', label: 'Errors' }
            ]}
          />
        </div>

        {filteredVisible.length === 0 ? (
          <EmptyState
            icon={<Icon name="layers" size={20} />}
            text={laneFilter === 'all' ? 'No requests yet — hit "Fire" to watch them overlap.' : 'No requests match this filter.'}
          />
        ) : (
          <div className="lanes">
            {[...filteredVisible].reverse().map((r) => {
              const start = r.startedAt;
              const end = r.endedAt ?? now;
              const left = ((start - t0) / span) * 100;
              const width = Math.max(((end - start) / span) * 100, 0.6);
              const status = laneStatus(r);
              const toneClass = status === 'active' ? 'running' : status;
              const dotTone =
                status === 'blocked' || status === 'error' ? 'bad'
                : status === 'active' ? 'scan'
                : 'ok';
              return (
                <div className="lane" key={r.id}>
                  <div className="lane-meta">
                    <StatusDot tone={dotTone} size="sm" />
                    <span className="mono">#{shortId(r.id)}</span>
                    <span className="lane-prompt">{r.userPrompt ?? ''}</span>
                    <span className={'p-pill ' + (status === 'active' ? 'active' : status)}>{pillLabel(status === 'active' ? 'active' : status)}</span>
                    <span className="lane-time">{fmtTime(start)} → {r.endedAt ? fmtTime(end) : '…'}</span>
                  </div>
                  <div className="lane-track">
                    <div className={'lane-bar ' + toneClass} style={{ left: left + '%', width: width + '%' }} />
                  </div>
                </div>
              );
            })}
            <div className="lane-axis">
              <span>{fmtTime(t0)}</span>
              <span>{fmtTime(t1)}</span>
            </div>
          </div>
        )}
      </Card>

      {selected && (
        <Card title={'Stage detail — #' + shortId(selected.id)} subtitle={fmtTime(selected.startedAt)}>
          <div className="sd">
            {STAGES.map((stage) => {
              const state = stageState(selected, stage.id);
              const info = selected.stages[stage.id];
              if (!info && state === 'pending') return null;
              return (
                <div className="sd-row" key={stage.id}>
                  <span className="sd-key">{stage.label}</span>
                  <span className={'p-pill ' + state}>{pillLabel(state)}</span>
                  <span className="sd-msg">{info?.message ?? (state === 'blocked' ? selected.blockedMessage : '')}</span>
                  <span className="sd-dur">{info?.duration ? info.duration + ' ms' : ''}</span>
                </div>
              );
            })}
          </div>
        </Card>
      )}
    </div>
  );
}