// Keys & Secrets — §5 API key provisioning/lifecycle and §6 provider secrets.

import * as api from "../api.js";
import { PROVIDER_SLUGS } from "../api.js";
import { setup, setUserKey } from "../store.js";
import { esc, sectionHead, card, table, statusBadge, toastOk, toastError, confirmDialog, errorBox, loadingBox, copyText, icon, badge } from "../ui.js";
import { fmtDateTime, relTime } from "../format.js";

export const meta = {
  id: "keys",
  label: "Keys & Secrets",
  eyebrow: "Provider secrets & API keys",
  group: "Admin",
  icon: "key",
  description: "Store upstream provider credentials, then provision and revoke user API keys.",
};

export function render(ctx) {
  if (!ctx.hasAdminToken()) {
    ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Keys & Secrets", description: meta.description }) +
      `<div role="alert"><div><strong>Admin token required.</strong>
        <div>Secret storage and provisioning are admin operations.
        <button class="ghost small" id="ky-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`;
    ctx.root.querySelector("#ky-settings").addEventListener("click", ctx.openSettings);
    return;
  }

  ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Keys & Secrets", description: meta.description }) + `
    <div class="split">
      <section class="stack">
        ${card("Store a provider secret", `
          <form id="ky-secret-form" class="vstack">
            <label data-field>
              Provider
              <select id="ky-provider" aria-label="Provider">${PROVIDER_SLUGS.map((p) => `<option value="${esc(p)}">${esc(p)}</option>`).join("")}</select>
            </label>
            <label data-field>
              API key
              <input id="ky-secret" type="password" autocomplete="off" spellcheck="false" class="mono" placeholder="sk-…" required>
              <small data-hint>Written to the configured secret store (Vault by default) as <code>&lt;PROVIDER&gt;_API_KEY</code> and never read back by this client.</small>
            </label>
            <div class="hstack" style="justify-content:flex-end"><button type="submit">${icon("plus")} Store secret</button></div>
          </form>
          <div id="ky-secret-result" style="margin-top:var(--space-4)"></div>
          <p class="hint foot-hint">${icon("lock", 12)} The gateway runs with <code>SECRET_STORE=vault</code> and a KV-v2 mount at <code>agentplane/</code> — writing a key updates <code>agentplane/&lt;PROVIDER&gt;_API_KEY</code>.</p>`,
          { subtitle: "POST /admin/secrets/provider", actions: badge("vault", "secondary") })}

        ${card("Provision a user & API key", `
          <form id="ky-provision-form" class="vstack">
            <div class="grid-2" style="gap:var(--space-4)">
              <label data-field>
                Username
                <input id="ky-username" placeholder="demo-user" required autocomplete="off">
              </label>
              <label data-field>
                Email <small data-hint>Optional</small>
                <input id="ky-email" type="email" placeholder="user@example.com" autocomplete="off">
              </label>
            </div>
            <div class="grid-2" style="gap:var(--space-4)">
              <label data-field>
                Key name
                <input id="ky-keyname" placeholder="demo key" autocomplete="off">
              </label>
              <label data-field>
                Role <small data-hint>Optional — deny by default without one</small>
                <select id="ky-role"><option value="">no role — no access</option></select>
              </label>
            </div>
            <div class="hstack" style="justify-content:flex-end"><button type="submit">${icon("plus")} Create user & key</button></div>
          </form>
          <div id="ky-reveal" style="margin-top:var(--space-4)"></div>`,
          { subtitle: "POST /provision/user · the full key is returned once" })}
      </section>

      <section id="ky-list"></section>
    </div>`;

  const listEl = ctx.root.querySelector("#ky-list");
  const revealEl = ctx.root.querySelector("#ky-reveal");
  const secretResultEl = ctx.root.querySelector("#ky-secret-result");
  const roleEl = ctx.root.querySelector("#ky-role");

  /* ── Roles for the provisioning selector ─────────────────────── */

  (async () => {
    try {
      const roles = await api.listRoles();
      roleEl.innerHTML = `<option value="">no role — no access</option>` + roles.map((r) => `<option value="${esc(r.name)}">${esc(r.name)}</option>`).join("");
    } catch { /* leave the empty option if roles cannot be listed */ }
  })();

  /* ── §6 store secret ───────────────────────────────────────── */

  ctx.root.querySelector("#ky-secret-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const provider = ctx.root.querySelector("#ky-provider").value;
    const apiKey = ctx.root.querySelector("#ky-secret").value.trim();
    if (!apiKey) return;
    try {
      const res = await api.storeProviderSecret(provider, apiKey);
      ctx.root.querySelector("#ky-secret").value = "";
      setup.set("secret");
      const ref = res?.secret_key || `${provider.toUpperCase()}_API_KEY`;
      secretResultEl.innerHTML = `<div role="alert" data-variant="success"><div>
          <strong>Secret stored</strong>
          <div>Reference it from a provider as <span class="mono-id">${esc(ref)}</span> — not the raw key.</div>
        </div></div>
        <div class="hstack" style="margin-top:var(--space-3)">
          <button class="outline small" id="ky-copy-ref">${icon("copy")} Copy reference</button>
          <button class="outline small" id="ky-go-routing">Open routing</button>
        </div>`;
      secretResultEl.querySelector("#ky-copy-ref").addEventListener("click", () => copyText(ref));
      secretResultEl.querySelector("#ky-go-routing").addEventListener("click", () => ctx.navigate("routing"));
    } catch (err) {
      secretResultEl.innerHTML = errorBox(err);
    }
  });

  /* ── §5 provision (two-step: user+key, then role) ───────────── */

  ctx.root.querySelector("#ky-provision-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const username = ctx.root.querySelector("#ky-username").value.trim();
    const email = ctx.root.querySelector("#ky-email").value.trim();
    const key_name = ctx.root.querySelector("#ky-keyname").value.trim();
    const role = roleEl.value;
    try {
      const res = await api.provisionUser({ username, email, key_name });
      setup.set("provisioned");
      let roleNote = "";
      if (role && res.user_id) {
        try {
          await api.grantUserRole(res.user_id, role);
          roleNote = `<div role="alert" data-variant="success" style="margin-top:var(--space-3)"><div>Assigned role <strong>${esc(role)}</strong> to ${esc(username)}.</div></div>`;
        } catch (err) {
          roleNote = `<div role="alert" data-variant="warning" style="margin-top:var(--space-3)"><div><strong>User and key created, but role assignment failed</strong><div>HTTP ${esc(err.status || "")} · ${esc(err.message || "")}</div></div></div>`;
        }
      }
      revealEl.innerHTML = `<div role="alert" data-variant="success"><div>
        <strong>Key created — copy it now</strong>
        <div>The full key is shown only once; only its hash is stored.</div>
      </div></div>
      <div class="secret-reveal" style="margin-top:var(--space-3)">
        <span id="ky-key-value" style="flex:1">${esc(res.api_key || "")}</span>
        <button class="outline small" id="ky-copy">${icon("copy")} Copy</button>
        <button class="outline small" id="ky-use">Use as user key</button>
      </div>
      <div class="text-light" style="font-size:var(--text-8);margin-top:var(--space-2)">user_id ${esc(res.user_id || "—")} · key_id ${esc(res.key_id || "—")}</div>
      ${roleNote}`;
      revealEl.querySelector("#ky-copy").addEventListener("click", () => copyText(res.api_key));
      revealEl.querySelector("#ky-use").addEventListener("click", () => {
        setUserKey(res.api_key);
        ctx.onUserKeyChange?.(res.api_key);
        toastOk("Stored as the active user key");
      });
      ctx.root.querySelector("#ky-username").value = "";
      ctx.root.querySelector("#ky-email").value = "";
      ctx.root.querySelector("#ky-keyname").value = "";
      roleEl.value = "";
      loadKeys();
    } catch (err) {
      revealEl.innerHTML = errorBox(err);
    }
  });

  /* ── §5 list / revoke ──────────────────────────────────────── */

  async function loadKeys() {
    listEl.innerHTML = loadingBox("Loading API keys");
    try {
      const keys = await api.listApiKeys();
      const rows = keys.map((k) => [
        `<span class="mono-id" title="${esc(k.key_id)}">${esc(k.key_id || "—")}</span>`,
        `<strong>${esc(k.username || "—")}</strong>`,
        esc(k.name || "—"),
        statusBadge(k.status || "active"),
        esc(k.created_at ? fmtDateTime(k.created_at) : "—"),
        esc(k.revoked_at ? relTime(k.revoked_at) : "—"),
        `<button class="outline small" data-variant="danger" data-revoke="${esc(k.key_id)}" ${(k.status || "") === "revoked" ? "disabled" : ""}>Revoke</button>`,
      ]);
      listEl.innerHTML = card(
        "",
        table(["Key ID", "User", "Name", "Status", "Created", "Revoked", ""], rows, { empty: "No API keys minted yet" }),
        { subtitle: `${keys.length} key(s)`, actions: `<button class="outline" id="ky-refresh">${icon("refresh")} Refresh</button>` }
      );
      listEl.querySelector("#ky-refresh").addEventListener("click", loadKeys);
    } catch (e) {
      listEl.innerHTML = errorBox(e);
    }
  }

  listEl.addEventListener("click", async (e) => {
    const b = e.target.closest("[data-revoke]");
    if (!b) return;
    const id = b.dataset.revoke;
    const ok = await confirmDialog({ title: "Revoke API key", body: `Requests using key ${id} will immediately be rejected.`, confirmLabel: "Revoke", danger: true });
    if (!ok) return;
    try { await api.revokeApiKey(id); toastOk("Key revoked"); loadKeys(); ctx.refreshChrome?.(); }
    catch (err) { toastError(err, "Revoke key"); }
  });

  loadKeys();
}
