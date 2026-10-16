// Event Log — the raw SSE stream (§4): every stage, decision and error.

import { STAGES } from "../stream.js";
import { esc, sectionHead, card, icon, monoId } from "../ui.js";
import { fmtInt, fmtMs, fmtTime } from "../format.js";

export const meta = {
  id: "events",
  label: "Event Log",
  eyebrow: "Live stream",
  group: "Observe",
  icon: "list",
  description: "The raw gateway event stream — every stage, decision and error, filterable.",
};

const MAX_ROWS = 600;
const MAX_RENDER = 500;

/** Stage-family colour coding (mirrors the gateway's pipeline phases). */
function stageClass(stage) {
  if (!stage) return "";
  if (stage.startsWith("guardrail")) return "sg-guardrail";
  if (stage === "error") return "sg-error";
  if (stage === "response_sent") return "sg-done";
  if (stage === "request_received" || stage === "info") return "sg-recd";
  if (stage === "body_read" || stage === "json_validated") return "sg-read";
  if (stage.startsWith("routing") || stage.startsWith("provider")) return "sg-route";
  return "sg-proxy";
}

export function render(ctx) {
  const filter = { q: "", status: "", stage: "", requestId: "", paused: false };
  let snapshot = [];
  let count = 0;

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow, title: "Event Log", description: meta.description,
    actions: `<label class="hstack" style="gap:var(--space-2);margin:0"><input type="checkbox" role="switch" id="ev-pause"> Pause</label>
      <button class="outline" id="ev-clear">${icon("trash")} Clear</button>`,
  }) + card("", `
    <div class="log-toolbar" style="margin-bottom:var(--space-4)">
      <fieldset class="group" style="flex:1;min-width:14rem">
        <input id="ev-q" placeholder="Filter by stage, message or request id…" aria-label="Filter events" value="">
        <span id="ev-q-clear"></span>
      </fieldset>
      <select id="ev-request" aria-label="Request filter" style="width:auto"><option value="">All requests</option></select>
      <select id="ev-status" aria-label="Status filter" style="width:auto"><option value="">All statuses</option><option>info</option><option>warn</option><option>error</option></select>
      <select id="ev-stage" aria-label="Stage filter" style="width:auto"><option value="">All stages</option>${STAGES.map((s) => `<option value="${esc(s)}">${esc(s)}</option>`).join("")}</select>
      <span id="ev-count" class="badge" data-variant="secondary">0 events</span>
    </div>
    <div class="log">
      <div class="log-head"><span>Time</span><span>Stage</span><span>Status</span><span>Request</span><span>Duration</span><span>Message</span></div>
      <div class="log-body" id="ev-body"></div>
    </div>`,
    { subtitle: "Newest at the bottom · retained history capped at 5000" });

  const body = ctx.root.querySelector("#ev-body");
  const countEl = ctx.root.querySelector("#ev-count");
  const reqSelect = ctx.root.querySelector("#ev-request");

  const source = () => (filter.paused ? snapshot : ctx.stream.list);

  const matches = (e) => {
    if (filter.status && e.status !== filter.status) return false;
    if (filter.stage && e.stage !== filter.stage) return false;
    if (filter.requestId && e.request_id !== filter.requestId) return false;
    if (filter.q) {
      const hay = `${e.stage} ${e.status} ${e.message || ""} ${e.request_id || ""}`.toLowerCase();
      if (!hay.includes(filter.q.toLowerCase())) return false;
    }
    return true;
  };

  const rowHTML = (e) => {
    const cls = e.status === "error" ? "status-error" : e.status === "warn" ? "status-warn" : "";
    const active = filter.requestId && e.request_id === filter.requestId;
    return `<div class="log-row ${cls}">
      <span class="log-time">${esc(fmtTime(e.timestamp))}</span>
      <span class="log-stage ${stageClass(e.stage)}"><i class="e-dot"></i>${esc(e.stage)}</span>
      <span>${esc(e.status || "info")}</span>
      <span><button class="req-chip${active ? " is-active" : ""}" data-req-filter="${esc(e.request_id || "")}" title="Filter to this request">#${esc(String(e.request_id || "").replace(/^req-/, "").slice(0, 8))}</button></span>
      <span class="log-time">${e.duration ? esc(fmtMs(e.duration)) : "—"}</span>
      <span class="log-msg" title="${esc(e.message || "")}">${esc(e.message || "")}</span>
    </div>`;
  };

  function drawFull() {
    const list = source().filter(matches).slice(-MAX_RENDER);
    body.innerHTML = list.length ? list.map(rowHTML).join("") : `<div class="log-empty">${filter.requestId || filter.q || filter.status || filter.stage ? "No events match the current filter." : "No events yet — send a request to see the stream."}</div>`;
    updateCount();
    body.scrollTop = body.scrollHeight;
  }

  function updateCount() {
    countEl.textContent = `${fmtInt(ctx.stream.list.length)} events`;
  }

  function appendLive(e) {
    if (!matches(e)) return;
    if (body.querySelector(".log-empty")) body.innerHTML = "";
    const atBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
    body.insertAdjacentHTML("beforeend", rowHTML(e));
    while (body.childElementCount > MAX_ROWS) body.removeChild(body.firstElementChild);
    if (atBottom) body.scrollTop = body.scrollHeight;
    updateCount();
  }

  function refreshRequestOptions() {
    const ids = [...new Set(ctx.stream.list.map((e) => e.request_id).filter(Boolean))].slice(-100).reverse();
    const cur = filter.requestId;
    reqSelect.innerHTML = `<option value="">All requests</option>` + ids.map((id) => `<option value="${esc(id)}"${id === cur ? " selected" : ""}>#${esc(String(id).replace(/^req-/, "").slice(0, 8))}</option>`).join("");
  }

  const unsub = ctx.stream.on(({ evt }) => {
    if (!evt) { updateCount(); refreshRequestOptions(); return; }
    if (!filter.paused) {
      refreshRequestOptions();
      appendLive(evt);
    }
  });

  ctx.root.querySelector("#ev-pause").addEventListener("change", (e) => {
    filter.paused = e.target.checked;
    if (filter.paused) { snapshot = ctx.stream.list.slice(); drawFull(); }
    else drawFull();
  });
  ctx.root.querySelector("#ev-clear").addEventListener("click", () => { ctx.stream.clear(); drawFull(); });
  ctx.root.querySelector("#ev-q").addEventListener("input", (e) => {
    filter.q = e.target.value;
    ctx.root.querySelector("#ev-q-clear").innerHTML = filter.q ? `<button class="ghost small" id="ev-q-x" aria-label="Clear search">${icon("x", 13)}</button>` : "";
    drawFull();
  });
  ctx.root.querySelector("#ev-q-clear").addEventListener("click", (e) => {
    if (!e.target.closest("#ev-q-x")) return;
    filter.q = "";
    ctx.root.querySelector("#ev-q").value = "";
    ctx.root.querySelector("#ev-q-clear").innerHTML = "";
    drawFull();
  });
  reqSelect.addEventListener("change", (e) => { filter.requestId = e.target.value; drawFull(); });
  ctx.root.querySelector("#ev-status").addEventListener("change", (e) => { filter.status = e.target.value; drawFull(); });
  ctx.root.querySelector("#ev-stage").addEventListener("change", (e) => { filter.stage = e.target.value; drawFull(); });

  body.addEventListener("click", (e) => {
    const b = e.target.closest("[data-req-filter]");
    if (!b) return;
    const id = b.dataset.reqFilter;
    filter.requestId = filter.requestId === id ? "" : id;
    reqSelect.value = filter.requestId;
    drawFull();
  });

  drawFull();
  refreshRequestOptions();
  return () => unsub();
}
