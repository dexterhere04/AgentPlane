package dashboard

const HTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>AgentPlane Live</title>
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #0b0f18;
  --surface: #151a26;
  --surface2: #1c2232;
  --border: #2a3040;
  --text: #c8d0dc;
  --text-dim: #6b7385;
  --accent: #5b8def;
  --green: #3fb950;
  --amber: #d29922;
  --red: #f85149;
  --purple: #a371f7;
  --cyan: #39c5cf;
}
body {
  font-family: 'IBM Plex Mono', 'SF Mono', 'Fira Code', monospace;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
  overflow-x: hidden;
}
header {
  background: var(--surface);
  border-bottom: 1px solid var(--border);
  padding: 16px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 10;
}
.logo { font-size: 18px; font-weight: 700; color: var(--text); }
.logo span { color: var(--accent); }
.status-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.status-dot.connected { background: var(--green); box-shadow: 0 0 8px var(--green); }
.status-dot.disconnected { background: var(--red); }
.header-right { display: flex; gap: 16px; align-items: center; font-size: 13px; color: var(--text-dim); }
.header-right .connected-text { color: var(--green); }
main { padding: 24px; max-width: 1400px; margin: 0 auto; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
.full { grid-column: 1 / -1; }
.card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 20px;
}
.card h2 {
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-dim);
  margin-bottom: 16px;
}
.pipeline {
  display: flex;
  align-items: flex-start;
  gap: 0;
  overflow-x: auto;
  padding: 12px 0;
}
.stage-container {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.stage {
  width: 140px;
  padding: 14px 10px;
  border-radius: 8px;
  border: 2px solid var(--border);
  background: var(--surface2);
  text-align: center;
  transition: all 0.3s ease;
  position: relative;
}
.stage .icon {
  font-size: 22px;
  margin-bottom: 6px;
  display: block;
}
.stage .label {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-dim);
  margin-bottom: 4px;
}
.stage .detail {
  font-size: 10px;
  color: var(--text-dim);
  min-height: 14px;
}
.stage .time {
  font-size: 10px;
  color: var(--accent);
  margin-top: 4px;
  opacity: 0;
  transition: opacity 0.3s;
}
.stage.in-progress {
  border-color: var(--amber);
  box-shadow: 0 0 12px rgba(210,153,34,0.3);
}
.stage.in-progress .label { color: var(--amber); }
.stage.in-progress .icon { animation: pulse 1s infinite; }
.stage.completed {
  border-color: var(--green);
  background: rgba(63,185,80,0.06);
}
.stage.completed .label { color: var(--green); }
.stage.completed .time { opacity: 1; }
.stage.error {
  border-color: var(--red);
  background: rgba(248,81,73,0.06);
}
.stage.error .label { color: var(--red); }
.connector {
  width: 28px;
  height: 2px;
  background: var(--border);
  flex-shrink: 0;
  margin: 0 4px;
  align-self: center;
  transition: background 0.5s;
}
.connector.flow { background: var(--accent); }
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
pre.payload {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 14px;
  font-size: 12px;
  line-height: 1.6;
  overflow-x: auto;
  max-height: 320px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.event-log {
  max-height: 360px;
  overflow-y: auto;
  font-size: 12px;
}
.event-row {
  display: flex;
  gap: 10px;
  padding: 5px 0;
  border-bottom: 1px solid rgba(42,48,64,0.5);
  align-items: baseline;
}
.event-time { color: var(--text-dim); white-space: nowrap; min-width: 70px; }
.event-stage {
  min-width: 130px;
  white-space: nowrap;
  font-weight: 600;
}
.event-status { min-width: 70px; }
.event-msg { color: var(--text-dim); }
.event-stage.stage-RECEIVED { color: var(--accent); }
.event-stage.stage-READ { color: var(--purple); }
.event-stage.stage-VALID { color: var(--cyan); }
.event-stage.stage-PROXY { color: var(--amber); }
.event-stage.stage-COMPLETE { color: var(--green); }
.event-stage.stage-ERROR { color: var(--red); }
.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 700;
}
.badge.started { background: rgba(210,153,34,0.15); color: var(--amber); }
.badge.completed { background: rgba(63,185,80,0.15); color: var(--green); }
.badge.error { background: rgba(248,81,73,0.15); color: var(--red); }
.badge.info { background: rgba(91,141,239,0.15); color: var(--accent); }
.req-list { max-height: 240px; overflow-y: auto; }
.req-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
  cursor: pointer;
  border-radius: 4px;
  transition: background 0.2s;
}
.req-item:hover { background: var(--surface2); }
.req-item.active { background: rgba(91,141,239,0.1); border-left: 3px solid var(--accent); }
.req-id { color: var(--accent); font-weight: 600; }
.req-time { color: var(--text-dim); }
.tabs { display: flex; gap: 0; border-bottom: 2px solid var(--border); margin-bottom: 16px; }
.tab {
  padding: 8px 16px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-dim);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: all 0.2s;
}
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }
.tab:hover { color: var(--text); }
.empty-state { text-align: center; padding: 40px; color: var(--text-dim); font-size: 13px; }
.empty-state .emoji { font-size: 36px; margin-bottom: 12px; }
#send-form { display: flex; gap: 8px; margin-bottom: 16px; }
#send-form input {
  flex: 1;
  padding: 10px 14px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
}
#send-form button {
  padding: 10px 20px;
  background: var(--accent);
  border: none;
  border-radius: 6px;
  color: #fff;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  transition: background 0.2s;
}
#send-form button:hover { background: #4a7de0; }
.concurrent-section { border-top: 1px solid var(--border); margin-top: 16px; padding-top: 16px; }
.concurrent-section h2 { margin-bottom: 10px !important; }
.concurrent-form { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
.concurrent-form input[type="number"] {
  width: 80px;
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text);
  font-family: inherit;
  font-size: 12px;
}
.concurrent-form input[type="text"] {
  flex: 1;
  min-width: 100px;
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text);
  font-family: inherit;
  font-size: 12px;
}
.concurrent-form label {
  font-size: 11px;
  color: var(--text-dim);
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  cursor: pointer;
}
.concurrent-form label input { cursor: pointer; }
.concurrent-form button {
  padding: 8px 16px;
  background: var(--purple);
  border: none;
  border-radius: 6px;
  color: #fff;
  font-weight: 600;
  font-family: inherit;
  cursor: pointer;
  font-size: 12px;
  transition: background 0.2s;
}
.concurrent-form button:hover { background: #8b5cf6; }
.concurrent-form button:disabled { opacity: 0.5; cursor: not-allowed; }
.concurrent-progress { margin-top: 8px; display: none; }
.concurrent-progress-bar-wrap {
  width: 100%;
  height: 6px;
  background: var(--bg);
  border-radius: 3px;
  overflow: hidden;
  margin-bottom: 6px;
}
.concurrent-progress-bar {
  height: 100%;
  background: var(--purple);
  border-radius: 3px;
  width: 0%;
  transition: width 0.2s ease;
}
.concurrent-progress-text { font-size: 11px; color: var(--text-dim); margin-bottom: 8px; }
.concurrent-dots { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 10px; }
.concurrent-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--border);
  transition: background 0.3s, box-shadow 0.3s;
}
.concurrent-dot.pending { background: var(--border); }
.concurrent-dot.success { background: var(--green); box-shadow: 0 0 4px var(--green); }
.concurrent-dot.error { background: var(--red); box-shadow: 0 0 4px var(--red); }
.concurrent-summary {
  width: 100%;
  font-size: 12px;
  border-collapse: collapse;
  margin-top: 8px;
}
.concurrent-summary td {
  padding: 3px 10px;
  border-bottom: 1px solid rgba(42,48,64,0.4);
}
.concurrent-summary td:first-child { color: var(--text-dim); }
.concurrent-summary td:last-child { text-align: right; font-variant-numeric: tabular-nums; }
</style>
</head>
<body>
<header>
  <div class="logo">Agent<span>Plane</span> Live</div>
  <div class="header-right">
    <span id="connection-status"><span class="status-dot disconnected"></span> Disconnected</span>
    <span>Events: <span id="event-count">0</span></span>
    <span>Requests: <span id="request-count">0</span></span>
  </div>
