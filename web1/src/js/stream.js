// Live telemetry stream (§4). Consumes the gateway SSE event stream, de-dupes
// by event id, caps retained history, folds events into per-request records, and
// paces events through a FIFO queue so the pipeline visibly flows.
//
// Event shape (internal/observability/event.go):
//   { id, stage, status, timestamp (ms), duration (ms), data, message, request_id }

const HISTORY_CAP = 5000;
const STEP_MS = 45;

/** Every stage the gateway can emit — used by the event-log filter. */
export const STAGES = [
  "request_received", "body_read", "json_validated", "loading_api_key",
  "building_request", "setting_headers", "sending_request", "waiting_response",
  "reading_response", "validating_status", "response_sent",
  "guardrail_input", "guardrail_output", "guardrail_blocked", "guardrail_passed",
  "routing_decision", "routing_failover", "provider_retry", "provider_failover",
  "token_stream", "info", "error",
];

/**
 * Canonical request pipeline, in the order the gateway emits it
 * (internal/handlers/chat.go + internal/proxy/openai.go):
 *   request_received → body_read → json_validated → guardrail_input
 *     → loading_api_key → building_request → setting_headers → sending_request
 *     → reading_response → validating_status → response_sent (provider returns)
 *     → guardrail_output → info (handler wrote the response back).
 */
export const PIPELINE = [
  { key: "request_received", label: "Received", icon: "bolt" },
  { key: "body_read", label: "Body", icon: "message" },
  { key: "json_validated", label: "Validated", icon: "check" },
  { key: "guardrail_input", label: "Input guards", icon: "shield" },
  { key: "loading_api_key", label: "API key", icon: "key" },
  { key: "building_request", label: "Build", icon: "route" },
  { key: "setting_headers", label: "Headers", icon: "list" },
  { key: "sending_request", label: "Send", icon: "arrow" },
  { key: "reading_response", label: "Read", icon: "message" },
  { key: "validating_status", label: "Status", icon: "check" },
  { key: "response_sent", label: "Provider", icon: "bolt" },
  { key: "guardrail_output", label: "Output guards", icon: "shield" },
  { key: "info", label: "Responded", icon: "check" },
];

/** Ordered keys used for "stage reached" ranking. */
const RANK = new Map(PIPELINE.map((s, i) => [s.key, i]));
const GUARDRAIL_STAGES = new Set(["guardrail_input", "guardrail_output"]);

function stageRank(stage) {
  if (RANK.has(stage)) return RANK.get(stage);
  if (stage === "provider_retry" || stage === "provider_failover") return RANK.get("sending_request");
  if (stage === "guardrail_blocked" || stage === "guardrail_passed") return RANK.get("guardrail_input");
  return -1;
}

/* ── Stream consumer ─────────────────────────────────────────────────── */

/**
 * @param {() => string} getToken
 * @param {{stepMs?: number}} [opts]
 */
