import { ApiEvent } from './types';

export type FlowMode = 'stepped' | 'realtime';

export interface EventQueue {
  enqueue: (e: ApiEvent) => void;
  size: () => number;
  setMode: (m: FlowMode) => void;
  dispose: () => void;
}

const STEP_MS = 45;

/**
 * A FIFO event queue. Incoming SSE events are buffered here and drained,
 * in arrival order, into the stream store.
 *
 * - "stepped"   drains one event per tick so the pipeline visibly flows.
 * - "realtime"  drains the whole buffer immediately (still FIFO).
 */
export function createEventQueue(
  onEvent: (e: ApiEvent) => void,
  onDepth: (n: number) => void = () => {}
): EventQueue {
  let q: ApiEvent[] = [];
  let mode: FlowMode = 'stepped';
  let timer: ReturnType<typeof setTimeout> | null = null;

  const emitDepth = () => onDepth(q.length);

  const drainAll = () => {
    while (q.length) onEvent(q.shift()!);
    emitDepth();
  };

  const tick = () => {
    timer = null;
    if (!q.length) return;
    onEvent(q.shift()!);
    emitDepth();
    if (q.length) timer = setTimeout(tick, STEP_MS);
  };

  return {
    enqueue(e) {
      q.push(e);
      emitDepth();
      if (mode === 'realtime') {
        if (timer) {
          clearTimeout(timer);
          timer = null;
        }
        drainAll();
      } else if (timer === null) {
        timer = setTimeout(tick, STEP_MS);
      }
    },
    size: () => q.length,
    setMode(m) {
      mode = m;
      if (m === 'realtime') {
        if (timer) {
          clearTimeout(timer);
          timer = null;
        }
        drainAll();
      }
    },
    dispose() {
      if (timer) clearTimeout(timer);
      timer = null;
      q = [];
    }
  };
}