</header>
<main>
  <div class="card full" style="margin-bottom:20px">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">
      <h2 style="margin:0">Request Pipeline</h2>
    </div>
    <div id="pipeline-container" class="pipeline"></div>
  </div>

  <div class="grid">
    <div class="card">
      <h2>Sent Request</h2>
      <div id="request-payload"><div class="empty-state"><span class="emoji">&#x1F4E4;</span><br>Send a request to see payload</div></div>
    </div>
    <div class="card">
      <h2>Response</h2>
      <div id="response-payload"><div class="empty-state"><span class="emoji">&#x1F4E5;</span><br>Response will appear here</div></div>
    </div>
    <div class="card">
      <h2>Live Events</h2>
      <div class="event-log" id="event-log"><div class="empty-state">Waiting for events...</div></div>
    </div>
    <div class="card">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px">
        <h2 style="margin:0">Quick Request</h2>
      </div>
      <form id="send-form" onsubmit="sendRequest(event)">
        <input type="text" id="prompt-input" placeholder="Type a message..." value="Hello, how are you?">
        <button type="submit">Send</button>
      </form>
      <div class="tabs">
        <div class="tab" onclick="setStreaming(false)" id="tab-normal" style="border-bottom-color:var(--accent);color:var(--accent)">Normal</div>
        <div class="tab" onclick="setStreaming(true)" id="tab-stream">Streaming</div>
      </div>
      <div class="req-list" id="req-list"><div class="empty-state">No requests yet</div></div>
      <div class="concurrent-section">
        <h2>Concurrent Test</h2>
        <div class="concurrent-form">
          <input type="number" id="concurrent-count" min="1" max="100" value="10" placeholder="N">
          <input type="text" id="concurrent-prompt" value="Hello" placeholder="Prompt">
          <label><input type="checkbox" id="concurrent-stream"> Stream</label>
          <button id="concurrent-fire-btn" onclick="sendConcurrent()">Fire</button>
        </div>
        <div class="concurrent-progress" id="concurrent-progress">
          <div class="concurrent-progress-bar-wrap"><div class="concurrent-progress-bar" id="concurrent-progress-bar"></div></div>
          <div class="concurrent-progress-text" id="concurrent-progress-text"></div>
          <div class="concurrent-dots" id="concurrent-dots"></div>
        </div>
        <div id="concurrent-results"></div>
      </div>
    </div>
  </div>
