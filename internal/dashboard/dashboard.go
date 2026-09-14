package dashboard

const HTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>AgentPlane</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#07080d;
  --surface:#0d1018;
  --surface2:#121620;
  --border:#1c2030;
  --border-light:#262c3c;
  --text:#c4ccdc;
  --text-dim:#49516b;
  --text-bright:#e8ecf6;
  --accent:#6888f0;
  --accent-dim:rgba(104,136,240,0.12);
  --green:#4cb868;
  --green-dim:rgba(76,184,104,0.1);
  --amber:#d4a030;
  --amber-dim:rgba(212,160,48,0.12);
  --red:#e05050;
  --red-dim:rgba(224,80,80,0.1);
  --purple:#9878d8;
  --cyan:#3cb8c8;
}
body{
  font-family:'JetBrains Mono','Cascadia Code','Fira Code','SF Mono','Consolas','Menlo',monospace;
  background:var(--bg);
  background-image:
    radial-gradient(ellipse at 50% 0%,rgba(104,136,240,0.04) 0%,transparent 55%),
    radial-gradient(circle at 1px 1px,rgba(255,255,255,0.018) 1px,transparent 1px);
  background-size:100% 100%,18px 18px;
  color:var(--text);
  min-height:100vh;
  -webkit-font-smoothing:antialiased;
}
header{
  display:flex;align-items:center;justify-content:space-between;
  padding:14px 28px;border-bottom:1px solid var(--border);
  background:rgba(13,16,24,0.85);
  -webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px);
  position:sticky;top:0;z-index:20;
}
.logo{font-size:16px;font-weight:700;color:var(--text-bright);letter-spacing:-0.01em}
.logo i{color:var(--accent);font-style:normal}
.header-right{display:flex;gap:20px;align-items:center;font-size:12px;color:var(--text-dim)}
.header-right .stat{display:flex;align-items:center;gap:6px}
.header-right .stat-val{color:var(--text);font-weight:600;font-variant-numeric:tabular-nums}
.status-dot{width:7px;height:7px;border-radius:50%;display:inline-block}
.status-dot.on{background:var(--green);box-shadow:0 0 6px var(--green)}
.status-dot.off{background:var(--red);box-shadow:0 0 6px var(--red)}
main{padding:20px 28px 40px;max-width:1440px;margin:0 auto}
.stats-bar{display:grid;grid-template-columns:repeat(5,1fr);gap:12px;margin-bottom:18px}
.stat-card{
  background:var(--surface);border:1px solid var(--border);border-radius:8px;
  padding:14px 18px;display:flex;flex-direction:column;gap:4px;transition:border-color 0.25s;
}
.stat-card:hover{border-color:var(--border-light)}
.stat-card .stat-label{font-size:10px;text-transform:uppercase;letter-spacing:0.08em;color:var(--text-dim);font-weight:600}
.stat-card .stat-value{font-size:22px;font-weight:700;color:var(--text-bright);font-variant-numeric:tabular-nums;letter-spacing:-0.02em}
.stat-card .stat-sub{font-size:10px;color:var(--text-dim);margin-top:2px}
.block-stat{border-color:var(--red);background:rgba(224,80,80,0.05)}
.block-stat .stat-value{color:var(--red)}

.pipeline-card{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
  padding:20px 24px;margin-bottom:18px;
}
.pipeline-card h2{font-size:11px;text-transform:uppercase;letter-spacing:0.1em;color:var(--text-dim);margin-bottom:14px;font-weight:600}
.pipeline{
  display:flex;align-items:flex-start;gap:0;overflow-x:auto;padding:8px 0 4px;
}
.stage-wrap{display:flex;align-items:center;flex-shrink:0}
.stage{
  width:110px;padding:10px 4px;border-radius:8px;border:1px solid var(--border);
  background:var(--surface2);text-align:center;transition:all 0.35s cubic-bezier(0.4,0,0.2,1);
  position:relative;overflow:hidden;
}
.stage .icon{font-size:16px;margin-bottom:4px;display:block;transition:transform 0.35s}
.stage .label{font-size:9px;font-weight:600;color:var(--text-dim);transition:color 0.3s}
.stage .detail{font-size:8px;color:var(--text-dim);min-height:10px}
.stage .time{font-size:8px;color:var(--accent);opacity:0;margin-top:2px}
.stage.completed .time{opacity:1}
@keyframes blinkPulse{
  0%{box-shadow:0 0 0 rgba(212,160,48,0);border-color:rgba(212,160,48,0.15)}
  30%{box-shadow:0 0 18px rgba(212,160,48,0.45);border-color:rgba(212,160,48,0.7)}
  100%{box-shadow:0 0 0 rgba(212,160,48,0);border-color:rgba(212,160,48,0)}
}
.stage.in-progress{animation:blinkPulse 0.15s ease-out;border-color:rgba(212,160,48,0.7)}
.stage.in-progress .label{color:var(--amber)}
.stage.completed{border-color:rgba(76,184,104,0.25);background:rgba(76,184,104,0.04)}
.stage.completed .label{color:var(--green)}
.stage.error{border-color:rgba(224,80,80,0.3);background:rgba(224,80,80,0.04)}
.stage.error .label{color:var(--red)}
.connector{
  width:16px;height:1.5px;background:var(--border);flex-shrink:0;
  align-self:center;margin:0 2px;
}
.connector.flow{background:var(--accent);box-shadow:0 0 6px rgba(104,136,240,0.5)}

