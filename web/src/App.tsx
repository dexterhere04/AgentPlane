import { useEffect, useRef, useState } from 'react';
import { ApiEvent, StreamState, initialState } from './types';
import { reduceStream, streamConnected } from './stream';
import { getAdminToken, setAdminToken, getUserKey, setUserKey } from './api';
import { createEventQueue, EventQueue, FlowMode } from './queue';
import { Icon, IconName } from './icons';
import { Section } from './components/ui';
import Overview from './components/Overview';
import PipelineView from './components/PipelineView';
import ChatPanel from './components/ChatPanel';
import GuardrailsView from './components/GuardrailsView';
import EventLog from './components/EventLog';
import VaultView from './components/VaultView';
import UsageView from './components/UsageView';
import ObservabilityView from './components/ObservabilityView';
import ChatOverlay from './components/ChatOverlay';

interface SectionDef {
  id: string;
  index: string;
  label: string;
  icon: IconName;
}

const SECTIONS: SectionDef[] = [
  { id: 'overview', index: '00', label: 'Overview', icon: 'gauge' },
  { id: 'pipeline', index: '01', label: 'Pipeline', icon: 'route' },
  { id: 'chat', index: '02', label: 'Live Chat', icon: 'message' },
  { id: 'guardrails', index: '03', label: 'Guardrails', icon: 'shield' },
  { id: 'events', index: '04', label: 'Event Log', icon: 'list' },
  { id: 'keys', index: '05', label: 'Keys & Vault', icon: 'key' },
  { id: 'usage', index: '06', label: 'Usage & Cost', icon: 'activity' },
  { id: 'observability', index: '07', label: 'Observability', icon: 'chart' }
];