export function createGatewayStream(getToken, { stepMs = STEP_MS } = {}) {
  const listeners = new Set();
  const seen = new Set();
  const state = { list: [], connected: false, queueDepth: 0 };
  let es = null;
  let currentToken = null;

  // FIFO queue: drains one event per tick in "stepped" mode so the pipeline can
  // be watched advancing; drains instantly in "realtime".
  let mode = "stepped";
  let queue = [];
  let timer = null;

  const notify = (evt) => {
    for (const fn of listeners) {
      try { fn({ evt, connected: state.connected, queueDepth: state.queueDepth, flowMode: mode }); }
      catch (e) { console.error(e); }
    }
  };

  const setConnected = (v) => {
    if (state.connected === v) return;
    state.connected = v;
    notify(null);
  };

  const commit = (raw) => {
    state.list.push(raw);
    if (state.list.length > HISTORY_CAP) {
      const drop = state.list.splice(0, state.list.length - HISTORY_CAP);
      for (const d of drop) seen.delete(d.id);
    }
    notify(raw);
  };

  const updateDepth = (n) => {
    if (state.queueDepth === n) return;
    state.queueDepth = n;
    notify(null);
  };

  const drainAll = () => {
    if (timer) { clearTimeout(timer); timer = null; }
    while (queue.length) commit(queue.shift());
    updateDepth(0);
  };

  const tick = () => {
    timer = null;
    if (!queue.length) { updateDepth(0); return; }
    commit(queue.shift());
    updateDepth(queue.length);
    if (queue.length) timer = setTimeout(tick, stepMs);
  };

  const enqueue = (raw) => {
    queue.push(raw);
    updateDepth(queue.length);
    if (mode === "realtime") drainAll();
    else if (timer === null) timer = setTimeout(tick, stepMs);
  };

  function close() {
    if (es) { es.close(); es = null; }
  }

  function open() {
    close();
    const token = getToken();
    currentToken = token;
    if (!token) { setConnected(false); return; }
    es = new EventSource(`/events?token=${encodeURIComponent(token)}`);
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.onmessage = (e) => {
      let raw;
      try { raw = JSON.parse(e.data); } catch { return; /* ignore malformed frame */ }
      if (!raw || !raw.id || seen.has(raw.id)) return;
      seen.add(raw.id);
      enqueue(raw);
    };
  }

  return {
    get list() { return state.list; },
    get connected() { return state.connected; },
    get queueDepth() { return state.queueDepth; },
    get flowMode() { return mode; },
    sync() { if (getToken() !== currentToken) open(); else if (!es) open(); },
    open,
    close,
    setFlowMode(next) {
      if (next !== "stepped" && next !== "realtime") return;
      if (mode === next) return;
      mode = next;
      if (next === "realtime") drainAll();
      else if (queue.length && timer === null) timer = setTimeout(tick, stepMs);
      notify(null);
    },
    on(fn) { listeners.add(fn); return () => listeners.delete(fn); },
    off(fn) { listeners.delete(fn); },
    clear() {
      state.list = [];
      seen.clear();
      queue = [];
      if (timer) { clearTimeout(timer); timer = null; }
      updateDepth(0);
      notify(null);
    },
  };
}

/* ── Request folding ─────────────────────────────────────────────────── */

/** Parse the guardrail detail the gateway attaches to guardrail_input/output. */
function parseGuardrail(evt) {
  const d = evt.data;
  if (!d || typeof d !== "object" || typeof d.guardrail !== "string") return null;
  return {
    guardrail: d.guardrail,
    decision: String(d.decision || "pass"),
    direction: d.direction === "output" ? "output" : "input",
    message: typeof d.message === "string" ? d.message : "",
    findings: Array.isArray(d.findings) ? d.findings : [],
    before: typeof d.before === "string" ? d.before : undefined,
    after: typeof d.after === "string" ? d.after : undefined,
  };
}

function isEnforced(decision) {
  const d = String(decision || "pass").toLowerCase();
  return d !== "pass" && d !== "log_only";
}

/** Which guardrail phase blocked a request (inferred from how far it got). */
export function blockStage(req) {
  const reachedProvider = req.events.some(
    (e) => e.stage === "response_sent" || e.stage === "reading_response" || e.stage === "loading_api_key"
  );
  return reachedProvider ? "guardrail_output" : "guardrail_input";
}

/** Stage state for the pipeline visualisation: pending | active | done | error | blocked. */
export function stageState(req, key) {
  const blockedAt = req.blocked ? blockStage(req) : null;
  const evts = req.events.filter((e) => e.stage === key);

  if (GUARDRAIL_STAGES.has(key)) {
    if (blockedAt === key) return "blocked";
    return evts.length ? "done" : "pending";
  }
  if (!evts.length) return "pending";
  if (evts.some((e) => e.status === "error")) return "error";
  if (key === "info") return req.blocked ? "pending" : "done";

  const last = evts[evts.length - 1];
  if (last.status === "started" || last.status === "authenticated") return "active";
  return "done";
}

