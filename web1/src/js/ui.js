// Shared UI kit. Everything here composes Oat primitives (cards, tables,
// dialogs, badges, alerts, toasts) so views stay declarative.

import { esc, pretty } from "./format.js";

export { esc };

/* ── Tiny DOM helpers ────────────────────────────────────────────────── */

export function el(html) {
  const t = document.createElement("template");
  t.innerHTML = html.trim();
  return t.content.firstElementChild;
}

export function setHTML(target, html) {
  target.replaceChildren(...el(`<div>${html}</div>`).childNodes);
}

export function on(root, selector, event, handler) {
  root.addEventListener(event, (e) => {
    const match = e.target.closest(selector);
    if (match && root.contains(match)) handler(e, match);
  });
}

/* ── Icons ───────────────────────────────────────────────────────────── */

const ICONS = {
  plus: '<path d="M12 5v14M5 12h14"/>',
  minus: '<path d="M5 12h14"/>',
  trash: '<path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>',
  refresh: '<path d="M3 12a9 9 0 0 1 15.5-6.3L21 8M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-15.5 6.3L3 16M3 21v-5h5"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  play: '<path d="m6 4 14 8-14 8V4Z"/>',
  pause: '<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  chevron: '<path d="m6 9 6 6 6-6"/>',
  chevronRight: '<path d="m9 6 6 6-6 6"/>',
  arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
  external: '<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  shield: '<path d="M12 3 4 6v6c0 5 3.5 7.5 8 9 4.5-1.5 8-4 8-9V6l-8-3Z"/>',
  zap: '<path d="M13 2 4 14h7l-1 8 9-12h-7l1-8Z"/>',
  bolt: '<path d="M13 2 4 14h7l-1 8 9-12h-7l1-8Z"/>',
  gauge: '<path d="M12 14 8 10"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>',
  route: '<circle cx="6" cy="19" r="3"/><path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15"/><circle cx="18" cy="5" r="3"/>',
  list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  chart: '<path d="M3 3v18h18"/><path d="M7 15l4-4 3 3 5-6"/>',
  key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="m10.5 12.5 8-8M17 4l3 3M15 8l2 2"/>',
  users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  lock: '<rect x="3" y="11" width="18" height="10" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  message: '<path d="M21 14.5a2 2 0 0 1-2 2H8.5L3.5 20V5.5a2 2 0 0 1 2-2h13.5a2 2 0 0 1 2 2v9z"/><path d="M7.5 8.5h9M7.5 12h6"/>',
  layers: '<path d="M12 3l9 5-9 5-9-5 9-5z"/><path d="M3 13.5l9 5 9-5"/>',
  warning: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  help: '<circle cx="12" cy="12" r="9"/><path d="M9.3 9.3a2.8 2.8 0 0 1 5.5.6c0 1.9-2.8 2.2-2.8 3.6"/><path d="M12 17h.01"/>',
  eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>',
  server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z"/>',
  dollar: '<path d="M12 2v20"/><path d="M17 6.5c0-2-2.2-3-5-3s-5 1-5 3 2.2 2.7 5 3.3 5 1.3 5 3.4-2.2 3.3-5 3.3-5-1-5-3.3"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  filter: '<path d="M3 5h18l-7 8v6l-4-2v-4L3 5Z"/>',
  download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4"/><path d="M4 19h16"/>',
  sparkle: '<path d="M12 3v4M12 17v4M3 12h4M17 12h4M6.3 6.3l2.8 2.8M14.9 14.9l2.8 2.8M17.7 6.3l-2.8 2.8M9.1 14.9l-2.8 2.8"/>',
};

