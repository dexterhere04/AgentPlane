import { useState } from 'react';
import { StreamState } from '../types';
import { FlowMode } from '../queue';
import PipelineView from './PipelineView';
import EventLog from './EventLog';
import { Tabs } from './ui';
// Re-export the shared Disclosure as Collapsible so existing imports
// in Diagnostics.tsx (and anywhere else) keep working.
export { Disclosure as Collapsible } from './ui';

type Pane = 'pipeline' | 'events';

export default function RequestActivity(props: {
  stream: StreamState;
  flowMode: FlowMode;
  onToggleFlow: () => void;
  queueDepth: number;
  userKey: string;
}) {
  const [pane, setPane] = useState<Pane>('pipeline');

  const panes: { id: Pane; label: string; count?: number }[] = [
    { id: 'pipeline', label: 'Live pipeline' },
    { id: 'events', label: 'Raw events', count: props.stream.log.length }
  ];

  return (
    <div className="stack">
      <Tabs
        ariaLabel="Request activity panes"
        active={pane}
        onChange={(id) => setPane(id as Pane)}
        items={panes.map((p) => ({
          id: p.id,
          label: typeof p.count === 'number' ? `${p.label} · ${p.count}` : p.label
        }))}
      />

      {pane === 'pipeline' && (
        <PipelineView
          stream={props.stream}
          flowMode={props.flowMode}
          onToggleFlow={props.onToggleFlow}
          queueDepth={props.queueDepth}
          userKey={props.userKey}
        />
      )}

      {pane === 'events' && <EventLog stream={props.stream} />}
    </div>
  );
}