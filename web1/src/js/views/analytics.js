// Analytics — §10.1 typed observability + §10.2 routing analytics, presented as
// a single vertical, filter-driven dashboard: a global filter bar, KPI strip,
// time-series charts, breakdowns, guardrail outcomes, a failure/trace explorer
// and per-user spend. Auto-refreshes every 5s.

import * as api from "../api.js";
import { createPoller } from "../poll.js";
import {
  esc, sectionHead, card, table, badge, statusBadge, errorBox, loadingBox,
  emptyState, icon, statTile, monoId, decisionBadge, kv,
} from "../ui.js";
import { barList, donut, timeSeries } from "../charts.js";
import { fmtInt, fmtCompact, fmtMoney, fmtMs, fmtRate, fmtPct, fmtDateTime, fmtTime } from "../format.js";

export const meta = {
  id: "analytics",
  label: "Analytics",
  eyebrow: "Traces, tokens & cost",
  group: "Observe",
  icon: "chart",
  description: "ClickHouse-backed traces, token usage, cost, latency percentiles and guardrail actions.",
};

const RANGES = [
  { value: "15m", label: "Last 15 minutes" },
  { value: "1h", label: "Last hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];
const STATUSES = [
  { value: "", label: "All statuses" },
  { value: "success", label: "Success only" },
  { value: "error", label: "Errors only" },
];
const COL = {
  req: "var(--primary)",
  err: "var(--danger)",
  p50: "var(--success)",
  p95: "var(--warning)",
  p99: "var(--danger)",
  in: "var(--primary)",
  out: "#8b5cf6",
  cost: "var(--warning)",
};

const traceShort = (id) => String(id || "").replace(/^(trace|req)-/, "").slice(0, 10);

export function render(ctx) {
  if (!ctx.hasAdminToken()) {
    ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Analytics", description: meta.description }) +
      `<div role="alert"><div><strong>Admin token required.</strong>
        <div>Observability and analytics use the admin token (ap:ViewMetrics).
        <button class="ghost small" id="an-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`;
    ctx.root.querySelector("#an-settings").addEventListener("click", ctx.openSettings);
    return;
  }

  /* ── Filter state (deep-linkable via the URL hash) ───────────── */

  const filters = readFilters();
  let auto = true;
  let promptUser = "";
  let promptSearch = "";
  let promptDebounce = null;
  let traceCursor = "";
  let tracePrev = [];

  function readFilters() {
    const p = new URLSearchParams((location.hash.split("?")[1] || ""));
    return {
      range: RANGES.some((r) => r.value === p.get("range")) ? p.get("range") : "24h",
      status: ["", "success", "error"].includes(p.get("status") || "") ? (p.get("status") || "") : "",
      provider: p.get("provider") || "",
      model: p.get("model") || "",
      route: p.get("route") || "",
      user_id: p.get("user_id") || "",
    };
  }
  function writeFilters() {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(filters)) if (v) p.set(k, v);
    const qs = p.toString();
    history.replaceState(null, "", `#analytics${qs ? "?" + qs : ""}`);
  }
  const activeFilters = () => ({
    status: filters.status, provider: filters.provider, model: filters.model,
    route: filters.route, user_id: filters.user_id,
  });
  const hasFilters = () => !!(filters.status || filters.provider || filters.model || filters.route || filters.user_id);

  /* ── Markup ──────────────────────────────────────────────────── */

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow, title: "Analytics", description: meta.description,
    actions: `
      <label class="hstack" style="gap:var(--space-2);margin:0"><input type="checkbox" role="switch" id="an-auto" checked> Auto 5s</label>
      <button class="outline" id="an-refresh">${icon("refresh")} Refresh</button>
      <a class="outline button" id="an-grafana" href="/grafana/" target="_blank" rel="noopener" title="Open the Grafana deep-dive dashboards (requires the grafana service)">${icon("external")} Grafana</a>
      <span id="an-health"></span>`,
  }) + `
    <section class="filter-bar" id="an-filters"></section>
    <section id="an-kpis"></section>
    <section id="an-traffic" style="margin-top:var(--space-6)"></section>
    <section class="split" style="margin-top:var(--space-6)">
      <section id="an-latency"></section>
      <section id="an-tokens"></section>
    </section>
    <section id="an-breakdowns" style="margin-top:var(--space-6)"></section>
    <section class="split" style="margin-top:var(--space-6)">
      <section id="an-guardrails"></section>
      <section id="an-failures"></section>
    </section>
    <section id="an-traces" style="margin-top:var(--space-6)"></section>
    <section id="an-users" style="margin-top:var(--space-6)"></section>
    <section id="an-prompts" style="margin-top:var(--space-6)"></section>`;

  const q = (id) => ctx.root.querySelector("#" + id);
  const healthEl = q("an-health");

  /* ── Filter bar ──────────────────────────────────────────────── */

  function drawFilters(opts = {}) {
    const options = (list, value, placeholder) =>
      `<option value="">${esc(placeholder)}</option>` + list.map((o) => `<option value="${esc(o)}"${o === value ? " selected" : ""}>${esc(o)}</option>`).join("");
    q("an-filters").innerHTML = `
      <div class="hstack">
        <select id="f-range" aria-label="Time range" style="width:auto">${RANGES.map((r) => `<option value="${r.value}"${r.value === filters.range ? " selected" : ""}>${esc(r.label)}</option>`).join("")}</select>
        <select id="f-status" aria-label="Status" style="width:auto">${STATUSES.map((s) => `<option value="${s.value}"${s.value === filters.status ? " selected" : ""}>${esc(s.label)}</option>`).join("")}</select>
        <select id="f-provider" aria-label="Provider" style="width:auto">${options(opts.providers || [], filters.provider, "All providers")}</select>
        <select id="f-model" aria-label="Model" style="width:auto">${options(opts.models || [], filters.model, "All models")}</select>
        <select id="f-route" aria-label="Route" style="width:auto">${options(opts.routes || [], filters.route, "All routes")}</select>
        <input id="f-user" aria-label="User id" placeholder="user id" value="${esc(filters.user_id)}" style="max-width:13rem">
        ${hasFilters() ? `<button class="outline small" id="f-clear">${icon("x")} Clear</button>` : ""}
        <span class="topbar-spacer"></span>
        <span class="text-light" style="font-size:var(--text-8)">${esc(filters.range)} window${hasFilters() ? " · filtered" : ""}</span>
      </div>`;

    q("f-range").addEventListener("change", (e) => { filters.range = e.target.value; writeFilters(); reload(); });
    q("f-status").addEventListener("change", (e) => { filters.status = e.target.value; writeFilters(); reload(); });
    q("f-provider").addEventListener("change", (e) => { filters.provider = e.target.value; writeFilters(); reload(); });
    q("f-model").addEventListener("change", (e) => { filters.model = e.target.value; writeFilters(); reload(); });
    q("f-route").addEventListener("change", (e) => { filters.route = e.target.value; writeFilters(); reload(); });
    q("f-user").addEventListener("change", (e) => { filters.user_id = e.target.value.trim(); writeFilters(); reload(); });
    q("f-clear")?.addEventListener("click", () => {
      Object.assign(filters, { status: "", provider: "", model: "", route: "", user_id: "" });
      writeFilters(); reload();
    });
  }

  let lastOptions = {};
  async function loadFilterOptions() {
    try {
      const bd = await api.obsBreakdowns(filters.range, activeFilters());
      const names = (arr) => (arr || []).map((x) => x.key).filter(Boolean);
      lastOptions = { providers: names(bd.provider), models: names(bd.model), routes: names(bd.route) };
      drawFilters(lastOptions);
    } catch { /* keep the current options */ }
  }

  /* ── Health ──────────────────────────────────────────────────── */

  async function loadHealth() {
    try {
      const h = await api.obsHealth();
      const state = h?.data_state || (h?.clickhouse_reachable === false ? "unavailable" : "unknown");
      const map = { fresh: "success", stale: "warning", empty: "secondary", unavailable: "danger" };
      healthEl.innerHTML = badge(`ClickHouse: ${state}`, map[state] || "secondary", { dot: true });
    } catch {
      healthEl.innerHTML = badge("ClickHouse: unavailable", "danger", { dot: true });
    }
  }

  /* ── KPI strip ───────────────────────────────────────────────── */

  async function loadKpis() {
    const el = q("an-kpis");
    el.innerHTML = loadingBox("Loading overview");
    try {
      const o = await api.obsOverview(filters.range, activeFilters());
      if (!o) { el.innerHTML = ""; return; }
      const lat = o.latency_ms || {};
      const tok = o.tokens || {};
      el.innerHTML = `<div class="grid-kpi tight">
        ${statTile({ label: "Requests", value: fmtInt(o.requests), sub: `${fmtInt(o.successes)} succeeded`, icon: "bolt" })}
        ${statTile({ label: "Success rate", value: fmtRate(1 - (o.error_rate || 0)), sub: `${fmtInt(o.errors)} errors`, tone: (o.error_rate || 0) > 0.05 ? "bad" : "good", icon: "check" })}
        ${statTile({ label: "p50 latency", value: fmtMs(lat.p50_ms), icon: "activity" })}
        ${statTile({ label: "p95 latency", value: fmtMs(lat.p95_ms), tone: (lat.p95_ms || 0) > 2000 ? "warn" : "", icon: "activity" })}
        ${statTile({ label: "p99 latency", value: fmtMs(lat.p99_ms), icon: "activity" })}
        ${statTile({ label: "Tokens", value: fmtCompact(tok.total), sub: `in ${fmtCompact(tok.input)} · out ${fmtCompact(tok.output)}`, icon: "message" })}
        ${statTile({ label: "Estimated cost", value: fmtMoney(o.estimated_cost), icon: "dollar" })}
      </div>`;
    } catch (e) { el.innerHTML = card("Overview", errorBox(e)); }
  }

  /* ── Time series ─────────────────────────────────────────────── */

  function xFormatter() {
    if (filters.range === "7d") return (t) => new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
    return (t) => new Date(t).toLocaleTimeString(undefined, { hour12: false, hour: "2-digit", minute: "2-digit" });
  }
  const moneyAxis = (n) => "$" + (Math.abs(n) >= 1 ? n.toFixed(2) : n.toFixed(3));

  async function loadSeries() {
    const trafficEl = q("an-traffic");
    const latencyEl = q("an-latency");
    const tokensEl = q("an-tokens");
    try {
      const s = await api.obsSeries(filters.range, activeFilters());
      const buckets = s.buckets || [];
      const sub = `Bucketed every ${esc(s.bucket_size || "—")} · ${buckets.length} bucket(s)`;

      trafficEl.innerHTML = card("Traffic & error rate", timeSeries({
        buckets, height: 190, xFormat: xFormatter(), leftFormat: (n) => fmtCompact(n), rightFormat: (n) => `${n.toFixed(n < 10 ? 1 : 0)}%`,
        series: [
          { label: "Requests", accessor: (b) => b.requests, kind: "bar", color: COL.req, fmt: fmtInt },
          { label: "Error rate", accessor: (b) => (b.error_rate || 0) * 100, kind: "line", color: COL.err, axis: "right", fmt: (n) => fmtPct(n) },
        ],
        empty: "No requests in this window",
      }), { subtitle: sub });

      latencyEl.innerHTML = card("Latency percentiles", timeSeries({
        buckets, height: 190, xFormat: xFormatter(), leftFormat: (n) => fmtMs(n),
        series: [
          { label: "p95", accessor: (b) => b.latency_ms?.p95_ms, kind: "area", color: COL.p95, fmt: fmtMs },
          { label: "p50", accessor: (b) => b.latency_ms?.p50_ms, kind: "line", color: COL.p50, fmt: fmtMs },
          { label: "p99", accessor: (b) => b.latency_ms?.p99_ms, kind: "line", color: COL.p99, fmt: fmtMs },
        ],
        empty: "No latency data in this window",
      }), { subtitle: "Milliseconds over time" });

      tokensEl.innerHTML = card("Tokens & cost", timeSeries({
        buckets, height: 190, xFormat: xFormatter(), leftFormat: (n) => fmtCompact(n), rightFormat: moneyAxis,
        series: [
          { label: "Input", accessor: (b) => b.tokens?.input, kind: "bar", stack: "tok", color: COL.in, fmt: fmtCompact },
          { label: "Output", accessor: (b) => b.tokens?.output, kind: "bar", stack: "tok", color: COL.out, fmt: fmtCompact },
          { label: "Cost", accessor: (b) => b.estimated_cost, kind: "line", color: COL.cost, axis: "right", fmt: (n) => fmtMoney(n) },
        ],
        empty: "No token usage in this window",
      }), { subtitle: "Stacked tokens · estimated cost" });
    } catch (e) {
      trafficEl.innerHTML = card("Series", errorBox(e));
      latencyEl.innerHTML = "";
      tokensEl.innerHTML = "";
    }
  }

  /* ── Breakdowns ──────────────────────────────────────────────── */

  async function loadBreakdowns() {
    const el = q("an-breakdowns");
    el.innerHTML = loadingBox("Loading breakdowns");
    try {
      const bd = await api.obsBreakdowns(filters.range, activeFilters());
      const providers = bd.provider || [];
      const models = bd.model || [];
      const routes = bd.route || [];
      const row = (r) => {
        const lat = r.latency_ms || {};
        const tok = r.tokens || {};
        return [
          `<strong>${esc(r.key || "—")}</strong>`,
          fmtInt(r.requests), fmtInt(r.errors), fmtRate(r.error_rate),
          fmtMs(lat.p50_ms), fmtMs(lat.p95_ms),
          fmtCompact(tok.total), fmtMoney(r.estimated_cost),
        ];
      };
      const cols = ["Key", "Requests", "Errors", "Error rate", "p50", "p95", "Tokens", "Cost"];

      el.innerHTML = `
        <div class="split">
          ${card("By provider", table(cols, providers.map(row), { empty: "No provider activity in this window" }))}
          ${card("Request share", donut(providers.map((p) => ({ label: p.key || "—", value: p.requests || 0 })), { empty: "No data" }))}
        </div>
        <div class="split split-even" style="margin-top:var(--space-6)">
          ${card("By model", table(cols, models.map(row), { empty: "No model activity in this window" }))}
          ${card("Requests by model", barList(models.map((m) => ({ label: m.key || "—", value: m.requests || 0, text: fmtInt(m.requests) })), { empty: "No data" }))}
        </div>
        <div class="split" style="margin-top:var(--space-6)">
          ${card("By route", table(cols, routes.map(row), { empty: "No route activity in this window" }))}
          ${card("Error rate by provider", barList(providers.map((p) => ({ label: p.key || "—", value: (p.error_rate || 0) * 100, text: fmtRate(p.error_rate), alt: true })), { empty: "No data" }))}
        </div>`;
    } catch (e) { el.innerHTML = card("Breakdowns", errorBox(e)); }
  }

  /* ── Guardrails ──────────────────────────────────────────────── */

  async function loadGuardrails() {
    const el = q("an-guardrails");
    el.innerHTML = loadingBox("Loading guardrails");
    try {
      const g = await api.obsGuardrailsFull(filters.range, 100, activeFilters());
      const rows = g.breakdown.map((x) => [esc(x.name || "—"), esc(x.phase || "—"), decisionBadge(x.action), fmtInt(x.count)]);
      const bar = g.breakdown.filter((x) => Number.isFinite(x.count)).map((x) => ({ label: `${x.name || "?"} · ${x.phase || ""}`, value: x.count, text: fmtInt(x.count) }));
      const recent = g.recent.slice(0, 8).map((r) => `<div class="timeline-row"><span class="log-time">${esc(r.timestamp ? fmtTime(r.timestamp) : "—")}</span><span class="mono-id">${esc(r.name || "—")}</span>${decisionBadge(r.action)}</div>`).join("");
      el.innerHTML = card("Guardrail outcomes",
        `<div class="vstack">
           ${table(["Guardrail", "Phase", "Action", "Count"], rows, { empty: "No guardrail actions in this window" })}
           ${bar.length ? `<div style="margin-top:var(--space-4)">${barList(bar)}</div>` : ""}
           ${recent ? `<details><summary>Recent guardrail events</summary><div class="timeline">${recent}</div></details>` : ""}
         </div>`,
        { subtitle: "Aggregated actions from ClickHouse" });
    } catch (e) { el.innerHTML = card("Guardrails", errorBox(e)); }
  }

  /* ── Failures + trace explorer ───────────────────────────────── */

  function traceTable(items, { empty, compact = false }) {
    const cols = compact
      ? ["Trace", "Model", "Status", "Latency"]
      : ["Trace", "Time", "Provider", "Model", "Status", "Latency", "Tokens", "Cost"];
    const rows = items.map((t) => {
      const first = `<button class="link-btn" data-trace="${esc(t.trace_id)}">${esc(traceShort(t.trace_id))}</button>`;
      const status = t.guardrail_action ? decisionBadge(t.guardrail_action) : statusBadge(t.status);
      if (compact) return [first, esc(t.model || "—"), status, fmtMs(t.latency_ms)];
      return [
        first,
        esc(t.timestamp ? fmtTime(t.timestamp) : "—"),
        esc(t.provider || "—"),
        esc(t.model || "—"),
        status,
        fmtMs(t.latency_ms),
        fmtCompact(t.total_tokens),
        fmtMoney(t.estimated_cost),
      ];
    });
    return table(cols, rows, { empty });
  }

  async function loadFailures() {
    const el = q("an-failures");
    el.innerHTML = loadingBox("Loading failures");
    try {
      const items = await api.obsFailures(filters.range, 6, activeFilters());
      el.innerHTML = card("Recent failures", traceTable(items, { empty: "No failures in this window", compact: true }),
        { subtitle: "Newest failed traces", actions: badge(`${items.length}`, items.length ? "danger" : "secondary") });
    } catch (e) { el.innerHTML = card("Recent failures", errorBox(e)); }
  }

  async function loadTraces(reset = false) {
    const el = q("an-traces");
    if (reset) { traceCursor = ""; tracePrev = []; }
    el.innerHTML = loadingBox("Loading traces");
    try {
      const page = await api.obsTraces({ range: filters.range, limit: 25, cursor: traceCursor || undefined, ...activeFilters() });
      const items = page.items || [];
      el.innerHTML = card("Trace explorer", `
        ${traceTable(items, { empty: "No traces in this window" })}
        <div class="row-between" style="margin-top:var(--space-4)">
          <span class="text-light" style="font-size:var(--text-8)">${items.length} row(s) · newest first</span>
          <span class="hstack">
            <button class="outline small" id="tr-prev" ${tracePrev.length ? "" : "disabled"}>← Prev</button>
            <button class="outline small" id="tr-next" ${page.next_cursor ? "" : "disabled"}>Next →</button>
          </span>
        </div>`,
        { subtitle: "Keyset-paginated trace summaries — click a trace to inspect it" });

      el.querySelector("#tr-next")?.addEventListener("click", () => { tracePrev.push(traceCursor); traceCursor = page.next_cursor; loadTraces(); });
      el.querySelector("#tr-prev")?.addEventListener("click", () => { traceCursor = tracePrev.pop() || ""; loadTraces(); });
      el.querySelectorAll("[data-trace]").forEach((b) => b.addEventListener("click", () => openTrace(b.dataset.trace)));
    } catch (e) { el.innerHTML = card("Trace explorer", errorBox(e)); }
  }

  async function openTrace(id) {
    const dlg = document.createElement("dialog");
    dlg.classList.add("wide");
    dlg.innerHTML = `<form method="dialog">
      <header><h3>Trace ${esc(traceShort(id))}</h3><p class="text-light">Loading…</p></header>
      <div class="vstack" id="td-body">${loadingBox()}</div>
      <footer><button value="close" class="outline">Close</button></footer>
    </form>`;
    document.body.appendChild(dlg);
    dlg.addEventListener("close", () => dlg.remove());
    dlg.showModal();
    try {
      const t = await api.obsTrace(id, filters.range);
      const cap = (c) => (c?.available ? badge(c.mode || "captured", "secondary") : badge("not captured", "secondary"));
      dlg.querySelector("header p").textContent = `${t.provider || "—"} · ${t.model || "—"} · ${t.timestamp ? fmtDateTime(t.timestamp) : "—"}`;
      dlg.querySelector("#td-body").innerHTML = `
        <div class="grid-2">
          <div>${kv([
            ["Status", statusBadge(t.status)],
            ["Latency", esc(fmtMs(t.latency_ms))],
            ["Route", esc(t.route || "—")],
            ["Guardrail", t.guardrail_action ? decisionBadge(t.guardrail_action) : "—"],
          ])}</div>
          <div>${kv([
            ["User", esc(t.username || t.user_id || "—")],
            ["Tokens", `${fmtCompact(t.total_tokens)} <span class="text-light">(in ${fmtCompact(t.input_tokens)} / out ${fmtCompact(t.output_tokens)})</span>`],
            ["Est. cost", esc(fmtMoney(t.estimated_cost))],
            ["Trace ID", monoId(t.trace_id)],
          ])}</div>
        </div>
        <div class="row-between">
          <span class="hstack">${cap(t.prompt)} prompt ${cap(t.response)} response</span>
          <span class="text-light" style="font-size:var(--text-8)">capture metadata only — payloads are never returned</span>
        </div>
        <h4 style="margin:0">Guardrail events</h4>
        ${(t.guardrails || []).length
          ? `<div class="timeline">${t.guardrails.map((g) => `<div class="timeline-row"><span class="log-time">${esc(g.timestamp ? fmtTime(g.timestamp) : "—")}</span><span class="mono-id">${esc(g.name)} · ${esc(g.phase)}</span>${decisionBadge(g.action)}</div>`).join("")}</div>`
          : `<div class="hint">No guardrail events recorded for this trace.</div>`}
        <h4 style="margin:0">Tools</h4>
        ${(t.tools || []).length
          ? table(["Tool", "Success", "Latency"], t.tools.map((x) => [esc(x.name), x.success ? badge("ok", "success") : badge("failed", "danger"), fmtMs(x.latency_ms)]))
          : `<div class="hint">Tool execution is not instrumented.</div>`}`;
    } catch (e) {
      dlg.querySelector("#td-body").innerHTML = errorBox(e);
      dlg.querySelector("header p").textContent = "";
    }
  }

  /* ── Users + prompts ─────────────────────────────────────────── */

  async function loadUsers() {
    const el = q("an-users");
    el.innerHTML = loadingBox("Loading user usage");
    try {
      const users = await api.obsUsers(filters.range, 100);
      const rows = users.map((u) => [
        `<button class="link-btn" data-user="${esc(u.username || "")}" data-uid="${esc(u.user_id || "")}">${esc(u.username || u.user_id || "—")}</button>`,
        fmtInt(u.request_count), fmtCompact(u.input_tokens), fmtCompact(u.output_tokens), fmtCompact(u.total_tokens),
        fmtMoney(u.total_cost), fmtMs(u.avg_latency_ms), esc(u.last_seen ? fmtTime(u.last_seen) : "—"),
      ]);
      el.innerHTML = card("Per-user usage", table(["User", "Requests", "In", "Out", "Tokens", "Cost", "Avg latency", "Last seen"], rows, { empty: "No per-user usage in this window" }),
        { subtitle: "Click a user to filter the whole page by them" });
      el.querySelectorAll("[data-user]").forEach((b) => b.addEventListener("click", () => {
        promptUser = b.dataset.user; promptSearch = "";
        filters.user_id = b.dataset.uid || filters.user_id;
        writeFilters(); reload({ skipOptions: true });
      }));
    } catch (e) { el.innerHTML = card("Per-user usage", errorBox(e)); }
  }

  async function loadPrompts() {
    const el = q("an-prompts");
    el.innerHTML = `<div class="vstack">
      <div class="hstack">
        <fieldset class="group" style="flex:1;min-width:14rem"><input id="pm-q" placeholder="Search prompts…" aria-label="Search prompts" value="${esc(promptSearch)}"><button id="pm-go">${icon("search")}</button></fieldset>
        <span id="pm-chip">${promptUser ? `<span class="chip">user: ${esc(promptUser)}<button data-clear-user aria-label="Clear user filter">${icon("x", 11)}</button></span>` : ""}</span>
      </div>
      <div id="pm-results"></div>
    </div>`;
    const results = el.querySelector("#pm-results");
    const run = async () => {
      results.innerHTML = loadingBox("Searching prompts");
      try {
        const prompts = await api.obsPrompts({ range: filters.range, user: promptUser, q: promptSearch, limit: 50 });
        results.innerHTML = table(
          ["Prompt", "User", "Model", "Tokens", "Cost", "Trace", "Time"],
          prompts.map((p) => [
            `<span class="truncate" title="${esc(p.prompt)}" style="max-width:40ch;display:inline-block">${esc((p.prompt || "").slice(0, 200))}</span>`,
            esc(p.username || "—"), esc(p.model || "—"), fmtCompact(p.total_tokens), fmtMoney(p.estimated_cost),
            `<button class="link-btn" data-trace="${esc(p.trace_id)}">${esc(traceShort(p.trace_id))}</button>`,
            esc(p.created_at ? fmtDateTime(p.created_at) : "—"),
          ]),
          { empty: "No prompts matched" });
        results.querySelectorAll("[data-trace]").forEach((b) => b.addEventListener("click", () => openTrace(b.dataset.trace)));
      } catch (e) { results.innerHTML = errorBox(e); }
    };
    el.querySelector("#pm-go").addEventListener("click", run);
    el.querySelector("#pm-q").addEventListener("input", (e) => {
      clearTimeout(promptDebounce);
      promptDebounce = setTimeout(() => { promptSearch = e.target.value.trim(); run(); }, 300);
    });
    el.querySelector("#pm-chip").addEventListener("click", (e) => {
      if (!e.target.closest("[data-clear-user]")) return;
      promptUser = ""; filters.user_id = ""; writeFilters(); reload({ skipOptions: true });
    });
    run();
  }

  /* ── Orchestration ───────────────────────────────────────────── */

  function reload({ skipOptions = false } = {}) {
    drawFilters(lastOptions);
    if (!skipOptions) loadFilterOptions();
    loadKpis();
    loadSeries();
    loadBreakdowns();
    loadGuardrails();
    loadFailures();
    loadTraces(true);
    loadUsers();
    loadPrompts();
  }

  async function full() { await Promise.allSettled([loadKpis(), loadSeries(), loadBreakdowns(), loadGuardrails(), loadFailures(), loadTraces(true), loadUsers()]); loadPrompts(); }

  ctx.root.addEventListener("click", (e) => {
    if (e.target.closest("#an-refresh")) reload();
  });
  ctx.root.querySelector("#an-auto").addEventListener("change", (e) => { auto = e.target.checked; });

  // Initial paint; loadFilterOptions redraws the bar once options are known.
  drawFilters();
  loadHealth();
  loadFilterOptions();
  full();

  const stop = createPoller(async () => {
    if (!auto) return;
    loadHealth();
    if (document.activeElement?.id === "pm-q" || document.activeElement?.id === "f-user") return;
    await Promise.allSettled([loadKpis(), loadSeries(), loadBreakdowns(), loadGuardrails(), loadFailures(), loadTraces(true), loadUsers()]);
  }, 5000, { immediate: false });

  return () => stop();
}
