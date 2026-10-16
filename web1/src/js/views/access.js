// Access Control — §8 Identity & Access Management.
// Policies, roles, groups and users, with entity pickers (no raw UUID typing),
// a policy-document viewer, a per-role policy attach picker, and an attachment
// inspector. Deny-by-default; an explicit Deny overrides any Allow.

import * as api from "../api.js";
import { esc, sectionHead, card, table, badge, statusBadge, formDialog, confirmDialog, toastOk, toastError, errorBox, loadingBox, icon, monoId } from "../ui.js";
import { fmtDateTime } from "../format.js";

export const meta = {
  id: "access",
  label: "Access Control",
  eyebrow: "Policies, roles & groups",
  group: "Admin",
  icon: "users",
  description: "Author AWS-style JSON policies, compose roles and groups, and set permission boundaries.",
};

const POLICY_TEMPLATE = {
  Version: "2026-01-01",
  Statement: [{ Sid: "AllowInvoke", Effect: "Allow", Action: ["ap:InvokeChat"], Resource: ["arn:ap:model/*"] }],
};

const PRINCIPAL_TYPES = [
  { value: "user", label: "user" },
  { value: "group", label: "group" },
  { value: "role", label: "role" },
];

export function render(ctx) {
  if (!ctx.hasAdminToken()) {
    ctx.root.innerHTML = sectionHead({ eyebrow: meta.eyebrow, title: "Access Control", description: meta.description }) +
      `<div role="alert"><div><strong>Admin token required.</strong>
        <div>IAM administration needs the admin token. Open <strong>Settings</strong> (the ⚙ icon in the top bar) and paste your <code>AGENTPLANE_ADMIN_TOKEN</code>.
        <button class="ghost small" id="ac-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`;
    ctx.root.querySelector("#ac-settings").addEventListener("click", ctx.openSettings);
    return;
  }

  ctx.root.innerHTML = sectionHead({
    eyebrow: meta.eyebrow, title: "Access Control", description: meta.description,
    actions: `<button class="outline" id="ac-refresh">${icon("refresh")} Refresh</button>`,
  }) + card("", `<ot-tabs id="ac-tabs">
      <div role="tablist">
        <button role="tab">Policies</button>
        <button role="tab">Roles</button>
        <button role="tab">Groups</button>
        <button role="tab">Users</button>
      </div>
      <div role="tabpanel" id="ac-policies"></div>
      <div role="tabpanel" id="ac-roles"></div>
      <div role="tabpanel" id="ac-groups"></div>
      <div role="tabpanel" id="ac-users"></div>
    </ot-tabs>`)
    + `<section id="ac-inspector" style="margin-top:var(--space-6)"></section>`;

  const P = (id) => ctx.root.querySelector("#" + id);
  const inspector = P("ac-inspector");

  // Directory cache — powers every entity picker (no raw UUID entry).
  const dir = { policies: [], roles: [], groups: [], users: [] };

  const setInspector = (title, subtitle, html) => {
    inspector.innerHTML = card(title, html, { subtitle, actions: `<button class="outline small" id="ac-close-inspector">${icon("x")} Close</button>` });
    inspector.querySelector("#ac-close-inspector").addEventListener("click", () => { inspector.innerHTML = ""; });
    inspector.scrollIntoView({ behavior: "smooth", block: "nearest" });
  };

  function authGuidance(e) {
    if (e?.status === 401) {
      return `<div role="alert" data-variant="error"><div><strong>HTTP 401 · unauthorized</strong>
        <div>Your credentials are missing or invalid. Open Settings (⚙ in the top bar) and paste your <code>AGENTPLANE_ADMIN_TOKEN</code>.
        <button class="ghost small" id="ac-err-settings" style="margin-inline-start:var(--space-2)">Open settings</button></div></div></div>`;
    }
    return errorBox(e);
  }

  function optionsFor(ptype, { exclude = [] } = {}) {
    const list = ptype === "role" ? dir.roles : ptype === "group" ? dir.groups : dir.users;
    return list
      .map((x) => ({ id: x.id, label: x.name || x.username || x.id }))
      .filter((o) => o.id && !exclude.includes(o.id));
  }

  /**
   * A single principal picker that encodes the type in its value
   * (`<type>::<uuid>`), so operators never type a raw UUID and the type never
   * desyncs from the chosen entity.
   */
  function principalChoices(types = ["user", "group", "role"]) {
    const out = [];
    for (const type of types) for (const o of optionsFor(type)) out.push({ value: `${type}::${o.id}`, label: `${type}: ${o.label}` });
    return out;
  }
  const splitPrincipal = (v) => {
    const i = String(v).indexOf("::");
    return { principal_type: String(v).slice(0, i), principal_id: String(v).slice(i + 2) };
  };

  /* ── Directory ───────────────────────────────────────────────── */

  async function loadDirectory() {
    const [p, r, g, u] = await Promise.allSettled([
      api.listPolicies(), api.listRoles(), api.listGroups(), api.listUsers(),
    ]);
    if (p.status === "fulfilled") dir.policies = p.value || [];
    if (r.status === "fulfilled") dir.roles = r.value || [];
    if (g.status === "fulfilled") dir.groups = g.value || [];
    if (u.status === "fulfilled") dir.users = u.value || [];
    return { p, r, g, u };
  }

  /* ── Policies (§8.1) ───────────────────────────────────────── */

  async function loadPolicies() {
    const el = P("ac-policies");
    el.innerHTML = loadingBox();
    try {
      const res = await loadDirectory();
      if (res.p.status === "rejected") throw res.p.reason;
      const rows = dir.policies.map((p) => {
        const stmts = p.document?.Statement?.length ?? 0;
        const builtin = p.is_builtin ? badge("built-in", "secondary") : "";
        const del = p.is_builtin
          ? `<button class="outline small" disabled title="Built-in policies cannot be deleted">Delete</button>`
          : `<button class="outline small" data-variant="danger" data-del-policy="${esc(p.name)}">Delete</button>`;
        return [
          `<strong>${esc(p.name)}</strong> ${builtin}<div class="text-light" style="font-size:var(--text-8)">${esc(p.description || "")}</div>`,
          `<details class="doc"><summary class="doc-sum">v${esc(p.version || "—")} · view document</summary><pre class="mono doc-pre">${esc(JSON.stringify(p.document, null, 2))}</pre></details>`,
          `<span class="hstack" style="justify-content:flex-end">
            <button class="outline small" data-att-policy="${esc(p.name)}">Attachments</button>
            <button class="outline small" data-attach="${esc(p.name)}">Attach</button>
            ${del}
          </span>`,
        ];
      });
      el.innerHTML = card("Policies", table(["Policy", "Document", ""], rows, { empty: "No policies defined" }),
        { subtitle: "AWS-style JSON documents — deny by default, an explicit Deny overrides any Allow",
          actions: `<button id="ac-add-policy">${icon("plus")} New policy</button>` });
    } catch (e) { el.innerHTML = card("Policies", authGuidance(e)); }
  }

  async function addPolicy() {
    const v = await formDialog({
      title: "Create policy", wide: true,
      description: "Policies use the ap: action prefix and arn:ap:{type}/{id} resources.",
      fields: [
        { name: "name", label: "Name", required: true, autocomplete: false },
        { name: "description", label: "Description" },
        { name: "document", label: "Policy document (JSON)", type: "json", value: POLICY_TEMPLATE, required: true },
      ],
      submitLabel: "Create policy",
      validate: (x) => (!x.name ? "A name is required." : !x.document ? "A valid JSON document is required." : null),
    });
    if (!v) return;
    try { await api.createPolicy({ name: v.name, description: v.description || "", document: v.document }); toastOk("Policy created"); loadPolicies(); }
    catch (e) { toastError(e, "Create policy"); }
  }

  async function attachDialog(name, mode) {
    return formDialog({
      title: `${mode === "attach" ? "Attach" : "Detach"} policy · ${name}`,
      description: "Bind this policy to a principal.",
      fields: [
        { name: "principal", label: "Principal", type: "select", placeholder: "select a user, group or role…", options: principalChoices(), required: true },
      ],
      submitLabel: mode === "attach" ? "Attach" : "Detach",
    });
  }

  async function manageAttachments(name) {
    setInspector(`Attachments · ${name}`, "Principals this policy is bound to", loadingBox());
    try {
      const list = await api.getPolicyAttachments(name);
      const rows = list.map((a) => {
        const type = a.principal_type || a.type || "";
        const id = a.principal_id || a.id || a;
        const found = optionsFor(type).find((o) => o.id === id);
        return [
          esc(type || "—"),
          `<strong>${esc(found?.label || "—")}</strong> <span class="mono-id">${esc(String(id).slice(0, 8))}</span>`,
          `<button class="outline small" data-variant="danger" data-detach="${esc(name)}" data-ptype="${esc(type)}" data-pid="${esc(id)}">Detach</button>`,
        ];
      });
      setInspector(`Attachments · ${name}`, "Principals this policy is bound to",
        table(["Type", "Principal", ""], rows, { empty: "Not attached to any principal" }));
    } catch (e) { setInspector(`Attachments · ${name}`, "", authGuidance(e)); }
  }

  /* ── Roles (§8.2) ──────────────────────────────────────────── */

  async function loadRoles() {
    const el = P("ac-roles");
    el.innerHTML = loadingBox();
    try {
      const res = await loadDirectory();
      if (res.r.status === "rejected") throw res.r.reason;
      const rows = dir.roles.map((r) => {
        const available = dir.policies.filter((p) => !(r.policies || []).includes(p.name));
        const chips = (r.policies || []).map((p) => `<span class="chip">${esc(p)}<button data-detach="${esc(p)}" data-ptype="role" data-pid="${esc(r.id)}" title="Detach ${esc(p)}">${icon("x", 11)}</button></span>`).join("") || '<span class="text-light">none</span>';
        return [
          `<strong>${esc(r.name)}</strong><div class="mono-id">${esc(r.id || "")}</div>`,
          `<span class="truncate" title="${esc(r.description)}" style="max-width:26ch;display:inline-block">${esc(r.description || "—")}</span>`,
          `<div class="vstack" style="gap:var(--space-2)">
             <div class="hstack" style="gap:var(--space-1);flex-wrap:wrap">${chips}</div>
             ${available.length ? `<div class="hstack" style="gap:var(--space-2)">
               <select class="role-attach" data-role="${esc(r.name)}" data-rid="${esc(r.id)}" aria-label="Attach policy to ${esc(r.name)}" style="max-width:12rem">
                 <option value="">attach policy…</option>${available.map((p) => `<option value="${esc(p.name)}">${esc(p.name)}</option>`).join("")}
               </select>
               <button class="outline small" data-attach-role-policy="${esc(r.id)}">Attach</button>
             </div>` : ""}
           </div>`,
          `<span class="hstack" style="justify-content:flex-end">
            <button class="outline small" data-grants="${esc(r.name)}">Grants</button>
            <button class="outline small" data-grant-role="${esc(r.name)}">Grant</button>
            <button class="outline small" data-variant="danger" data-del-role="${esc(r.name)}">Delete</button>
          </span>`,
        ];
      });
      el.innerHTML = card("Roles", table(["Role", "Description", "Policies", ""], rows, { empty: "No roles defined" }),
        { subtitle: "A role is an assumable identity carrying policies",
          actions: `<button id="ac-add-role">${icon("plus")} New role</button>` });
    } catch (e) { el.innerHTML = card("Roles", authGuidance(e)); }
  }

  async function addRole() {
    const v = await formDialog({
      title: "Create role", wide: true,
      fields: [
        { name: "name", label: "Name", required: true, autocomplete: false },
        { name: "description", label: "Description" },
        { name: "trust_policy", label: "Trust policy (JSON)", type: "json", value: {}, hint: "Optional" },
        { name: "permissions_boundary", label: "Permissions boundary", type: "select", placeholder: "none", options: dir.policies.map((p) => ({ value: p.name, label: p.name })) },
      ],
      submitLabel: "Create role",
      validate: (x) => (!x.name ? "A name is required." : null),
    });
    if (!v) return;
    const body = { name: v.name, description: v.description || "" };
    if (v.trust_policy && Object.keys(v.trust_policy).length) body.trust_policy = v.trust_policy;
    if (v.permissions_boundary) body.permissions_boundary = v.permissions_boundary;
    try { await api.createRole(body); toastOk("Role created"); loadRoles(); }
    catch (e) { toastError(e, "Create role"); }
  }

  async function manageGrants(name) {
    setInspector(`Grants · ${name}`, "Users and groups granted this role", loadingBox());
    try {
      const list = await api.listRoleGrants(name);
      const rows = list.map((g) => [
        esc(g.principal_type || g.type || "—"),
        `<span class="mono-id">${esc(g.principal_id || g.id || g || "—")}</span>`,
        `<button class="outline small" data-variant="danger" data-revoke-grant="${esc(name)}" data-ptype="${esc(g.principal_type || g.type || "")}" data-pid="${esc(g.principal_id || g.id || g || "")}">Revoke</button>`,
      ]);
      setInspector(`Grants · ${name}`, "Users and groups granted this role",
        table(["Type", "Principal ID", ""], rows, { empty: "No grants" }));
    } catch (e) { setInspector(`Grants · ${name}`, "", authGuidance(e)); }
  }

  async function grantRoleDialog(name) {
    if (!dir.users.length && !dir.groups.length) await loadDirectory();
    const v = await formDialog({
      title: `Grant role · ${name}`,
      fields: [
        { name: "principal", label: "Principal", type: "select", placeholder: "select a user or group…", options: principalChoices(["user", "group"]), required: true },
      ],
      submitLabel: "Grant",
    });
    if (!v) return;
    const { principal_type, principal_id } = splitPrincipal(v.principal);
    try { await api.grantRole(name, principal_type, principal_id); toastOk("Role granted"); manageGrants(name); }
    catch (e) { toastError(e, "Grant role"); }
  }

  /* ── Groups (§8.3) ─────────────────────────────────────────── */

  async function loadGroups() {
    const el = P("ac-groups");
    el.innerHTML = loadingBox();
    try {
      const res = await loadDirectory();
      if (res.g.status === "rejected") throw res.g.reason;
      const rows = dir.groups.map((g) => [
        `<strong>${esc(g.name)}</strong><div class="mono-id">${esc(g.id || "")}</div>`,
        `<span class="truncate" title="${esc(g.description)}" style="max-width:34ch;display:inline-block">${esc(g.description || "—")}</span>`,
        `<span class="hstack" style="justify-content:flex-end">
          <button class="outline small" data-members="${esc(g.name)}">Members</button>
          <button class="outline small" data-variant="danger" data-del-group="${esc(g.name)}">Delete</button>
        </span>`,
      ]);
      el.innerHTML = card("Groups", table(["Group", "Description", ""], rows, { empty: "No groups defined" }),
        { subtitle: "Policies attached to a group apply to every member",
          actions: `<button id="ac-add-group">${icon("plus")} New group</button>` });
    } catch (e) { el.innerHTML = card("Groups", authGuidance(e)); }
  }

  async function addGroup() {
    const v = await formDialog({
      title: "Create group",
      fields: [{ name: "name", label: "Name", required: true, autocomplete: false }, { name: "description", label: "Description" }],
      submitLabel: "Create group",
      validate: (x) => (!x.name ? "A name is required." : null),
    });
    if (!v) return;
    try { await api.createGroup({ name: v.name, description: v.description || "" }); toastOk("Group created"); loadGroups(); }
    catch (e) { toastError(e, "Create group"); }
  }

  async function manageMembers(name) {
    setInspector(`Members · ${name}`, "Users belonging to this group", loadingBox());
    try {
      const members = await api.listGroupMembers(name);
      const ids = members.map((m) => (typeof m === "string" ? m : m.user_id || m.id));
      const rows = ids.map((id) => {
        const name = dir.users.find((u) => u.id === id)?.username || id;
        return [`<strong>${esc(name)}</strong><div class="mono-id">${esc(id)}</div>`, `<button class="outline small" data-variant="danger" data-rm-member="${esc(name)}" data-uid="${esc(id)}">Remove</button>`];
      });
      const available = optionsFor("user", { exclude: ids });
      setInspector(`Members · ${name}`, "Users belonging to this group", `
        ${table(["User", ""], rows, { empty: "No members" })}
        <div class="hstack" style="margin-top:var(--space-4)">
          <select id="ac-member-pick" style="max-width:16rem"><option value="">add member…</option>${available.map((o) => `<option value="${esc(o.id)}">${esc(o.label)}</option>`).join("")}</select>
          <button class="outline" id="ac-add-member">${icon("plus")} Add member</button>
        </div>`);
      inspector.querySelector("#ac-add-member").addEventListener("click", async () => {
        const user_id = inspector.querySelector("#ac-member-pick").value;
        if (!user_id) return;
        try { await api.addUserToGroup(name, user_id); toastOk("Member added"); manageMembers(name); }
        catch (e) { toastError(e, "Add member"); }
      });
    } catch (e) { setInspector(`Members · ${name}`, "", authGuidance(e)); }
  }

  /* ── Users (§8.4) ──────────────────────────────────────────── */

  async function loadUsers() {
    const el = P("ac-users");
    el.innerHTML = loadingBox();
    try {
      const res = await loadDirectory();
      if (res.u.status === "rejected") throw res.u.reason;
      const rows = dir.users.map((u) => {
        const roles = u.roles || [];
        return [
          `<strong>${esc(u.username || "—")}</strong><div class="mono-id">${esc(u.id || "")}</div>`,
          esc(u.email || "—"),
          statusBadge(u.status || "active"),
          `<span class="hstack" style="gap:var(--space-1);flex-wrap:wrap">${roles.length ? roles.map((r) => badge(r, "secondary")).join("") : badge("no role", "warning")}</span>`,
          esc(u.created_at ? fmtDateTime(u.created_at) : "—"),
          `<span class="hstack" style="justify-content:flex-end">
            <button class="outline small" data-user-roles="${esc(u.id)}" data-name="${esc(u.username || "")}" data-roles="${esc(roles.join(","))}">Roles</button>
            <button class="outline small" data-boundary="${esc(u.id)}" data-name="${esc(u.username || "")}">Boundary</button>
            <button class="outline small" data-variant="danger" data-del-user="${esc(u.id)}" data-username="${esc(u.username || "")}">Delete</button>
          </span>`,
        ];
      });
      el.innerHTML = card("Users", table(["User", "Email", "Status", "Roles", "Created", ""], rows, { empty: "No users provisioned" }),
        { subtitle: "Deny-by-default — a user with no policies or roles can do nothing" });
    } catch (e) { el.innerHTML = card("Users", authGuidance(e)); }
  }

  async function manageUserRoles(id, username, rolesCsv) {
    const roles = rolesCsv ? rolesCsv.split(",").filter(Boolean) : [];
    const available = dir.roles.filter((r) => !roles.includes(r.name));
    setInspector(`Roles · ${username || id}`, "Roles granted to this user", `
      ${table(["Role", ""], roles.map((r) => [`<span>${esc(r)}</span>`, `<button class="outline small" data-variant="danger" data-revoke-user-role="${esc(id)}" data-role="${esc(r)}">Revoke</button>`]), { empty: "No roles granted" })}
      <div class="hstack" style="margin-top:var(--space-4)">
        <select id="ac-user-role-pick" style="max-width:16rem"><option value="">select role…</option>${available.map((r) => `<option value="${esc(r.name)}">${esc(r.name)}</option>`).join("")}</select>
        <button class="outline" id="ac-grant-user-role">${icon("plus")} Grant role</button>
      </div>`);
    inspector.querySelector("#ac-grant-user-role").addEventListener("click", async () => {
      const role = inspector.querySelector("#ac-user-role-pick").value;
      if (!role) return;
      try { await api.grantUserRole(id, role); toastOk("Role granted"); loadUsers(); manageUserRoles(id, username, [...roles, role].join(",")); }
      catch (e) { toastError(e, "Grant role"); }
    });
  }

  async function boundaryDialog(id, username) {
    const v = await formDialog({
      title: `Permissions boundary · ${username || id}`,
      description: "Set a policy as the effective maximum. Leave empty to clear.",
      fields: [{ name: "policy", label: "Policy", type: "select", placeholder: "empty clears the boundary", options: dir.policies.map((p) => ({ value: p.name, label: p.name })) }],
      submitLabel: "Set boundary",
    });
    if (!v) return;
    try { await api.setPermissionsBoundary(id, v.policy || ""); toastOk("Boundary updated"); loadUsers(); }
    catch (e) { toastError(e, "Set boundary"); }
  }

  /* ── Wiring ────────────────────────────────────────────────── */

  const tabLoaders = [loadPolicies, loadRoles, loadGroups, loadUsers];
  const loaded = new Set();
  async function loadTab(i) { if (loaded.has(i)) return; loaded.add(i); await tabLoaders[i](); }

  ctx.root.addEventListener("click", async (e) => {
    const t = e.target;
    if (t.closest("#ac-err-settings")) return ctx.openSettings();

    const tab = t.closest('[role="tab"]');
    if (tab) {
      const idx = [...ctx.root.querySelectorAll('[role="tab"]')].indexOf(tab);
      if (idx >= 0) loadTab(idx);
      return;
    }

    if (t.closest("#ac-add-policy")) return addPolicy();
    if (t.closest("#ac-add-role")) return addRole();
    if (t.closest("#ac-add-group")) return addGroup();

    const ap = t.closest("[data-attach-role-policy]");
    if (ap) {
      const sel = ctx.root.querySelector(`select.role-attach[data-rid="${CSS.escape(ap.dataset.attachRolePolicy)}"]`);
      const policy = sel?.value;
      if (!policy) { toast("Pick a policy first", { variant: "warning" }); return; }
      try { await api.attachPolicy(policy, "role", ap.dataset.attachRolePolicy); toastOk("Policy attached"); loadRoles(); }
      catch (err) { toastError(err, "Attach policy"); }
      return;
    }

    const at = t.closest("[data-attach]");
    if (at) {
      const v = await attachDialog(at.dataset.attach, "attach");
      if (v) {
        const { principal_type, principal_id } = splitPrincipal(v.principal);
        try { await api.attachPolicy(at.dataset.attach, principal_type, principal_id); toastOk("Policy attached"); }
        catch (err) { toastError(err, "Attach policy"); }
      }
      return;
    }
    const ad = t.closest("[data-att-policy]"); if (ad) return manageAttachments(ad.dataset.attPolicy);
    const dt = t.closest("[data-detach]");
    if (dt) { try { await api.detachPolicy(dt.dataset.detach, dt.dataset.ptype, dt.dataset.pid); toastOk("Policy detached"); manageAttachments(dt.dataset.detach); if (dt.dataset.ptype === "role") loadRoles(); } catch (err) { toastError(err, "Detach policy"); } return; }
    const dp = t.closest("[data-del-policy]");
    if (dp) { const ok = await confirmDialog({ title: "Delete policy", body: `Delete policy "${dp.dataset.delPolicy}"?`, confirmLabel: "Delete", danger: true }); if (!ok) return; try { await api.deletePolicy(dp.dataset.delPolicy); toastOk("Policy deleted"); loadPolicies(); } catch (err) { toastError(err, "Delete policy"); } return; }

    const gr = t.closest("[data-grant-role]"); if (gr) return grantRoleDialog(gr.dataset.grantRole);
    const gs = t.closest("[data-grants]"); if (gs) return manageGrants(gs.dataset.grants);
    const rg = t.closest("[data-revoke-grant]");
    if (rg) { try { await api.revokeRole(rg.dataset.revokeGrant, rg.dataset.ptype, rg.dataset.pid); toastOk("Grant revoked"); manageGrants(rg.dataset.revokeGrant); } catch (err) { toastError(err, "Revoke grant"); } return; }
    const dr = t.closest("[data-del-role]");
    if (dr) { const ok = await confirmDialog({ title: "Delete role", body: `Delete role "${dr.dataset.delRole}"?`, confirmLabel: "Delete", danger: true }); if (!ok) return; try { await api.deleteRole(dr.dataset.delRole); toastOk("Role deleted"); loadRoles(); } catch (err) { toastError(err, "Delete role"); } return; }

    const mb = t.closest("[data-members]"); if (mb) return manageMembers(mb.dataset.members);
    const rm = t.closest("[data-rm-member]");
    if (rm) { try { await api.removeUserFromGroup(rm.dataset.rmMember, rm.dataset.uid); toastOk("Member removed"); manageMembers(rm.dataset.rmMember); } catch (err) { toastError(err, "Remove member"); } return; }
    const dg = t.closest("[data-del-group]");
    if (dg) { const ok = await confirmDialog({ title: "Delete group", body: `Delete group "${dg.dataset.delGroup}"?`, confirmLabel: "Delete", danger: true }); if (!ok) return; try { await api.deleteGroup(dg.dataset.delGroup); toastOk("Group deleted"); loadGroups(); } catch (err) { toastError(err, "Delete group"); } return; }

    const ur = t.closest("[data-user-roles]"); if (ur) return manageUserRoles(ur.dataset.userRoles, ur.dataset.name, ur.dataset.roles);
    const rur = t.closest("[data-revoke-user-role]");
    if (rur) { try { await api.revokeUserRole(rur.dataset.revokeUserRole, rur.dataset.role); toastOk("Role revoked"); loadUsers(); } catch (err) { toastError(err, "Revoke role"); } return; }
    const bd = t.closest("[data-boundary]"); if (bd) return boundaryDialog(bd.dataset.boundary, bd.dataset.name);

    const du = t.closest("[data-del-user]");
    if (du) {
      const id = du.dataset.delUser;
      const name = du.dataset.username || id;
      const ok = await confirmDialog({
        title: "Delete user",
        body: `Delete "${name}" permanently, along with their API keys, role grants and group memberships? Their API keys stop working immediately. This cannot be undone.`,
        confirmLabel: "Delete user",
        danger: true,
      });
      if (!ok) return;
      try {
        await api.deleteUser(id);
        toastOk("User deleted");
        inspector.innerHTML = "";
        loadUsers();
      } catch (err) {
        toastError(err, "Delete user");
      }
      return;
    }
  });

  function activeIndex() {
    const sel = ctx.root.querySelector('[role="tab"][aria-selected="true"]');
    const idx = sel ? [...ctx.root.querySelectorAll('[role="tab"]')].indexOf(sel) : 0;
    return idx >= 0 ? idx : 0;
  }

  ctx.root.querySelector("#ac-refresh").addEventListener("click", () => { loaded.clear(); loadTab(activeIndex()); });

  loadTab(0);
}
