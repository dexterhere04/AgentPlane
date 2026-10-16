// Pipeline — §4 live telemetry rendered as a stage-by-stage flow, plus
// concurrency (Gantt) and load generation (fire N in parallel).

import { deriveRequests, PIPELINE, stageState, isOpen } from "../stream.js";
import { sendChat } from "../api.js";
import { esc, sectionHead, card, statusBadge, kv, emptyState, icon, badge, toast, toastError } from "../ui.js";
import { gantt, sparkline } from "../charts.js";
import { fmtInt, fmtMs, fmtTime, relTime } from "../format.js";

export const meta = {
  id: "pipeline",
  label: "Pipeline",
  eyebrow: "Request flow",
  group: "Run",
  icon: "route",
  description: "Every stage a request passes through, live concurrency and per-request latency.",
};

const PILL = { pending: "Waiting", active: "Running", done: "Done", error: "Error", blocked: "Blocked" };

export function render(ctx) {
  let frozen = false;
  let selected = null;
  let count = 5;
  let firing = false;

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow,
    title: "Pipeline",
    description: meta.description,
    actions: `
      <span id="pl-buffer"></span>
      <div class="seg" id="pl-flow" role="group" aria-label="Event playback">
        <button data-mode="stepped" class="is-on">Stepped</button>
        <button data-mode="realtime">Realtime</button>
      </div>
      <label class="hstack" style="gap:var(--space-2);margin:0"><input type="checkbox" role="switch" id="pl-freeze"> Freeze</label>
      <button class="outline" id="pl-clear">Clear</button>`,
  }) + `
    <div class="grid-kpi" id="pl-kpi"></div>
    <div class="split" style="margin-top:var(--space-6)">
      <section id="pl-main"></section>
      <section id="pl-list"></section>
    </div>
    <section id="pl-gantt" style="margin-top:var(--space-6)"></section>
    <section id="pl-stages" style="margin-top:var(--space-6)"></section>`;

  const kpiEl = ctx.root.querySelector("#pl-kpi");
  const mainEl = ctx.root.querySelector("#pl-main");
  const listEl = ctx.root.querySelector("#pl-list");
  const ganttEl = ctx.root.querySelector("#pl-gantt");
  const stagesEl = ctx.root.querySelector("#pl-stages");
  const bufferEl = ctx.root.querySelector("#pl-buffer");

  function drawKpi(reqs) {
    const inflight = reqs.filter(isOpen).length;
    const blocking = reqs.filter((r) => r.blocked).length;
    const errs = reqs.filter((r) => r.status === "error").length;
    const ended = reqs.filter((r) => r.endedAt);
    const lat = ended.map((r) => r.endedAt - r.startedAt);
    const avg = lat.length ? lat.reduce((a, b) => a + b, 0) / lat.length : null;
    const min = lat.length ? Math.min(...lat) : null;
    const max = lat.length ? Math.max(...lat) : null;
    const tiles = [
      ["Requests", fmtInt(reqs.length), "observed this session", "bolt", ""],
      ["In flight", fmtInt(inflight), inflight ? "processing now" : "idle", "layers", inflight ? "warn" : ""],
      ["Blocked", fmtInt(blocking), "guardrail blocks", "shield", blocking ? "bad" : ""],
      ["Errors", fmtInt(errs), reqs.length ? `${((errs / reqs.length) * 100).toFixed(0)}% of requests` : "none", "warning", errs ? "bad" : "good"],
      ["Avg latency", avg == null ? "—" : fmtMs(avg), "completed requests", "activity", ""],
      ["Min / Max", min == null ? "—" : `${fmtMs(min)} / ${fmtMs(max)}`, "latency range", "gauge", ""],
    ];
    kpiEl.innerHTML = tiles.map(([l, v, s, ic, t]) => `<article class="card stat has-icon"><span class="stat-icon">${icon(ic, 15)}</span><span class="stat-label">${esc(l)}</span><span class="stat-value${t ? " is-" + t : ""}">${esc(v)}</span><span class="stat-sub">${esc(s)}</span></article>`).join("");
  }

  function drawBuffer() {
    const n = ctx.stream.queueDepth;
    bufferEl.innerHTML = n > 0 ? badge(`buffer ${n}`, "warning", { dot: true }) : "";
  }

  function drawFlowToggle() {
    for (const b of ctx.root.querySelectorAll("#pl-flow [data-mode]")) {
      b.classList.toggle("is-on", b.dataset.mode === ctx.stream.flowMode);
    }
  }

  function drawList(reqs) {
    if (!reqs.length) {
      listEl.innerHTML = card("Recent requests", emptyState({ icon: "route", title: ctx.stream.connected ? "Waiting for traffic" : "Stream offline", body: ctx.stream.connected ? "Requests will appear here as the gateway processes them." : "Set the admin token to connect the event stream." }));
      return;
    }
    const rows = reqs.slice(0, 60).map((r) => `
      <button class="req-row" data-req="${esc(r.id)}" aria-current="${selected === r.id}">
        <span class="status-dot ${dotCls(r)}"></span>
        <span class="truncate"><span class="mono-id">${esc(r.model || r.user || r.id)}</span></span>
        <span class="text-light" style="font-size:var(--text-8)">${esc(r.endedAt ? fmtMs(r.endedAt - r.startedAt) : "…")}</span>
        <span class="text-light" style="font-size:var(--text-8)">${esc(relTime(r.endedAt || r.startedAt))}</span>
      </button>`).join("");
    const lat = reqs.filter((r) => r.endedAt).slice(0, 24).reverse().map((r) => r.endedAt - r.startedAt);
    const trend = lat.length >= 2
      ? `<span class="spark-wrap" title="End-to-end latency trend (recent first)">${sparkline(lat, { width: 130, height: 30 })}</span>`
      : "";
    listEl.innerHTML = card("Recent requests", `<div class="req-list">${rows}</div>`,
      { subtitle: `${reqs.length} request(s) on the stream`, actions: trend });
  }

  function dotCls(r) {
    if (r.blocked || r.status === "error") return "off";
    if (isOpen(r)) return "on";
    if (r.status === "warn") return "warn";
    return "on";
  }

  function drawDetail(reqs) {
    const r = reqs.find((x) => x.id === selected) || reqs[0];
    if (!r) { mainEl.innerHTML = ""; return; }
    selected = r.id;

    const open = isOpen(r);
    const doneCount = PIPELINE.filter((st) => ["done", "blocked"].includes(stageState(r, st.key))).length;
    const progress = (doneCount / PIPELINE.length) * 100;
    const reqState = r.blocked ? "blocked" : r.status === "error" ? "error" : r.endedAt ? "done" : "active";

    const stages = PIPELINE.map((st, i) => {
      const state = stageState(r, st.key);
      const info = r.stages.get(st.key);
      const meta = info?.duration ? fmtMs(info.duration) : info?.message ? esc(info.message.slice(0, 18)) : "";
      return `<div class="pipe-stage ${state}" title="${esc(st.key)}">
        <span class="pipe-node">${state === "error" ? "!" : state === "blocked" ? "✕" : state === "done" ? "✓" : i + 1}</span>
        <span class="pipe-stage-label">${esc(st.label)}</span>
        <span class="pipe-stage-ms">${meta}</span>
      </div>`;
    }).join("");

    mainEl.innerHTML = card(`Request #${esc(String(r.id).replace(/^req-/, "").slice(0, 8))}`, `
      <div class="vstack">
        <div class="flow-progress">
          <div class="progress-track"><div class="progress-fill${reqState === "error" || reqState === "blocked" ? " is-bad" : ""}" style="width:${progress}%"></div></div>
          <div class="row-between" style="font-size:var(--text-8)">
            <span class="text-light">${doneCount} / ${PIPELINE.length} stages complete</span>
            <span class="text-light">${r.endedAt ? esc(fmtMs(r.endedAt - r.startedAt)) + " total" : "in progress"}</span>
          </div>
        </div>
        ${r.blocked ? `<div role="alert" data-variant="warning"><div><strong>Blocked by guardrails</strong><div>${esc(r.blockedMessage || "request was denied before reaching the provider")}</div></div></div>` : ""}
        <div class="pipe">${stages}</div>
        ${kv([
          ["Status", statusBadge(open ? "in flight" : r.status)],
          r.authenticated ? ["Identity", `${esc(r.authenticated.username || "—")} <span class="mono-id">${esc(String(r.authenticated.user_id).slice(0, 8))}</span>`] : null,
          r.model ? ["Model", esc(r.model)] : null,
          r.provider ? ["Provider", esc(r.provider)] : null,
          r.usage ? ["Tokens", esc(fmtInt(r.usage.total_tokens))] : null,
          ["Started", esc(fmtTime(r.startedAt))],
          ["Events", esc(fmtInt(r.events.length))],
          r.error ? ["Error", `<span style="color:var(--danger)">${esc(r.error)}</span>`] : null,
        ])}
        <details><summary>Event timeline</summary><div class="timeline">
          ${r.events.slice(-24).map((e) => `<div class="timeline-row">
            <span class="log-time">${esc(fmtTime(e.timestamp))}</span>
            <span class="mono-id">${esc(e.stage)}${e.message ? ` — ${esc(e.message)}` : ""}</span>
            ${statusBadge(e.status)}
          </div>`).join("")}
        </div></details>
      </div>`);
  }

  function drawGantt(reqs) {
    const visible = [...reqs].sort((a, b) => a.startedAt - b.startedAt).slice(-24);
    const rows = visible.map((r) => ({
      id: r.id,
      label: `#${String(r.id).replace(/^req-/, "").slice(0, 8)}`,
      start: r.startedAt,
      end: r.endedAt || Date.now(),
      tone: r.blocked ? "blocked" : r.status === "error" ? "error" : isOpen(r) ? "running" : "done",
      meta: r.model || r.user || "",
    }));
    const legend = `<div class="legend">
      <span class="lg"><i class="lg-running"></i> in-flight</span>
      <span class="lg"><i class="lg-done"></i> completed</span>
      <span class="lg"><i class="lg-blocked"></i> blocked</span>
      <span class="lg"><i class="lg-error"></i> error</span>
    </div>`;
    ganttEl.innerHTML = card("Concurrent requests", legend + gantt(rows, { empty: "No requests yet — hit “Fire” to watch them overlap." }), {
      subtitle: "Each request's stages advance independently on the shared event bus — overlap below is real",
      actions: `
        <input type="number" min="1" max="50" value="${count}" id="pl-count" style="width:5rem" aria-label="Parallel requests">
        <button id="pl-fire" ${firing ? "disabled aria-busy=\"true\"" : ""}>${icon("bolt")} Fire ${count} in parallel</button>`,
    });
  }

  function drawStages(reqs) {
    const recent = reqs.slice(0, 40);
    const agg = new Map();
    for (const r of recent) for (const [k, v] of r.stages) {
      if (!v.duration) continue;
      const a = agg.get(k) || [];
      a.push(v.duration);
      agg.set(k, a);
    }
    const rows = PIPELINE.filter((st) => agg.has(st.key)).map((st) => {
      const arr = agg.get(st.key);
      const mean = arr.reduce((a, b) => a + b, 0) / arr.length;
      return { label: st.label, value: mean, text: fmtMs(mean) };
    }).sort((a, b) => b.value - a.value);
    stagesEl.innerHTML = card("Average stage latency", rows.length ? barRows(rows) : emptyState({ title: "No stage timings yet" }), { subtitle: "Mean duration per stage over the last 40 requests" });
  }

  function barRows(rows) {
    const max = Math.max(...rows.map((r) => r.value), 1);
    return `<div class="barlist">${rows.map((r) => `<div class="barlist-row">
      <span class="truncate" title="${esc(r.label)}">${esc(r.label)}</span>
      <span class="barlist-track"><span class="barlist-fill" style="width:${Math.max(2, (r.value / max) * 100)}%"></span></span>
      <span class="barlist-value">${esc(r.text)}</span>
    </div>`).join("")}</div>`;
  }

  function draw() {
    const reqs = deriveRequests(ctx.stream.list);
    drawKpi(reqs);
    drawList(reqs);
    drawDetail(reqs);
    drawGantt(reqs);
    drawStages(reqs);
  }

  async function fire() {
    const n = Math.max(1, Math.min(50, Number(count) || 1));
    firing = true;
    drawGantt(deriveRequests(ctx.stream.list));
    const jobs = Array.from({ length: n }, (_, i) =>
      sendChat({ model: "gpt-4o", messages: [{ role: "user", content: `Parallel request #${i + 1} — tell me a short fact.` }] }).catch((e) => e)
    );
    const results = await Promise.allSettled(jobs);
    firing = false;
    const failed = results.filter((r) => r.status === "rejected" || (r.value && r.value.status)).length;
    if (failed) toast(`${failed} of ${n} parallel requests did not complete cleanly`, { variant: "warning" });
    else toastOk(`Fired ${n} requests in parallel`);
    draw();
  }

  const unsub = ctx.stream.on(() => {
    drawBuffer();
    if (frozen) { drawKpi(deriveRequests(ctx.stream.list)); return; }
    draw();
  });

  ctx.root.addEventListener("click", (e) => {
    const mode = e.target.closest("#pl-flow [data-mode]");
    if (mode) { ctx.stream.setFlowMode(mode.dataset.mode); drawFlowToggle(); return; }
    if (e.target.closest("#pl-fire")) return fire();
    const row = e.target.closest("[data-req]");
    if (row) { selected = row.dataset.req; const reqs = deriveRequests(ctx.stream.list); drawDetail(reqs); drawList(reqs); }
  });
  ctx.root.addEventListener("input", (e) => {
    if (e.target.id === "pl-count") count = Math.max(1, Math.min(50, Number(e.target.value) || 1));
  });

  ctx.root.querySelector("#pl-freeze").addEventListener("change", (e) => {
    frozen = e.target.checked;
    if (!frozen) draw();
  });
  ctx.root.querySelector("#pl-clear").addEventListener("click", () => { ctx.stream.clear(); selected = null; draw(); });

  drawBuffer();
  drawFlowToggle();
  draw();
  return () => unsub();
}
