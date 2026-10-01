export interface Finding {
  guardrail: string;
  type: string;
  severity: string;
  start: number;
  end: number;
  entity: string;
  value: string;
}

export interface GuardrailData {
  guardrail: string;
  decision: string;
  direction: 'input' | 'output';
  message?: string;
  findings?: Finding[];
  before?: string;
  after?: string;
}

export interface ApiEvent {
  id: string;
  stage: string;
  status: string;
  timestamp: number;
  duration: number;
  data?: unknown;
  message: string;
  request_id: string;
}

export interface StageInfo {
  status: string;
  message: string;
  duration: number;
  time: number;
}

export interface RequestState {
  id: string;
  events: ApiEvent[];
  stages: Record<string, StageInfo>;
  inputGuards: GuardrailData[];
  outputGuards: GuardrailData[];
  blocked: boolean;
  blockedBy?: string;
  blockedMessage?: string;
  startedAt: number;
  endedAt?: number;
  authenticated?: { user_id: string; username: string };
  requestBody?: Record<string, unknown>;
  responseBody?: Record<string, unknown>;
  userPrompt?: string;
  assistantReply?: string;
}

export interface StreamState {
  requests: RequestState[];
  log: ApiEvent[];
  active: number;
  blocked: number;
  connected: boolean;
}

export const initialState: StreamState = {
  requests: [],
  log: [],
  active: 0,
  blocked: 0,
  connected: false
};