export function icon(name, size = 16) {
  const body = ICONS[name] || "";
  return `<svg width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
}

/* ── Notifications ───────────────────────────────────────────────────── */

export function toast(message, { title = "", variant = "info", placement = "top-right", duration = 4200 } = {}) {
  if (window.ot && typeof window.ot.toast === "function") {
    window.ot.toast(message, title, { variant, placement, duration });
  } else {
    console.log(`[${variant}] ${title}: ${message}`);
  }
}

export function toastError(err, title = "Request failed") {
  const msg = err?.message || String(err);
  toast(msg, { title, variant: "danger", duration: 6000 });
}

export function toastOk(message, title = "Done") {
  toast(message, { title, variant: "success" });
}

/* ── Presentational fragments ────────────────────────────────────────── */

const goodWords = /^(ok|success|active|enabled|allow|allowed|fresh|passed|pass|live|healthy|closed|completed|delivered|authenticated)$/i;
const warnWords = /^(warn|warning|pending|stale|retry|degraded|half_open|half-open|log_only|started|in flight|in progress)$/i;
const badWords = /^(error|denied|block|blocked|revoke|revoked|disabled|invalid|fail|failed|unavailable|empty|expired|open)$/i;

export function statusVariant(status) {
  if (!status) return "secondary";
  const s = String(status).toLowerCase();
  if (goodWords.test(s)) return "success";
  if (warnWords.test(s)) return "warning";
  if (badWords.test(s)) return "danger";
  return "secondary";
}

export function badge(text, variant = "secondary", { dot = false, title = "" } = {}) {
  const d = dot ? '<span class="status-dot"></span>' : "";
  const t = title ? ` title="${esc(title)}"` : "";
  return `<span class="badge" data-variant="${variant}"${t}>${d}${esc(text)}</span>`;
}

export function statusBadge(status) {
  return badge(status ?? "unknown", statusVariant(status));
}

/** Guardrail decision badge: pass / redact / warn / block / log_only. */
export function decisionBadge(decision) {
  const d = String(decision || "pass").toLowerCase();
  const tone = d === "block" ? "danger" : d === "redact" ? "warning" : d === "warn" ? "warning" : d === "log_only" ? "secondary" : "success";
  return badge(d.replace("_", " ").toUpperCase(), tone);
}

/** Inline live pill used in headers/cards. */
export function livePill(live) {
  return badge(live ? "Live" : "Offline", live ? "success" : "secondary", { dot: true });
}

export function statTile({ label, value, sub = "", tone = "", icon: ic = "" }) {
  return `<article class="card stat${ic ? " has-icon" : ""}">
    ${ic ? `<span class="stat-icon">${icon(ic, 15)}</span>` : ""}
    <span class="stat-label">${esc(label)}</span>
    <span class="stat-value${tone ? ` is-${tone}` : ""}">${value}</span>
    ${sub ? `<span class="stat-sub">${sub}</span>` : ""}
  </article>`;
}

/** A tile with a large inline metric (Ø old app's Metric). */
export function metricTile({ label, value, sub = "", tone = "", icon: ic = "" }) {
  return statTile({ label, value, sub, tone, icon: ic });
}

export function sectionHead({ eyebrow, title, description, actions = "" }) {
  return `<header class="section-head">
    <div>
      ${eyebrow ? `<span class="eyebrow">${esc(eyebrow)}</span>` : ""}
      <h1>${esc(title)}</h1>
      ${description ? `<p>${esc(description)}</p>` : ""}
    </div>
    ${actions ? `<div class="head-actions">${actions}</div>` : ""}
  </header>`;
}

export function emptyState({ title, body = "", action = "", icon: ic = "" }) {
  return `<div class="empty">
    ${ic ? `<span class="empty-icon">${icon(ic, 20)}</span>` : ""}
    <strong>${esc(title)}</strong>
    ${body ? `<span>${esc(body)}</span>` : ""}
    ${action}
  </div>`;
}

export function alertBox({ variant = "info", title = "", body = "", actions = "" }) {
  const v = variant === "error" ? "error" : variant;
  return `<div role="alert"${variant && variant !== "info" ? ` data-variant="${v}"` : ""}>
    <div>${title ? `<strong>${esc(title)}</strong>` : ""}${body ? `<div>${body}</div>` : ""}${actions}</div>
  </div>`;
}

export function errorBox(err) {
  const status = err?.status ? `${err.status} ` : "";
  return `<div role="alert" data-variant="error">
    <div><strong>${esc(status)}${esc(err?.type || "Request failed")}</strong><div>${esc(err?.message || String(err))}</div></div>
  </div>`;
}

export function loadingBox(label = "Loading…") {
  return `<div class="skeleton-stack" role="status" aria-busy="true" aria-label="${esc(label)}">
    <div class="skeleton line"></div><div class="skeleton line"></div><div class="skeleton line" style="width:60%"></div>
  </div>`;
}

export function spinner() {
  return `<span class="spinner" role="status" aria-hidden="true"></span>`;
}

export function progressBar(pct, { tone = "" } = {}) {
  const v = Math.max(0, Math.min(100, Number(pct) || 0));
  return `<div class="progress-track"><div class="progress-fill${tone ? " is-" + tone : ""}" style="width:${v}%"></div></div>`;
}

export function refreshButton(id = "", label = "Refresh") {
  return `<button class="outline"${id ? ` id="${id}"` : ""}>${icon("refresh")} ${esc(label)}</button>`;
}

export function card(title, body, { subtitle = "", actions = "", id = "", accent = "" } = {}) {
  const head =
    title || subtitle || actions
      ? `<div class="card-head">
          ${title || subtitle ? `<div>${title ? `<h3>${esc(title)}</h3>` : ""}${subtitle ? `<p>${esc(subtitle)}</p>` : ""}</div>` : ""}
          ${actions ? `<div class="hstack">${actions}</div>` : ""}
        </div>`
      : "";
  return `<article class="card${accent ? " accent-" + accent : ""}"${id ? ` id="${esc(id)}"` : ""}>
    ${head}
    ${body}
  </article>`;
}

/** columns: string[] | {label, align}[] ; rows: (string|number)[][] (cells are HTML) */
export function table(columns, rows, { empty = "Nothing to show" } = {}) {
  if (!rows || !rows.length) return emptyState({ title: empty });
  const cols = columns.map((c) => (typeof c === "string" ? { label: c } : c));
  const head = cols.map((c) => `<th${c.align ? ` style="text-align:${c.align}"` : ""}>${esc(c.label)}</th>`).join("");
  const body = rows
    .map((r) => `<tr>${r.map((cell, i) => `<td${cols[i]?.align ? ` style="text-align:${cols[i].align}"` : ""}>${cell ?? "—"}</td>`).join("")}</tr>`)
    .join("");
  return `<div class="table"><table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table></div>`;
}

export function kv(pairs) {
  return `<dl class="kv">${pairs
    .filter(Boolean)
    .map(([k, v]) => `<dt>${esc(k)}</dt><dd>${v}</dd>`)
    .join("")}</dl>`;
}

export function monoId(id, { short = 0 } = {}) {
  const s = String(id ?? "");
  const label = short && s.length > short * 2 ? `${s.slice(0, short)}…${s.slice(-4)}` : s;
  return `<span class="mono-id" title="${esc(s)}">${esc(label)}</span>`;
}

/** Clickable request-id chip that filters other views by that request. */
export function requestChip(id, { active = false, label } = {}) {
  const short = label || String(id || "").replace(/^req-/, "").slice(0, 8);
  return `<button class="req-chip${active ? " is-active" : ""}" data-req-filter="${esc(id)}" title="Filter by ${esc(id)}">#${esc(short)}</button>`;
}