/** A request is "open" while it has not produced its terminal event. */
export function isOpen(req) {
  return !req.endedAt && Date.now() - (req.endedAt || 0) < 120000;
}

/** Fold the raw event list into per-request records (newest first). */
export function deriveRequests(list) {
  const map = new Map();
  for (const evt of list) {
    const id = evt.request_id || evt.id;
    let req = map.get(id);
    if (!req) {
      req = {
        id,
        events: [],
        stages: new Map(),     // stage -> { status, message, duration, time }
        inputGuards: [],
        outputGuards: [],
        blocked: false,
        blockedMessage: "",
        authenticated: null,
        prompt: "",
        reply: "",
        usage: null,
        responseModel: "",
        status: "info",
        model: "",
        provider: "",
        user: "",
        startedAt: evt.timestamp,
        endedAt: 0,
        error: "",
      };
      map.set(id, req);
    }

    req.events.push(evt);
    req.startedAt = Math.min(req.startedAt ?? evt.timestamp, evt.timestamp ?? Infinity);
    req.endedAt = Math.max(req.endedAt || 0, 0);

    req.stages.set(evt.stage, {
      status: evt.status,
      message: evt.message || "",
      duration: evt.duration || 0,
      time: evt.timestamp,
    });

    if (evt.status === "error") { req.status = "error"; req.error = evt.message || req.error; }
    else if (evt.status === "warn" && req.status !== "error") req.status = "warn";

    if (evt.stage === "guardrail_input" || evt.stage === "guardrail_output") {
      const g = parseGuardrail(evt);
      if (g) (g.direction === "output" ? req.outputGuards : req.inputGuards).push(g);
    }
    if (evt.stage === "guardrail_blocked") { req.blocked = true; req.blockedMessage = evt.message || req.blockedMessage; }

    if (evt.stage === "request_received" && evt.status === "authenticated" && evt.data && typeof evt.data === "object") {
      req.authenticated = {
        user_id: String(evt.data.user_id || ""),
        username: String(evt.data.username || ""),
      };
      req.user = req.authenticated.username || req.authenticated.user_id || req.user;
    }

    if (evt.stage === "body_read" && evt.data && typeof evt.data === "object") {
      const d = evt.data;
      if (typeof d.prompt === "string") req.prompt = d.prompt;
      if (Array.isArray(d.messages) && d.messages.length) {
        const last = d.messages[d.messages.length - 1];
        if (last && typeof last.content === "string") req.prompt = last.content;
      }
    }

    if (evt.stage === "response_sent" && evt.data && typeof evt.data === "object") {
      const d = evt.data;
      if (typeof d.model === "string") req.responseModel = d.model;
      if (d.usage && typeof d.usage === "object") req.usage = d.usage;
      if (Array.isArray(d.choices) && d.choices.length) {
        const c = d.choices[0];
        if (c && c.message && typeof c.message.content === "string") req.reply = c.message.content;
      }
    }

    const d = evt.data;
    if (d && typeof d === "object" && !Array.isArray(d)) {
      if (typeof d.model === "string") req.model = d.model;
      if (typeof d.provider === "string") req.provider = d.provider;
      if (typeof d.username === "string") req.user = d.username;
    }

    if (evt.stage === "info") req.endedAt = evt.timestamp;
    else if (evt.stage === "error" || evt.stage === "guardrail_blocked") req.endedAt = evt.timestamp;
    else if (evt.stage === "response_sent" && evt.status === "completed" && !req.endedAt) req.endedAt = evt.timestamp;

    if (req.status !== "error" && req.endedAt) req.status = "ok";
  }

  return [...map.values()].sort((a, b) => (b.startedAt || 0) - (a.startedAt || 0));
}

export { isEnforced, stageRank };
