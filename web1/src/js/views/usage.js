// Usage — §2 per-key attribution: verify a user key, see the identity it
// resolves to, and every request attributed to it — plus the ClickHouse-backed
// per-user spend and a searchable prompt log (§10).

import * as api from "../api.js";
import { getUserKey, setUserKey } from "../store.js";
import {
  esc, sectionHead, card, table, emptyState, icon, badge, errorBox, loadingBox,
  statTile, statusBadge, monoId, livePill,
} from "../ui.js";
import { deriveRequests } from "../stream.js";
import { fmtInt, fmtCompact, fmtMoney, fmtTime, fmtDateTime, relTime } from "../format.js";

export const meta = {
  id: "usage",
  label: "Usage",
  eyebrow: "Per-key attribution",
  group: "Observe",
  icon: "activity",
  description: "Verify a user key, see the identity it resolves to, and every request attributed to it.",
};

export function render(ctx) {
  ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Usage", description: meta.description }) + `
    <section id="us-verify"></section>

    <section id="us-requests" style="margin-top:var(--space-6)"></section>

    <section id="us-analytics" style="margin-top:var(--space-6)"></section>`;

  const verifyEl = ctx.root.querySelector("#us-verify");
  const reqEl = ctx.root.querySelector("#us-requests");
  const anaEl = ctx.root.querySelector("#us-analytics");

  /* ── Verify a key ────────────────────────────────────────────── */

  function drawVerify() {
    verifyEl.innerHTML = card("Verify a key", `
      <div class="vstack">
        <div class="row-form">
          <label data-field style="flex:1;margin:0">
            user api key
            <input id="us-key" class="mono" type="password" autocomplete="off" spellcheck="false" placeholder="ap_live_…" value="${esc(getUserKey())}">
          </label>
          <button id="us-test">${icon("key")} Verify & test</button>
        </div>
        <div id="us-result"></div>
        <div class="foot-hint">
          <span>${icon("info", 12)} The gateway parses <code>ap_live_&lt;key_id&gt;_&lt;secret&gt;</code>, hashes the secret with the server pepper, compares in constant time, then resolves and marks the key used.</span>
        </div>
      </div>`,
      { subtitle: "Paste a user's API key and confirm the identity it resolves to on the gateway",
        actions: `<button class="outline small" id="us-settings">${icon("help")} Settings</button>` });

    const keyEl = verifyEl.querySelector("#us-key");
    const resultEl = verifyEl.querySelector("#us-result");

    verifyEl.querySelector("#us-settings").addEventListener("click", ctx.openSettings);

    keyEl.addEventListener("change", () => { setUserKey(keyEl.value.trim()); drawRequests(); });

    verifyEl.querySelector("#us-test").addEventListener("click", async () => {
      const key = keyEl.value.trim();
      if (!key) { keyEl.focus(); return; }
      setUserKey(key);
      const btn = verifyEl.querySelector("#us-test");
      btn.disabled = true;
      btn.setAttribute("aria-busy", "true");
      resultEl.innerHTML = `<div class="hstack text-light">${icon("activity", 14)} Pinging the gateway…</div>`;
      try {
        const res = await api.sendChat({ model: "gpt-4o", messages: [{ role: "user", content: "Ping — confirm you are reachable." }] });
        const content = res?.choices?.[0]?.message?.content ?? "";
        resultEl.innerHTML = `<div role="alert" data-variant="success"><div>
          <strong>Key accepted · HTTP 200</strong>
          <div>Reply: ${esc(content) || "<em>(empty)</em>"}</div>
          <div class="text-light" style="margin-top:var(--space-1)">Identity is resolved on the SSE stream below (see the <code>request_received</code> → <code>authenticated</code> event).</div>
        </div></div>`;
      } catch (e) {
        resultEl.innerHTML = errorBox(e);
      } finally {
        btn.disabled = false;
        btn.removeAttribute("aria-busy");
        drawRequests();
      }
    });
  }

  /* ── Requests attributed to keys ─────────────────────────────── */

  function drawRequests() {
    const reqs = deriveRequests(ctx.stream.list).filter((r) => r.authenticated);
    if (!reqs.length) {
      reqEl.innerHTML = card("Requests attributed to keys",
        emptyState({ icon: "activity", title: "No authenticated requests yet", body: "Send one from the Playground or verify a key above." }),
        { subtitle: "Authenticated requests observed on the SSE stream this session" });
      return;
    }
    const rows = reqs.slice(0, 100).map((r) => [
      monoId(String(r.id).replace(/^req-/, "").slice(0, 8)),
      `<strong>${esc(r.authenticated.username || "—")}</strong><div class="mono-id">${esc(r.authenticated.user_id || "")}</div>`,
      `<span class="truncate" title="${esc(r.prompt)}" style="max-width:34ch;display:inline-block">${esc(r.prompt || r.model || "—")}</span>`,
      esc(fmtTime(r.startedAt)),
      r.blocked
        ? badge("block", "danger", { dot: true })
        : r.status === "error"
          ? badge("error", "danger", { dot: true })
          : r.endedAt
            ? badge("pass", "success", { dot: true })
            : statusBadge("in flight"),
    ]);
    reqEl.innerHTML = card("Requests attributed to keys",
      table(["Request", "Identity", "Prompt", "Time", "Outcome"], rows, { empty: "No requests" }),
      { subtitle: "Authenticated requests observed on the SSE stream this session", actions: badge(`${reqs.length}`, "secondary") });
  }

  /* ── Per-user spend & prompt log (§10) ───────────────────────── */

  function drawAnalytics() {
    if (!ctx.hasAdminToken()) {
      anaEl.innerHTML = card("User analytics",
        `<div role="alert"><div><strong>Admin token required.</strong><div>ClickHouse analytics use the admin token (ap:ViewMetrics).
        <button class="ghost small" id="us-open-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`);
      anaEl.querySelector("#us-open-settings").addEventListener("click", ctx.openSettings);
      return;
    }

    let range = "24h";
    let filterUser = "";
    let search = "";
    let debounce = null;

    anaEl.innerHTML = card("", `
      <div class="vstack">
        <div class="hstack" style="justify-content:space-between">
          <div class="hstack">
            <select id="us-range" aria-label="Time range" style="width:auto">
              <option value="24h">24h</option>
              <option value="7d">7d</option>
            </select>
            <button class="outline small" id="us-refresh">${icon("refresh")} Refresh</button>
          </div>
          <fieldset class="group" style="max-width:24rem">
            <input id="us-q" placeholder="Search prompts…" aria-label="Search prompts">
            <span id="us-chip"></span>
          </fieldset>
        </div>
        <div id="us-totals"></div>
        <div id="us-users"></div>
        <div id="us-prompts"></div>
      </div>`,
      { subtitle: "GET /api/observability/users & /prompts — per-user spend, token usage, and a searchable prompt log from ClickHouse" });

    const q = (id) => anaEl.querySelector("#" + id);

    async function load() {
      q("us-users").innerHTML = loadingBox("Loading user usage");
      try {
        const [users, prompts] = await Promise.all([
          api.obsUsers(range, 100),
          api.obsPrompts({ range, user: filterUser, q: search, limit: 100 }),
        ]);

        const spend = users.reduce((n, u) => n + (u.total_cost || 0), 0);
        const requests = users.reduce((n, u) => n + (u.request_count || 0), 0);
        const tokens = users.reduce((n, u) => n + (u.total_tokens || 0), 0);
        q("us-totals").innerHTML = `<div class="grid-kpi">
          ${statTile({ label: "Total spend", value: fmtMoney(spend), icon: "dollar" })}
          ${statTile({ label: "Requests", value: fmtInt(requests), icon: "bolt" })}
          ${statTile({ label: "Tokens", value: fmtCompact(tokens), icon: "message" })}
          ${statTile({ label: "Users", value: fmtInt(users.length), icon: "users" })}
        </div>`;

        const rows = users.map((u) => [
          `<button class="link-btn" data-user="${esc(u.username || "")}">${esc(u.username || u.user_id || "—")}</button>`,
          fmtInt(u.request_count), fmtCompact(u.input_tokens), fmtCompact(u.output_tokens), fmtCompact(u.total_tokens),
          fmtMoney(u.total_cost), esc(u.avg_latency_ms != null ? `${Math.round(u.avg_latency_ms)} ms` : "—"),
          esc(u.last_seen ? relTime(u.last_seen) : "—"),
        ]);
        q("us-users").innerHTML = card("Per-user usage",
          table(["User", "Requests", "In", "Out", "Tokens", "Cost", "Avg latency", "Last seen"], rows, { empty: "No per-user usage in this window" }),
          { subtitle: `${users.length} user(s) · click a user to filter the prompt log` });

        const chips = [];
        if (filterUser) chips.push(`<span class="chip">user: ${esc(filterUser)}<button data-clear="user" aria-label="Clear user filter">${icon("x", 11)}</button></span>`);
        q("us-chip").innerHTML = chips.join("");

        const prows = prompts.map((p) => [
          `<span class="truncate" title="${esc(p.prompt)}" style="max-width:38ch;display:inline-block">${esc((p.prompt || "").slice(0, 140))}</span>`,
          esc(p.username || "—"),
          esc(p.model || "—"),
          fmtCompact(p.total_tokens),
          fmtMoney(p.estimated_cost),
          monoId(String(p.trace_id || "").slice(0, 10)),
          esc(p.created_at ? fmtDateTime(p.created_at) : "—"),
        ]);
        q("us-prompts").innerHTML = table(["Prompt", "User", "Model", "Tokens", "Cost", "Trace", "Time"], prows, { empty: "No prompts matched" });
      } catch (e) {
        q("us-users").innerHTML = errorBox(e);
        q("us-totals").innerHTML = "";
        q("us-prompts").innerHTML = "";
      }
    }

    q("us-range").addEventListener("change", (e) => { range = e.target.value; load(); });
    q("us-refresh").addEventListener("click", load);
    q("us-q").addEventListener("input", (e) => {
      clearTimeout(debounce);
      debounce = setTimeout(() => { search = e.target.value.trim(); load(); }, 300);
    });
    anaEl.addEventListener("click", (e) => {
      const ub = e.target.closest("[data-user]");
      if (ub) { filterUser = filterUser === ub.dataset.user ? "" : ub.dataset.user; load(); return; }
      if (e.target.closest('[data-clear="user"]')) { filterUser = ""; load(); }
    });

    load();
  }

  drawVerify();
  drawRequests();
  drawAnalytics();

  const unsub = ctx.stream.on(() => drawRequests());
  return () => unsub();
}
