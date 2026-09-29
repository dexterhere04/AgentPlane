import {
  ApiEvent,
  GuardrailData,
  RequestState,
  StreamState,
  initialState
} from './types';

const MAX_LOG = 500;

function parseGuardrail(evt: ApiEvent): GuardrailData | null {
  if (!evt.data || typeof evt.data !== 'object') return null;
  const d = evt.data as Record<string, unknown>;
  if (typeof d.guardrail !== 'string') return null;
  return {
    guardrail: d.guardrail,
    decision: String(d.decision ?? 'pass'),
    direction: d.direction === 'output' ? 'output' : 'input',
    message: typeof d.message === 'string' ? d.message : undefined,
    findings: Array.isArray(d.findings) ? (d.findings as GuardrailData['findings']) : undefined,
    before: typeof d.before === 'string' ? d.before : undefined,
    after: typeof d.after === 'string' ? d.after : undefined
  };
}

export function reduceStream(state: StreamState, evt: ApiEvent): StreamState {
  const requests = state.requests.slice();
  let idx = requests.findIndex((r) => r.id === evt.request_id);
  let active = state.active;

  if (idx === -1) {
    idx = requests.length;
    requests.push({
      id: evt.request_id,
      events: [],
      stages: {},
      inputGuards: [],
      outputGuards: [],
      blocked: false,
      startedAt: evt.timestamp
    });
    active += 1;
  }

  const req: RequestState = { ...requests[idx] };
  req.events = [...req.events, evt];
  req.stages = {
    ...req.stages,
    [evt.stage]: {
      status: evt.status,
      message: evt.message,
      duration: evt.duration,
      time: evt.timestamp
    }
  };

  if (evt.stage === 'guardrail_input' || evt.stage === 'guardrail_output') {
    const g = parseGuardrail(evt);
    if (g) {
      if (g.direction === 'output') req.outputGuards = [...req.outputGuards, g];
      else req.inputGuards = [...req.inputGuards, g];
    }
  }

  if (evt.stage === 'guardrail_blocked') {
    req.blocked = true;
    req.blockedMessage = evt.message;
  }

  if (
    evt.stage === 'request_received' &&
    evt.status === 'authenticated' &&
    evt.data &&
    typeof evt.data === 'object'
  ) {
    const d = evt.data as Record<string, unknown>;
    req.authenticated = {
      user_id: String(d.user_id ?? ''),
      username: String(d.username ?? '')
    };
  }

  if (evt.stage === 'body_read' && evt.data && typeof evt.data === 'object') {
    const d = evt.data as Record<string, unknown>;
    req.requestBody = d;
    if (Array.isArray(d.messages) && d.messages.length) {
      const last = d.messages[d.messages.length - 1] as Record<string, unknown>;
      if (typeof last.content === 'string') req.userPrompt = last.content;
    }
  }

  if (evt.stage === 'response_sent' && evt.data && typeof evt.data === 'object') {
    const d = evt.data as Record<string, unknown>;
    req.responseBody = d;
    if (Array.isArray(d.choices) && d.choices.length) {
      const c = d.choices[0] as Record<string, unknown>;
      const m = c.message as Record<string, unknown> | undefined;
      if (m && typeof m.content === 'string') req.assistantReply = m.content;
    }
  }

  if (!req.endedAt) {
    if (
      evt.stage === 'response_sent' &&
      (evt.status === 'completed' || evt.status === 'error')
    ) {
      req.endedAt = evt.timestamp;
      active = Math.max(0, active - 1);
    } else if (evt.stage === 'error') {
      req.endedAt = evt.timestamp;
      active = Math.max(0, active - 1);
    }
  }

  let blocked = state.blocked;
  if (evt.stage === 'guardrail_blocked') blocked += 1;

  requests[idx] = req;

  const log = [evt, ...state.log].slice(0, MAX_LOG);

  return { requests, log, active, blocked, connected: state.connected };
}

export function streamConnected(state: StreamState, connected: boolean): StreamState {
  return { ...state, connected };
}

export function clearStream(): StreamState {
  return { ...initialState, connected: false };
}

export { initialState };
