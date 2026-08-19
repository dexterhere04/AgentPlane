import { useRef, useState } from 'react';
import { sendChat } from '../api';
import { Card } from './ui';
import { Icon } from '../icons';

interface Msg {
  role: 'user' | 'assistant' | 'error';
  content: string;
  meta?: string;
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

  const push = (m: Msg) =>
    setMessages((prev) => {
      const next = [...prev, m];
      requestAnimationFrame(() => {
        if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      });
      return next;
    });

  const send = async (text?: string) => {
    const prompt = (text ?? input).trim();
    if (!prompt || busy) return;
    setInput('');
    push({ role: 'user', content: prompt });
    setBusy(true);
    try {
      const res = await sendChat(prompt, { model, stream, userKey });
      if (res.ok) {
        const body = res.body as { choices?: { message?: { content?: string } }[] };
        const content = body?.choices?.[0]?.message?.content ?? JSON.stringify(res.body);
        push({ role: 'assistant', content, meta: 'HTTP ' + res.status });
      } else {
        const body = res.body as { error?: { type?: string; message?: string } } | string;
        if (typeof body === 'object' && body?.error) {
          push({ role: 'error', content: body.error.message ?? body.error.type ?? 'request blocked', meta: 'HTTP ' + res.status + ' · ' + (body.error.type ?? '') });
        } else {
          push({ role: 'error', content: String(res.body), meta: 'HTTP ' + res.status });
        }
      }
    } catch (err) {
      push({ role: 'error', content: String(err) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title="Client application"
      subtitle="Mimics a real app calling POST /chat — auth → input guardrails → provider → output guardrails."
      right={
        <div className="row">
          <label className="field" style={{ flexDirection: 'row', alignItems: 'center', gap: 7 }}>
            <input className="input key mono" type="password" value={userKey} onChange={(e) => onUserKeyChange(e.target.value)} placeholder="ap_live_..." />
          </label>
          <select className="select" value={model} onChange={(e) => setModel(e.target.value)}>
            <option value="gpt-4o">gpt-4o</option>
            <option value="gpt-4o-mini">gpt-4o-mini</option>
            <option value="gpt-4-turbo">gpt-4-turbo</option>
          </select>
          <label className="check-label">
            <input type="checkbox" checked={stream} onChange={(e) => setStream(e.target.checked)} />
            stream
          </label>
        </div>
      }
    >
      <div className="chat" ref={scrollRef}>
        {messages.length === 0 && (
          <div className="empty">
            <span className="empty-icon"><Icon name="message" size={20} /></span>
            <span>Send a message — it will pass through the whole gateway, and you'll see it in the Event Log.</span>
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} className={'msg ' + m.role}>
            <div className="msg-head">
              <span className="msg-role">{m.role}</span>
              {m.meta && <span className="msg-meta">{m.meta}</span>}
            </div>
            <div className="msg-body">{m.content}</div>
          </div>
        ))}
      </div>
      <div className="chat-input">
        <input className="input" value={input} onChange={(e) => setInput(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && send()} placeholder="Type a message…" />
        <button className="btn" disabled={busy} onClick={() => send()}>
          {busy ? '…' : 'Send'}
        </button>
      </div>
    </Card>
  );
}
