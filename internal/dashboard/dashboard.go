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
  background-color:var(--bg);
  background-image:
    radial-gradient(ellipse at 50% 0%,rgba(104,136,240,0.04) 0%,transparent 55%),
    radial-gradient(circle at 1px 1px,rgba(255,255,255,0.018) 1px,transparent 1px);
  background-size:100% 100%,18px 18px;
  color:var(--text);
  min-height:100vh;
  -webkit-font-smoothing:antialiased;
}
header{
  display:flex;
  align-items:center;
  justify-content:space-between;
  padding:14px 28px;
  border-bottom:1px solid var(--border);
  background:rgba(13,16,24,0.85);
  -webkit-backdrop-filter:blur(8px);
  backdrop-filter:blur(8px);
  position:sticky;
  top:0;
  z-index:20;
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
.stats-bar{
  display:grid;
  grid-template-columns:repeat(4,1fr);
  gap:12px;
  margin-bottom:18px;
}
.stat-card{
  background:var(--surface);
  border:1px solid var(--border);
  border-radius:8px;
  padding:14px 18px;
  display:flex;
  flex-direction:column;
  gap:4px;
  transition:border-color 0.25s;
}
.stat-card:hover{border-color:var(--border-light)}
.stat-card .stat-label{font-size:10px;text-transform:uppercase;letter-spacing:0.08em;color:var(--text-dim);font-weight:600}
.stat-card .stat-value{font-size:22px;font-weight:700;color:var(--text-bright);font-variant-numeric:tabular-nums;letter-spacing:-0.02em}
.stat-card .stat-sub{font-size:10px;color:var(--text-dim);margin-top:2px}
.pipeline-card{
  background:var(--surface);
  border:1px solid var(--border);
  border-radius:10px;
  padding:20px 24px;
  margin-bottom:18px;
  position:relative;
  overflow:hidden;
}
.pipeline-card::before{
  content:'';
  position:absolute;
  inset:0;
  border-radius:10px;
  padding:1px;
  background:linear-gradient(135deg,rgba(255,255,255,0.03),transparent 50%,rgba(104,136,240,0.05));
  -webkit-mask:linear-gradient(#fff 0 0) content-box,linear-gradient(#fff 0 0);
  -webkit-mask-composite:xor;
  mask-composite:exclude;
  pointer-events:none;
}
.pipeline-card h2{
  font-size:11px;
  text-transform:uppercase;
  letter-spacing:0.1em;
  color:var(--text-dim);
  margin-bottom:16px;
  font-weight:600;
}
.pipeline{
  display:flex;
  align-items:flex-start;
  gap:0;
  overflow-x:auto;
  padding:8px 0 4px;
}
.stage-wrap{display:flex;align-items:center;flex-shrink:0}
.stage{
  width:122px;
  padding:12px 6px;
  border-radius:8px;
  border:1px solid var(--border);
  background:var(--surface2);
  text-align:center;
  transition:all 0.35s cubic-bezier(0.4,0,0.2,1);
  position:relative;
  overflow:hidden;
}
.stage::after{
  content:'';
  position:absolute;
  top:0;left:0;right:0;
  height:2px;
  background:transparent;
  transition:background 0.3s,box-shadow 0.3s;
  border-radius:2px;
}
@keyframes blinkPulse{
  0%{box-shadow:0 0 0 rgba(212,160,48,0);border-color:rgba(212,160,48,0.15)}
  30%{box-shadow:0 0 18px rgba(212,160,48,0.45),0 0 36px rgba(212,160,48,0.15);border-color:rgba(212,160,48,0.7)}
  100%{box-shadow:0 0 0 rgba(212,160,48,0);border-color:rgba(212,160,48,0)}
}
.stage.in-progress{animation:blinkPulse 0.15s ease-out}
.stage.in-progress::after{background:var(--amber);box-shadow:0 0 12px var(--amber)}
.stage.in-progress .icon{transform:scale(1.2)}
.stage.completed::after{background:var(--green);box-shadow:0 0 6px rgba(76,184,104,0.4)}
.stage.completed{border-color:rgba(76,184,104,0.25);background:rgba(76,184,104,0.04)}
.stage.error::after{background:var(--red);box-shadow:0 0 6px rgba(224,80,80,0.4)}
.stage.error{border-color:rgba(224,80,80,0.3);background:rgba(224,80,80,0.04)}
.stage .icon{font-size:20px;margin-bottom:5px;display:block;transition:transform 0.35s}
.stage .label{font-size:10px;font-weight:600;color:var(--text-dim);margin-bottom:2px;transition:color 0.3s}
.stage.in-progress .label{color:var(--amber)}
.stage.completed .label{color:var(--green)}
.stage.error .label{color:var(--red)}
.stage .detail{font-size:9px;color:var(--text-dim);min-height:12px;transition:color 0.3s}
.stage .time{font-size:9px;color:var(--accent);opacity:0;transition:opacity 0.3s;margin-top:3px}
.stage.completed .time,.stage.error .time{opacity:1}
.connector{
  width:22px;
  height:1.5px;
  background:var(--border);
  flex-shrink:0;
  align-self:center;
  margin:0 3px;
  position:relative;
  transition:background 0.3s;
}
.connector.flow{background:var(--accent);box-shadow:0 0 6px rgba(104,136,240,0.5)}
.connector.flow::after{
  content:'';
  position:absolute;
  top:-2px;right:-3px;
  width:5px;height:5px;
  border-radius:50%;
  background:var(--accent);
  box-shadow:0 0 6px var(--accent);
  animation:connectorPulse 0.5s ease-out;
}
.pipeline-progress{
  margin-top:14px;
  height:2px;
  background:var(--border);
  border-radius:1px;
  overflow:hidden;
}
.pipeline-progress-fill{
  height:100%;
  background:var(--accent);
  border-radius:1px;
  width:0%;
  transition:width 0.4s cubic-bezier(0.4,0,0.2,1);
  box-shadow:0 0 8px rgba(104,136,240,0.5);
}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.card{
  background:var(--surface);
  border:1px solid var(--border);
  border-radius:10px;
  padding:18px 20px;
  position:relative;
}
.card h2{
  font-size:11px;
  text-transform:uppercase;
  letter-spacing:0.1em;
  color:var(--text-dim);
  margin-bottom:14px;
  font-weight:600;
}
pre.payload{
  background:var(--bg);
  border:1px solid var(--border);
  border-radius:6px;
  padding:14px 16px;
  font-size:11px;
  line-height:1.65;
  overflow-x:auto;
  max-height:280px;
  overflow-y:auto;
  white-space:pre-wrap;
  word-break:break-all;
  color:var(--text);
}
pre.payload .key{color:var(--accent)}
pre.payload .str{color:var(--green)}
pre.payload .bool{color:var(--purple)}
pre.payload .num{color:var(--amber)}
pre.payload .null{color:var(--red)}
.card-scroll{display:flex;flex-direction:column;min-height:0}
.card-scroll .event-log{max-height:560px;overflow-y:auto;font-size:11px}
.event-log{max-height:560px;overflow-y:auto;font-size:11px}
.event-row{
  display:flex;
  gap:10px;
  padding:5px 8px;
  border-bottom:1px solid rgba(28,32,48,0.5);
  align-items:baseline;
  transition:background 0.2s;
  border-radius:3px;
}
.event-row:hover{background:rgba(255,255,255,0.015)}
.event-row:first-child{animation:slideIn 0.3s ease-out}
.event-time{color:var(--text-dim);white-space:nowrap;min-width:68px;font-size:10px}
.event-stage{min-width:100px;white-space:nowrap;font-weight:600;font-size:10px}
.event-status{min-width:60px}
.event-msg{color:var(--text-dim);flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.event-stage.s-RECEIVED{color:var(--accent)}
.event-stage.s-READ{color:var(--purple)}
.event-stage.s-VALID{color:var(--cyan)}
.event-stage.s-PROXY{color:var(--amber)}
.event-stage.s-COMPLETE{color:var(--green)}
.event-stage.s-ERROR{color:var(--red)}
.badge{
  display:inline-block;
  padding:1px 7px;
  border-radius:3px;
  font-size:9px;
  font-weight:700;
  text-transform:uppercase;
  letter-spacing:0.04em;
}
.badge.started{background:var(--amber-dim);color:var(--amber)}
.badge.completed{background:var(--green-dim);color:var(--green)}
.badge.error{background:var(--red-dim);color:var(--red)}
.badge.info{background:var(--accent-dim);color:var(--accent)}
.req-list{max-height:230px;overflow-y:auto}
.req-item{
  display:flex;
  align-items:center;
  justify-content:space-between;
  padding:8px 12px;
  border-bottom:1px solid var(--border);
  font-size:11px;
  cursor:pointer;
  border-radius:4px;
  transition:background 0.2s;
}
.req-item:hover{background:var(--surface2)}
.req-item.active{background:var(--accent-dim);border-left:3px solid var(--accent);padding-left:13px}
.req-id{color:var(--accent);font-weight:600}
.req-time{color:var(--text-dim);font-size:10px}
.tabs{display:flex;gap:0;border-bottom:2px solid var(--border);margin-bottom:14px}
.tab{
  padding:7px 14px;
  font-size:11px;
  font-weight:600;
  color:var(--text-dim);
  cursor:pointer;
  border-bottom:2px solid transparent;
  margin-bottom:-2px;
  transition:all 0.2s;
  text-transform:uppercase;
  letter-spacing:0.05em;
}
.tab.active{color:var(--accent);border-bottom-color:var(--accent)}
.tab:hover{color:var(--text)}
.empty-state{text-align:center;padding:36px 20px;color:var(--text-dim);font-size:12px}
.empty-state .em{font-size:32px;margin-bottom:10px;display:block;opacity:0.5}
#send-form{display:flex;gap:8px;margin-bottom:14px}
#send-form input{
  flex:1;
  padding:9px 12px;
  background:var(--bg);
  border:1px solid var(--border);
  border-radius:6px;
  color:var(--text);
  font-family:inherit;
  font-size:12px;
  transition:border-color 0.2s,box-shadow 0.2s;
  outline:none;
}
#send-form input:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-dim)}
#send-form button,.btn{
  padding:9px 18px;
  background:var(--accent);
  border:none;
  border-radius:6px;
  color:#fff;
  font-weight:600;
  font-family:inherit;
  font-size:12px;
  cursor:pointer;
  transition:background 0.2s,box-shadow 0.2s;
  outline:none;
  letter-spacing:0.03em;
}
#send-form button:hover,.btn:hover{background:#5a78e0;box-shadow:0 2px 12px rgba(104,136,240,0.3)}
#send-form button:active,.btn:active{transform:scale(0.98)}
.concurrent-section{border-top:1px solid var(--border);margin-top:14px;padding-top:14px}
.concurrent-section h2{margin-bottom:10px!important}
.concurrent-form{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:10px}
.concurrent-form input[type="number"]{
  width:60px;
  padding:7px 8px;
  background:var(--bg);
  border:1px solid var(--border);
  border-radius:6px;
  color:var(--text);
  font-family:inherit;
  font-size:11px;
  outline:none;
  transition:border-color 0.2s;
}
.concurrent-form input[type="number"]:focus{border-color:var(--accent)}
.concurrent-form input[type="text"]{
  flex:1;
  min-width:80px;
  padding:7px 10px;
  background:var(--bg);
  border:1px solid var(--border);
  border-radius:6px;
  color:var(--text);
  font-family:inherit;
  font-size:11px;
  outline:none;
  transition:border-color 0.2s;
}
.concurrent-form input[type="text"]:focus{border-color:var(--accent)}
.concurrent-form label{
  font-size:10px;
  color:var(--text-dim);
  display:flex;
  align-items:center;
  gap:4px;
  white-space:nowrap;
  cursor:pointer;
  text-transform:uppercase;
  letter-spacing:0.04em;
}
.concurrent-form label input{cursor:pointer;accent-color:var(--accent)}
.concurrent-form .btn{background:var(--purple);padding:7px 16px;font-size:11px}
.concurrent-form .btn:hover{background:#8658d0;box-shadow:0 2px 12px rgba(152,120,216,0.3)}
.concurrent-form .btn:disabled{opacity:0.4;cursor:not-allowed;box-shadow:none}
.concurrent-progress{display:none;margin-top:10px}
.concurrent-progress-bar-wrap{
  width:100%;
  height:4px;
  background:var(--bg);
  border-radius:2px;
  overflow:hidden;
  margin-bottom:6px;
}
.concurrent-progress-bar{
  height:100%;
  background:var(--purple);
  border-radius:2px;
  width:0%;
  transition:width 0.25s ease;
  box-shadow:0 0 8px rgba(152,120,216,0.5);
}
.concurrent-progress-text{font-size:10px;color:var(--text-dim);margin-bottom:7px}
.concurrent-dots{display:flex;flex-wrap:wrap;gap:4px;margin-bottom:8px}
.concurrent-dot{
  width:8px;
  height:8px;
  border-radius:50%;
  background:var(--border);
  transition:all 0.3s;
}
.concurrent-dot.success{background:var(--green);box-shadow:0 0 5px var(--green)}
.concurrent-dot.error{background:var(--red);box-shadow:0 0 5px var(--red)}
.concurrent-summary{
  width:100%;
  font-size:11px;
  border-collapse:collapse;
  margin-top:6px;
}
.concurrent-summary td{padding:3px 10px;border-bottom:1px solid rgba(28,32,48,0.4)}
.concurrent-summary td:first-child{color:var(--text-dim)}
.concurrent-summary td:last-child{text-align:right;font-variant-numeric:tabular-nums;font-weight:600}
@keyframes connectorPulse{
  0%{transform:scale(0);opacity:1}
  100%{transform:scale(1.5);opacity:0}
}
@keyframes slideIn{
  from{opacity:0;transform:translateY(-6px)}
  to{opacity:1;transform:translateY(0)}
}
::-webkit-scrollbar{width:5px;height:5px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:var(--border-light);border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:var(--text-dim)}
@media(max-width:900px){
  .grid{grid-template-columns:1fr}
  .stats-bar{grid-template-columns:repeat(2,1fr)}
  .header-right{gap:10px}
}
</style>
</head>
<body>
<header>
  <div class="logo"><i>A</i>gentPlane</div>
  <div class="header-right">
    <span id="connection-status"><span class="status-dot off"></span> Offline</span>
    <span class="stat">Req<span class="stat-val" id="request-count">0</span></span>
    <span class="stat">Evt<span class="stat-val" id="event-count">0</span></span>
  </div>
</header>
<main>
  <div class="stats-bar">
    <div class="stat-card">
      <span class="stat-label">Requests</span>
      <span class="stat-value" id="stat-requests">0</span>
      <span class="stat-sub">total tracked</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Active</span>
      <span class="stat-value" id="stat-active">0</span>
      <span class="stat-sub">in progress</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Events</span>
      <span class="stat-value" id="stat-events">0</span>
      <span class="stat-sub">streamed via SSE</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">Last Latency</span>
      <span class="stat-value" id="stat-latency">--</span>
      <span class="stat-sub">request duration</span>
    </div>
  </div>

  <div class="pipeline-card">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:14px">
      <h2 style="margin:0">Request Pipeline</h2>
      <span style="font-size:10px;color:var(--text-dim)" id="pipeline-req-id"></span>
    </div>
    <div id="pipeline-container" class="pipeline"></div>
    <div class="pipeline-progress">
      <div class="pipeline-progress-fill" id="pipeline-progress-fill"></div>
    </div>
  </div>

  <div class="grid">
    <div class="card">
      <h2>Sent Request</h2>
      <div id="request-payload"><div class="empty-state"><span class="em">&#x2191;</span>Send a request to see payload</div></div>
    </div>
    <div class="card">
      <h2>Response</h2>
      <div id="response-payload"><div class="empty-state"><span class="em">&#x2193;</span>Response will appear here</div></div>
    </div>
    <div class="card card-scroll">
      <h2>Live Events</h2>
      <div class="event-log" id="event-log"><div class="empty-state">Waiting for events&#8230;</div></div>
    </div>
    <div class="card">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
        <h2 style="margin:0">Quick Request</h2>
      </div>
      <form id="send-form" onsubmit="sendRequest(event)">
        <input type="text" id="prompt-input" placeholder="Type a message&#8230;" value="Hello, how are you?" autocomplete="off">
        <button type="submit">Send</button>
      </form>
      <div class="tabs">
        <div class="tab active" id="tab-normal" onclick="setStreaming(false)">Normal</div>
        <div class="tab" id="tab-stream" onclick="setStreaming(true)">Streaming</div>
      </div>
      <div class="req-list" id="req-list"><div class="empty-state">No requests yet</div></div>
      <div class="concurrent-section">
        <h2>Concurrent Test</h2>
        <div class="concurrent-form">
          <input type="number" id="concurrent-count" min="1" max="100" value="10" placeholder="N">
          <input type="text" id="concurrent-prompt" value="Hello" placeholder="Prompt" autocomplete="off">
          <label><input type="checkbox" id="concurrent-stream"> Stream</label>
          <button type="button" class="btn" id="concurrent-fire-btn" onclick="sendConcurrent()">Fire</button>
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
var eventCount=0,requestCount=0,activeCount=0;
var currentRequestId=null;
var requests={};
var streaming=false;
var lastRequestTimestamps={};
var blinkQueue=[],blinking=false;

function setStreaming(s){
  streaming=s;
  document.getElementById('tab-normal').className=s?'tab':'tab active';
  document.getElementById('tab-stream').className=s?'tab active':'tab';
}

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

  var stageName=STAGES.find(function(s){return s.id===evt.stage;});
  var shortName=stageName?stageName.short:evt.stage;

  if(!requests[evt.request_id]){
    requests[evt.request_id]={id:evt.request_id,events:[],startTime:evt.timestamp,ended:false};
    lastRequestTimestamps[evt.request_id]=evt.timestamp;
    requestCount++;
    activeCount++;
    document.getElementById('request-count').textContent=requestCount;
    document.getElementById('stat-requests').textContent=requestCount;
    document.getElementById('stat-active').textContent=activeCount;
    addRequestToList(evt.request_id);
  }

  requests[evt.request_id].events.push(evt);
  lastRequestTimestamps[evt.request_id]=evt.timestamp;

  if(evt.status==='started'&&(
    evt.stage==='sending_request'||evt.stage==='reading_response'||
    evt.stage==='loading_api_key'||evt.stage==='validating_status'||
    evt.stage==='building_request'||evt.stage==='setting_headers')){
    requests[evt.request_id].proxyActive=true;
  }

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
    document.getElementById('request-payload').innerHTML='<pre class="payload">'+syntaxHighlight(evt.data)+'</pre>';
  }
  if(evt.stage==='response_sent'&&evt.status==='completed'&&evt.data){
    document.getElementById('response-payload').innerHTML='<pre class="payload">'+syntaxHighlight(evt.data)+'</pre>';
  }
  if(evt.stage==='error'&&evt.message){
    document.getElementById('response-payload').innerHTML='<pre class="payload" style="color:var(--red)">'+escHtml(evt.message)+'</pre>';
  }

  updatePipelineProgress(evt.request_id);
}

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
    if(evt.message)detailEl.textContent=evt.message;
    return;
  }

  if(evt.status==='started'||evt.status==='info'){
    if(evt.message)detailEl.textContent=evt.message;
  }else if(evt.status==='completed'){
    if(evt.message)detailEl.textContent=evt.message;
    if(evt.duration)timeEl.textContent=evt.duration+'ms';
  }

  var isCompleted=evt.status==='completed';
  enqueueBlink(stageEl,isCompleted);

  var idx=STAGES.findIndex(function(s){return s.id===evt.stage;});
  if(idx>0){
    var conn=document.getElementById('conn-'+(idx-1));
    if(conn){
      conn.classList.add('flow');
      setTimeout(function(){conn.classList.remove('flow')},500);
    }
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

function updatePipelineProgress(reqId){
  var req=requests[reqId];
  if(!req)return;
  var completedStages=0;
  var completedSet={};
  req.events.forEach(function(evt){
    if(evt.status==='completed'&&!completedSet[evt.stage]){
      completedSet[evt.stage]=true;
      completedStages++;
    }
    if(evt.stage==='error'){
      completedSet[evt.stage]=true;
      completedStages++;
    }
  });
  var pct=Math.min(100,(completedStages/STAGES.length)*100);
  document.getElementById('pipeline-progress-fill').style.width=pct+'%';
}

function addEventLog(evt,stageName){
  var log=document.getElementById('event-log');
  var emptyState=log.querySelector('.empty-state');
  if(emptyState)log.innerHTML='';

  var clsMap={
    'request_received':'s-RECEIVED',
    'body_read':'s-READ',
    'json_validated':'s-VALID',
    'loading_api_key':'s-PROXY',
    'building_request':'s-PROXY',
    'setting_headers':'s-PROXY',
    'sending_request':'s-PROXY',
    'reading_response':'s-PROXY',
    'validating_status':'s-PROXY',
    'response_sent':'s-COMPLETE',
    'error':'s-ERROR',
    'token_stream':'s-PROXY',
    'info':'s-RECEIVED'
  };
  var cls=clsMap[evt.stage]||'s-RECEIVED';

  var d=new Date(evt.timestamp);
  var time=d.toLocaleTimeString('en-US',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'})+'.'+String(evt.timestamp%1000).padStart(3,'0');
  var badgeClass=evt.status==='started'?'started':evt.status==='completed'?'completed':evt.status==='error'?'error':'info';

  var row=document.createElement('div');
  row.className='event-row';
  row.innerHTML='<span class="event-time">'+time+'</span><span class="event-stage '+cls+'">'+stageName+'</span><span class="event-status"><span class="badge '+badgeClass+'">'+evt.status+'</span></span><span class="event-msg" title="'+escAttr(evt.message||'')+'">'+(evt.message||'')+'</span>';

  log.insertBefore(row,log.firstChild);
  if(log.children.length>200)log.removeChild(log.lastChild);
}

function addRequestToList(reqId){
  var list=document.getElementById('req-list');
  var emptyState=list.querySelector('.empty-state');
  if(emptyState)list.innerHTML='';
  var item=document.createElement('div');
  item.className='req-item';
  item.id='req-item-'+reqId;
  item.onclick=function(){selectRequest(reqId);};
  var short=reqId.replace('req-','').slice(-8);
  item.innerHTML='<span class="req-id">#'+short+'</span><span class="req-time">just now</span>';
  list.insertBefore(item,list.firstChild);

  var items=list.querySelectorAll('.req-item');
  for(var i=0;i<items.length;i++){
    if(items[i].id!=='req-item-'+reqId){
      var ts=lastRequestTimestamps[items[i].id.replace('req-item-','')];
      if(ts){
        var ago=Math.round((Date.now()-ts)/1000);
        var timeEl=items[i].querySelector('.req-time');
        if(timeEl)timeEl.textContent=ago<60?ago+'s ago':Math.round(ago/60)+'m ago';
      }
    }
  }
}

function selectRequest(reqId){
  currentRequestId=reqId;
  var items=document.querySelectorAll('.req-item');
  items.forEach(function(el){el.classList.remove('active');});
  var item=document.getElementById('req-item-'+reqId);
  if(item)item.classList.add('active');

  document.getElementById('pipeline-req-id').textContent='#'+reqId.replace('req-','').slice(-8);

  buildPipeline();
  var req=requests[reqId];
  if(req){
    req.events.forEach(function(evt){updatePipeline(evt);});
    updatePipelineProgress(reqId);
  }
}

var concurrencyRunning=false;

async function sendConcurrent(){
  if(concurrencyRunning)return;
  concurrencyRunning=true;

  var btn=document.getElementById('concurrent-fire-btn');
  btn.disabled=true;
  btn.textContent='Firing\u2026';

  var countInput=document.getElementById('concurrent-count');
  var count=Math.min(Math.max(parseInt(countInput.value)||10,1),100);
  countInput.value=count;

  var prompt=document.getElementById('concurrent-prompt').value||'Hello';
  var stream=document.getElementById('concurrent-stream').checked;
  var resultsDiv=document.getElementById('concurrent-results');
  var progressBar=document.getElementById('concurrent-progress-bar');
  var progressText=document.getElementById('concurrent-progress-text');
  var dotsDiv=document.getElementById('concurrent-dots');
  var progressWrap=document.getElementById('concurrent-progress');

  resultsDiv.innerHTML='';
  progressBar.style.width='0%';
  dotsDiv.innerHTML='';
  progressWrap.style.display='block';

  var startTime=performance.now();
  var completed=0;
  var succeeded=0;
  var failed=0;
  var times=[];

  for(var i=0;i<count;i++){
    var dot=document.createElement('span');
    dot.className='concurrent-dot';
    dot.title='Request #'+(i+1);
    dotsDiv.appendChild(dot);
  }

  for(var i=0;i<count;i++){
    var idx=i;
    var body={model:'gpt-4o',messages:[{role:'user',content:prompt+' #'+(idx+1)}],stream:stream};

    fetch('/chat',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify(body)
    }).then(async function(r){
      var elapsed=performance.now()-startTime;
      if(!r.ok){
        var text=await r.text();
        failed++;
        var d=dotsDiv.children[idx];
        if(d){d.className='concurrent-dot error';d.title='Request #'+(idx+1)+': '+text.substring(0,100);}
        return{ok:false,error:text,time:elapsed,index:idx};
      }
      var data=await r.json();
      succeeded++;
      var d=dotsDiv.children[idx];
      if(d){d.className='concurrent-dot success';d.title='Request #'+(idx+1)+': OK';}
      return{ok:true,data:data,time:elapsed,index:idx};
    }).catch(function(err){
      var elapsed=performance.now()-startTime;
      failed++;
      var d=dotsDiv.children[idx];
      if(d){d.className='concurrent-dot error';d.title='Request #'+(idx+1)+': '+err.message;}
      return{ok:false,error:err.message,time:elapsed,index:idx};
    }).then(function(r){
      completed++;
      times.push(r.time);
      var pct=(completed/count)*100;
      progressBar.style.width=pct+'%';
      progressText.textContent=completed+'/'+count+'  OK: '+succeeded+'  Fail: '+failed;
      if(completed===count){
        var sorted=times.slice().sort(function(a,b){return a-b;});
        var avgTime=times.length?(times.reduce(function(a,b){return a+b},0)/times.length).toFixed(0):'-';
        var minTime=times.length?sorted[0].toFixed(0):'-';
        var maxTime=times.length?sorted[sorted.length-1].toFixed(0):'-';
        var p50=sorted.length?sorted[Math.floor(sorted.length*0.5)].toFixed(0):'-';
        var p95=sorted.length?sorted[Math.floor(sorted.length*0.95)].toFixed(0):'-';
        var totalTime=(sorted.length?sorted[sorted.length-1]:0).toFixed(0);

        resultsDiv.innerHTML=
          '<table class="concurrent-summary">'+
          '<tr><td>Total</td><td><b>'+count+'</b></td></tr>'+
          '<tr><td>OK</td><td style="color:var(--green)"><b>'+succeeded+'</b></td></tr>'+
          '<tr><td>Failed</td><td style="color:var(--red)"><b>'+failed+'</b></td></tr>'+
          '<tr><td>Avg</td><td><b>'+avgTime+'ms</b></td></tr>'+
          '<tr><td>P50</td><td><b>'+p50+'ms</b></td></tr>'+
          '<tr><td>P95</td><td><b>'+p95+'ms</b></td></tr>'+
          '<tr><td>Min</td><td><b>'+minTime+'ms</b></td></tr>'+
          '<tr><td>Max</td><td><b>'+maxTime+'ms</b></td></tr>'+
          '</table>';

        concurrencyRunning=false;
        btn.disabled=false;
        btn.textContent='Fire';
      }
    });
  }
}

function sendRequest(e){
  e.preventDefault();
  var prompt=document.getElementById('prompt-input').value||'Hello';
  var body={model:'gpt-4o',messages:[{role:'user',content:prompt}],stream:streaming};
  fetch('/chat',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(body)
  }).then(function(r){
    if(!r.ok)return r.text().then(function(t){throw new Error(t)});
    if(streaming)return r.text().then(function(){
      document.getElementById('response-payload').innerHTML='<pre class="payload">[Streaming \u2014 see events for token data]</pre>';
    });
    return r.json().then(function(data){
      document.getElementById('response-payload').innerHTML='<pre class="payload">'+syntaxHighlight(JSON.stringify(data))+'</pre>';
    });
  }).catch(function(err){
    document.getElementById('response-payload').innerHTML='<pre class="payload" style="color:var(--red)">'+escHtml(err.message)+'</pre>';
  });
}

function syntaxHighlight(json){
  json=json.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  return json.replace(/("(\\u[a-fA-F0-9]{4}|\\[^u]|[^"\\])*"(\s*:)?|\b(true|false|null)\b|\b-?\d+(\.\d+)?([eE][+-]?\d+)?\b)/g,function(match){
    if(/^"/.test(match)){
      return /:$/.test(match)?'<span class="key">'+match+'</span>':'<span class="str">'+match+'</span>';
    }
    if(/true|false/.test(match))return'<span class="bool">'+match+'</span>';
    if(/null/.test(match))return'<span class="null">'+match+'</span>';
    return'<span class="num">'+match+'</span>';
  });
}

function escHtml(s){
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function escAttr(s){
  return String(s).replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

buildPipeline();
</script>
</body>
</html>`