/* ── Clipboard ───────────────────────────────────────────────────────── */

export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("Copied to clipboard", { variant: "success" });
  } catch {
    toast("Could not copy to clipboard", { variant: "danger" });
  }
}

/* ── Dialogs ─────────────────────────────────────────────────────────── */

function fieldMarkup(f) {
  const name = esc(f.name);
  const val = f.value ?? "";
  const req = f.required ? " required" : "";
  const ph = f.placeholder ? ` placeholder="${esc(f.placeholder)}"` : "";
  const hint = f.hint ? `<small data-hint>${esc(f.hint)}</small>` : "";

  switch (f.type) {
    case "textarea":
      return `<label data-field>${esc(f.label)}<textarea name="${name}"${ph}${req} rows="${f.rows || 4}">${esc(val)}</textarea>${hint}</label>`;
    case "json":
      return `<div data-field>
        <label for="f-${name}">${esc(f.label)}</label>
        <textarea id="f-${name}" name="${name}" class="json-editor mono" spellcheck="false"${req}>${esc(typeof val === "string" ? val : pretty(val))}</textarea>
        <small data-hint class="json-state"></small>
      </div>`;
    case "select": {
      const opts = (f.options || []).map((o) => {
        const ov = typeof o === "string" ? o : o.value;
        const ol = typeof o === "string" ? o : o.label;
        return `<option value="${esc(ov)}"${String(ov) === String(val) ? " selected" : ""}>${esc(ol)}</option>`;
      }).join("");
      return `<div data-field><label for="f-${name}">${esc(f.label)}</label><select id="f-${name}" name="${name}"${req}>${f.placeholder ? `<option value="">${esc(f.placeholder)}</option>` : ""}${opts}</select>${hint}</div>`;
    }
    case "checkbox":
      return `<label data-field><span class="hstack" style="gap:var(--space-2)"><input type="checkbox" role="switch" name="${name}"${val ? " checked" : ""}> ${esc(f.label)}</span>${hint}</label>`;
    case "tags":
      return `<div data-field>
        <label for="f-${name}">${esc(f.label)}</label>
        <ot-taginput name="${name}" value="${esc(val)}"><input id="f-${name}" placeholder="${esc(f.placeholder || "Add…")}"></ot-taginput>
        ${hint}
      </div>`;
    case "number":
      return `<label data-field>${esc(f.label)}<input type="number" name="${name}" value="${esc(val)}"${ph}${req}${f.min !== undefined ? ` min="${f.min}"` : ""}${f.max !== undefined ? ` max="${f.max}"` : ""}${f.step !== undefined ? ` step="${f.step}"` : ""}>${hint}</label>`;
    default:
      return `<label data-field>${esc(f.label)}<input type="${f.type || "text"}" name="${name}" value="${esc(val)}"${ph}${req}${f.autocomplete === false ? ' autocomplete="off"' : ""}${f.mono ? ' class="mono"' : ""}>${hint}</label>`;
  }
}