.chat-section{margin-bottom:18px;display:grid;grid-template-columns:2fr 1fr;gap:16px}

.chat-panel{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
  display:flex;flex-direction:column;min-height:420px;
}
.chat-panel h2{
  font-size:11px;text-transform:uppercase;letter-spacing:0.1em;color:var(--text-dim);
  font-weight:600;padding:16px 20px 12px;border-bottom:1px solid var(--border);
}
.chat-messages{
  flex:1;overflow-y:auto;padding:14px 18px;display:flex;flex-direction:column;gap:10px;
  max-height:400px;
}
.chat-msg{
  max-width:90%;padding:10px 14px;border-radius:8px;font-size:11px;line-height:1.55;
  word-break:break-word;animation:slideIn 0.25s ease-out;
}
.chat-msg.user{
  align-self:flex-end;background:var(--accent-dim);border:1px solid rgba(104,136,240,0.2);
  color:var(--text-bright);
}
.chat-msg.assistant{
  align-self:flex-start;background:var(--surface2);border:1px solid var(--border-light);
  color:var(--text);
}
.chat-msg.error{
  align-self:flex-start;background:var(--red-dim);border:1px solid rgba(224,80,80,0.3);
  color:var(--red);
}
.chat-msg .msg-tag{
  font-size:8px;text-transform:uppercase;letter-spacing:0.06em;color:var(--text-dim);
  margin-bottom:4px;font-weight:700;
}
.chat-input-wrap{
  border-top:1px solid var(--border);padding:12px 16px;display:flex;gap:8px;
}
.chat-input-wrap input{
  flex:1;padding:9px 12px;background:var(--bg);border:1px solid var(--border);
  border-radius:6px;color:var(--text);font-family:inherit;font-size:12px;
  transition:border-color 0.2s,box-shadow 0.2s;outline:none;
}
.chat-input-wrap input:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-dim)}
.chat-input-wrap button{
  padding:9px 18px;background:var(--accent);border:none;border-radius:6px;
  color:#fff;font-weight:600;font-family:inherit;font-size:12px;cursor:pointer;
  transition:background 0.2s,box-shadow 0.2s;outline:none;letter-spacing:0.03em;
}
.chat-input-wrap button:hover{background:#5a78e0;box-shadow:0 2px 12px rgba(104,136,240,0.3)}
.chat-input-wrap button:active{transform:scale(0.98)}
.chat-input-wrap button:disabled{opacity:0.5;cursor:not-allowed}

.guardrail-summary{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
  display:flex;flex-direction:column;min-height:420px;
}
.guardrail-summary h2{
  font-size:11px;text-transform:uppercase;letter-spacing:0.1em;color:var(--text-dim);
  font-weight:600;padding:16px 20px 12px;border-bottom:1px solid var(--border);
  display:flex;justify-content:space-between;align-items:center;
}
.guardrail-summary h2 .dir-badge{
  font-size:9px;background:var(--accent-dim);color:var(--accent);padding:2px 8px;
  border-radius:3px;font-weight:600;
}
.guardrail-list{
  flex:1;overflow-y:auto;padding:10px 16px;display:flex;flex-direction:column;gap:6px;
  max-height:400px;
}
.guardrail-row{
  display:flex;align-items:center;gap:10px;padding:8px 10px;
  border-radius:6px;border:1px solid var(--border);background:var(--surface2);
  font-size:10px;animation:slideIn 0.2s ease-out;
}
.guardrail-row .gr-indicator{
  width:8px;height:8px;border-radius:50%;flex-shrink:0;
}
.gr-pass{background:var(--green);box-shadow:0 0 6px var(--green)}
.gr-block{background:var(--red);box-shadow:0 0 6px var(--red)}
.gr-warn{background:var(--amber);box-shadow:0 0 6px var(--amber)}
.gr-redact{background:var(--purple);box-shadow:0 0 6px var(--purple)}
.gr-pending{background:var(--border-light)}
.guardrail-row .gr-name{font-weight:600;color:var(--text-bright);min-width:100px}
.guardrail-row .gr-decision{font-weight:600;min-width:50px}
.guardrail-row .gr-msg{color:var(--text-dim);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.guardrail-row .gr-time{color:var(--text-dim);font-size:9px;white-space:nowrap}
.dec-pass{color:var(--green)}
.dec-block{color:var(--red)}
.dec-warn{color:var(--amber)}
.dec-redact{color:var(--purple)}

.guardrails-grid{
  display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:18px;
}
.guardrail-panel{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
  overflow:hidden;
}
.guardrail-panel h2{
  font-size:11px;text-transform:uppercase;letter-spacing:0.1em;color:var(--text-dim);
  font-weight:600;padding:14px 18px 10px;border-bottom:1px solid var(--border);
  display:flex;justify-content:space-between;align-items:center;
}
.guardrail-panel h2 .panel-count{
  font-size:9px;color:var(--text-dim);font-weight:400;
}
.guardrail-panel .panel-items{
  padding:10px 14px;max-height:280px;overflow-y:auto;
}
.guardrail-item{
  padding:10px 12px;margin-bottom:6px;border-radius:6px;
  border:1px solid var(--border);background:var(--surface2);
  font-size:10px;animation:slideIn 0.2s ease-out;
}
.guardrail-item .gi-header{
  display:flex;align-items:center;gap:8px;margin-bottom:4px;
}
.guardrail-item .gi-dot{width:7px;height:7px;border-radius:50%;flex-shrink:0}
.gi-pass .gi-dot{background:var(--green)}
.gi-block .gi-dot{background:var(--red)}
.gi-warn .gi-dot{background:var(--amber)}
.gi-redact .gi-dot{background:var(--purple)}
.gi-block{border-color:rgba(224,80,80,0.3);background:rgba(224,80,80,0.04)}
.guardrail-item .gi-name{font-weight:700;color:var(--text-bright)}
.guardrail-item .gi-decision{font-weight:600;margin-left:auto}
.guardrail-item .gi-msg{color:var(--text-dim)}
.guardrail-item .gi-findings{margin-top:6px}
.guardrail-item .gi-finding{
  padding:4px 8px;margin:3px 0;border-radius:3px;background:var(--bg);
  border:1px solid var(--border);font-size:9px;
  display:flex;align-items:center;gap:6px;
}
.guardrail-item .gi-finding .gf-sev{font-weight:600;text-transform:uppercase;font-size:8px}
.sev-critical{color:var(--red)}
.sev-high{color:var(--amber)}
.sev-medium{color:var(--amber)}
.sev-low{color:var(--text-dim)}
.sev-info{color:var(--text-dim)}
.guardrail-item .gi-finding .gf-type{color:var(--accent);font-weight:600}
.guardrail-item .gi-finding .gf-entity{color:var(--text-dim)}

.event-section{margin-bottom:18px}
.event-card{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
}
.event-card h2{
  font-size:11px;text-transform:uppercase;letter-spacing:0.1em;color:var(--text-dim);
  font-weight:600;padding:14px 18px 10px;border-bottom:1px solid var(--border);
}
.event-log{max-height:320px;overflow-y:auto;font-size:11px;padding:0 14px 10px}
.event-row{
  display:flex;gap:10px;padding:5px 8px;
  border-bottom:1px solid rgba(28,32,48,0.5);
  align-items:baseline;transition:background 0.2s;border-radius:3px;
  font-family:'JetBrains Mono','Cascadia Code','Fira Code','SF Mono','Consolas','Menlo',monospace;
}
.event-row:hover{background:rgba(255,255,255,0.015)}
.event-row:first-child{animation:slideIn 0.3s ease-out}
.event-time{color:var(--text-dim);white-space:nowrap;min-width:72px;font-size:10px}
.event-stage{min-width:105px;white-space:nowrap;font-weight:600;font-size:10px}
.event-status{min-width:60px}
.event-msg{color:var(--text-dim);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.event-stage.s-GUARD{color:var(--purple)}
.event-stage.s-BLOCKED{color:var(--red)}
.event-stage.s-PASSED{color:var(--green)}
.event-stage.s-RECD{color:var(--accent)}
.event-stage.s-READ{color:var(--purple)}
.event-stage.s-VALID{color:var(--cyan)}
.event-stage.s-PROXY{color:var(--amber)}
.event-stage.s-DONE{color:var(--green)}
.event-stage.s-ERROR{color:var(--red)}
.badge{
  display:inline-block;padding:1px 7px;border-radius:3px;font-size:9px;
  font-weight:700;text-transform:uppercase;letter-spacing:0.04em;
}
.badge.started{background:var(--amber-dim);color:var(--amber)}
.badge.completed{background:var(--green-dim);color:var(--green)}
.badge.block{background:var(--red-dim);color:var(--red)}
.badge.error{background:var(--red-dim);color:var(--red)}
.badge.info{background:var(--accent-dim);color:var(--accent)}
.badge.warn{background:var(--amber-dim);color:var(--amber)}
.badge.pass{background:var(--green-dim);color:var(--green)}
.empty-state{text-align:center;padding:30px 20px;color:var(--text-dim);font-size:12px}
.empty-state .em{font-size:28px;margin-bottom:8px;display:block;opacity:0.5}
@keyframes slideIn{
  from{opacity:0;transform:translateY(-6px)}
  to{opacity:1;transform:translateY(0)}
}
::-webkit-scrollbar{width:5px;height:5px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:var(--border-light);border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:var(--text-dim)}
@media(max-width:900px){
  .stats-bar{grid-template-columns:repeat(2,1fr)}
  .chat-section{grid-template-columns:1fr}
  .guardrails-grid{grid-template-columns:1fr}
}

/* Dashboard shell / information architecture */
.app-shell{display:grid;grid-template-columns:220px minmax(0,1fr);min-height:calc(100vh - 54px)}
.sidebar{border-right:1px solid var(--border);background:var(--surface);padding:18px 12px;position:sticky;top:54px;height:calc(100vh - 54px);align-self:start}
.sidebar-label{padding:0 10px 8px;font-size:9px;text-transform:uppercase;letter-spacing:.12em;color:var(--text-dim);font-weight:700}
.primary-nav{display:flex;flex-direction:column;gap:3px}
.nav-item{appearance:none;width:100%;border:1px solid transparent;background:transparent;color:var(--text-dim);font:600 12px/1.2 inherit;text-align:left;padding:10px;border-radius:7px;cursor:pointer;display:flex;align-items:center;gap:10px;outline:none}
.nav-item:hover{background:var(--surface2);color:var(--text)}
.nav-item:focus-visible{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-dim);color:var(--text-bright)}
.nav-item.active{background:var(--accent-dim);border-color:rgba(104,136,240,.2);color:var(--text-bright)}
.nav-icon{width:18px;text-align:center;color:inherit;font-size:13px}
.content-area{min-width:0}
.page-section{display:none}
.page-section.active{display:block}
.page-header{display:flex;justify-content:space-between;align-items:flex-start;gap:20px;margin-bottom:20px}
.page-eyebrow{font-size:10px;text-transform:uppercase;letter-spacing:.1em;color:var(--text-dim);font-weight:700;margin-bottom:5px}
.page-title{font-family:inherit;font-size:22px;line-height:1.2;letter-spacing:-.02em;color:var(--text-bright);font-weight:700}
.page-description{margin-top:6px;color:var(--text-dim);font-size:11px;line-height:1.5;max-width:650px}
.section-stack{display:flex;flex-direction:column}
.section-note{font-size:11px;color:var(--text-dim);padding:28px;border:1px dashed var(--border-light);border-radius:10px;background:var(--surface);text-align:center}
@media(max-width:900px){
  .app-shell{grid-template-columns:1fr}
  .sidebar{position:sticky;top:54px;height:auto;border-right:0;border-bottom:1px solid var(--border);padding:8px 12px;z-index:15}
  .sidebar-label{display:none}
  .primary-nav{flex-direction:row;overflow-x:auto;padding-bottom:1px}
  .nav-item{width:auto;white-space:nowrap;padding:9px 11px}
  .nav-icon{display:none}
}
@media(max-width:600px){
  main{padding:16px 14px 30px}
  .stats-bar{grid-template-columns:1fr 1fr;gap:8px}
  .page-title{font-size:19px}
  .page-description{font-size:10px}
}
</style>
</head>
<body>
<header>
  <div class="logo"><i>A</i>gentPlane</div>
  <div class="header-right">
    <span id="connection-status"><span class="status-dot off"></span> Offline</span>
    <span class="stat">Req<span class="stat-val" id="request-count">0</span></span>
    <span class="stat">Block<span class="stat-val" id="block-count" style="color:var(--red)">0</span></span>
    <span class="stat">Evt<span class="stat-val" id="event-count">0</span></span>
  </div>
</header>
<div class="app-shell">
  <aside class="sidebar" aria-label="Dashboard navigation">
    <div class="sidebar-label">Workspace</div>
    <nav class="primary-nav" id="primary-nav">
      <button class="nav-item active" type="button" data-section="overview" aria-current="page"><span class="nav-icon">⌂</span>Overview</button>
      <button class="nav-item" type="button" data-section="requests"><span class="nav-icon">↗</span>Requests</button>
      <button class="nav-item" type="button" data-section="guardrails"><span class="nav-icon">◇</span>Guardrails</button>
      <button class="nav-item" type="button" data-section="usage"><span class="nav-icon">▥</span>Usage</button>
      <button class="nav-item" type="button" data-section="diagnostics"><span class="nav-icon">⌁</span>Diagnostics</button>
    </nav>
  </aside>

  <div class="content-area">
    <main>
      <section class="page-section active" id="section-overview" data-page-section="overview" aria-labelledby="overview-title">
        <div class="page-header">
          <div>
            <div class="page-eyebrow">Operations</div>
            <h1 class="page-title" id="overview-title">Overview</h1>
            <p class="page-description">Monitor request health, guardrail outcomes, and recent activity from a single operational view.</p>
          </div>
        </div>

        <div class="stats-bar">
          <div class="stat-card">
            <span class="stat-label">Requests</span><span class="stat-value" id="stat-requests">0</span><span class="stat-sub">total tracked</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Active</span><span class="stat-value" id="stat-active">0</span><span class="stat-sub">in progress</span>
          </div>
          <div class="stat-card block-stat">
            <span class="stat-label">Blocked</span><span class="stat-value" id="stat-blocked">0</span><span class="stat-sub">guardrail blocks</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Events</span><span class="stat-value" id="stat-events">0</span><span class="stat-sub">streamed via SSE</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Last Latency</span><span class="stat-value" id="stat-latency">--</span><span class="stat-sub">request duration</span>
          </div>
        </div>

        <div id="overview-pipeline-slot"></div>
        <div id="overview-activity-slot"></div>
      </section>

      <section class="page-section" id="section-requests" data-page-section="requests" aria-labelledby="requests-title">
        <div class="page-header">
          <div>
            <div class="page-eyebrow">Traffic</div>
            <h1 class="page-title" id="requests-title">Requests</h1>
            <p class="page-description">Inspect the current request pipeline and interact with the gateway through the existing request surface.</p>
          </div>
        </div>
        <div id="requests-pipeline-slot"></div>
        <div id="requests-chat-slot"></div>
      </section>

      <section class="page-section" id="section-guardrails" data-page-section="guardrails" aria-labelledby="guardrails-title">
        <div class="page-header">
          <div>
            <div class="page-eyebrow">Security</div>
            <h1 class="page-title" id="guardrails-title">Guardrails</h1>
            <p class="page-description">Review input and output policy decisions, including blocked, warned, redacted, and passed checks.</p>
          </div>
        </div>
        <div id="guardrails-summary-slot"></div>
        <div id="guardrails-panels-slot"></div>
      </section>

      <section class="page-section" id="section-usage" data-page-section="usage" aria-labelledby="usage-title">
        <div class="page-header">
          <div>
            <div class="page-eyebrow">Capacity</div>
            <h1 class="page-title" id="usage-title">Usage</h1>
            <p class="page-description">Usage and cost analytics will be surfaced here when the existing telemetry provides those metrics.</p>
          </div>
        </div>
        <div class="section-note">No usage or cost metrics are currently exposed by the dashboard data stream.</div>
      </section>

      <section class="page-section" id="section-diagnostics" data-page-section="diagnostics" aria-labelledby="diagnostics-title">
        <div class="page-header">
          <div>
            <div class="page-eyebrow">Observability</div>
            <h1 class="page-title" id="diagnostics-title">Diagnostics</h1>
            <p class="page-description">Follow the live event stream and inspect request lifecycle activity as it arrives.</p>
          </div>
        </div>
        <div id="diagnostics-events-slot"></div>
      </section>
    </main>
  </div>
</div>

<!-- Existing operational surfaces are placed into the navigation sections below. -->
<div id="legacy-surfaces" style="display:none">
  <div class="pipeline-card" id="pipeline-card">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px">
      <h2 style="margin:0">Request Pipeline</h2>
      <span style="font-size:10px;color:var(--text-dim)" id="pipeline-req-id"></span>
    </div>
    <div id="pipeline-container" class="pipeline"></div>
  </div>

  <div class="chat-section" id="chat-section">
    <div class="chat-panel" id="chat-panel">
      <h2>Chat</h2>
      <div class="chat-messages" id="chat-messages">
        <div class="empty-state"><span class="em">&#x1F4AC;</span>Send a message to get started</div>
      </div>
      <div class="chat-input-wrap">
        <input type="text" id="chat-input" placeholder="Type a message..." value="Hello, how are you?" autocomplete="off" aria-label="Chat message" onkeydown="if(event.key==='Enter')sendChatMessage()">
        <button onclick="sendChatMessage()" id="chat-send-btn" type="button">Send</button>
      </div>
    </div>

    <div class="guardrail-summary" id="gr-summary">
      <h2>Guardrails <span class="dir-badge" id="gr-current-dir">waiting</span></h2>
      <div class="guardrail-list" id="gr-summary-list">
        <div class="empty-state"><span class="em">&#x1F6E1;</span>No guardrail data yet</div>
      </div>
    </div>
  </div>

  <div class="guardrails-grid" id="guardrails-grid">
    <div class="guardrail-panel">
      <h2>Input Guardrails <span class="panel-count" id="input-gr-count">0 checks</span></h2>
      <div class="panel-items" id="input-gr-items"><div class="empty-state"><span class="em">&#x2B06;</span>Awaiting request</div></div>
    </div>
    <div class="guardrail-panel">
      <h2>Output Guardrails <span class="panel-count" id="output-gr-count">0 checks</span></h2>
      <div class="panel-items" id="output-gr-items"><div class="empty-state"><span class="em">&#x2B07;</span>Awaiting response</div></div>
    </div>
  </div>

  <div class="event-section" id="event-section">
    <div class="event-card">
      <h2>Live Events</h2>
      <div class="event-log" id="event-log"><div class="empty-state" style="padding:20px">Waiting for events&#8230;</div></div>
    </div>
  </div>
</div>

<script>
var STAGES=[
  {id:'request_received',label:'RECEIVED',icon:'\u2B06',short:'RECD'},
  {id:'body_read',label:'BODY READ',icon:'\uD83D\uDCE5',short:'READ'},
  {id:'json_validated',label:'VALIDATED',icon:'\u2705',short:'VALD'},
  {id:'loading_api_key',label:'API KEY',icon:'\uD83D\uDD11',short:'AUTH'},
  {id:'building_request',label:'BUILD REQ',icon:'\uD83D\uDD27',short:'BLD'},
  {id:'setting_headers',label:'HEADERS',icon:'\uD83D\uDCCB',short:'HDRS'},
  {id:'sending_request',label:'SENDING',icon:'\uD83D\uDE80',short:'SEND'},
  {id:'reading_response',label:'READ RES',icon:'\uD83D\uDCE6',short:'RRES'},
  {id:'validating_status',label:'STATUS',icon:'\u2714',short:'STAT'},
  {id:'response_sent',label:'DONE',icon:'\uD83C\uDF89',short:'DONE'}
];

var bus=new EventSource('/events');
var eventCount=0,requestCount=0,activeCount=0,blockCount=0;
var currentRequestId=null;
var requests={};
var lastRequestTimestamps={};
var blinkQueue=[],blinking=false;

var chatMessages=[];
var currentInputGuards=[];
var currentOutputGuards=[];

bus.onopen=function(){
  document.getElementById('connection-status').innerHTML='<span class="status-dot on"></span> Online';
};
bus.onerror=function(){
  document.getElementById('connection-status').innerHTML='<span class="status-dot off"></span> Offline';
};
bus.onmessage=function(e){
  var evt=JSON.parse(e.data);
  handleEvent(evt);
};

function handleEvent(evt){
  eventCount++;
  document.getElementById('event-count').textContent=eventCount;
  document.getElementById('stat-events').textContent=eventCount;

  var stageName=STAGES.find(function(s){return s.id===evt.stage});
  var shortName=stageName?stageName.short:evt.stage;

  if(!requests[evt.request_id]){
    requests[evt.request_id]={id:evt.request_id,events:[],startTime:evt.timestamp,ended:false};
    lastRequestTimestamps[evt.request_id]=evt.timestamp;
    requestCount++;
    activeCount++;
    document.getElementById('request-count').textContent=requestCount;
    document.getElementById('stat-requests').textContent=requestCount;
    document.getElementById('stat-active').textContent=activeCount;
  }

  requests[evt.request_id].events.push(evt);
  lastRequestTimestamps[evt.request_id]=evt.timestamp;

  if(evt.stage==='response_sent'&&(evt.status==='completed'||evt.status==='error')){
    if(!requests[evt.request_id].ended){
      requests[evt.request_id].ended=true;
      activeCount=Math.max(0,activeCount-1);
      document.getElementById('stat-active').textContent=activeCount;
    }
    var latency=evt.duration||(evt.timestamp-requests[evt.request_id].startTime);
    if(latency)document.getElementById('stat-latency').textContent=latency+'ms';
  }
  if(evt.stage==='error'){
    if(!requests[evt.request_id].ended){
      requests[evt.request_id].ended=true;
      activeCount=Math.max(0,activeCount-1);
      document.getElementById('stat-active').textContent=activeCount;
    }
  }

  if(!currentRequestId||currentRequestId===evt.request_id){
    updatePipeline(evt);
  }
  addEventLog(evt,shortName);

  if(evt.stage==='body_read'&&evt.status==='completed'&&evt.data){
    try{
      var data=JSON.parse(evt.data);
      var content=data.messages?data.messages[data.messages.length-1].content:'';
      if(content)addChatMessage('user',content);
    }catch(_){}
    resetGuardrails(evt.request_id);
  }

  if(evt.stage==='guardrail_input'&&evt.request_id===currentRequestId){
    handleGuardrailEvent(evt,'input');
  }
  if(evt.stage==='guardrail_output'&&evt.request_id===currentRequestId){
    handleGuardrailEvent(evt,'output');
  }
  if(evt.stage==='guardrail_blocked'){
    blockCount++;
    document.getElementById('block-count').textContent=blockCount;
    document.getElementById('stat-blocked').textContent=blockCount;
    handleGuardrailBlocked(evt);
  }
  if(evt.stage==='guardrail_passed'){
    handleGuardrailPassed(evt);
  }

  if(evt.stage==='response_sent'&&evt.status==='completed'&&evt.data){
    try{
      var resp=JSON.parse(evt.data);
      var reply=resp.choices?resp.choices[0].message.content:'';
      if(reply)addChatMessage('assistant',reply);
    }catch(_){}
  }
  if(evt.stage==='response_sent'&&evt.status==='error'||evt.stage==='error'){
    if(evt.message)addChatMessage('error',evt.message);
  }
}

function resetGuardrails(reqId){
  currentRequestId=reqId;
  document.getElementById('pipeline-req-id').textContent='#'+reqId.replace('req-','').slice(-8);
  currentInputGuards=[];
  currentOutputGuards=[];
  document.getElementById('input-gr-items').innerHTML='<div class="empty-state"><span class="em">&#x2B06;</span>Scanning...</div>';
  document.getElementById('output-gr-items').innerHTML='<div class="empty-state"><span class="em">&#x2B07;</span>Awaiting response</div>';
  document.getElementById('input-gr-count').textContent='0 checks';
  document.getElementById('output-gr-count').textContent='0 checks';
  document.getElementById('gr-summary-list').innerHTML='<div class="empty-state"><span class="em">&#x1F6E1;</span>Scanning input...</div>';
  document.getElementById('gr-current-dir').textContent='input';
  buildPipeline();
}

function handleGuardrailEvent(evt,dir){
  document.getElementById('gr-current-dir').textContent=dir;

  var name=extractGuardName(evt.message);
  var decision=extractDecision(evt.message);
  var msg=evt.message||'';

  var item=document.createElement('div');
  var decClass=decision==='block'?'gi-block':decision==='warn'?'gi-warn':decision==='redact'?'gi-redact':'gi-pass';
  item.className='guardrail-item '+decClass;
  item.innerHTML=buildGuardrailItemHTML(name,decision,msg,evt.timestamp);

  var panelId=dir==='input'?'input-gr-items':'output-gr-items';
  var panel=document.getElementById(panelId);
  var empty=panel.querySelector('.empty-state');
  if(empty)panel.innerHTML='';
  panel.insertBefore(item,panel.firstChild);

  if(dir==='input'){
    currentInputGuards.push({name:name,decision:decision,time:evt.timestamp});
    document.getElementById('input-gr-count').textContent=currentInputGuards.length+' checks';
  }else{
    currentOutputGuards.push({name:name,decision:decision,time:evt.timestamp});
    document.getElementById('output-gr-count').textContent=currentOutputGuards.length+' checks';
  }
  updateSummaryList();
}

function handleGuardrailBlocked(evt){
  var name=extractGuardName(evt.message);
  var item=document.createElement('div');
  item.className='guardrail-item gi-block';
  item.innerHTML=buildGuardrailItemHTML(name,'block',evt.message||'',evt.timestamp);
  var panel=document.getElementById('input-gr-items');
  var empty=panel.querySelector('.empty-state');
  if(empty)panel.innerHTML='';
  panel.insertBefore(item,panel.firstChild);
  updateSummaryList();
  document.getElementById('gr-current-dir').textContent='blocked';
}

function handleGuardrailPassed(evt){
  updateSummaryList();
}

function extractGuardName(msg){
  if(!msg)return'unknown';
  var parts=msg.split(':');
  return parts[0].trim();
}

function extractDecision(msg){
  if(!msg)return'pass';
  var lower=msg.toLowerCase();
  if(lower.indexOf('block')>=0)return'block';
  if(lower.indexOf('warn')>=0)return'warn';
  if(lower.indexOf('redact')>=0)return'redact';
  return'pass';
}

function buildGuardrailItemHTML(name,decision,msg,ts){
  var decClass='dec-'+decision;
  var dotClass=decision==='block'?'gi-dot':(decision==='warn'?'gi-dot':(decision==='redact'?'gi-dot':'gi-dot'));
  var decBadge='<span class="badge '+decision+'">'+decision.toUpperCase()+'</span>';
  var time=ts?new Date(ts).toLocaleTimeString('en-US',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'}):'';
  return '<div class="gi-header">'+
    '<span class="'+dotClass+'"></span>'+
    '<span class="gi-name">'+escHtml(name)+'</span>'+
    '<span class="gi-decision '+decClass+'">'+decBadge+'</span>'+
    '<span style="font-size:9px;color:var(--text-dim);margin-left:auto">'+time+'</span>'+
    '</div>'+
    '<div class="gi-msg">'+escHtml(msg)+'</div>';
}

function updateSummaryList(){
  var list=document.getElementById('gr-summary-list');
  var all=[];
  currentInputGuards.forEach(function(g){all.push({dir:'in',name:g.name,decision:g.decision,time:g.time});});
  currentOutputGuards.forEach(function(g){all.push({dir:'out',name:g.name,decision:g.decision,time:g.time});});
  if(all.length===0){
    list.innerHTML='<div class="empty-state"><span class="em">&#x1F6E1;</span>No guardrail data yet</div>';
    return;
  }
  list.innerHTML=all.map(function(g){
    var dot=g.decision==='pass'?'gr-pass':g.decision==='block'?'gr-block':g.decision==='warn'?'gr-warn':'gr-redact';
    var dec=g.decision==='pass'?'dec-pass':g.decision==='block'?'dec-block':g.decision==='warn'?'dec-warn':'dec-redact';
    var time=g.time?new Date(g.time).toLocaleTimeString('en-US',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'}):'';
    return '<div class="guardrail-row">'+
      '<span class="gr-indicator '+dot+'"></span>'+
      '<span style="font-size:9px;color:var(--text-dim);width:18px">'+g.dir+'</span>'+
      '<span class="gr-name">'+escHtml(g.name)+'</span>'+
      '<span class="gr-decision '+dec+'">'+g.decision.toUpperCase()+'</span>'+
      '<span class="gr-time">'+time+'</span>'+
      '</div>';
  }).join('');
}

function addChatMessage(role,content){
  chatMessages.push({role:role,content:content,time:Date.now()});
  var msgs=document.getElementById('chat-messages');
  var empty=msgs.querySelector('.empty-state');
  if(empty)msgs.innerHTML='';
  var div=document.createElement('div');
  div.className='chat-msg '+role;
  var tag='';
  if(role==='user')tag='<div class="msg-tag">YOU</div>';
  else if(role==='assistant')tag='<div class="msg-tag">ASSISTANT</div>';
  else if(role==='error')tag='<div class="msg-tag">ERROR</div>';
  div.innerHTML=tag+escHtml(content);
  msgs.appendChild(div);
  msgs.scrollTop=msgs.scrollHeight;
}

function sendChatMessage(){
  var input=document.getElementById('chat-input');
  var prompt=input.value.trim();
  if(!prompt)return;
  input.value='';
  addChatMessage('user',prompt);

  var btn=document.getElementById('chat-send-btn');
  btn.disabled=true;
  btn.textContent='...';

  var headers={'Content-Type':'application/json'};

if(userKey){
  headers['Authorization']='Bearer '+userKey;
}

fetch('/chat',{
  method:'POST',
  headers:headers,
  body:JSON.stringify(...)
})

function buildPipeline(){
  var container=document.getElementById('pipeline-container');
  container.innerHTML='';
  STAGES.forEach(function(stage,i){
    var wrap=document.createElement('div');
    wrap.className='stage-wrap';
    var el=document.createElement('div');
    el.className='stage';
    el.id='stage-'+stage.id;
    el.innerHTML='<span class="icon">'+stage.icon+'</span><div class="label">'+stage.label+'</div><div class="detail"></div><div class="time"></div>';
    wrap.appendChild(el);
    container.appendChild(wrap);
    if(i<STAGES.length-1){
      var conn=document.createElement('div');
      conn.className='connector';
      conn.id='conn-'+i;
      container.appendChild(conn);
    }
  });
}

function updatePipeline(evt){
  var stageEl=document.getElementById('stage-'+evt.stage);
  if(!stageEl)return;
  var detailEl=stageEl.querySelector('.detail');
  var timeEl=stageEl.querySelector('.time');
  if(evt.status==='error'){
    stageEl.classList.remove('in-progress','completed');
    stageEl.classList.add('error');
    if(evt.message)detailEl.textContent=evt.message.substring(0,30);
    return;
  }
  if(evt.status==='started'||evt.status==='info'){
    if(evt.message)detailEl.textContent=evt.message.substring(0,28);
  }else if(evt.status==='completed'){
    if(evt.message)detailEl.textContent=evt.message.substring(0,28);
    if(evt.duration)timeEl.textContent=evt.duration+'ms';
  }
  var isCompleted=evt.status==='completed';
  enqueueBlink(stageEl,isCompleted);
  var idx=STAGES.findIndex(function(s){return s.id===evt.stage});
  if(idx>0){
    var conn=document.getElementById('conn-'+(idx-1));
    if(conn){conn.classList.add('flow');setTimeout(function(){conn.classList.remove('flow')},500);}
  }
}

function enqueueBlink(el,isCompleted){
  blinkQueue.push({el:el,completed:isCompleted});
  if(!blinking)processBlink();
}
function processBlink(){
  if(blinkQueue.length===0){blinking=false;return}
  blinking=true;
  var item=blinkQueue.shift();
  var el=item.el;
  el.classList.remove('completed','error');
  el.classList.add('in-progress');
  setTimeout(function(){
    el.classList.remove('in-progress');
    if(item.completed)el.classList.add('completed');
    processBlink();
  },25);
}

function addEventLog(evt,stageName){
  var log=document.getElementById('event-log');
  var emptyState=log.querySelector('.empty-state');
  if(emptyState)log.innerHTML='';

  var clsMap={
    'guardrail_input':'s-GUARD','guardrail_output':'s-GUARD',
    'guardrail_blocked':'s-BLOCKED','guardrail_passed':'s-PASSED',
    'request_received':'s-RECD','body_read':'s-READ',
    'json_validated':'s-VALID','loading_api_key':'s-PROXY',
    'building_request':'s-PROXY','setting_headers':'s-PROXY',
    'sending_request':'s-PROXY','reading_response':'s-PROXY',
    'validating_status':'s-PROXY','response_sent':'s-DONE',
    'error':'s-ERROR','token_stream':'s-PROXY','info':'s-RECD'
  };
  var cls=clsMap[evt.stage]||'s-RECD';

  var d=new Date(evt.timestamp);
  var time=d.toLocaleTimeString('en-US',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'})+'.'+String(evt.timestamp%1000).padStart(3,'0');

  var badgeClass;
  if(evt.status==='started')badgeClass='started';
  else if(evt.status==='completed')badgeClass='completed';
  else if(evt.status==='error')badgeClass='error';
  else if(evt.status==='block')badgeClass='block';
  else if(evt.status==='warn')badgeClass='warn';
  else if(evt.status==='pass')badgeClass='pass';
  else badgeClass='info';

  var row=document.createElement('div');
  row.className='event-row';
  row.innerHTML='<span class="event-time">'+time+'</span><span class="event-stage '+cls+'">'+stageName+'</span><span class="event-status"><span class="badge '+badgeClass+'">'+evt.status+'</span></span><span class="event-msg" title="'+escAttr(evt.message||'')+'">'+(evt.message||'')+'</span>';
  log.insertBefore(row,log.firstChild);
  if(log.children.length>300)log.removeChild(log.lastChild);
}

function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}
function escAttr(s){return String(s).replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}

function initNavigation(){
  var buttons=document.querySelectorAll('.nav-item');
  var sections=document.querySelectorAll('[data-page-section]');
  buttons.forEach(function(btn){
    btn.addEventListener('click',function(){
      var target=btn.getAttribute('data-section');
      buttons.forEach(function(b){
        var active=b.getAttribute('data-section')===target;
        b.classList.toggle('active',active);
        if(active)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current');
      });
      sections.forEach(function(section){section.classList.toggle('active',section.getAttribute('data-page-section')===target);});
      renderSection(target);
    });
  });
}

function renderSection(target){
  var targets={
    overview:[['pipeline-card','overview-pipeline-slot'],['event-section','overview-activity-slot']],
    requests:[['pipeline-card','requests-pipeline-slot'],['chat-section','requests-chat-slot']],
    guardrails:[['gr-summary','guardrails-summary-slot'],['guardrails-grid','guardrails-panels-slot']],
    diagnostics:[['event-section','diagnostics-events-slot']]
  };
  Object.keys(targets).forEach(function(key){
    targets[key].forEach(function(pair){
      var node=document.getElementById(pair[0]);
      if(node)node.style.display=(key===target)?'':'none';
    });
  });
  (targets[target]||[]).forEach(function(pair){
    var node=document.getElementById(pair[0]);
    var host=document.getElementById(pair[1]);
    if(node&&host&&node.parentNode!==host)host.appendChild(node);
    if(node)node.style.display='';
  });
}

initNavigation();
renderSection('overview');
buildPipeline();

setInterval(function(){
  Object.keys(lastRequestTimestamps).forEach(function(id){
    var items=document.querySelectorAll('.req-item');
    if(items.length===0)return;
  });
},5000);
</script>
</body>
</html>`
