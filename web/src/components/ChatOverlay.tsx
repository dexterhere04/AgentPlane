import { useRef, useState } from 'react';
import { sendChat } from '../api';
import { Badge, Spinner, fmtTime } from './ui';
import { Icon } from '../icons';

interface Msg {
  role: 'user' | 'assistant' | 'error';
  content: string;
  status?: number;
  ts: number;
}

function statusTone(status?: number): string {
  if (status === undefined) return 'neutral';
  if (status < 300) return 'pass';
  if (status === 401 || status === 403) return 'block';
  return 'error';
}

export default function ChatOverlay({ userKey }: { userKey: string }) {
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
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

  const msgTitle = (role: Msg['role']) =>
    role === 'user' ? 'You' : role === 'assistant' ? 'AgentPlane' : 'Gateway';

  const send = async () => {
    const prompt = input.trim();
    if (!prompt || busy) return;
    setInput('');
    if (!userKey) {
      push({ role: 'error', content: 'No API key set — open Settings (gear → key icon) and paste a user key first.' });
      return;
    }
    push({ role: 'user', content: prompt });
    setBusy(true);
    try {
      const res = await sendChat(prompt, { userKey, model: 'gpt-4o' });
      if (res.ok) {
        const body = res.body as { choices?: { message?: { content?: string } }[] };
        push({ role: 'assistant', content: body?.choices?.[0]?.message?.content ?? JSON.stringify(res.body), status: res.status });
      } else {
        const body = res.body as { error?: { message?: string } } | string;
        if (typeof body === 'object' && body?.error) push({ role: 'error', content: body.error.message ?? 'request blocked', status: res.status });
        else push({ role: 'error', content: String(res.body), status: res.status });
      }
    } catch (err) {
      push({ role: 'error', content: String(err) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="chat-overlay">
      {open ? (
        <div className="chat-pop">
          <div className="chat-pop-head">
            <span className="cp-logo"><Icon name="bolt" size={15} /></span>
            <div className="cp-titles">
              <span className="cp-title">Test console</span>
              <span className="cp-sub">client → gateway round-trip</span>
            </div>
            <span className="cp-badges">
              <span className={'live-dot ' + (userKey ? 'on' : 'off')} />
              <Badge tone={userKey ? 'pass' : 'warn'}>{userKey ? 'key set' : 'no key'}</Badge>
            </span>
            <button className="cp-close" onClick={() => setOpen(false)} aria-label="Close">
              <Icon name="x" size={16} />
            </button>
          </div>

          <div className="chat-pop-msgs" ref={scrollRef}>
            {messages.length === 0 && (
              <div className="empty">
                <span className="empty-icon"><Icon name="message" size={18} /></span>
                <span>Test a prompt right here while you scroll — every request still flows through the full gateway.</span>
              </div>
            )}
            {messages.map((m, i) => (
              <div key={i} className={'msg ' + m.role}>
                <span className={'msg-avatar ' + m.role}>
                  {m.role === 'user' ? <Icon name="bolt" size={12} /> : m.role === 'assistant' ? 'A' : <Icon name="warning" size={12} />}
                </span>
                <div className="msg-main">
                  <div className="msg-head">
                    <span className="msg-role">{msgTitle(m.role)}</span>
                    <span className="msg-time">{fmtTime(m.ts)}</span>
                    {m.status !== undefined && <Badge tone={statusTone(m.status)}>HTTP {m.status}</Badge>}
                  </div>
                  <div className="msg-bubble">
                    <div className="msg-body">{m.content}</div>
                  </div>
                </div>
              </div>
            ))}
            {busy && (
              <div className="msg assistant">
                <span className="msg-avatar assistant">A</span>
                <div className="msg-main">
                  <div className="msg-head"><span className="msg-role">AgentPlane</span></div>
                  <div className="msg-bubble is-typing">
                    <span className="typing-dot" /><span className="typing-dot" /><span className="typing-dot" />
                  </div>
                </div>
              </div>
            )}
          </div>

          <div className="chat-pop-input">
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
            <button className="btn sm composer-send" disabled={busy || !input.trim()} onClick={send} aria-label="Send">
              {busy ? <Spinner /> : <Icon name="arrow" size={15} />}
            </button>
          </div>
          <div className="chat-pop-hint">
            <Icon name="key" size={11} />
            key: {userKey ? 'ap_live_…' + userKey.slice(-8) : 'not set'}
          </div>
        </div>
      ) : (
        <button className="chat-fab" onClick={() => setOpen(true)} aria-label="Open test console">
          <Icon name="message" size={22} />
          <span className={'fab-dot' + (userKey ? '' : ' off')} />
        </button>
      )}
    </div>
  );
}