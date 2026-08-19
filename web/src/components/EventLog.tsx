import { useEffect, useMemo, useRef, useState } from 'react';
import { ApiEvent, StreamState } from '../types';
import { Card, StatusBadge, fmtTime, shortId } from './ui';
import { Icon } from '../icons';

function stageClass(stage: string): string {
  if (stage.startsWith('guardrail')) return 'sg-guardrail';
  if (stage === 'error') return 'sg-error';
  if (stage === 'response_sent') return 'sg-done';
  if (stage === 'request_received' || stage === 'info') return 'sg-recd';
  if (stage === 'body_read' || stage === 'json_validated') return 'sg-read';
  return 'sg-proxy';
}

export default function EventLog({ stream }: { stream: StreamState }) {
  const [requestId, setRequestId] = useState('');
  const [search, setSearch] = useState('');
  const [paused, setPaused] = useState(false);
  const [snapshot, setSnapshot] = useState<ApiEvent[]>([]);
  const bodyRef = useRef<HTMLDivElement>(null);

  const togglePause = () => {
    if (!paused) setSnapshot(stream.log);
    setPaused((p) => !p);
  };

  // Events are shown in the order they arrive (oldest → newest), like the
  // gateway actually emits them on the bus.
  const log = useMemo(() => {
    let l = paused ? snapshot : stream.log;
    if (requestId) l = l.filter((e) => e.request_id === requestId);
    if (search) {
      const q = search.toLowerCase();
      l = l.filter(
        (e) => e.stage.toLowerCase().includes(q) || e.message.toLowerCase().includes(q) || e.status.toLowerCase().includes(q)
      );
    }
    return [...l].reverse();
  }, [paused, snapshot, stream.log, requestId, search]);

  useEffect(() => {
    if (!paused && bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [log.length, paused]);

  return (
    <Card
      title="Live event stream"
      subtitle="Raw SSE from the gateway — every stage, decision, and error, in the order it arrives."
      right={
        <div className="row">
          <input className="input" placeholder="filter…" value={search} onChange={(e) => setSearch(e.target.value)} />
          <select className="select" value={requestId} onChange={(e) => setRequestId(e.target.value)}>
            <option value="">all requests</option>
            {[...stream.requests].reverse().map((r) => (
              <option key={r.id} value={r.id}>#{shortId(r.id)}</option>
            ))}
          </select>
          <button className="btn ghost sm" onClick={togglePause}>
            {paused ? <Icon name="bolt" size={14} /> : <Icon name="list" size={14} />}
            {paused ? 'Resume' : 'Pause'}
          </button>
        </div>
      }
    >
      <div className="event-table">
        <div className="event-head">
          <span>time</span>
          <span>stage</span>
          <span>status</span>
          <span>request</span>
          <span>message</span>
        </div>
        <div className="event-body" ref={bodyRef}>
          {log.length === 0 && (
            <div className="empty">
              <span className="empty-icon"><Icon name="list" size={20} /></span>
              <span>No events yet.</span>
            </div>
          )}
          {log.map((e) => (
            <div className="event-row" key={e.id}>
              <span className="e-time">{fmtTime(e.timestamp)}</span>
              <span className={'e-stage ' + stageClass(e.stage)}>{e.stage}</span>
              <span className="e-status"><StatusBadge status={e.status} /></span>
              <span className="e-req">#{shortId(e.request_id)}</span>
              <span className="e-msg" title={e.message}>{e.message}</span>
            </div>
          ))}
        </div>
      </div>
    </Card>
  );
}