</main>
<script>
const STAGES = [
  { id: 'request_received', label: 'RECEIVED', icon: '&#x2B06;', short: 'RECD' },
  { id: 'body_read', label: 'BODY READ', icon: '&#x1F4E5;', short: 'READ' },
  { id: 'json_validated', label: 'VALIDATED', icon: '&#x2705;', short: 'VALD' },
  { id: 'loading_api_key', label: 'API KEY', icon: '&#x1F511;', short: 'AUTH' },
  { id: 'building_request', label: 'BUILD REQ', icon: '&#x1F527;', short: 'BLD' },
  { id: 'setting_headers', label: 'HEADERS', icon: '&#x1F4CB;', short: 'HDRS' },
  { id: 'sending_request', label: 'SENDING', icon: '&#x1F680;', short: 'SEND' },
  { id: 'reading_response', label: 'READ RES', icon: '&#x1F4E6;', short: 'RRES' },
  { id: 'validating_status', label: 'STATUS', icon: '&#x2714;', short: 'STAT' },
  { id: 'response_sent', label: 'DONE', icon: '&#x1F389;', short: 'DONE' }
];

const bus = new EventSource('/events');
let eventCount = 0, requestCount = 0;
let currentRequestId = null;
let pipelineState = {};
let requests = {};
let streaming = false;

