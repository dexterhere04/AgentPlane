import { useEffect, useRef, useState } from 'react';
import { ApiEvent, StreamState, initialState } from './types';
import { reduceStream, streamConnected } from './stream';
import { getAdminToken, setAdminToken, getUserKey, setUserKey } from './api';
import { createEventQueue, EventQueue, FlowMode } from './queue';
import { Icon, IconName } from './icons';
import { Section } from './components/ui';
import Overview from './components/Overview';
import RequestActivity from './components/RequestActivity';
import GuardrailsView from './components/GuardrailsView';
import UsageView from './components/UsageView';
import Diagnostics from './components/Diagnostics';
import ChatOverlay from './components/ChatOverlay';

interface SectionDef {
  id: string;
  index: string;
  label: string;
  icon: IconName;
  /** One-line purpose shown in the page header. */
  blurb: string;
  /** Optional grouping label rendered above the item in the sidebar. */
  group?: string;
}

/**
 * Primary navigation — five destinations, each with a single clear job.
 * Everything that used to be a top-level page is now either a tab
 * inside Request Activity, or a panel inside Diagnostics.
 */
const SECTIONS: SectionDef[] = [
  {
    id: 'overview',
    index: '01',
    label: 'Overview',
    icon: 'gauge',
    blurb: 'Health, throughput, and guardrail outcomes at a glance.'
  },
  {
    id: 'activity',
    index: '02',
    label: 'Request Activity',
    icon: 'route',
    blurb: 'Live pipeline, concurrency, and the raw event stream.'
  },
  {
    id: 'guardrails',
    index: '03',
    label: 'Guardrails',
    icon: 'shield',
    blurb: 'Send crafted payloads and inspect every guardrail decision.'
  },
  {
    id: 'usage',
    index: '04',
    label: 'Usage & Cost',
    icon: 'activity',
    blurb: 'Tokens, estimated spend, and per-user attribution.'
  },
  {
    id: 'diagnostics',
    index: '05',
    label: 'Diagnostics',
    icon: 'chart',
    blurb: 'Secrets, keys, and ClickHouse-backed analytics.'
  }
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

  const closeSidebarOnMobile = () => {
    if (window.innerWidth <= 860) {
      setSidebarOpen(false);
      localStorage.setItem('agentplane.sidebar', 'closed');
    }
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

  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth > 860 && !sidebarOpen) {
        if (localStorage.getItem('agentplane.sidebar') !== 'closed') {
          setSidebarOpen(true);
        }
      }
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [sidebarOpen]);

  const updateAdminToken = (v: string) => {
    setAdminTokenState(v);
    setAdminToken(v);
  };
  const updateUserKey = (v: string) => {
    setUserKeyState(v);
    setUserKey(v);
  };

  const active = SECTIONS.find((s) => s.id === tab) ?? SECTIONS[0];

  return (
    <div className={'app' + (sidebarOpen ? '' : ' collapsed')}>
      {sidebarOpen && <div className="sidebar-backdrop" onClick={closeSidebarOnMobile} aria-hidden="true" />}

      <aside className={`sidebar ${sidebarOpen ? 'open' : ''}`}>
        <div className="brand">
          <div className="brand-mark">A</div>
          <div>
            <div className="brand-name">AgentPlane</div>
            <div className="brand-sub">Control Plane</div>
          </div>
        </div>
        <nav className="side-nav" aria-label="Primary">
          <div className="nav-group-label">Workspace</div>
          {SECTIONS.map((s) => (
            <button
              key={s.id}
              className={'nav-item' + (tab === s.id ? ' active' : '')}
              onClick={() => {
                setTab(s.id);
                closeSidebarOnMobile();
              }}
              aria-current={tab === s.id ? 'page' : undefined}
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
          <button
            className="icon-btn menu-btn"
            onClick={toggleSidebar}
            title={sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
          >
            <Icon name="menu" size={17} />
          </button>

          <span className="crumb">
            AgentPlane <b>/</b> {active.label}
          </span>
          <div className="topbar-spacer" />
          <span className="lstat lstat-live">
            <span className={'live-dot ' + (stream.connected ? 'on' : 'off')} />
            {stream.connected ? 'Live' : 'Offline'}
          </span>
          <span className="lstat lstat-requests">Requests <b>{stream.requests.length}</b></span>
          <span className="lstat lstat-active">Active <b>{stream.active}</b></span>
          <span className="lstat lstat-blocked">Blocked <b>{stream.blocked}</b></span>
          <span className="lstat lstat-queue" title="Events buffered in the processing queue">
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
          <Section
            id={active.id}
            index={active.index}
            title={active.label}
            description={active.blurb}
          >
            {tab === 'overview' && <Overview stream={stream} />}

            {tab === 'activity' && (
              <RequestActivity
                stream={stream}
                flowMode={flowMode}
                onToggleFlow={toggleFlow}
                queueDepth={queueDepth}
                userKey={userKey}
              />
            )}

            {tab === 'guardrails' && (
              <GuardrailsView stream={stream} userKey={userKey} />
            )}

            {tab === 'usage' && (
              <UsageView userKey={userKey} onUserKeyChange={updateUserKey} stream={stream} />
            )}

            {tab === 'diagnostics' && (
              <Diagnostics />
            )}
          </Section>
        </main>
      </div>

      <ChatOverlay userKey={userKey} />
    </div>
  );
}