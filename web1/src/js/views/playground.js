// Playground — §3 inference (chat completion). A small client app: a
// conversation transcript that exercises auth → guardrails → routing →
// provider → response, and surfaces the spec's error taxonomy.

import { getUserKey, setUserKey } from "../store.js";
import { ApiError, isRetryable } from "../api.js";
import { esc, sectionHead, card, toast, toastError, statusBadge, kv, icon, livePill } from "../ui.js";
import { pretty } from "../format.js";

export const meta = {
  id: "playground",
  label: "Playground",
  eyebrow: "Test a request",
  group: "Run",
  icon: "play",
  description: "Send prompts like a real client app — auth, guardrails and the provider round-trip, transparently.",
};

const MODEL_SUGGESTIONS = ["gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "claude-3-5-sonnet", "gemini-1.5-pro", "llama-3.1-70b", "deepseek-chat"];

/** Friendly mapping for the gateway's error contract (§3). */
function explain(err) {
  if (!(err instanceof ApiError)) return { title: "Request failed", body: String(err?.message || err), variant: "error" };
  switch (err.status) {
    case 400: return { title: "No model resolved", body: "Send a model, or configure DEFAULT_MODEL on the gateway.", variant: "warning" };
    case 401: return { title: "Missing or invalid API key", body: "Set a user API key (ap_live_…) above.", variant: "error" };
    case 403:
      if (err.type === "guardrail_blocked") return { title: "Blocked by a guardrail", body: err.message, variant: "warning" };
      return { title: "Denied by IAM", body: err.message || "ap:InvokeChat / ap:InvokeModel was denied.", variant: "error" };
    case 502: return { title: "Upstream provider failure", body: err.message || "The provider could not be reached.", variant: "error" };
    case 503:
      if (isRetryable(err)) return { title: "Temporarily unavailable (retryable)", body: err.message, variant: "warning" };
      return { title: "Gateway unavailable", body: err.message, variant: "error" };
    default: return { title: `HTTP ${err.status || "error"}`, body: err.message, variant: "error" };
  }
}

export function render(ctx) {
  ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Playground", description: meta.description }) + `
    <div class="split">
      <section>${card("Client application", `
        <div class="vstack">
          <div class="grid-2" style="gap:var(--space-4)">
            <label data-field>
              Model
              <input id="pg-model" list="pg-models" placeholder="gpt-4o" value="gpt-4o" autocomplete="off">
              <datalist id="pg-models">${MODEL_SUGGESTIONS.map((m) => `<option value="${esc(m)}"></option>`).join("")}</datalist>
            </label>
            <label data-field>
              API key
              <input id="pg-key" type="password" class="mono" autocomplete="off" spellcheck="false" placeholder="ap_live_…" value="${esc(getUserKey())}">
            </label>
          </div>
          <label data-field>
            System message <small data-hint>Optional</small>
            <textarea id="pg-system" rows="2" placeholder="You are a helpful assistant."></textarea>
          </label>
          <div class="chat-thread" id="pg-thread"></div>
          <label data-field style="margin:0">
            <textarea id="pg-input" rows="2" placeholder="Type a message…"></textarea>
          </label>
          <div class="row-between">
            <span class="hstack">
              <label class="hstack" style="gap:var(--space-2);margin:0"><input type="checkbox" role="switch" id="pg-stream"> Stream</label>
              <small class="text-light">Enter to send · Shift+Enter for a new line</small>
            </span>
            <span class="hstack">
              <button class="outline" id="pg-clear">Clear</button>
              <button id="pg-send">Send message</button>
            </span>
          </div>
          <div class="foot-hint" id="pg-foot"></div>
        </div>`,
        { subtitle: "Mimics a real app calling POST /chat: auth → input guardrails → provider → output guardrails" })}
      </section>
      <section id="pg-side"></section>
    </div>`;

  const threadEl = ctx.root.querySelector("#pg-thread");
  const inputEl = ctx.root.querySelector("#pg-input");
  const keyEl = ctx.root.querySelector("#pg-key");
  const footEl = ctx.root.querySelector("#pg-foot");
  const sideEl = ctx.root.querySelector("#pg-side");

  /** @type {{role:string, content:string, status?:number, latency?:number, model?:string, ts:number}[]} */
  const history = [];
  let busy = false;

  const model = () => ctx.root.querySelector("#pg-model").value.trim() || "gpt-4o";
  const system = () => ctx.root.querySelector("#pg-system").value.trim();
  const streaming = () => ctx.root.querySelector("#pg-stream").checked;

  function drawFoot() {
    const k = getUserKey();
    footEl.innerHTML = `<span>${icon("key", 12)} ${k ? `ap_live_…${esc(k.slice(-8))}` : "no key set"}</span><span>${icon("server", 12)} POST /chat</span>`;
  }

  function bubbleHTML(m, i) {
    const label = m.role === "user" ? "You" : m.role === "assistant" ? "AgentPlane" : "Gateway";
    const body = esc(m.content) || '<em class="text-light">(empty)</em>';
    const foot = [
      m.model ? `<span class="msg-tag">${esc(m.model)}</span>` : "",
      m.latency != null ? `<span class="msg-tag">${esc(m.latency)} ms</span>` : "",
      m.status != null ? badgeHTTP(m.status) : "",
    ].filter(Boolean).join("");
    return `<div class="msg ${m.role}" data-msg="${i}">
      <span class="msg-avatar ${m.role}">${m.role === "user" ? icon("bolt", 12) : m.role === "assistant" ? "A" : icon("warning", 12)}</span>
      <div class="msg-main">
        <div class="msg-head"><span class="msg-role">${esc(label)}</span>${foot}</div>
        <div class="msg-bubble"><div class="msg-body">${body}</div></div>
      </div>
    </div>`;
  }

  function badgeHTTP(status) {
    const tone = statusVariantHTTP(status);
    return `<span class="badge" data-variant="${tone}">HTTP ${status}</span>`;
  }

  function statusVariantHTTP(status) {
    if (status < 300) return "success";
    if (status === 401 || status === 403) return "warning";
    return "danger";
  }

  function drawThread() {
    if (!history.length) {
      threadEl.innerHTML = `<div class="msg system"><div class="msg-bubble"><div class="msg-body">Send a prompt and watch it flow through the full gateway — auth, guardrails, and the provider round-trip.</div></div></div>`;
      return;
    }
    threadEl.innerHTML = history.map(bubbleHTML).join("");
    threadEl.scrollTop = threadEl.scrollHeight;
  }

  function push(m) {
    history.push({ ts: Date.now(), ...m });
    drawThread();
  }

  function setBusy(on) {
    busy = on;
    const btn = ctx.root.querySelector("#pg-send");
    btn.disabled = on;
    btn.setAttribute("aria-busy", on ? "true" : "false");
  }

  function showTyping() {
    threadEl.insertAdjacentHTML("beforeend", `<div class="msg assistant" id="pg-typing">
      <span class="msg-avatar assistant">A</span>
      <div class="msg-main"><div class="msg-head"><span class="msg-role">AgentPlane</span></div>
      <div class="msg-bubble is-typing"><span class="typing-dot"></span><span class="typing-dot"></span><span class="typing-dot"></span></div></div>
    </div>`);
    threadEl.scrollTop = threadEl.scrollHeight;
  }

  function messagesFor(text) {
    const msgs = [];
    if (system()) msgs.push({ role: "system", content: system() });
    for (const m of history) if (m.role === "user" || m.role === "assistant") msgs.push({ role: m.role, content: m.content });
    msgs.push({ role: "user", content: text });
    return msgs;
  }

  async function sendNonStreaming(body, started) {
    const res = await fetch("/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${getUserKey()}` },
      body: JSON.stringify(body),
    });
    const json = await res.json().catch(() => null);
    if (!res.ok) {
      const e = json?.error || null;
      throw new ApiError(res.status, e?.type, e?.message || `HTTP ${res.status}`, json);
    }
    const content = json?.choices?.[0]?.message?.content ?? "";
    push({ role: "assistant", content, status: res.status, latency: Date.now() - started, model: json?.model || model() });
    if (json?.usage) push({ role: "system", content: `tokens: ${json.usage.total_tokens ?? "—"} (in ${json.usage.prompt_tokens ?? "—"} / out ${json.usage.completion_tokens ?? "—"})` });
    toast("Completion received", { variant: "success" });
  }

  async function sendStreaming(body, started) {
    const res = await fetch("/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${getUserKey()}` },
      body: JSON.stringify({ ...body, stream: true }),
    });
    if (!res.ok || !res.body) {
      let json = null;
      try { json = await res.json(); } catch { /* ignore */ }
      const e = json?.error || null;
      throw new ApiError(res.status, e?.type, e?.message || `HTTP ${res.status}`, json);
    }

    history.push({ role: "assistant", content: "", status: res.status, model: model(), ts: Date.now() });
    drawThread();
    const bodyEl = threadEl.querySelector(`[data-msg="${history.length - 1}"] .msg-body`);
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let text = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split("\n");
      buffer = parts.pop() || "";
      for (const line of parts) {
        const trimmed = line.trim();
        if (!trimmed.startsWith("data:")) continue;
        const payload = trimmed.slice(5).trim();
        if (payload === "[DONE]") continue;
        try {
          const chunk = JSON.parse(payload);
          const delta = chunk.choices?.[0]?.delta?.content || "";
          if (delta) { text += delta; bodyEl.textContent = text; threadEl.scrollTop = threadEl.scrollHeight; }
        } catch { /* ignore partial frame */ }
      }
    }
    history[history.length - 1].content = text;
    history[history.length - 1].latency = Date.now() - started;
    drawThread();
  }

  async function send() {
    if (busy) return;
    const text = inputEl.value.trim();
    if (!text) { inputEl.focus(); return; }
    setUserKey(keyEl.value.trim());
    const body = { model: model(), messages: messagesFor(text) };
    const started = Date.now();
    push({ role: "user", content: text });
    inputEl.value = "";
    setBusy(true);
    showTyping();
    try {
      if (streaming()) await sendStreaming(body, started);
      else await sendNonStreaming(body, started);
      drawFoot();
    } catch (err) {
      ctx.root.querySelector("#pg-typing")?.remove();
      const e = explain(err);
      push({ role: "error", content: `${e.title} — ${e.body}`, status: err?.status, latency: Date.now() - started });
      if (err?.status === 401) toastError(err, "Authentication");
    } finally {
      ctx.root.querySelector("#pg-typing")?.remove();
      setBusy(false);
      inputEl.focus();
    }
  }

  function drawSide() {
    const last = [...history].reverse().find((m) => m.role === "assistant");
    sideEl.innerHTML = card("Last response", last
      ? `<div class="vstack">
          <div class="row-between">${statusBadge(last.status ? "success" : "ok")}<span class="text-light" style="font-size:var(--text-8)">${esc(last.latency != null ? last.latency + " ms" : "")}</span></div>
          <div class="msg-bubble" style="max-width:100%"><div class="msg-body">${esc(last.content) || '<em class="text-light">(empty)</em>'}</div></div>
          ${kv([last.model ? ["Model", esc(last.model)] : null])}
        </div>`
      : `<div class="empty"><strong>No request yet</strong><span>Send a message to inspect the provider's reply.</span></div>`,
      { subtitle: "Newest assistant reply", actions: livePill(ctx.stream.connected) });
  }

  ctx.root.querySelector("#pg-send").addEventListener("click", send);
  ctx.root.querySelector("#pg-clear").addEventListener("click", () => { history.length = 0; drawThread(); drawSide(); });
  keyEl.addEventListener("change", () => { setUserKey(keyEl.value.trim()); drawFoot(); toast("User key saved", { variant: "success" }); });
  inputEl.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); }
  });

  const unsub = ctx.stream.on(() => drawSide());
  drawFoot();
  drawThread();
  drawSide();
  return () => unsub();
}