function setStreaming(s) {
  streaming = s;
  document.getElementById('tab-normal').style.cssText = s ? '' : 'border-bottom-color:var(--accent);color:var(--accent)';
  document.getElementById('tab-stream').style.cssText = s ? 'border-bottom-color:var(--accent);color:var(--accent)' : '';
}

bus.onopen = () => {
  document.getElementById('connection-status').innerHTML = '<span class="status-dot connected"></span> Connected';
};

bus.onerror = () => {
  document.getElementById('connection-status').innerHTML = '<span class="status-dot disconnected"></span> Disconnected';
};

bus.onmessage = (e) => {
  const evt = JSON.parse(e.data);
  handleEvent(evt);
};

function handleEvent(evt) {
  eventCount++;
  document.getElementById('event-count').textContent = eventCount;

  const stageName = STAGES.find(s => s.id === evt.stage)?.short || evt.stage;

  if (!requests[evt.request_id]) {
    requests[evt.request_id] = { id: evt.request_id, events: [], startTime: evt.timestamp };
    requestCount++;
    document.getElementById('request-count').textContent = requestCount;
    addRequestToList(evt.request_id);
  }
  requests[evt.request_id].events.push(evt);

  updatePipeline(evt);
  addEventLog(evt, stageName);

  if (evt.stage === 'body_read' && evt.status === 'completed' && evt.data) {
    document.getElementById('request-payload').innerHTML = '<pre class="payload">' + syntaxHighlight(evt.data) + '</pre>';
  }
  if (evt.stage === 'response_sent' && evt.status === 'completed' && evt.data) {
    document.getElementById('response-payload').innerHTML = '<pre class="payload">' + syntaxHighlight(evt.data) + '</pre>';
  }
  if (evt.stage === 'error') {
    updateStageStatus(evt.stage, 'error', evt.message);
  }
}

function buildPipeline() {
  const container = document.getElementById('pipeline-container');
  container.innerHTML = '';
  STAGES.forEach((stage, i) => {
    const wrap = document.createElement('div');
    wrap.className = 'stage-container';
    const el = document.createElement('div');
    el.className = 'stage';
    el.id = 'stage-' + stage.id;
    el.innerHTML = '<span class="icon">' + stage.icon + '</span><div class="label">' + stage.label + '</div><div class="detail"></div><div class="time"></div>';
    wrap.appendChild(el);
    container.appendChild(wrap);
    if (i < STAGES.length - 1) {
      const conn = document.createElement('div');
      conn.className = 'connector';
      conn.id = 'conn-' + i;
      container.appendChild(conn);
    }
  });
}

function updatePipeline(evt) {
  const stageEl = document.getElementById('stage-' + evt.stage);
  if (!stageEl) return;

  const detailEl = stageEl.querySelector('.detail');
  const timeEl = stageEl.querySelector('.time');

  if (evt.status === 'error') {
    stageEl.classList.remove('in-progress', 'completed');
    stageEl.classList.add('error');
    detailEl.textContent = evt.message || 'Error';
    return;
  }

  if (evt.status === 'started' || evt.status === 'info') {
    if (evt.message) detailEl.textContent = evt.message;
    timeEl.textContent = '';
  } else if (evt.status === 'completed') {
    if (evt.message) detailEl.textContent = evt.message;
    if (evt.duration) timeEl.textContent = evt.duration + 'ms';
  }

  stageEl.classList.remove('completed', 'error');
  stageEl.classList.add('in-progress');
  setTimeout(() => stageEl.classList.remove('in-progress'), 500);

  const idx = STAGES.findIndex(s => s.id === evt.stage);
  if (idx > 0) {
    const conn = document.getElementById('conn-' + (idx - 1));
    if (conn) {
      conn.classList.add('flow');
      setTimeout(() => conn.classList.remove('flow'), 500);
    }
  }
}

