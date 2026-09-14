import { useRef, useState } from 'react';
import { sendChat } from '../api';
import { Badge, Card, Disclosure, EmptyState, JsonBlock, Spinner, fmtTime } from './ui';
import { Icon } from '../icons';

interface Msg {
  role: 'user' | 'assistant' | 'error';
  content: string;
  status?: number;
  latency?: number;
  model?: string;
  raw?: unknown;
  ts: number;
}

function statusTone(status?: number): string {
  if (status === undefined) return 'neutral';
  if (status < 300) return 'pass';
  if (status === 401 || status === 403) return 'block';
  return 'error';
}

function latencyTone(ms?: number): string {
  if (ms === undefined) return '';
  if (ms < 500) return 'tone-fast';
  if (ms < 2000) return 'tone-mid';
  return 'tone-slow';
}

function errorCode(status?: number): string {
  if (status === undefined) return 'error';
  if (status === 401) return 'unauthorized';
  if (status === 403) return 'forbidden';
  if (status === 429) return 'rate limited';
  if (status >= 500) return 'upstream error';
  if (status >= 400) return 'request rejected';
  return 'error';
}

export default function ChatPanel({
  userKey,
  onUserKeyChange
}: {
  userKey: string;
  onUserKeyChange: (v: string) => void;
}) {
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('Hello! Explain what you are in one sentence.');
  const [stream, setStream] = useState(false);
  const [model, setModel] = useState('gpt-4o');
  const [busy, setBusy] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const push = (m: Omit<Msg, 'ts'>) =>
    setMessages((prev) => {
      const next = [...prev, { ...m, ts: Date.now() }];
      requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      });
      return next;
    });

  const send = async (text?: string) => {
    const prompt = (text ?? input).trim();
    if (!prompt || busy) return;
    setInput('');
    push({ role: 'user', content: prompt, model });
    setBusy(true);
    const t0 = performance.now();
    try {
      const res = await sendChat(prompt, { model, stream, userKey });
      const latency = Math.round(performance.now() - t0);
      if (res.ok) {
        const body = res.body as { choices?: { message?: { content?: string } }[] };
        const content = body?.choices?.[0]?.message?.content ?? JSON.stringify(res.body);
        push({ role: 'assistant', content, status: res.status, latency, model, raw: res.body });
      } else {
        const body = res.body as { error?: { type?: string; message?: string } } | string;
        if (typeof body === 'object' && body?.error) {
          push({
            role: 'error',
            content: body.error.message ?? body.error.type ?? 'request blocked',
            status: res.status,
            latency,
            model,
            raw: res.body
          });
        } else {
          push({ role: 'error', content: String(res.body), status: res.status, latency, model, raw: res.body });
        }
      }
    } catch (err) {
      push({ role: 'error', content: String(err), model });
    } finally {
      setBusy(false);
    }
  };

  const msgTitle = (role: Msg['role']) =>
    role === 'user' ? 'You' : role === 'assistant' ? 'AgentPlane' : 'Gateway';

  return (
    <Card
      title="Client application"
      subtitle="Mimics a real app calling POST /chat — auth → input guardrails → provider → output guardrails."
      right={
        <div className="row">
          <div className="field">
            <span className="f-label">API key</span>
            <input
              className="input mono ckey"
              type="password"
              value={userKey}
              onChange={(e) => onUserKeyChange(e.target.value)}
              placeholder="ap_live_…"
            />
          </div>
          <div className="field">
            <span className="f-label">Model</span>
            <select className="select" value={model} onChange={(e) => setModel(e.target.value)}>
              <option value="gpt-4o">gpt-4o</option>
              <option value="gpt-4o-mini">gpt-4o-mini</option>
              <option value="gpt-4-turbo">gpt-4-turbo</option>
            </select>
          </div>
          <label className="check-label" title="Stream the response (SSE)">
            <input type="checkbox" checked={stream} onChange={(e) => setStream(e.target.checked)} />
            stream
          </label>
        </div>
      }
    >
      <div className="chat" ref={scrollRef}>
        {messages.length === 0 && (
          <EmptyState
            icon={<Icon name="message" size={20} />}
            text="No messages yet — send a prompt and watch it flow through the full gateway: auth, guardrails, and the provider round-trip."
          />
        )}

        {messages.map((m, i) => (
          <div key={i} className={'msg ' + m.role}>
            <span className={'msg-avatar ' + m.role}>
              {m.role === 'user'
                ? <Icon name="bolt" size={13} />
                : m.role === 'assistant'
                  ? 'A'
                  : <Icon name="warning" size={13} />}
            </span>
            <div className="msg-main">
              <div className="msg-head">
                <span className="msg-role">{msgTitle(m.role)}</span>
                <span className="msg-time">{fmtTime(m.ts)}</span>
                {m.status !== undefined && <Badge tone={statusTone(m.status)}>HTTP {m.status}</Badge>}
              </div>

              {m.role === 'error' ? (
                <div className="msg-bubble">
                  <div className="msg-error-head">
                    <Icon name="warning" size={14} />
                    <span>Request failed</span>
                    <span className="msg-error-code">{errorCode(m.status)}</span>
                  </div>
                  <div className="msg-error-body">{m.content}</div>
                  {(m.latency !== undefined || m.model || m.raw !== undefined) && (
                    <div className="msg-meta-rows">
                      <div className="msg-meta-row">
                        <span className="msg-meta-label">Timing</span>
                        {m.latency !== undefined && (
                          <span className={'msg-tag ' + latencyTone(m.latency)}>{m.latency} ms</span>
                        )}
                      </div>
                      <div className="msg-meta-row">
                        <span className="msg-meta-label">Model</span>
                        {m.model && <span className="msg-tag">{m.model}</span>}
                      </div>
                    </div>
                  )}
                  {m.raw !== undefined && (
                    <div className="msg-error-details">
                      <Disclosure title="Raw response" subtitle="Gateway error payload">
                        <JsonBlock value={m.raw} maxHeight={220} />
                      </Disclosure>
                    </div>
                  )}
                </div>
              ) : (
                <div className="msg-bubble">
                  <div className="msg-body">{m.content}</div>
                  {(m.latency !== undefined || m.model) && (
                    <div className="msg-meta-rows">
                      {m.model && (
                        <div className="msg-meta-row">
                          <span className="msg-meta-label">Model</span>
                          <span className="msg-tag">{m.model}</span>
                        </div>
                      )}
                      {m.latency !== undefined && (
                        <div className="msg-meta-row">
                          <span className="msg-meta-label">Latency</span>
                          <span className={'msg-tag ' + latencyTone(m.latency)}>{m.latency} ms</span>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        ))}

        {busy && (
          <div className="msg assistant">
            <span className="msg-avatar assistant">A</span>
            <div className="msg-main">
              <div className="msg-head">
                <span className="msg-role">AgentPlane</span>
                <span className="msg-time">{fmtTime(Date.now())}</span>
              </div>
              <div className="msg-bubble is-typing">
                <span className="typing-dot" /><span className="typing-dot" /><span className="typing-dot" />
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="chat-composer">
        <div className="composer-row">
          <textarea
            className="input composer-input"
            rows={1}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                send();
              }
            }}
            placeholder="Type a message…"
          />
          <button className="btn composer-send" disabled={busy || !input.trim()} onClick={() => send()} aria-label="Send message">
            {busy ? <Spinner /> : <Icon name="arrow" size={16} />}
          </button>
        </div>
        <div className="composer-hint">
          <span className="composer-key">
            <Icon name="key" size={11} />
            {userKey ? 'ap_live_…' + userKey.slice(-8) : 'no key set — add one above'}
          </span>
          <span>Enter to send · Shift+Enter for a new line</span>
        </div>
      </div>
    </Card>
  );
}