export default function App() {
  const [stream, setStream] = useState<StreamState>(initialState);
  const [adminToken, setAdminTokenState] = useState(getAdminToken());
  const [userKey, setUserKeyState] = useState(getUserKey());
  const [showSettings, setShowSettings] = useState(false);
  const [theme, setTheme] = useState<'light' | 'dark'>(
    () => (localStorage.getItem('agentplane.theme') as 'light' | 'dark') || 'light'
  );
  const [tab, setTab] = useState('overview');
  const [sidebarOpen, setSidebarOpen] = useState(
  () => window.innerWidth > 860 && localStorage.getItem('agentplane.sidebar') !== 'closed'
);
  
  const [flowMode, setFlowMode] = useState<FlowMode>('stepped');
  const [queueDepth, setQueueDepth] = useState(0);
  const seenRef = useRef<Set<string>>(new Set());
  const queueRef = useRef<EventQueue | undefined>(undefined);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('agentplane.theme', theme);
  }, [theme]);

  const toggleTheme = () => setTheme((t) => (t === 'light' ? 'dark' : 'light'));

  const toggleSidebar = () => {
    setSidebarOpen((v) => {
      localStorage.setItem('agentplane.sidebar', v ? 'closed' : 'open');
      return !v;
    });
  };

  const toggleFlow = () => {
    setFlowMode((prev) => {
      const next: FlowMode = prev === 'stepped' ? 'realtime' : 'stepped';
      queueRef.current?.setMode(next);
      return next;
    });
  };

  useEffect(() => {
    const queue = createEventQueue(
      (evt) => setStream((s) => reduceStream(s, evt)),
      (n) => setQueueDepth(n)
    );
    queueRef.current = queue;

    const es = new EventSource('/events');
    es.onopen = () => setStream((s) => streamConnected(s, true));
    es.onerror = () => setStream((s) => streamConnected(s, false));
    es.onmessage = (e) => {
      let evt: ApiEvent;
      try {
        evt = JSON.parse(e.data);
      } catch {
        return;
      }
      if (seenRef.current.has(evt.id)) return;
      seenRef.current.add(evt.id);
      if (seenRef.current.size > 5000) seenRef.current.clear();
      queue.enqueue(evt);
    };
    return () => {
      es.close();
      queue.dispose();
      queueRef.current = undefined;
    };
  }, []);

  const updateAdminToken = (v: string) => {
    setAdminTokenState(v);
    setAdminToken(v);
  };
  const updateUserKey = (v: string) => {
    setUserKeyState(v);
    setUserKey(v);
  };

  const activeLabel = SECTIONS.find((s) => s.id === tab)?.label ?? '';

  return (
    <div className={'app' + (sidebarOpen ? '' : ' collapsed')}>
      <aside className={`sidebar ${sidebarOpen ? 'open' : ''}`}>
        <div className="brand">
          <div className="brand-mark">A</div>
          <div>
            <div className="brand-name">AgentPlane</div>
            <div className="brand-sub">Control Plane</div>
          </div>
        </div>
        <nav className="side-nav">
          <div className="nav-group-label">Gateway</div>
          {SECTIONS.map((s) => (
            <button
              key={s.id}
              className={'nav-item' + (tab === s.id ? ' active' : '')}
              onClick={() => {
  setTab(s.id);
  if (window.innerWidth <= 860) {
    setSidebarOpen(false);
    localStorage.setItem('agentplane.sidebar', 'closed');
  }
}}
            >
              <span className="nav-icon"><Icon name={s.icon} size={16} /></span>
              {s.label}
              <span className="nav-idx">{s.index}</span>
            </button>
          ))}
        </nav>
        <div className="side-foot">
          AgentPlane · v0.1.0
          <br />
          AI gateway control plane
        </div>
      </aside>

      <div className="shell">
        <header className="topbar">

          <button className="icon-btn menu-btn" onClick={toggleSidebar} title={sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}>
            <Icon name="menu" size={17} />
          </button>
          
          <span className="crumb">AgentPlane <b>/</b> {activeLabel}</span>
          <div className="topbar-spacer" />
          <span className="lstat">
            <span className={'live-dot ' + (stream.connected ? 'on' : 'off')} />
            {stream.connected ? 'Live' : 'Offline'}
          </span>
          <span className="lstat">Requests <b>{stream.requests.length}</b></span>
          <span className="lstat">Active <b>{stream.active}</b></span>
          <span className="lstat">Blocked <b>{stream.blocked}</b></span>
          <span className="lstat" title="Events buffered in the processing queue">
            Queue <b>{queueDepth}</b>
          </span>
          <button className="icon-btn" onClick={toggleTheme} title="Toggle theme">
            <Icon name={theme === 'light' ? 'moon' : 'sun'} size={16} />
          </button>
          <button className="icon-btn" onClick={() => setShowSettings((v) => !v)} title="Connection & credentials">
            <Icon name="key" size={16} />
          </button>
        </header>

        {showSettings && (
          <div className="settings-panel">
            <label>
              <span>Admin token</span>
              <input
                className="input mono"
                type="password"
                value={adminToken}
                onChange={(e) => updateAdminToken(e.target.value)}
                placeholder="AGENTPLANE_ADMIN_TOKEN"
              />
            </label>
            <label>
              <span>User API key</span>
              <input
                className="input mono"
                type="password"
                value={userKey}
                onChange={(e) => updateUserKey(e.target.value)}
                placeholder="ap_live_..."
              />
            </label>
            <span className="hint" style={{ marginTop: 0 }}>
              Stored locally · admin token guards provisioning, Vault writes &amp; analytics.
            </span>
          </div>
        )}

        <main className="content">
          {tab === 'overview' && (
            <Section id="overview" index="00" title="Overview"
              description="Live gateway telemetry, guardrail outcomes, and system status at a glance.">
              <Overview stream={stream} />
            </Section>
          )}

          {tab === 'pipeline' && (
            <Section id="pipeline" index="01" title="Pipeline"
              description="Every stage a request passes through, live concurrency, and per-request latency.">
              <PipelineView stream={stream} flowMode={flowMode} onToggleFlow={toggleFlow} queueDepth={queueDepth} userKey={userKey} />
            </Section>
          )}

          {tab === 'chat' && (
            <Section id="chat" index="02" title="Live Chat"
              description="A real client application talking to the gateway — auth, guardrails, and provider, transparently.">
              <ChatPanel userKey={userKey} onUserKeyChange={updateUserKey} />
            </Section>
          )}

          {tab === 'guardrails' && (
            <Section id="guardrails" index="03" title="Guardrails"
              description="Send crafted payloads and inspect before/after processing for every guardrail decision.">
              <GuardrailsView stream={stream} userKey={userKey} />
            </Section>
          )}

          {tab === 'events' && (
            <Section id="events" index="04" title="Event Log"
              description="The raw SSE stream — every stage, decision, and error the gateway emits.">
              <EventLog stream={stream} />
            </Section>
          )}

          {tab === 'keys' && (
            <Section id="keys" index="05" title="Keys & Vault"
              description="Store provider credentials in Vault and provision / revoke user API keys.">
              <VaultView />
            </Section>
          )}

          {tab === 'usage' && (
            <Section id="usage" index="06" title="Usage & Cost"
              description="Track token consumption, estimated cost, model/provider usage, and key-attributed activity.">
              <UsageView userKey={userKey} onUserKeyChange={updateUserKey} stream={stream} />
            </Section>
          )}

          {tab === 'observability' && (
            <Section id="observability" index="07" title="Observability"
              description="ClickHouse-backed traces, token usage, cost, models, and guardrail actions.">
              <ObservabilityView />
            </Section>
          )}
        </main>
      </div>

      <ChatOverlay userKey={userKey} />
    </div>
  );
}
