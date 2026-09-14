import { useEffect, useMemo, useRef, useState } from 'react';
import { ApiEvent, StreamState } from '../types';
import { Card, EmptyState, FilterBar, JsonBlock, StatusBadge, fmtNum, shortId } from './ui';
import { Icon } from '../icons';

const MAX_AGE_MS = { '5m': 5 * 60_000, '15m': 15 * 60_000, '1h': 60 * 60_000, 'all': Infinity } as const;
type AgeKey = keyof typeof MAX_AGE_MS;

const REQ_BORDER_CLASSES = ['req-a', 'req-b', 'req-c', 'req-d', 'req-e'] as const;
function reqBorder(reqId: string): string {
  let h = 0;
  for (let i = 0; i < reqId.length; i++) h = (h * 31 + reqId.charCodeAt(i)) | 0;
  return REQ_BORDER_CLASSES[Math.abs(h) % REQ_BORDER_CLASSES.length];
}

function stageClass(stage: string): string {
  if (stage.startsWith('guardrail')) return 'sg-guardrail';
  if (stage === 'error') return 'sg-error';
  if (stage === 'response_sent') return 'sg-done';
  if (stage === 'request_received' || stage === 'info') return 'sg-recd';
  if (stage === 'body_read' || stage === 'json_validated') return 'sg-read';
  return 'sg-proxy';
}

function isError(e: ApiEvent): boolean {
  return e.status === 'error' || e.stage === 'error' || e.stage === 'guardrail_blocked';
}

const STAGE_FILTERS = [
  { value: '', label: 'all stages' },
  { value: 'request_received', label: 'request_received' },
  { value: 'body_read', label: 'body_read' },
  { value: 'json_validated', label: 'json_validated' },
  { value: 'guardrail_input', label: 'guardrail_input' },
  { value: 'guardrail_output', label: 'guardrail_output' },
  { value: 'guardrail_blocked', label: 'guardrail_blocked' },
  { value: 'sending_request', label: 'sending_request' },
  { value: 'response_sent', label: 'response_sent' },
  { value: 'error', label: 'error' }
];

