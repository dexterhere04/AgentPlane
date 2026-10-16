// AgentPlane Control Plane — bootstrap and router.

import { createGatewayStream } from "./stream.js";
import * as store from "./store.js";
import { validateAdminToken } from "./api.js";
import { icon, toast, toastOk, errorBox } from "./ui.js";
import { mountChatOverlay } from "./chat.js";

import * as overview from "./views/overview.js";
import * as playground from "./views/playground.js";
import * as pipeline from "./views/pipeline.js";
import * as guardrails from "./views/guardrails.js";
import * as routing from "./views/routing.js";
import * as analytics from "./views/analytics.js";
import * as events from "./views/events.js";
import * as usage from "./views/usage.js";
import * as keys from "./views/keys.js";
import * as access from "./views/access.js";

const VIEWS = [overview, playground, pipeline, guardrails, routing, analytics, events, usage, keys, access];
const byId = new Map(VIEWS.map((v) => [v.meta.id, v]));

/* ── Theme ───────────────────────────────────────────────────────────── */

function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  document.documentElement.style.colorScheme = theme;
  store.setTheme(theme);
}

function currentTheme() {
  return store.getTheme() === "dark" ? "dark" : "light";
}

/* ── Sidebar (Oat layout) ────────────────────────────────────────────── */

function applySidebar() {
  const layout = document.querySelector("[data-sidebar-layout]");
  if (!layout) return;
  if (store.isSidebarOpen()) layout.setAttribute("data-sidebar-open", "");
  else layout.removeAttribute("data-sidebar-open");
}

document.addEventListener("click", (e) => {
  if (e.target.closest("[data-sidebar-toggle]")) {
    const layout = document.querySelector("[data-sidebar-layout]");
    // Oat toggles the attribute synchronously; mirror the resulting state.
    if (layout) store.setSidebarOpen(layout.hasAttribute("data-sidebar-open"));
  }
});

/* ── Stream ──────────────────────────────────────────────────────────── */

const stream = createGatewayStream(() => store.getAdminToken());
const statusEl = document.getElementById("stream-status");
const sideStreamEl = document.getElementById("side-stream");

function updateStatus() {
  const live = stream.connected;
  statusEl.className = "badge";
  statusEl.setAttribute("data-variant", live ? "success" : "secondary");
  statusEl.innerHTML = `<span class="status-dot ${live ? "on" : "off"}"></span> ${live ? "Live" : "Offline"}`;
  statusEl.title = live ? "Connected to the gateway event stream" : "Event stream offline";

  sideStreamEl.innerHTML = `<span class="status-dot ${live ? "on" : "off"}"></span> ${live ? "Gateway live" : "Gateway offline"}`;
}

stream.on(() => updateStatus());

/* ── Navigation ──────────────────────────────────────────────────────── */

const navEl = document.getElementById("side-nav");
const crumbEl = document.getElementById("crumb-current");

let viewRoot = document.getElementById("view");

/** Replace the view container so the previous view's DOM listeners are discarded. */
function newViewRoot() {
  const div = document.createElement("div");
  div.id = "view";
  div.className = "view reveal";
  viewRoot.replaceWith(div);
  viewRoot = div;
  ctx.root = div;
  return div;
}

function buildNav() {
  const groups = new Map();
  for (const v of VIEWS) {
    if (!groups.has(v.meta.group)) groups.set(v.meta.group, []);
    groups.get(v.meta.group).push(v.meta);
  }
  let html = "";
  for (const [label, items] of groups) {
    html += `<li class="nav-label">${label}</li>`;
    for (const m of items) {
      html += `<li><a href="#${m.id}" data-nav="${m.id}">${icon(m.icon, 16)}<span>${m.label}</span></a></li>`;
    }
  }
  navEl.innerHTML = html;
}

let disposeView = null;
let activeId = null;

const ctx = {
  root: viewRoot,
  stream,
  hasAdminToken: () => !!store.getAdminToken(),
  navigate: (id) => { location.hash = `#${id}`; },
  openSettings: () => openSettings(),
  onUserKeyChange: () => {},
  refreshChrome: () => updateStatus(),
};

function activate(id) {
  const view = byId.get(id) || byId.get("overview");
  if (!view) return;
  if (disposeView) { try { disposeView(); } catch (e) { console.error(e); } disposeView = null; }

  activeId = view.meta.id;
  newViewRoot();
  document.title = `${view.meta.label} · AgentPlane`;
  crumbEl.textContent = view.meta.label;

  for (const a of navEl.querySelectorAll("[data-nav]")) {
    if (a.dataset.nav === activeId) a.setAttribute("aria-current", "page");
    else a.removeAttribute("aria-current");
  }

  try {
    disposeView = view.render(ctx) || null;
  } catch (e) {
    console.error(e);
    ctx.root.innerHTML = errorBox(e);
  }
  document.querySelector("main")?.scrollTo?.(0, 0);
}

function currentFromHash() {
  const id = (location.hash || "").replace(/^#/, "").split("?")[0];
  return byId.has(id) ? id : "overview";
}

window.addEventListener("hashchange", () => activate(currentFromHash()));

/* ── Settings dialog ─────────────────────────────────────────────────── */

const settingsDlg = document.getElementById("settings-dialog");
const adminInput = document.getElementById("admin-token");
const userInput = document.getElementById("user-key");
const settingsStatus = document.getElementById("settings-status");

function openSettings() {
  adminInput.value = store.getAdminToken();
  userInput.value = store.getUserKey();
  settingsStatus.innerHTML = "";
  settingsDlg.showModal();
}

document.getElementById("settings-open").addEventListener("click", openSettings);

settingsDlg.querySelector("form").addEventListener("submit", () => {
  store.setAdminToken(adminInput.value.trim());
  store.setUserKey(userInput.value.trim());
  stream.sync();
  updateStatus();
  toastOk("Credentials saved");
  activate(activeId); // re-render the active view with the new credentials
});

document.getElementById("settings-validate").addEventListener("click", async () => {
  store.setAdminToken(adminInput.value.trim());
  settingsStatus.innerHTML = `<div class="hstack" style="gap:var(--space-2)">${'<span class="status-dot"></span>'} Validating…</div>`;
  const res = await validateAdminToken();
  if (res.ok) {
    settingsStatus.innerHTML = `<div role="alert" data-variant="success"><div><strong>Token accepted</strong><div>The gateway authorised GET /admin/providers.</div></div></div>`;
    stream.sync();
    updateStatus();
  } else {
    settingsStatus.innerHTML = `<div role="alert" data-variant="error"><div><strong>Token rejected</strong><div>${res.reason}</div></div></div>`;
  }
});

document.getElementById("settings-reset").addEventListener("click", () => {
  store.setup.reset();
  toast("Setup guide reset", { variant: "info" });
  settingsDlg.close();
  if (activeId === "overview") activate("overview");
});

/* ── Theme toggle ────────────────────────────────────────────────────── */

document.getElementById("theme-toggle").addEventListener("click", () => {
  applyTheme(currentTheme() === "dark" ? "light" : "dark");
});

/* ── Boot ────────────────────────────────────────────────────────────── */

function boot() {
  applyTheme(currentTheme());
  applySidebar();
  buildNav();
  stream.open();
  updateStatus();
  mountChatOverlay(ctx);
  activate(currentFromHash());
}

boot();
