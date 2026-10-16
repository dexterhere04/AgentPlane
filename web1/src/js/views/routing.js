// Routing — §9 providers & routing rules, §7 live upstream / breaker status,
// plus routing analytics (§10.2), a live routing feed, an admin-token validator
// and per-card refresh.

import * as api from "../api.js";
import { createPoller } from "../poll.js";
import { getAdminToken, setAdminToken } from "../store.js";
import { esc, sectionHead, card, table, badge, statusBadge, formDialog, confirmDialog, toastOk, toastError, errorBox, loadingBox, icon, statTile, emptyState, alertBox } from "../ui.js";
import { barList, donut, latencyBars } from "../charts.js";
import { fmtInt, fmtCompact, fmtMoney, fmtMs, fmtPct, fmtTime } from "../format.js";

export const meta = {
  id: "routing",
  label: "Routing",
  eyebrow: "Providers & rules",
  group: "Observe",
  icon: "route",
  description: "Register upstreams, control availability and weights, and define model/role/user routing rules.",
};

const NUM = (v) => (v === "" || v === undefined || v === null ? undefined : Number(v));
const HOURS = [
  { value: 1, label: "Last hour" },
  { value: 6, label: "Last 6 hours" },
  { value: 24, label: "Last 24 hours" },
  { value: 168, label: "Last 7 days" },
  { value: 720, label: "Last 30 days" },
];
const BREAKER_PRESETS = [
  { value: 3, label: "Strict (3 failures)" },
  { value: 5, label: "Default (5 failures)" },
  { value: 10, label: "Lenient (10 failures)" },
];
const ANALYTICS_TYPES = [
  ["provider_traffic", "Provider traffic share"],
  ["latency_percentiles", "Latency percentiles"],
  ["error_rate_summary", "Error rate summary"],
  ["failover_count", "Failover count"],
  ["failover_transitions", "Failover transitions"],
  ["tokens_by_route", "Tokens by route"],
  ["tokens_by_provider", "Tokens by provider"],
  ["cost_by_route", "Cost by route"],
  ["cost_by_provider", "Cost by provider"],
];

function matcherSummary(value) {
  if (value == null || value === "" || (typeof value === "object" && !Object.keys(value).length)) return badge("any", "secondary");
  let m = value;
  if (typeof value === "string") { try { m = JSON.parse(value); } catch { return badge("invalid", "danger"); } }
  if (!m || typeof m !== "object" || !Object.keys(m).length) return badge("any", "secondary");
  const parts = [];
  const flat = (v) => (Array.isArray(v) ? v.join(", ") : String(v));
  if (typeof m.equals === "string") parts.push(m.equals);
  if (typeof m.prefix === "string") parts.push(`${m.prefix}*`);
  if (Array.isArray(m.in)) parts.push(m.in.join(", "));
  if (Array.isArray(m.usernames)) parts.push(m.usernames.join(", "));
  if (Array.isArray(m.ids)) parts.push(m.ids.join(", "));
  if (!parts.length) for (const [k, v] of Object.entries(m)) parts.push(`${k}:${flat(v)}`);
  return `<span class="hstack" style="gap:var(--space-1);flex-wrap:wrap">${(parts.length ? parts : ["any"]).slice(0, 3).map((t) => badge(t, "secondary")).join("")}</span>`;
}

