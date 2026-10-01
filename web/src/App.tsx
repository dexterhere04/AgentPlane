import { useEffect, useRef, useState } from 'react';
import { ApiEvent, StreamState, initialState } from './types';
import { reduceStream, streamConnected } from './stream';
import { getAdminToken, setAdminToken, getUserKey, setUserKey } from './api';
import { createEventQueue, EventQueue, FlowMode } from './queue';
import { resetSetup } from './setup';
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
import AccessControlView from './components/AccessControlView';
import ChatOverlay from './components/ChatOverlay';

interface SectionDef {
  id: string;
  label: string;
  eyebrow: string;
  icon: IconName;
}

const SECTIONS: Record<string, SectionDef> = {
  overview: { id: 'overview', label: 'Overview', eyebrow: 'Get started & system status', icon: 'gauge' },
  chat: { id: 'chat', label: 'Playground', eyebrow: 'Test a request', icon: 'play' },
  pipeline: { id: 'pipeline', label: 'Pipeline', eyebrow: 'Request flow', icon: 'route' },
  guardrails: { id: 'guardrails', label: 'Guardrails', eyebrow: 'Safety checks', icon: 'shield' },
  events: { id: 'events', label: 'Event Log', eyebrow: 'Live event stream', icon: 'list' },
  usage: { id: 'usage', label: 'Usage', eyebrow: 'Per-key attribution', icon: 'activity' },
  observability: { id: 'observability', label: 'Analytics', eyebrow: 'Traces, tokens & cost', icon: 'chart' },
  keys: { id: 'keys', label: 'Keys & Vault', eyebrow: 'Provider secrets & API keys', icon: 'key' },
  access: { id: 'access', label: 'Access Control', eyebrow: 'Roles & permissions', icon: 'users' }
};