function addEventLog(evt, stageName) {
  const log = document.getElementById('event-log');
  if (log.querySelector('.empty-state')) log.innerHTML = '';

  const cls = evt.stage === 'error' ? 'stage-ERROR' :
    evt.stage === 'response_sent' ? 'stage-COMPLETE' :
    ['sending_request','building_request','setting_headers','loading_api_key','reading_response','validating_status'].includes(evt.stage) ? 'stage-PROXY' :
    ['body_read','request_received'].includes(evt.stage) ? 'stage-RECEIVED' :
    'stage-VALID';

  const time = new Date(evt.timestamp).toLocaleTimeString('en-US', {hour12: false, hour:'2-digit',minute:'2-digit',second:'2-digit'}) + '.' + String(evt.timestamp % 1000).padStart(3,'0');
  const badgeClass = evt.status === 'started' ? 'started' : evt.status === 'completed' ? 'completed' : evt.status === 'error' ? 'error' : 'info';

  const row = document.createElement('div');
  row.className = 'event-row';
  row.innerHTML = '<span class="event-time">' + time + '</span><span class="event-stage ' + cls + '">' + stageName + '</span><span class="event-status"><span class="badge ' + badgeClass + '">' + evt.status + '</span></span><span class="event-msg">' + (evt.message || '') + '</span>';

  log.insertBefore(row, log.firstChild);
  if (log.children.length > 200) log.removeChild(log.lastChild);
}

function addRequestToList(reqId) {
  const list = document.getElementById('req-list');
  if (list.querySelector('.empty-state')) list.innerHTML = '';
  const item = document.createElement('div');
  item.className = 'req-item';
  item.id = 'req-item-' + reqId;
  item.onclick = () => selectRequest(reqId);
  const short = reqId.replace('req-', '').slice(-8);
  item.innerHTML = '<span class="req-id">#' + short + '</span><span class="req-time">just now</span>';
  list.insertBefore(item, list.firstChild);
}

function selectRequest(reqId) {
  currentRequestId = reqId;
  document.querySelectorAll('.req-item').forEach(el => el.classList.remove('active'));
  const item = document.getElementById('req-item-' + reqId);
  if (item) item.classList.add('active');

  buildPipeline();
  const req = requests[reqId];
  if (req) {
    req.events.forEach(evt => updatePipeline(evt));
  }
}

let concurrencyRunning = false;