export default function EventLog({ stream }: { stream: StreamState }) {
  const [requestId, setRequestId] = useState('');
  const [stage, setStage] = useState('');
  const [search, setSearch] = useState('');
  const [age, setAge] = useState<AgeKey>('15m');
  const [paused, setPaused] = useState(false);
  const [snapshot, setSnapshot] = useState<ApiEvent[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const bodyRef = useRef<HTMLDivElement>(null);

  const togglePause = () => {
    if (!paused) setSnapshot(stream.log);
    setPaused((p) => !p);
  };

  const toggleExpanded = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const cutoff = useMemo(() => (MAX_AGE_MS[age] === Infinity ? 0 : Date.now() - MAX_AGE_MS[age]), [age, stream.log.length]);

  const log = useMemo(() => {
    let l = paused ? snapshot : stream.log;
    if (cutoff) l = l.filter((e) => e.timestamp >= cutoff);
    if (requestId) l = l.filter((e) => e.request_id === requestId);
    if (stage) l = l.filter((e) => e.stage === stage);
    if (search) {
      const q = search.toLowerCase();
      l = l.filter(
        (e) =>
          e.stage.toLowerCase().includes(q) ||
          e.message.toLowerCase().includes(q) ||
          e.status.toLowerCase().includes(q)
      );
    }
    return [...l].reverse();
  }, [paused, snapshot, stream.log, requestId, stage, search, cutoff]);

  useEffect(() => {
    if (!paused && bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [log.length, paused]);

  const hasFilters = !!(requestId || stage || search || age !== '15m');

  const chips: { label: string; onClear: () => void }[] = [];
  if (requestId) chips.push({ label: '#' + shortId(requestId), onClear: () => setRequestId('') });
  if (stage) chips.push({ label: 'stage: ' + stage, onClear: () => setStage('') });
  if (search) chips.push({ label: 'text: ' + search, onClear: () => setSearch('') });
  if (age !== '15m') chips.push({ label: 'window: ' + age, onClear: () => setAge('15m') });

  return (
    <Card
      title="Request activity"
      subtitle="Every stage, decision and error the gateway emits — in arrival order. Click a row to inspect its raw payload."
      right={
        <div className="row">
          <span className="ev-count">{log.length} events</span>
          <button className="btn ghost sm" onClick={togglePause}>
            <span className={'live-dot ' + (paused ? '' : 'on')} />
            {paused ? 'Resume' : 'Pause'}
          </button>
        </div>
      }
    >
      <FilterBar
        search={{ value: search, onChange: setSearch, placeholder: 'filter stage, message, status…' }}
        selects={[
          {
            value: requestId,
            onChange: setRequestId,
            ariaLabel: 'Request',
            options: [
              { value: '', label: 'all requests' },
              ...[...stream.requests].reverse().map((r) => ({
                value: r.id,
                label: '#' + shortId(r.id)
              }))
            ]
          },
          {
            value: stage,
            onChange: setStage,
            ariaLabel: 'Stage',
            options: STAGE_FILTERS
          }
        ]}
        timeRange={{
          value: age,
          onChange: (v) => setAge(v as AgeKey),
          options: [
            { value: '5m', label: 'last 5m' },
            { value: '15m', label: 'last 15m' },
            { value: '1h', label: 'last 1h' },
            { value: 'all', label: 'all time' }
          ]
        }}
        chips={chips}
      />

      <div className="event-table">
        <div className="event-head">
          <span />
          <span>time</span>
          <span>stage</span>
          <span>status</span>
          <span>request</span>
          <span>message</span>
        </div>
        <div className="event-body" ref={bodyRef}>
          {log.length === 0 && (
            <EmptyState
              icon={<Icon name="list" size={20} />}
              text={
                hasFilters
                  ? 'No events match the current filters.'
                  : 'No events yet — send a request from the chat, pipeline, or guardrail playground.'
              }
            />
          )}
          {log.map((e) => {
            const err = isError(e);
            const isOpen = expanded.has(e.id);
            return (
              <div
                className={
                  'event-row ' +
                  reqBorder(e.request_id) +
                  (err ? ' is-error' : '') +
                  (isOpen ? ' expanded' : '')
                }
                key={e.id}
              >
                <button
                  className={'e-toggle' + (isOpen ? ' open' : '')}
                  onClick={() => toggleExpanded(e.id)}
                  aria-expanded={isOpen}
                  aria-label={isOpen ? 'Collapse event' : 'Expand event'}
                >
                  <Icon name="chevron" size={12} />
                </button>
                <span className="e-time" title={new Date(e.timestamp).toISOString()}>
                  {new Date(e.timestamp).toLocaleTimeString('en-US', { hour12: false })}.
                  {String(e.timestamp % 1000).padStart(3, '0')}
                </span>
                <span className={'e-stage ' + stageClass(e.stage)}>
                  <i className="e-dot" />
                  {e.stage}
                </span>
                <span className="e-status"><StatusBadge status={e.status} /></span>
                <button
                  className="e-req"
                  onClick={(ev) => {
                    ev.stopPropagation();
                    setRequestId(e.request_id === requestId ? '' : e.request_id);
                  }}
                  title={requestId === e.request_id ? 'Clear request filter' : 'Filter to this request'}
                >
                  #{shortId(e.request_id)}
                </button>
                <span className="e-msg" title={e.message}>{e.message}</span>

                {isOpen && (
                  <div className="event-detail">
                    <div className="event-detail-grid">
                      <span className="event-detail-key">id</span>
                      <span className="event-detail-val mono">{e.id}</span>
                      <span className="event-detail-key">request</span>
                      <span className="event-detail-val mono">{e.request_id}</span>
                      <span className="event-detail-key">timestamp</span>
                      <span className="event-detail-val">{new Date(e.timestamp).toISOString()}</span>
                      <span className="event-detail-key">duration</span>
                      <span className="event-detail-val">{e.duration > 0 ? fmtNum(e.duration) + ' ms' : '—'}</span>
                      <span className="event-detail-key">message</span>
                      <span className="event-detail-val">{e.message || '—'}</span>
                    </div>
                    {e.data !== undefined && e.data !== null ? (
                      <>
                        <div className="event-detail-label">payload</div>
                        <JsonBlock value={e.data} maxHeight={280} />
                      </>
                    ) : (
                      <div className="hint" style={{ marginTop: 0 }}>No payload attached to this event.</div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </Card>
  );
}