const NAV_GROUPS: { label: string; items: string[] }[] = [
  { label: 'Run', items: ['overview', 'chat', 'pipeline'] },
  { label: 'Safety', items: ['guardrails'] },
  { label: 'Observe', items: ['events', 'usage', 'observability'] },
  { label: 'Admin', items: ['keys', 'access'] }
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
    () => localStorage.getItem('agentplane.sidebar') !== 'closed'
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

    const es = new EventSource('/events?token=' + encodeURIComponent(getAdminToken()));
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

  const active = SECTIONS[tab] ?? SECTIONS.overview;

  return (
    <div className={'app' + (sidebarOpen ? '' : ' collapsed')}>
      <header className="topbar">
        <button
          className="icon-btn chrome-menu"
          onClick={toggleSidebar}
          title={sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
          aria-label="Toggle navigation"
        >
          <Icon name="menu" size={17} />
        </button>
        <div className="brand">
          <div className="brand-mark">A</div>
          <div className="brand-text">
            <div className="brand-name">AgentPlane</div>
            <div className="brand-sub">Gateway Control Plane</div>
          </div>
        </div>
        <span className="crumb">
          <Icon name="chevron" size={13} />
          {active.label}
        </span>
        <div className="topbar-spacer" />
        <span
          className={'status-pill ' + (stream.connected ? 'live' : 'offline')}
          title={stream.connected ? 'Connected to the gateway event stream' : 'Event stream offline'}
        >
          <span className="status-dot" />
          {stream.connected ? 'Live' : 'Offline'}
        </span>
        <button className="icon-btn chrome-btn" onClick={toggleTheme} title="Toggle light / dark">
          <Icon name={theme === 'light' ? 'moon' : 'sun'} size={16} />
        </button>
        <button
          className={'icon-btn chrome-btn' + (showSettings ? ' is-active' : '')}
          onClick={() => setShowSettings((v) => !v)}
          title="Connection & credentials"
        >
          <Icon name="settings" size={16} />
        </button>
      </header>

      <aside className="sidebar">
        <nav className="side-nav">
          {NAV_GROUPS.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-label">{group.label}</div>
              {group.items.map((id) => {
                const s = SECTIONS[id];
                return (
                  <button
                    key={s.id}
                    className={'nav-item' + (tab === s.id ? ' active' : '')}
                    onClick={() => setTab(s.id)}
                  >
                    <span className="nav-icon"><Icon name={s.icon} size={16} /></span>
                    {s.label}
                  </button>
                );
              })}
            </div>
          ))}
        </nav>
        <div className="side-foot">
          <span className="side-foot-status">
            <span className={'status-dot ' + (stream.connected ? 'on' : 'off')} />
            {stream.connected ? 'Gateway live' : 'Gateway offline'}
          </span>
          <span>AgentPlane · v0.1.0</span>
        </div>
      </aside>

      <div className="shell">
        {showSettings && (
          <div className="settings-panel">
            <div className="settings-field">
              <label htmlFor="admin-token">Admin token</label>
              <input
                id="admin-token"
                className="input mono"
                type="password"
                value={adminToken}
                onChange={(e) => updateAdminToken(e.target.value)}
                placeholder="AGENTPLANE_ADMIN_TOKEN"
              />
            </div>
            <div className="settings-field">
              <label htmlFor="user-key">User API key</label>
              <input
                id="user-key"
                className="input mono"
                type="password"
                value={userKey}
                onChange={(e) => updateUserKey(e.target.value)}
                placeholder="ap_live_..."
              />
            </div>
            <div className="settings-actions">
              <button
                className="btn ghost sm"
                onClick={() => {
                  resetSetup();
                  setShowSettings(false);
                }}
                title="Show the getting-started guide again"
              >
                <Icon name="refresh" size={14} />
                Reset setup guide
              </button>
            </div>
            <span className="settings-hint">
              <Icon name="lock" size={12} />
              Stored locally in your browser. The admin token guards provisioning, Vault writes &amp; analytics.
            </span>
          </div>
        )}

        <main className="content">
          {tab === 'overview' && (
            <Section id="overview" eyebrow="Get started & system status" title="Overview"
              description="Your setup progress, live gateway telemetry, and system status at a glance.">
              <Overview
                stream={stream}
                adminToken={adminToken}
                onNavigate={setTab}
                onOpenSettings={() => setShowSettings(true)}
              />
            </Section>
          )}

          {tab === 'chat' && (
            <Section id="chat" eyebrow="Test a request" title="Playground"
              description="Send a prompt like a real client app — auth, guardrails and the provider round-trip, transparently.">
              <ChatPanel userKey={userKey} onUserKeyChange={updateUserKey} />
            </Section>
          )}

          {tab === 'pipeline' && (
            <Section id="pipeline" eyebrow="Request flow" title="Pipeline"
              description="Every stage a request passes through, live concurrency, and per-request latency.">
              <PipelineView stream={stream} flowMode={flowMode} onToggleFlow={toggleFlow} queueDepth={queueDepth} userKey={userKey} />
            </Section>
          )}

          {tab === 'guardrails' && (
            <Section id="guardrails" eyebrow="Safety checks" title="Guardrails"
              description="Send crafted payloads and inspect before/after processing for every guardrail decision.">
              <GuardrailsView stream={stream} userKey={userKey} />
            </Section>
          )}

          {tab === 'events' && (
            <Section id="events" eyebrow="Live event stream" title="Event Log"
              description="The raw SSE stream — every stage, decision and error the gateway emits.">
              <EventLog stream={stream} />
            </Section>
          )}

          {tab === 'keys' && (
            <Section id="keys" eyebrow="Provider secrets & API keys" title="Keys & Vault"
              description="Store provider credentials in Vault, then provision and revoke user API keys.">
              <VaultView />
            </Section>
          )}

          {tab === 'usage' && (
            <Section id="usage" eyebrow="Per-key attribution" title="Usage"
              description="Verify a user key, see the identity it resolves to, and every request attributed to it.">
              <UsageView userKey={userKey} onUserKeyChange={updateUserKey} stream={stream} />
            </Section>
          )}

          {tab === 'observability' && (
            <Section id="observability" eyebrow="Traces, tokens & cost" title="Analytics"
              description="ClickHouse-backed traces, token usage, cost, models and guardrail actions.">
              <ObservabilityView />
            </Section>
          )}

          {tab === 'access' && (
            <Section id="access" eyebrow="Roles & permissions" title="Access Control"
              description="Manage RBAC roles and permissions, and assign roles to users. Access is deny-by-default.">
              <AccessControlView />
            </Section>
          )}
        </main>
      </div>

      <ChatOverlay userKey={userKey} />
    </div>
  );
}
