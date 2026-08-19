import { useRef, useState } from 'react';
import { sendChat } from '../api';
import { Icon } from '../icons';

interface Msg {
  role: 'user' | 'assistant' | 'error';
  content: string;
}

export default function ChatOverlay({ userKey }: { userKey: string }) {
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
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
        push({ role: 'assistant', content: body?.choices?.[0]?.message?.content ?? JSON.stringify(res.body) });
      } else {
        const body = res.body as { error?: { message?: string } } | string;
        if (typeof body === 'object' && body?.error) push({ role: 'error', content: body.error.message ?? 'request blocked' });
        else push({ role: 'error', content: String(res.body) });
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
            <span className="cp-title">Test console</span>
            <span className="cp-status">
              <span className="live-dot on" />
              gateway
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
                <div className="msg-head"><span className="msg-role">{m.role}</span></div>
                <div className="msg-body">{m.content}</div>
              </div>
            ))}
          </div>
          <div className="chat-pop-input">
            <input
              className="input"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && send()}
              placeholder="Type a message…"
            />
            <button className="btn sm" disabled={busy} onClick={send}>
              {busy ? '…' : 'Send'}
            </button>
          </div>
          <div className="chat-pop-hint">Using key: {userKey ? 'ap_live_…' + userKey.slice(-8) : 'not set'}</div>
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
