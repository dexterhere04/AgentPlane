// Floating assistant. A minimal chat client that posts to POST /chat using the
// stored user API key — always available from anywhere in the control plane.

import { getUserKey } from "./store.js";
import { ApiError } from "./api.js";
import { esc, icon, toast, toastError } from "./ui.js";

export function mountChatOverlay(ctx) {
  const host = document.getElementById("chat-overlay");
  const history = [];

  const fab = document.createElement("button");
  fab.className = "chat-fab";
  fab.title = "Open assistant";
  fab.setAttribute("aria-label", "Open assistant");
  fab.innerHTML = icon("zap", 18);
  document.body.appendChild(fab);

  let open = false;

  function setOpen(v) {
    open = v;
    host.hidden = !v;
    fab.hidden = v;
    if (v) renderPanel();
  }

  fab.addEventListener("click", () => setOpen(true));

  function renderPanel() {
    host.innerHTML = `<article class="card">
      <div class="card-head">
        <div><h3>Assistant</h3><p class="text-light">Ask the gateway anything</p></div>
        <button class="outline small" id="as-close" aria-label="Close">${icon("x")}</button>
      </div>
      <div class="chat">
        <div class="chat-thread" id="as-thread"></div>
        <label data-field>
          <textarea id="as-input" rows="2" placeholder="${getUserKey() ? "Send a message…" : "Set a user API key in settings first"}" ${getUserKey() ? "" : "disabled"}></textarea>
        </label>
        <div class="row-between">
          <small class="text-light">POST /chat</small>
          <span class="hstack">
            ${getUserKey() ? "" : `<button class="outline small" id="as-settings">Settings</button>`}
            <button id="as-send" ${getUserKey() ? "" : "disabled"}>Send</button>
          </span>
        </div>
      </div>
    </article>`;

    host.querySelector("#as-close").addEventListener("click", () => setOpen(false));
    host.querySelector("#as-settings")?.addEventListener("click", () => { setOpen(false); ctx.openSettings(); });
    const input = host.querySelector("#as-input");
    host.querySelector("#as-send").addEventListener("click", send);
    input?.addEventListener("keydown", (e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") send(); });

    drawThread();
    input?.focus();
  }

  function bubble(role, content) {
    const avatar = role === "user" ? "You" : role === "assistant" ? "A" : "!";
    const cls = role === "system" ? "error" : role;
    return `<div class="msg ${role}">
      <span class="msg-avatar ${cls}">${esc(avatar)}</span>
      <div class="msg-main"><div class="msg-bubble"><div class="msg-body">${esc(content)}</div></div></div>
    </div>`;
  }

  function drawThread() {
    const thread = host.querySelector("#as-thread");
    if (!thread) return;
    if (!history.length) {
      thread.innerHTML = `<div class="msg system"><div class="msg-main"><div class="msg-bubble"><div class="msg-body">Hi — this is a quick chat client pointed at your gateway. Try: “Explain what this control plane does.”</div></div></div></div>`;
    } else {
      thread.innerHTML = history.map((m) => bubble(m.role, m.content)).join("");
    }
    thread.scrollTop = thread.scrollHeight;
  }

  async function send() {
    const input = host.querySelector("#as-input");
    const text = input.value.trim();
    if (!text) return;
    const key = getUserKey();
    if (!key) { toast("No user API key configured", { variant: "danger" }); return; }

    history.push({ role: "user", content: text });
    input.value = "";
    drawThread();

    const thread = host.querySelector("#as-thread");
    thread.insertAdjacentHTML("beforeend", `<div class="msg assistant" id="as-pending"><span class="msg-avatar assistant">A</span><div class="msg-main"><div class="msg-bubble is-typing"><span class="typing-dot"></span><span class="typing-dot"></span><span class="typing-dot"></span></div></div></div>`);
    thread.scrollTop = thread.scrollHeight;

    const btn = host.querySelector("#as-send");
    btn.disabled = true;
    btn.setAttribute("aria-busy", "true");

    try {
      const res = await fetch("/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${key}` },
        body: JSON.stringify({ model: "gpt-4o", messages: history.map((m) => ({ role: m.role, content: m.content })) }),
      });
      const json = await res.json().catch(() => null);
      if (!res.ok) {
        const e = json?.error || null;
        throw new ApiError(res.status, e?.type, e?.message || `HTTP ${res.status}`, json);
      }
      const content = json?.choices?.[0]?.message?.content ?? "(empty response)";
      history.push({ role: "assistant", content });
      host.querySelector("#as-pending")?.remove();
      drawThread();
    } catch (err) {
      host.querySelector("#as-pending")?.remove();
      history.push({ role: "system", content: `⚠ ${err.message || err}` });
      drawThread();
      toastError(err, "Assistant");
    } finally {
      btn.disabled = false;
      btn.removeAttribute("aria-busy");
      input.focus();
    }
  }

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && open) setOpen(false);
  });
}