function gatherValues(form, fields) {
  const out = {};
  for (const f of fields) {
    if (f.type === "tags") {
      const t = form.querySelector(`ot-taginput[name="${f.name}"]`);
      out[f.name] = t ? (t.value || []) : [];
    } else if (f.type === "checkbox") {
      const i = form.querySelector(`[name="${f.name}"]`);
      out[f.name] = !!i?.checked;
    } else if (f.type === "number") {
      const i = form.querySelector(`[name="${f.name}"]`);
      const raw = i?.value?.trim();
      out[f.name] = raw === "" || raw === undefined ? undefined : Number(raw);
    } else if (f.type === "json") {
      const i = form.querySelector(`[name="${f.name}"]`);
      const raw = i?.value?.trim() || "";
      if (!raw) { out[f.name] = undefined; continue; }
      try { out[f.name] = JSON.parse(raw); } catch (e) { throw new Error(`${f.label}: invalid JSON — ${e.message}`); }
    } else {
      const i = form.querySelector(`[name="${f.name}"]`);
      out[f.name] = i?.value ?? "";
    }
  }
  return out;
}

/**
 * Promise-based form dialog built from Oat form markup.
 * Resolves to a values object, or null when cancelled.
 */
export function formDialog({ title, description = "", fields = [], submitLabel = "Save", cancelLabel = "Cancel", danger = false, wide = false, medium = false, validate }) {
  return new Promise((resolve) => {
    const dlg = document.createElement("dialog");
    if (wide) dlg.classList.add("wide");
    if (medium) dlg.classList.add("medium");

    const body = fields.map(fieldMarkup).join("");
    dlg.innerHTML = `<form novalidate>
      <header>
        <h3>${esc(title)}</h3>
        ${description ? `<p class="text-light">${esc(description)}</p>` : ""}
      </header>
      <div class="vstack">${body}</div>
      <div class="dlg-error" hidden role="alert" data-variant="error"></div>
      <footer>
        <button type="button" class="outline" data-cancel>${esc(cancelLabel)}</button>
        <span class="topbar-spacer"></span>
        <button type="button"${danger ? ' data-variant="danger"' : ""} data-submit>${esc(submitLabel)}</button>
      </footer>
    </form>`;

    document.body.appendChild(dlg);
    const form = dlg.querySelector("form");
    const errEl = dlg.querySelector(".dlg-error");

    // Live JSON feedback
    for (const f of fields.filter((x) => x.type === "json")) {
      const ta = form.querySelector(`[name="${f.name}"]`);
      const note = ta?.parentElement.querySelector(".json-state");
      const check = () => {
        const v = ta.value.trim();
        if (!v) { if (note) { note.textContent = ""; note.className = "json-state"; } ta.removeAttribute("aria-invalid"); return; }
        try { JSON.parse(v); if (note) { note.textContent = "Valid JSON"; note.className = "json-state json-valid"; } ta.removeAttribute("aria-invalid"); }
        catch (e) { if (note) { note.textContent = e.message; note.className = "json-state json-invalid"; } ta.setAttribute("aria-invalid", "true"); }
      };
      ta?.addEventListener("input", check);
      check();
    }

    let settled = false;
    const finish = (v) => { if (!settled) { settled = true; resolve(v); } };

    const showError = (msg) => {
      if (!msg) { errEl.hidden = true; return; }
      errEl.innerHTML = `<div><strong>Check the form</strong><div>${esc(msg)}</div></div>`;
      errEl.hidden = false;
    };

    const submit = () => {
      if (!form.reportValidity()) return;
      let values;
      try { values = gatherValues(form, fields); }
      catch (e) { showError(e.message); return; }
      const custom = validate ? validate(values) : null;
      const problem = typeof custom === "string" ? custom : custom === false ? "Please correct the highlighted fields." : null;
      if (problem) { showError(problem); return; }
      finish(values);
      dlg.close();
    };

    dlg.querySelector("[data-cancel]").addEventListener("click", () => dlg.close());
    dlg.querySelector("[data-submit]").addEventListener("click", submit);
    form.addEventListener("submit", (e) => { e.preventDefault(); submit(); });

    dlg.addEventListener("close", () => { dlg.remove(); finish(null); });
    dlg.addEventListener("cancel", (e) => { e.preventDefault(); dlg.close(); });

    dlg.showModal();
    const first = form.querySelector("input:not([type=hidden]), textarea, select");
    setTimeout(() => first?.focus(), 30);
  });
}

/** Promise-based confirmation. Resolves true/false. */
export function confirmDialog({ title, body = "", confirmLabel = "Confirm", cancelLabel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    const dlg = document.createElement("dialog");
    dlg.innerHTML = `<form>
      <header><h3>${esc(title)}</h3>${body ? `<p class="text-light">${esc(body)}</p>` : ""}</header>
      <footer>
        <button type="button" class="outline" data-cancel>${esc(cancelLabel)}</button>
        <span class="topbar-spacer"></span>
        <button type="button"${danger ? ' data-variant="danger"' : ""} data-confirm>${esc(confirmLabel)}</button>
      </footer>
    </form>`;
    document.body.appendChild(dlg);
    let settled = false;
    const finish = (v) => { if (!settled) { settled = true; resolve(v); } };
    dlg.querySelector("[data-cancel]").addEventListener("click", () => dlg.close());
    dlg.querySelector("[data-confirm]").addEventListener("click", () => { finish(true); dlg.close(); });
    dlg.addEventListener("close", () => { dlg.remove(); finish(false); });
    dlg.addEventListener("cancel", (e) => { e.preventDefault(); dlg.close(); });
    dlg.showModal();
  });
}
