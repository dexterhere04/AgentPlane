// Overview — §11 setup guidance + §7 operational metrics + stream health.
// Auto-refreshes: metrics every 5s, ClickHouse health every 8s, routing
// analytics every 15s.

import * as api from "../api.js";
import { createPoller } from "../poll.js";
import { getAdminToken, setup } from "../store.js";
import { esc, statusBadge, table, errorBox, loadingBox, badge, card, sectionHead, icon, statTile, statusVariant } from "../ui.js";
import { barList } from "../charts.js";
import { deriveRequests, isOpen } from "../stream.js";
import { fmtInt, fmtCompact, fmtMs, fmtRate, relTime } from "../format.js";

export const meta = {
  id: "overview",
  label: "Overview",
  eyebrow: "Get started & system status",
  group: "Run",
  icon: "gauge",
  description: "Setup progress, gateway health and operational counters at a glance.",
};

const SERVICES = [
  { id: "playground", label: "Playground", desc: "Send a test prompt through the gateway", icon: "play" },
  { id: "pipeline", label: "Pipeline", desc: "Watch every stage of a request live", icon: "route" },
  { id: "routing", label: "Routing", desc: "Providers, rules and failover", icon: "globe" },
  { id: "guardrails", label: "Guardrails", desc: "Test PII, secret and injection checks", icon: "shield" },
  { id: "events", label: "Event Log", desc: "Raw SSE events for every request", icon: "list" },
  { id: "usage", label: "Usage", desc: "Identity and spend attributed to keys", icon: "activity" },
  { id: "analytics", label: "Analytics", desc: "Traces, cost and models from ClickHouse", icon: "chart" },
  { id: "keys", label: "Keys & Secrets", desc: "Provider secrets and user API keys", icon: "key" },
  { id: "access", label: "Access Control", desc: "IAM policies, roles and groups", icon: "users" },
];

const TELEMETRY = [
  { key: "guardrail_passed_total", label: "Passed", color: "var(--success)" },
  { key: "guardrail_blocks_total", label: "Blocked", color: "var(--danger)" },
  { key: "guardrail_redactions_total", label: "Redacted", color: "#8b5cf6" },
  { key: "guardrail_warns_total", label: "Warned", color: "var(--warning)" },
  { key: "guardrail_errors_total", label: "Errors", color: "var(--danger)" },
];

function steps(ctx) {
  const hasToken = !!getAdminToken();
  const hasSecret = setup.get("secret");
  const hasKey = setup.get("provisioned");
  const hasTraffic = ctx.stream.list.some((e) => e.stage === "request_received");
  return [
    { key: "token", label: "Connect with the admin token", done: hasToken, go: "settings", cta: "Set token" },
    { key: "secret", label: "Store an upstream provider secret", done: hasSecret, go: "keys", cta: "Add secret" },
    { key: "key", label: "Provision a user & API key", done: hasKey, go: "keys", cta: "Provision" },
    { key: "traffic", label: "Send a request through the gateway", done: hasTraffic, go: "playground", cta: "Open playground" },
  ];
}