export function render(ctx) {
  if (!ctx.hasAdminToken()) {
    ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Routing", description: meta.description }) +
      `<div role="alert"><div><strong>Admin token required.</strong>
        <div>Routing administration needs the gateway admin token.
        <button class="ghost small" id="rt-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`;
    ctx.root.querySelector("#rt-settings").addEventListener("click", ctx.openSettings);
    return;
  }

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow, title: "Routing", description: meta.description,
    actions: `<button class="outline" id="rt-refresh">${icon("refresh")} Refresh</button>`,
  }) + `
    <section id="rt-validator" style="margin-bottom:var(--space-6)"></section>
    ${card("", `<ot-tabs id="rt-tabs">
      <div role="tablist">
        <button role="tab">Providers</button>
        <button role="tab">Routing rules</button>
        <button role="tab">Upstreams</button>
        <button role="tab">Routing analytics</button>
      </div>
      <div role="tabpanel" id="rt-providers"></div>
      <div role="tabpanel" id="rt-rules"></div>
      <div role="tabpanel" id="rt-upstreams"></div>
      <div role="tabpanel" id="rt-analytics"></div>
    </ot-tabs>`)}`;

  const P = (id) => ctx.root.querySelector("#" + id);
  const providerEl = P("rt-providers");
  const rulesEl = P("rt-rules");
  const upstreamsEl = P("rt-upstreams");
  const analyticsEl = P("rt-analytics");
  const validatorEl = P("rt-validator");

  let providers = [];
  let rules = [];
  let upstreams = [];
  let tokenStatus = "unvalidated";

  /* ── Admin token validator + gate ────────────────────────────── */

  function drawValidator() {
    const tone = tokenStatus === "valid" ? "success" : tokenStatus === "invalid" ? "danger" : tokenStatus === "checking" ? "warning" : "secondary";
    const label = tokenStatus === "valid" ? "VALIDATED" : tokenStatus === "checking" ? "CHECKING" : tokenStatus === "invalid" ? "INVALID" : "NOT VALIDATED";
    const live = tokenStatus === "valid";
    validatorEl.innerHTML = card("Admin token validator", `
      <div class="row-form">
        <label data-field style="flex:1;margin:0">
          <input id="rt-token" class="mono" type="password" autocomplete="off" placeholder="Enter admin token" value="${esc(getAdminToken())}">
        </label>
        <button class="outline" id="rt-validate" ${tokenStatus === "checking" ? "disabled" : ""}>Validate token</button>
        <span class="badge" data-variant="${tone}"${live ? ` data-dot` : ""}>${live ? '<span class="status-dot on"></span>' : ""}${label}</span>
      </div>`,
      { subtitle: "Validate your admin token before managing providers or routing rules",
        actions: badge("write-gated", live ? "success" : "secondary") });

    validatorEl.querySelector("#rt-validate").addEventListener("click", async () => {
      const token = validatorEl.querySelector("#rt-token").value.trim();
      if (!token) { tokenStatus = "invalid"; drawValidator(); return; }
      tokenStatus = "checking"; drawValidator();
      setAdminToken(token);
      const res = await api.validateAdminToken();
      tokenStatus = res.ok ? "valid" : "invalid";
      drawValidator();
      if (res.ok) { load(); toastOk("Admin token validated"); }
      else toastError({ message: res.reason }, "Token rejected");
    });
  }

  function requireValidatedAdmin() {
    if (tokenStatus === "valid") return true;
    toast("Validate your admin token first — mutations are gated.", { variant: "warning", title: "Not validated" });
    return false;
  }

  /* ── Providers ───────────────────────────────────────────────── */

  function providerRow(p) {
    const id = p.id ?? p.name;
    const enabled = p.enabled !== false;
    const up = upstreams.find((u) => u.name === p.name);
    return [
      `<strong>${esc(p.name || "—")}</strong><div class="mono-id">${esc(p.base_url || p.id || "")}</div>`,
      esc(p.provider_group || "—"),
      `<span class="hstack" style="gap:var(--space-1);flex-wrap:wrap">${(p.supported_models || []).slice(0, 3).map((m) => badge(m, "secondary")).join("")}${(p.supported_models || []).length > 3 ? badge(`+${p.supported_models.length - 3}`, "secondary") : ""}</span>`,
      esc(fmtInt(p.weight ?? 1)),
      `<span class="mono-id">${esc(p.timeout_ms ?? "—")}ms · ${esc(p.max_retries ?? "—")}r</span>`,
      `<span class="hstack" style="gap:var(--space-1)">${breakerBadge(up?.breaker)}<span class="text-light" style="font-size:var(--text-8)">thr ${esc(up?.breaker_failure_threshold ?? p.breaker_failure_threshold ?? "—")}</span></span>`,
      statusBadge(enabled ? "enabled" : "disabled"),
      `<span class="hstack" style="justify-content:flex-end">
        <button class="outline small" data-toggle-provider="${esc(id)}" data-enable="${enabled ? "0" : "1"}">${enabled ? "Disable" : "Enable"}</button>
        <button class="outline small" data-edit-provider="${esc(id)}">Edit</button>
        <button class="outline small" data-variant="danger" data-del-provider="${esc(id)}">Delete</button>
      </span>`,
    ];
  }

  function breakerBadge(state) {
    switch (String(state || "").toLowerCase()) {
      case "open": return badge("OPEN", "danger");
      case "half_open":
      case "half-open": return badge("HALF-OPEN", "warning");
      case "closed": return badge("CLOSED", "success");
      default: return badge("—", "secondary");
    }
  }

  function providerFields(p = {}) {
    return [
      { name: "name", label: "Name", value: p.name, required: true, autocomplete: false },
      { name: "base_url", label: "Base URL", value: p.base_url, placeholder: "https://api.openai.com/v1", required: true, autocomplete: false },
      { name: "secret_key", label: "Secret reference", value: p.secret_ref ?? p.secret_key ?? "", hint: "Name of a secret already in the store (e.g. OPENAI_API_KEY) — not the raw key value.", autocomplete: false, mono: true },
      { name: "provider_group", label: "Provider group", value: p.provider_group, placeholder: "primary" },
      { name: "weight", label: "Weight", type: "number", value: p.weight ?? 1, min: 0, step: 1 },
      { name: "supported_models", label: "Supported models", type: "tags", value: (p.supported_models || []).join(","), placeholder: "gpt-4o, add a model…" },
      { name: "timeout_ms", label: "Timeout (ms)", type: "number", value: p.timeout_ms ?? 60000, min: 1000, step: 1000 },
      { name: "max_retries", label: "Max retries", type: "number", value: p.max_retries ?? 1, min: 0, step: 1 },
      { name: "breaker_failure_threshold", label: "Circuit breaker", type: "select", options: BREAKER_PRESETS, value: p.breaker_failure_threshold ?? 5 },
      { name: "enabled", label: "Enabled", type: "checkbox", value: p.enabled !== false },
    ];
  }

  function providerPayload(v) {
    return {
      name: v.name,
      base_url: v.base_url,
      secret_key: v.secret_key || "",
      provider_group: v.provider_group || "default",
      weight: NUM(v.weight) ?? 1,
      supported_models: v.supported_models || [],
      timeout_ms: NUM(v.timeout_ms) ?? 60000,
      max_retries: NUM(v.max_retries) ?? 1,
      breaker_failure_threshold: NUM(v.breaker_failure_threshold) ?? 5,
      enabled: v.enabled !== false,
    };
  }

  async function addProvider() {
    if (!requireValidatedAdmin()) return;
    const v = await formDialog({
      title: "Add provider", medium: true,
      description: "Register an upstream the gateway can route to.",
      fields: providerFields(),
      submitLabel: "Create provider",
      validate: (x) => (!x.name || !x.base_url ? "Name and base URL are required." : null),
    });
    if (!v) return;
    try { await api.createProvider(providerPayload(v)); toastOk("Provider created"); }
    catch (e) { toastError(e, "Create provider"); }
    finally { loadProviders(); }
  }

  async function editProvider(id) {
    if (!requireValidatedAdmin()) return;
    const p = providers.find((x) => String(x.id ?? x.name) === String(id));
    if (!p) return;
    const v = await formDialog({ title: `Edit ${p.name}`, medium: true, fields: providerFields(p), submitLabel: "Save changes" });
    if (!v) return;
    try { await api.updateProvider(p.id ?? p.name, providerPayload(v)); toastOk("Provider updated"); }
    catch (e) { toastError(e, "Update provider"); }
    finally { loadProviders(); }
  }

  /* ── Routing rules ───────────────────────────────────────────── */

  function ruleRow(r) {
    const id = r.id;
    const enabled = r.enabled !== false;
    return [
      `<strong>${esc(r.name || "—")}</strong><div class="mono-id">${esc(id || "")}</div>`,
      esc(fmtInt(r.priority ?? 0)),
      esc(r.action || "—"),
      esc(r.provider_group || "—"),
      `<div class="vstack" style="gap:2px;font-size:var(--text-8)"><div><span class="text-light">model</span> ${matcherSummary(r.model_matcher)}</div><div><span class="text-light">role</span> ${matcherSummary(r.role_matcher)}</div></div>`,
      statusBadge(enabled ? "enabled" : "disabled"),
      `<span class="hstack" style="justify-content:flex-end">
        <button class="outline small" data-toggle-rule="${esc(id)}" data-enable="${enabled ? "0" : "1"}">${enabled ? "Disable" : "Enable"}</button>
        <button class="outline small" data-edit-rule="${esc(id)}">Edit</button>
        <button class="outline small" data-variant="danger" data-del-rule="${esc(id)}">Delete</button>
      </span>`,
    ];
  }

  function ruleFields(r = {}) {
    return [
      { name: "name", label: "Name", value: r.name, required: true, autocomplete: false },
      { name: "priority", label: "Priority", type: "number", value: r.priority ?? 10, step: 1, hint: "Lower runs first" },
      { name: "action", label: "Action", value: r.action ?? "route", placeholder: "route", required: true },
      { name: "provider_group", label: "Target provider group", value: r.provider_group, placeholder: "primary", required: true },
      { name: "model_matcher", label: "Model matcher (JSON)", type: "json", value: r.model_matcher ?? {} },
      { name: "role_matcher", label: "Role matcher (JSON)", type: "json", value: r.role_matcher ?? {} },
      { name: "user_matcher", label: "User matcher (JSON)", type: "json", value: r.user_matcher ?? {} },
      { name: "timeout_retry_overrides", label: "Timeout / retry overrides (JSON)", type: "json", value: r.timeout_retry_overrides ?? {} },
      { name: "enabled", label: "Enabled", type: "checkbox", value: r.enabled !== false },
    ];
  }

  function rulePayload(v) {
    return {
      name: v.name,
      priority: NUM(v.priority) ?? 10,
      model_matcher: v.model_matcher || {},
      role_matcher: v.role_matcher || {},
      user_matcher: v.user_matcher || {},
      action: v.action,
      provider_group: v.provider_group,
      timeout_retry_overrides: v.timeout_retry_overrides || {},
      enabled: v.enabled !== false,
    };
  }

  async function addRule() {
    if (!requireValidatedAdmin()) return;
    const v = await formDialog({
      title: "Add routing rule", wide: true,
      description: "Rules select a provider group by model, role and user matchers.",
      fields: ruleFields(),
      submitLabel: "Create rule",
      validate: (x) => (!x.name || !x.provider_group ? "Name and target group are required." : null),
    });
    if (!v) return;
    try { await api.createRoutingRule(rulePayload(v)); toastOk("Rule created"); }
    catch (e) { toastError(e, "Create rule"); }
    finally { loadRules(); }
  }

  async function editRule(id) {
    if (!requireValidatedAdmin()) return;
    const r = rules.find((x) => String(x.id) === String(id));
    if (!r) return;
    const v = await formDialog({ title: `Edit ${r.name}`, wide: true, fields: ruleFields(r), submitLabel: "Save changes" });
    if (!v) return;
    try { await api.updateRoutingRule(r.id, rulePayload(v)); toastOk("Rule updated"); }
    catch (e) { toastError(e, "Update rule"); }
    finally { loadRules(); }
  }

  /* ── Rendering ───────────────────────────────────────────────── */

  function drawProviders() {
    providerEl.innerHTML = card("", table(
      ["Provider", "Group", "Models", "Weight", "Timeout", "Breaker", "Status", ""],
      providers.map(providerRow),
      { empty: "No providers configured yet" }
    ), { actions: `<button id="rt-add-provider">${icon("plus")} Add provider</button>` });
  }

  function drawRules() {
    rulesEl.innerHTML = card("", table(
      ["Rule", "Priority", "Action", "Group", "Matchers", "Status", ""],
      rules.map(ruleRow),
      { empty: "No routing rules defined yet" }
    ), { actions: `<button id="rt-add-rule">${icon("plus")} Add rule</button>` });
  }

  function drawUpstreams() {
    const rows = upstreams.map((u) => [
      `<strong>${esc(u.name || "—")}</strong>`,
      statusBadge(u.enabled === false ? "disabled" : "enabled"),
      esc(fmtInt(u.weight ?? 1)),
      `<span class="hstack" style="gap:var(--space-1);flex-wrap:wrap">${(u.models || []).slice(0, 3).map((m) => badge(m, "secondary")).join("")}${(u.models || []).length > 3 ? badge(`+${u.models.length - 3}`, "secondary") : ""}</span>`,
      breakerBadge(u.breaker),
      esc(u.breaker_failure_threshold ?? "—"),
    ]);
    upstreamsEl.innerHTML = card("", table(["Upstream", "Enabled", "Weight", "Models", "Breaker", "Threshold"], rows, { empty: "No upstream status available" }), {
      subtitle: "Live circuit-breaker state — auto-refreshes every 5s",
      actions: badge(ctx.stream.connected ? "live" : "offline", ctx.stream.connected ? "success" : "secondary", { dot: true }),
    });
  }

  /* ── Routing analytics + live feed (§10.2) ───────────────────── */

  let hours = 24;
  let rangeReady = false;

  function drawAnalytics() {
    if (rangeReady) { loadAnalytics(); return; }
    rangeReady = true;
    analyticsEl.innerHTML = `
      <div class="vstack">
        <div class="row-between">
          <div class="hstack">
            <select id="rt-hours" aria-label="Time range" style="width:auto">${HOURS.map((h) => `<option value="${h.value}"${h.value === hours ? " selected" : ""}>${esc(h.label)}</option>`).join("")}</select>
            <button class="outline small" id="rt-an-refresh">${icon("refresh")} Refresh</button>
          </div>
          <span class="hstack">
            <span class="badge" data-variant="secondary">live feed</span>
            ${badge(ctx.stream.connected ? "LIVE" : "OFFLINE", ctx.stream.connected ? "success" : "secondary", { dot: true })}
          </span>
        </div>
        <div id="rt-summary"></div>
        <div id="rt-feed"></div>
        <div id="rt-charts"></div>
      </div>`;
    analyticsEl.querySelector("#rt-hours").addEventListener("change", (e) => { hours = Number(e.target.value); loadAnalytics(); });
    analyticsEl.querySelector("#rt-an-refresh").addEventListener("click", loadAnalytics);
    loadAnalytics();
  }

  async function loadAnalytics() {
    if (!ctx.hasAdminToken()) return;
    const summary = analyticsEl.querySelector("#rt-summary");
    const charts = analyticsEl.querySelector("#rt-charts");
    summary.innerHTML = loadingBox("Loading routing analytics");
    try {
      const types = ANALYTICS_TYPES.map(([t]) => t);
      const results = await Promise.all(types.map((t) => api.analytics(t, hours).catch(() => null)));
      const byType = Object.fromEntries(types.map((t, i) => [t, results[i] || []]));
      drawAnalyticsSummary(byType);
      drawAnalyticsCharts(byType);
    } catch (e) {
      summary.innerHTML = errorBox(e);
      charts.innerHTML = "";
    }
    drawFeed();
  }

  function drawAnalyticsSummary(byType) {
    const summary = analyticsEl.querySelector("#rt-summary");
    const traffic = byType.provider_traffic || [];
    const failover = (byType.failover_count || [])[0] || null;
    const errs = byType.error_rate_summary || [];
    const errTot = errs.reduce((a, r) => ({ e: a.e + (Number(r.error_count) || 0), t: a.t + (Number(r.total_requests) || 0) }), { e: 0, t: 0 });
    const total = traffic.reduce((n, p) => n + (Number(p.request_count) || 0), 0);
    if (!total && !failover && !errs.length) {
      summary.innerHTML = emptyState({ icon: "route", title: `No routing telemetry in the last ${hours === 1 ? "hour" : hours === 168 ? "7 days" : hours === 720 ? "30 days" : hours + " hours"}`, body: "Send traffic through the gateway, then refresh." });
      return;
    }
    summary.innerHTML = `<div class="grid-kpi">
      ${statTile({ label: "Traffic", value: total ? fmtCompact(total) : "—", sub: `${traffic.length} provider(s)`, icon: "globe" })}
      ${statTile({ label: "Failover rate", value: failover ? fmtPct(failover.failover_percentage) : "—", sub: failover ? `${fmtInt(failover.failover_count)} of ${fmtInt(failover.total_requests)}` : "No data", tone: Number(failover?.failover_count) ? "warn" : "good", icon: "route" })}
      ${statTile({ label: "Error rate", value: errTot.t ? fmtPct((errTot.e / errTot.t) * 100) : "—", sub: errTot.t ? `${fmtInt(errTot.e)} errors` : "No data", tone: errTot.e ? "bad" : "good", icon: "warning" })}
    </div>`;
  }

  function drawAnalyticsCharts(byType) {
    const charts = analyticsEl.querySelector("#rt-charts");
    const traffic = byType.provider_traffic || [];
    const latency = byType.latency_percentiles || [];
    const tokensProvider = byType.tokens_by_provider || [];
    const costProvider = byType.cost_by_provider || [];

    const trafficCard = card("Provider traffic", barList(traffic.map((p) => ({ label: p.provider || "unknown", value: Number(p.traffic_percentage) || 0, text: fmtPct(p.traffic_percentage) })), { empty: "No provider traffic recorded" }));
    const shareCard = card("Traffic share", donut(traffic.map((p) => ({ label: p.provider || "unknown", value: Number(p.request_count) || 0 })), { empty: "No data" }));
    const latencyCard = card("Latency by provider", latency.length ? latencyBars(latency.map((r) => ({ label: r.provider, p50: Number(r.p50_latency_ms), p95: Number(r.p95_latency_ms), p99: Number(r.p99_latency_ms) }))) : emptyState({ title: "No latency data" }));
    const consumptionRows = tokensProvider.map((row) => {
      const c = costProvider.find((x) => x.provider === row.provider);
      return [esc(row.provider || "unknown"), fmtInt(row.request_count), fmtCompact(row.total_tokens), fmtMoney(c?.total_cost ?? 0)];
    });
    const consumptionCard = card("Provider consumption", table(["Provider", "Requests", "Tokens", "Cost"], consumptionRows, { empty: "No provider usage recorded" }));

    charts.innerHTML = `<div class="vstack">
      <div class="split split-even">${trafficCard}${shareCard}</div>
      <div class="split split-even">${latencyCard}${consumptionCard}</div>
    </div>`;
  }

  function drawFeed() {
    const feed = analyticsEl.querySelector("#rt-feed");
    if (!feed) return;
    const evts = ctx.stream.list.filter((e) => e.stage === "routing_decision" || e.stage === "routing_failover").slice(-30).reverse();
    const current = ctx.stream.list.filter((e) => e.stage === "routing_decision").length;
    const failovers = ctx.stream.list.filter((e) => e.stage === "routing_failover").length;
    const rows = evts.map((e) => {
      const d = e.data && typeof e.data === "object" ? e.data : {};
      const provider = d.selected_provider ?? d.upstream ?? d.to ?? "";
      const from = d.from ?? "";
      const to = d.to ?? "";
      return [
        `<span class="log-time">${esc(fmtTime(e.timestamp))}</span>`,
        e.stage === "routing_failover" ? badge("FAILOVER", "warning") : badge("DECISION", "secondary"),
        esc(d.model || "—"),
        esc(d.route || "default"),
        esc(provider || "unknown"),
        esc(d.attempt || "—"),
        `<span class="truncate" title="${esc(e.message)}">${e.stage === "routing_failover" && from && to ? `${esc(from)} → ${esc(to)}` : esc(e.message || "Provider selected")}</span>`,
      ];
    });
    feed.innerHTML = card("Live routing feed",
      `${table(["Time", "Event", "Model", "Route", "Provider", "Attempt", "Details"], rows, { empty: ctx.stream.connected ? "No routing events received in this session." : "Stream offline" })}
       <div class="row-between" style="margin-top:var(--space-4)">
         <span class="hstack" style="gap:var(--space-4)">
           <span class="text-light">Decisions <strong>${fmtInt(current)}</strong></span>
           <span class="text-light">Failovers <strong>${fmtInt(failovers)}</strong></span>
         </span>
         <span class="text-light" style="font-size:var(--text-8)">session counters</span>
       </div>`,
      { subtitle: "Routing decisions and failovers from the event stream" });
  }

  /* ── Loaders ─────────────────────────────────────────────────── */

  async function loadProviders() {
    providerEl.innerHTML = loadingBox();
    const [p, u] = await Promise.allSettled([api.listProviders(), api.listUpstreams()]);
    if (p.status === "fulfilled") providers = p.value || []; else { providers = []; providerEl.innerHTML = errorBox(p.reason); }
    if (u.status === "fulfilled") upstreams = u.value || [];
    drawProviders();
    drawUpstreams();
  }

  async function loadRules() {
    rulesEl.innerHTML = loadingBox();
    try { rules = await api.listRoutingRules() || []; drawRules(); }
    catch (e) { rulesEl.innerHTML = errorBox(e); }
  }

  async function loadUpstreams() {
    try { upstreams = await api.listUpstreams() || []; drawUpstreams(); drawProviders(); }
    catch { /* keep the previous snapshot */ }
  }

  function load() { loadProviders(); loadRules(); }

  ctx.root.querySelector("#rt-refresh").addEventListener("click", () => {
    load();
    if (rangeReady) loadAnalytics();
  });

  ctx.root.addEventListener("click", async (e) => {
    const t = e.target;
    if (t.closest("#rt-add-provider")) return addProvider();
    if (t.closest("#rt-add-rule")) return addRule();

    const tp = t.closest("[data-toggle-provider]");
    if (tp) {
      if (!requireValidatedAdmin()) return;
      const id = tp.dataset.toggleProvider;
      try { await (tp.dataset.enable === "1" ? api.enableProvider(id) : api.disableProvider(id)); toastOk("Provider updated"); loadProviders(); }
      catch (err) { toastError(err); }
      return;
    }
    const ep = t.closest("[data-edit-provider]"); if (ep) return editProvider(ep.dataset.editProvider);
    const dp = t.closest("[data-del-provider]");
    if (dp) {
      if (!requireValidatedAdmin()) return;
      const ok = await confirmDialog({ title: "Delete provider", body: "This removes the upstream from the registry.", confirmLabel: "Delete", danger: true });
      if (!ok) return;
      try { await api.deleteProvider(dp.dataset.delProvider); toastOk("Provider deleted"); loadProviders(); }
      catch (err) { toastError(err, "Delete provider"); }
      return;
    }
    const tr = t.closest("[data-toggle-rule]");
    if (tr) {
      if (!requireValidatedAdmin()) return;
      const id = tr.dataset.toggleRule;
      try { await (tr.dataset.enable === "1" ? api.enableRoutingRule(id) : api.disableRoutingRule(id)); toastOk("Rule updated"); loadRules(); }
      catch (err) { toastError(err); }
      return;
    }
    const er = t.closest("[data-edit-rule]"); if (er) return editRule(er.dataset.editRule);
    const dr = t.closest("[data-del-rule]");
    if (dr) {
      if (!requireValidatedAdmin()) return;
      const ok = await confirmDialog({ title: "Delete routing rule", body: "Requests matching this rule will fall back to other rules.", confirmLabel: "Delete", danger: true });
      if (!ok) return;
      try { await api.deleteRoutingRule(dr.dataset.delRule); toastOk("Rule deleted"); loadRules(); }
      catch (err) { toastError(err, "Delete rule"); }
    }
  });

  ctx.root.addEventListener("click", (e) => {
    const tab = e.target.closest('[role="tab"]');
    if (!tab) return;
    const idx = [...ctx.root.querySelectorAll('[role="tab"]')].indexOf(tab);
    if (idx === 3) drawAnalytics();
  });

  const unsub = ctx.stream.on(({ evt }) => { if (evt && rangeReady) drawFeed(); else if (rangeReady) drawFeed(); });

  drawValidator();
  load();
  const stop = createPoller(loadUpstreams, 5000);
  // auto-validate on mount when a token is already stored
  (async () => {
    if (!getAdminToken()) return;
    const res = await api.validateAdminToken();
    tokenStatus = res.ok ? "valid" : "invalid";
    drawValidator();
  })();

  return () => { unsub(); stop(); };
}