async function sendConcurrent() {
  if (concurrencyRunning) return;
  concurrencyRunning = true;

  const btn = document.getElementById('concurrent-fire-btn');
  btn.disabled = true;
  btn.textContent = 'Firing...';

  const countInput = document.getElementById('concurrent-count');
  const count = Math.min(Math.max(parseInt(countInput.value) || 10, 1), 100);
  countInput.value = count;

  const prompt = document.getElementById('concurrent-prompt').value || 'Hello';
  const stream = document.getElementById('concurrent-stream').checked;
  const resultsDiv = document.getElementById('concurrent-results');
  const progressBar = document.getElementById('concurrent-progress-bar');
  const progressText = document.getElementById('concurrent-progress-text');
  const dotsDiv = document.getElementById('concurrent-dots');
  const progressWrap = document.getElementById('concurrent-progress');

  resultsDiv.innerHTML = '';
  progressBar.style.width = '0%';
  dotsDiv.innerHTML = '';
  progressWrap.style.display = 'block';

  const startTime = performance.now();
  let completed = 0;
  let succeeded = 0;
  let failed = 0;
  const times = [];

  for (let i = 0; i < count; i++) {
    const dot = document.createElement('span');
    dot.className = 'concurrent-dot pending';
    dot.title = 'Request #' + (i + 1);
    dotsDiv.appendChild(dot);
  }

  const promises = [];

  for (let i = 0; i < count; i++) {
    const idx = i;
    const body = {
      model: 'gpt-4o',
      messages: [{ role: 'user', content: prompt + ' #' + (idx + 1) }],
      stream: stream
    };

    const p = fetch('/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    }).then(async r => {
      const elapsed = performance.now() - startTime;
      if (!r.ok) {
        const text = await r.text();
        failed++;
        const d = dotsDiv.children[idx];
        if (d) { d.className = 'concurrent-dot error'; d.title = 'Request #' + (idx + 1) + ': ' + text.substring(0, 100); }
        return { ok: false, error: text, time: elapsed, index: idx };
      }
      const data = await r.json();
      succeeded++;
      const d = dotsDiv.children[idx];
      if (d) { d.className = 'concurrent-dot success'; d.title = 'Request #' + (idx + 1) + ': OK'; }
      return { ok: true, data: data, time: elapsed, index: idx };
    }).catch(err => {
      const elapsed = performance.now() - startTime;
      failed++;
      const d = dotsDiv.children[idx];
      if (d) { d.className = 'concurrent-dot error'; d.title = 'Request #' + (idx + 1) + ': ' + err.message; }
      return { ok: false, error: err.message, time: elapsed, index: idx };
    });

    p.then(r => {
      completed++;
      times.push(r.time);
      const pct = (completed / count) * 100;
      progressBar.style.width = pct + '%';
      progressText.textContent = completed + '/' + count + '  OK: ' + succeeded + '  Fail: ' + failed;
      if (completed === count) {
        const avgTime = times.length ? (times.reduce((a, b) => a + b, 0) / times.length).toFixed(0) : '-';
        const minTime = times.length ? Math.min(...times).toFixed(0) : '-';
        const maxTime = times.length ? Math.max(...times).toFixed(0) : '-';
        const totalTime = (performance.now() - startTime).toFixed(0);

        resultsDiv.innerHTML =
          '<table class="concurrent-summary">' +
          '<tr><td>Total</td><td><b>' + count + '</b></td></tr>' +
          '<tr><td>OK</td><td style="color:var(--green)"><b>' + succeeded + '</b></td></tr>' +
          '<tr><td>Failed</td><td style="color:var(--red)"><b>' + failed + '</b></td></tr>' +
          '<tr><td>Avg</td><td><b>' + avgTime + 'ms</b></td></tr>' +
          '<tr><td>Min</td><td><b>' + minTime + 'ms</b></td></tr>' +
          '<tr><td>Max</td><td><b>' + maxTime + 'ms</b></td></tr>' +
          '<tr><td>Total</td><td><b>' + totalTime + 'ms</b></td></tr>' +
          '</table>';

        concurrencyRunning = false;
        btn.disabled = false;
        btn.textContent = 'Fire';
      }
    });

    promises.push(p);
  }
}

function sendRequest(e) {
  e.preventDefault();
  const prompt = document.getElementById('prompt-input').value || 'Hello';
  const body = {
    model: 'gpt-4o',
    messages: [{ role: 'user', content: prompt }],
    stream: streaming
  };
  fetch('/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(r => {
    if (!r.ok) return r.text().then(t => { throw new Error(t) });
    if (streaming) return r.text().then(t => {
      document.getElementById('response-payload').innerHTML = '<pre class="payload">[Streaming mode — see events for token data]</pre>';
    });
    return r.json().then(data => {
      document.getElementById('response-payload').innerHTML = '<pre class="payload">' + syntaxHighlight(JSON.stringify(data)) + '</pre>';
    });
  }).catch(err => {
    document.getElementById('response-payload').innerHTML = '<pre class="payload" style="color:var(--red)">' + err.message + '</pre>';
  });
}

function syntaxHighlight(json) {
  json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  return json.replace(/("(\\u[a-fA-F0-9]{4}|\\[^u]|[^"\\])*"(\s*:)?|\b(true|false|null)\b|\b-?\d+(\.\d+)?([eE][+-]?\d+)?\b)/g, match => {
    let cls = 'var(--cyan)';
    if (/^"/.test(match)) {
      cls = /:$/.test(match) ? 'var(--accent)' : 'var(--green)';
      return '<span style="color:' + cls + '">' + match + '</span>';
    }
    if (/true|false/.test(match)) return '<span style="color:var(--purple)">' + match + '</span>';
    if (/null/.test(match)) return '<span style="color:var(--red)">' + match + '</span>';
    return '<span style="color:var(--amber)">' + match + '</span>';
  });
}

buildPipeline();
</script>
</body>
</html>`