export function render(ctx) {
  const s = steps(ctx);
  const done = s.filter((x) => x.done).length;

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow,
    title: "Overview",
    description: meta.description,
    actions: `<button class="outline" id="ov-refresh">${icon("refresh")} Refresh</button>`,
  }) + `
    ${!getAdminToken() ? `<div role="alert" style="margin-bottom:var(--space-6)">
      <div><strong>No admin token yet.</strong>
      <div>Add the gateway admin token to unlock provisioning, secrets, routing and analytics.
      <button class="ghost small" id="ov-open-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div>
    </div>` : ""}

    <section class="ov-health" id="ov-health"></section>
    <div class="grid-kpi" id="ov-kpi" style="margin-top:var(--space-6)"></div>

    <div class="split" style="margin-top:var(--space-6)">
      <section id="ov-setup"></section>
      <section id="ov-stream"></section>
    </div>

    <section id="ov-main" class="split split-even" style="margin-top:var(--space-6)"></section>
    <section id="ov-services" style="margin-top:var(--space-6)"></section>
    <section id="ov-metrics" style="margin-top:var(--space-6)"></section>
    <section id="ov-routing" style="margin-top:var(--space-6)"></section>
    <section id="ov-upstreams" style="margin-top:var(--space-6)"></section>
  `;

  const q = (id) => ctx.root.querySelector("#" + id);
  const healthEl = q("ov-health");
  const kpi = q("ov-kpi");
  const setupEl = q("ov-setup");
  const streamEl = q("ov-stream");
  const mainEl = q("ov-main");
  const servicesEl = q("ov-services");
  const metricsEl = q("ov-metrics");
  const routingEl = q("ov-routing");
  const upstreamsEl = q("ov-upstreams");

  let providers = [];
  let keys = [];
  let metrics = {};
  let chState = null; // null = scanning
  let traffic = [];
  let failover = null;
  let errSummary = [];

  /* ── Session-derived numbers ─────────────────────────────────── */

  function session() {
    const reqs = deriveRequests(ctx.stream.list);
    const ended = reqs.filter((r) => r.endedAt);
    const failed = reqs.filter((r) => r.blocked || r.status === "error");
    const inflight = reqs.filter(isOpen).length;
    const events = ctx.stream.list.length;
    const tokens = reqs.reduce((n, r) => n + (r.usage?.total_tokens || 0), 0);
    const avg = ended.length ? ended.reduce((n, r) => n + (r.endedAt - r.startedAt), 0) / ended.length : null;
    const completed = Math.max(ended.length - failed.length, 0);
    const successRate = ended.length ? ((ended.length - failed.length) / ended.length) * 100 : null;
    return { reqs, ended, failed, inflight, events, tokens, avg, completed, successRate };
  }

  /* ── Health banner ───────────────────────────────────────────── */

  function drawHealth() {
    const { successRate, inflight, completed, failed } = session();
    const systems = [
      { label: "Gateway", desc: "SSE event stream", ok: ctx.stream.connected, live: "Live", down: "Offline", icon: "bolt" },
      { label: "Provider", desc: "upstreams registered", ok: providers.length ? true : null, live: `${providers.length} up`, down: "none", icon: "server" },
      { label: "Secret store", desc: "provider credentials", ok: setup.get("secret") ? true : null, live: "configured", down: "not set", icon: "lock" },
      { label: "ClickHouse", desc: "traces & analytics", ok: chState === null ? null : chState !== "unavailable", live: "connected", down: "unavailable", icon: "chart" },
    ];
    const degraded = !ctx.stream.connected || systems.some((x) => x.ok === false);
    const scanning = systems.some((x) => x.ok === null);
    const label = degraded ? "Degraded" : scanning ? "Scanning…" : "Operational";
    const tone = degraded ? "bad" : scanning ? "scan" : "good";

    const stats = [
      { lab: "Success rate", val: successRate == null ? "—" : `${successRate.toFixed(1)}%`, tone: successRate == null ? "" : successRate >= 99.5 ? "good" : successRate >= 90 ? "warn" : "bad" },
      { lab: "In flight", val: fmtInt(inflight), tone: inflight ? "warn" : "" },
      { lab: "Blocked", val: fmtInt(failed.length), tone: failed.length ? "bad" : "" },
      { lab: "Completed", val: fmtInt(completed), tone: "good" },
    ];

    healthEl.innerHTML = `
      <div class="ov-h-main">
        <span class="ov-h-dot ${tone}"></span>
        <div><span class="ov-h-kicker">Operational health</span><span class="ov-h-title ${tone}">${esc(label)}</span></div>
      </div>
      <div class="ov-h-stats">
        ${stats.map((x) => `<div class="ov-h-stat"><span class="ov-h-lab">${esc(x.lab)}</span><span class="ov-h-val ${x.tone}">${esc(x.val)}</span></div>`).join("")}
      </div>
      <div class="ov-sys">
        ${systems.map((x) => `<div class="ov-sys-chip" title="${esc(x.desc)}">
          <span class="ov-sys-icon">${icon(x.icon, 13)}</span>
          <span class="ov-sys-dot ${x.ok ? "ok" : x.ok === null ? "scan" : "bad"}"></span>
          <span class="ov-sys-name">${esc(x.label)}</span>
          <span class="ov-sys-val ${x.ok ? "ok" : x.ok === null ? "scan" : "bad"}">${esc(x.ok === null ? "checking…" : x.ok ? x.live : x.down)}</span>
        </div>`).join("")}
      </div>`;
  }

  /* ── KPI tiles ───────────────────────────────────────────────── */

  function drawKpi() {
    const { reqs, successRate, inflight, failed, events, tokens, avg } = session();
    const connected = ctx.stream.connected;
    const ev = metrics.guardrail_evaluations_total;
    const tiles = [
      { label: "Requests", value: fmtInt(reqs.length), icon: "bolt", tone: "" },
      { label: "Success rate", value: successRate == null ? "—" : `${successRate.toFixed(1)}%`, icon: "check", tone: successRate == null ? "" : successRate >= 99.5 ? "good" : successRate >= 90 ? "warn" : "bad" },
      { label: "In flight", value: fmtInt(inflight), icon: "layers", tone: inflight ? "warn" : "" },
      { label: "Blocked", value: fmtInt(failed.length), icon: "shield", tone: failed.length ? "bad" : "" },
      { label: "Events", value: fmtInt(events), icon: "list", tone: "" },
      { label: "Tokens", value: tokens ? fmtCompact(tokens) : "—", icon: "message", tone: "" },
      { label: "Avg latency", value: avg == null ? "—" : fmtMs(avg), icon: "activity", tone: "" },
      { label: "Stream", value: connected ? "Live" : "Offline", icon: "gauge", tone: connected ? "good" : "bad" },
    ];
    kpi.innerHTML = tiles.map((t) => statTile(t)).join("");
    // keep guardrail evals visible in the telemetry card
    void ev;
  }

  function drawSetup() {
    const pct = Math.round((done / s.length) * 100);
    const items = s.map((x) => `<li style="display:flex;align-items:center;gap:var(--space-3);padding:var(--space-2) 0">
        <span class="status-dot ${x.done ? "on" : "off"}"></span>
        <span style="flex:1" class="${x.done ? "" : "text-light"}">${esc(x.label)}</span>
        ${x.done ? badge("Done", "success") : `<button class="outline small" data-go="${x.go}">${esc(x.cta)}</button>`}
      </li>`).join("");
    setupEl.innerHTML = card(
      "Getting started",
      `<div class="progress-track" style="margin-bottom:var(--space-4)"><div class="progress-fill" style="width:${pct}%"></div></div>
       <div class="text-light" style="font-size:var(--text-7);margin-bottom:var(--space-2)">${done} of ${s.length} steps complete</div>
       <ul class="unstyled">${items}</ul>`,
      { subtitle: "Four steps to a working gateway" }
    );
  }

  function drawStream() {
    const evts = ctx.stream.list.slice(-8).reverse();
    const rows = evts.map((e) => [
      `<span class="mono-id">${esc(new Date(e.timestamp || Date.now()).toLocaleTimeString(undefined, { hour12: false }))}</span>`,
      `<span class="mono-id">${esc(e.stage)}</span>`,
      statusBadge(e.status),
    ]);
    streamEl.innerHTML = card(
      "Recent activity",
      table(["Time", "Stage", "Status"], rows, { empty: ctx.stream.connected ? "Waiting for traffic…" : "Stream offline" }),
      { subtitle: "Last 8 events", actions: badge(ctx.stream.connected ? "Live" : "Offline", ctx.stream.connected ? "success" : "secondary", { dot: true }) }
    );
  }

  /* ── Guardrail telemetry + session summary ───────────────────── */

  function drawMain() {
    const { ended, events, inflight, failed, completed } = session();
    const max = Math.max(1, ...TELEMETRY.map((t) => Number(metrics[t.key]) || 0));
    const tele = TELEMETRY.map((t) => {
      const v = Number(metrics[t.key]) || 0;
      return `<div class="tele-row">
        <span class="t-label"><i class="t-dot" style="background:${t.color}"></i>${esc(t.label)}</span>
        <span class="tele-track"><span class="tele-fill" style="width:${(v / max) * 100}%;background:${t.color}"></span></span>
        <span class="t-value">${fmtInt(v)}</span>
      </div>`;
    }).join("");
    const teleCard = card("Guardrail telemetry",
      `<div class="telemetry">${tele}</div>
       <div class="hint" style="margin-top:var(--space-2)">avg evaluation <b>${esc(fmtMs(metrics.guardrail_evaluation_duration_avg_ms ?? 0))}</b> · <b>${fmtInt(metrics.guardrail_evaluations_total ?? 0)}</b> evaluations</div>`,
      { subtitle: "Cumulative counters from /metrics", actions: badge("/metrics", "secondary") });

    const rows = [
      { label: "Completed", value: completed, pct: ended.length ? (completed / ended.length) * 100 : 0, tone: "good", icon: "check" },
      { label: "Blocked / failed", value: failed.length, pct: ended.length ? (failed.length / ended.length) * 100 : 0, tone: "bad", icon: "shield" },
      { label: "In flight", value: inflight, pct: session().reqs.length ? (inflight / session().reqs.length) * 100 : 0, tone: "warn", icon: "layers" },
    ];
    const sess = rows.map((r) => `<div class="sess-row">
      <div class="sess-head"><span class="sess-icon">${icon(r.icon, 14)}</span><span class="sess-label">${esc(r.label)}</span><span class="sess-val">${fmtInt(r.value)}</span></div>
      <div class="progress-track sm"><div class="progress-fill is-${r.tone}" style="width:${r.pct}%"></div></div>
    </div>`).join("");
    const sessCard = card("Session summary",
      `<div class="sess">${sess}<div class="sess-note">${fmtInt(events)} events emitted across all streamed requests</div></div>`,
      { subtitle: "Live breakdown of streamed requests" });

    mainEl.innerHTML = teleCard + sessCard;
  }

  function drawServices() {
    servicesEl.innerHTML = card("Quick access", `<div class="svc-grid">${SERVICES.map((sv) => `
      <button class="svc-card" data-go="${sv.id}">
        <span class="svc-icon">${icon(sv.icon, 16)}</span>
        <span class="svc-body"><span class="svc-title">${esc(sv.label)}${icon("chevronRight", 13)}</span><span class="svc-desc">${esc(sv.desc)}</span></span>
      </button>`).join("")}</div>`,
      { subtitle: "Jump straight to any part of the gateway" });
  }

  /* ── Operational metrics + upstreams ─────────────────────────── */

  function drawMetrics() {
    const tiles = [
      ["Routing decisions", metrics.routing_decisions_total],
      ["Failovers", metrics.routing_failovers_total],
      ["Retries", metrics.routing_retries_total],
      ["Breaker trips", metrics.routing_breaker_trips_total],
      ["Breaker recoveries", metrics.routing_breaker_recoveries_total],
      ["No provider", metrics.routing_no_provider_total],
      ["Denied", metrics.routing_denied_total],
    ];
    metricsEl.innerHTML = card(
      "Operational metrics",
      `<div class="grid-kpi">${tiles.map(([l, v]) =>
        `<article class="card stat"><span class="stat-label">${esc(l)}</span><span class="stat-value">${esc(fmtInt(v ?? 0))}</span></article>`
      ).join("")}</div>`,
      { subtitle: "In-process counters (available without ClickHouse)" }
    );

    const ups = metrics.routing_upstreams || [];
    upstreamsEl.innerHTML = card(
      "Upstream status",
      table(
        ["Provider", "Decisions", "Failovers", "Retries", "Breaker trips", "Recoveries"],
        ups.map((u) => [
          esc(u.name || "—"),
          fmtInt(u.decisions ?? 0),
          fmtInt(u.failovers ?? 0),
          fmtInt(u.retries ?? 0),
          fmtInt(u.breaker_trips ?? 0),
          fmtInt(u.breaker_recoveries ?? 0),
        ]),
        { empty: "No upstream activity recorded" }
      ),
      { subtitle: "Per-provider routing counters" }
    );
  }

  function drawRouting() {
    const total = traffic.reduce((n, p) => n + (Number(p.request_count) || 0), 0);
    const errs = errSummary.reduce((a, r) => ({ e: a.e + (Number(r.error_count) || 0), t: a.t + (Number(r.total_requests) || 0) }), { e: 0, t: 0 });
    const errRate = errs.t ? (errs.e / errs.t) * 100 : null;

    if (total === 0 && !failover && errs.t === 0) {
      routingEl.innerHTML = card("Routing telemetry",
        `<div class="empty"><strong>No routing analytics for the last 24h</strong><span>Send traffic through the gateway, then refresh.</span></div>`,
        { subtitle: "Provider selection, failover and traffic from ClickHouse" });
      return;
    }

    const trafficBars = barList(traffic.map((p) => ({ label: p.provider || "unknown", value: Number(p.traffic_percentage) || 0, text: `${(Number(p.traffic_percentage) || 0).toFixed(1)}%` })), { empty: "No provider traffic recorded yet" });
    const stats = `<div class="grid-kpi">
      ${statTile({ label: "Failover rate", value: failover ? `${(Number(failover.failover_percentage) || 0).toFixed(1)}%` : "—", sub: failover ? `${fmtInt(failover.failover_count)} of ${fmtInt(failover.total_requests)}` : "No data", tone: Number(failover?.failover_count) ? "warn" : "good" })}
      ${statTile({ label: "Error rate", value: errRate == null ? "—" : `${errRate.toFixed(1)}%`, sub: errs.t ? `${fmtInt(errs.e)} errors` : "No data", tone: errs.e ? "bad" : "good" })}
      ${statTile({ label: "Traffic (24h)", value: total ? fmtCompact(total) : "—", sub: "requests" })}
    </div>`;

    routingEl.innerHTML = `<div class="split split-even">
      ${card("Provider traffic · 24h", trafficBars)}
      ${card("Failover & errors · 24h", stats)}
    </div>`;
  }

  /* ── Loaders ─────────────────────────────────────────────────── */

  async function loadMetrics() {
    if (!getAdminToken()) return;
    try {
      const res = await api.getMetrics();
      metrics = res?.metrics || res || {};
      drawKpi(); drawMain(); drawMetrics();
    } catch (e) {
      if (!providers.length) metricsEl.innerHTML = errorBox(e);
    }
  }

  async function loadProviders() {
    if (!getAdminToken()) { drawHealth(); return; }
    const [p, k] = await Promise.allSettled([api.listProviders(), api.listApiKeys()]);
    if (p.status === "fulfilled") providers = p.value || [];
    if (k.status === "fulfilled") keys = k.value || [];
    drawHealth();
  }

  async function loadHealth() {
    if (!getAdminToken()) { chState = "unavailable"; drawHealth(); return; }
    try {
      const h = await api.obsHealth();
      chState = h?.data_state || (h?.reachable === false ? "unavailable" : "unknown");
    } catch { chState = "unavailable"; }
    drawHealth();
  }

  async function loadRouting() {
    if (!getAdminToken()) return;
    const [t, f, e] = await Promise.allSettled([
      api.analytics("provider_traffic", 24),
      api.analytics("failover_count", 24),
      api.analytics("error_rate_summary", 24),
    ]);
    if (t.status === "fulfilled") traffic = t.value || [];
    if (f.status === "fulfilled") failover = (f.value || [])[0] || null;
    if (e.status === "fulfilled") errSummary = e.value || [];
    drawRouting();
  }

  async function loadAll() {
    await Promise.allSettled([loadProviders(), loadMetrics(), loadRouting()]);
  }

  ctx.root.querySelector("#ov-refresh").addEventListener("click", () => { loadProviders(); loadMetrics(); loadRouting(); drawHealth(); });
  ctx.root.querySelector("#ov-open-settings")?.addEventListener("click", ctx.openSettings);
  ctx.root.addEventListener("click", (e) => {
    const go = e.target.closest("[data-go]");
    if (!go) return;
    if (go.dataset.go === "settings") ctx.openSettings();
    else ctx.navigate(go.dataset.go);
  });

  const unsub = ctx.stream.on(() => { drawKpi(); drawStream(); drawHealth(); drawMain(); });
  const stopMetrics = createPoller(loadMetrics, 5000);
  const stopHealth = createPoller(loadHealth, 8000);
  const stopRouting = createPoller(loadRouting, 15000);

  drawKpi(); drawSetup(); drawStream(); drawMain(); drawServices(); drawHealth();
  loadProviders().then(() => loadHealth());
  loadMetrics();
  loadRouting();

  return () => { unsub(); stopMetrics(); stopHealth(); stopRouting(); };
}
