# Frontend parity: `/web` (React) → `/web1` (Oat)

> **Purpose.** Catalogue every functional and UI feature present in the **old React
> control plane** (`web/`) that is **not yet present** in the **new Oat control plane**
> (`web1/`), so the gap is explicit and can be scheduled deliberately.
>
> **Scope.** Feature/behaviour parity only. It says nothing about which codebase is
> "better" — see *Where web1 is ahead* at the end for the reverse direction.
>
> **Method.** Full read of `web/src` (all 18 components, `api.ts`, `stream.ts`,
> `queue.ts`, `setup.ts`, `styles.css`) compared against a verified read of
> `web1/src`. Claims about web1 were confirmed by grepping the source, not assumed.
>
> **Priority legend.** **P1** = meaningful capability gap · **P2** = useful · **P3** = polish.

> **Status (implemented in `web1`).** The gaps below have since been closed in the
> Oat control plane — most of the P1 set and a large part of P2/P3:
>
> - **P1** — auto-refresh/polling (Overview, Analytics, Routing); guardrail
>   findings + before/after viewer; a new **Usage** section (verify-key +
>   per-key attribution); Access Control entity pickers; Playground
>   Enter-to-send; Pipeline parallel fire + concurrency Gantt.
> - **P2** — stepped/realtime event playback with queue depth; Overview
>   operational-health banner, system chips and session summary; policy-document
>   viewer; role selector at provisioning; Event-Log request-id click-to-filter;
>   bar transitions, reduced-motion and pulsing live indicators.
> - **P3** — expanded icon set, custom scrollbars, section reveal, typing dots,
>   provider-traffic bars on Overview, circuit-breaker threshold presets, Vault
>   mount-path hint, per-card refresh buttons, and a latency sparkline.
>
> Sections §2–§14 of this document remain the reference description of the old
> `web/` behaviour.

---

## 1. Summary

| Area | Gaps | Highest priority |
|---|---|---|
| Missing view (**Usage**) | 3 | P1 |
| Shell / cross-cutting | 6 | **P1 — no auto-refresh** |
| Overview | 8 | P2 |
| Playground | 5 | P2 |
| Pipeline | 6 | P1 — parallel fire, Gantt |
| Guardrails | 4 | **P1 — findings + before/after** |
| Routing | 7 | P2 |
| Event Log | 4 | P3 |
| Analytics | 1 | P2 |
| Keys & Vault | 2 | P2 |
| Access Control | 6 | **P1 — entity pickers** |
| UI primitives / visual system | 9 | P2 |
| Responsive & motion | 5 | P3 |

Two structural gaps account for most of the difference: **there is no auto-refresh
anywhere in web1** (the old app polls between 3 s and 15 s), and **web1's IAM screens
ask operators to type raw UUIDs** where the old app offers entity pickers.

---

## 2. Missing view

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Usage section** (entire nav entry) | `App.tsx:35` (`usage`), `UsageView.tsx` | web1 folds *some* of this into Analytics → Users/Prompts, but the dedicated per-key view does not exist. | P1 |
| **Verify a user key** — paste a key, "Verify & test" fires a ping and shows HTTP status + reply, proving identity resolution | `UsageView.tsx:20-57` | web1 has no way to check what identity a key resolves to. | P1 |
| **Requests attributed to keys** — from the stream: short id, username + `user_id`, prompt, time, pass/block/warn | `UsageView.tsx:33, 60-87` | web1's stream model doesn't capture identity/prompt at all (see §3). | P1 |

---

## 3. Shell / cross-cutting

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Polling / auto-refresh** — health 8 s, `/metrics` 3 s, routing counters 5 s, routing analytics 15 s, observability 5 s, upstreams 5 s | `Overview.tsx:97-166`, `RoutingView.tsx:126-153`, `RoutingAdminView.tsx:269-277`, `ObservabilityView.tsx:54-58` | **web1 has zero `setInterval`** — every dashboard is manual-refresh only. Biggest single behavioural gap. | **P1** |
| **Stepped event playback** — FIFO queue draining one SSE event per 45 ms so the pipeline visibly flows; realtime mode drains instantly; reports queue depth | `queue.ts:12,21-75` | web1's Pipeline has a *Freeze* toggle only (pauses rendering); there is no stepped drain or paced replay. | P1 |
| **Dismissible setup guide** — `dismissed` flag persisted | `setup.ts:1-15`, `SetupChecklist.tsx:81-84, 125-127` | web1 has setup flags + reset, but cannot dismiss the card. | P2 |
| **Sidebar collapse with persistence** (`agentplane.sidebar`) | `App.tsx:58-60, 73-78` | web1 uses Oat's sidebar: toggle is mobile-only, no desktop collapse, no persistence. | P2 |
| **Stream-derived request detail** — the reducer parses and stores per-request: **guardrail findings** (severity/type/entity/value), **before/after payloads**, **prompt text** (last user message), **response content**, **blocked message**, **authenticated identity** (`user_id`/`username`) | `stream.ts:11-24, 57-63, 70-100` | web1's `deriveRequests` captures stages/durations/status/model/provider/user only. This is what makes the Guardrails and Usage views possible. | P1 |
| **Dark-by-default theme** | `web/src/main.tsx:6` | web1 defaults to light. Cosmetic. | P3 |

*Not a gap:* credential storage keys, SSE dedupe-by-id with a 5000 cap, settings panel
(admin token / user key / reset guide), session-scoped flow state — web1 has equivalents.

---

## 4. Overview

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Operational health banner** — degraded / operational / scanning, from gateway SSE + provider + secret + ClickHouse probes | `Overview.tsx:183-194, 236-276` | P2 |
| **System status chips** — Gateway (SSE), Provider, **Secret store**, **ClickHouse reachability** | `Overview.tsx:264-275` | P2 |
| **Quick-access service grid** — 9 navigation cards | `Overview.tsx:48-77, 284-286` | P2 |
| **Guardrail telemetry bars** — passed / blocked / redacted / warned / errors + avg eval ms | `Overview.tsx:40-46, 289-314` | P2 |
| **Session summary** — completed / blocked-failed / in-flight / events with percentage tracks | `Overview.tsx:208-213, 316-342` | P2 |
| **KPI set** — success rate, in-flight, blocked, events count, tokens, avg latency, guardrail evaluations | `Overview.tsx:196-205, 278-282` | P1–P2 |
| **Provider traffic bars + failover/error stats** on Overview | `Overview.tsx:392-449` | P3 |
| **Per-tile icons** | `Overview.tsx` via `Metric` (`ui.tsx:4-19`) | P3 |

*Already in web1:* setup checklist, live/offline indicator, and the full routing counters
card (decisions / failovers / retries / breaker trips + recoveries / no-provider / denied)
plus the per-upstream table.

---

## 5. Playground

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Conversation transcript** — messages accumulate in a thread with avatars, not a single request/response | `ChatPanel.tsx:111-158` | web1 shows one request and one response. | P2 |
| **Enter to send / Shift+Enter newline** | `ChatPanel.tsx:168-173` | web1 only sends on **Ctrl/⌘+Enter** (verified `views/playground.js:213`). | P1 |
| **Inline API-key field** in the composer | `ChatPanel.tsx:87-94` | web1 requires opening Settings first. | P2 |
| **Animated typing indicator** (three bouncing dots) | `ChatPanel.tsx:145-158` | web1 uses a static "Sending…" button state. | P3 |
| **Masked-key footer** — `ap_live_…<last8>` / "no key set" | `ChatPanel.tsx:180-186` | web1 shows nothing. | P3 |

*Vice-versa:* web1's model field accepts free text with 6 suggestions vs the old
3-option dropdown.

---

## 6. Pipeline

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Fire N in parallel** — 1–50 concurrent `POST /chat` calls via `Promise.allSettled`, to exercise routing/failover | `PipelineView.tsx:130-140, 222-233` | web1 has no way to generate load. | **P1** |
| **Concurrency Gantt lanes** — up to 24 requests drawn as bars over a time axis with gridlines and a legend | `PipelineView.tsx:120-128, 242-272` | web1 has no concurrency visualisation. | P1 |
| **Avg / min / max latency** KPIs, and a **blocked** count | `PipelineView.tsx:144-151` | web1 shows requests / in-flight / errors / buffered. | P2 |
| **Flow progress bar** — N of 13 stages complete + total ms | `PipelineView.tsx:176-186` | web1 shows numbered nodes only. | P2 |
| **Dedicated blocked alert** with the block message | `PipelineView.tsx:189-193` | — | P2 |
| **13-stage canonical pipeline** | `PipelineView.tsx:18-32` | web1 models 10 stages — narrower stage coverage. | P2 |

*Vice-versa:* web1's clickable request list + per-request event timeline is arguably
better than the old `<select>` request picker.

---

## 7. Guardrails

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Guardrail findings list** — severity / type / entity / quoted value chips per decision | `GuardrailsView.tsx:27-55` | web1 renders decisions as text only; **no findings at all**. | **P1** |
| **Before → After diff viewer** — pre blocks per side; blocked side shows "request blocked — not forwarded" | `GuardrailsView.tsx:57-82` | web1 has no payload inspection. | **P1** |
| **Input vs Output check columns** with per-column counts and empty states | `GuardrailsView.tsx:223-248` | web1 shows one flat live list. | P2 |
| **Enforced-guardrail summary chips** + per-request metadata bar (id, username, model, time, duration, status) | `GuardrailsView.tsx:197-221` | web1 shows the result of its own probe only. | P2 |

*Already in web1:* payload textarea, Ctrl/⌘+Enter to run, preset payloads (web1 has 6
vs the old 5), live decision feed.

---

## 8. Routing

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Time-range selector on the routing analytics** (1h / 6h / 24h / 7d / 30d) with re-query | `RoutingView.tsx:177-188` | P2 |
| **Live routing feed** — `routing_decision` / `routing_failover` events with model, route, provider, attempt, details | `RoutingView.tsx:61-73, 155-166, 545-622` | P2 |
| **Provider consumption table** — tokens and cost joined per provider | `RoutingView.tsx:371-406` | P3 |
| **Routing activity (session)** counters | `RoutingView.tsx:408-429` | P3 |
| **Admin token validator card** — persistent `NOT VALIDATED / CHECKING / VALID / INVALID` badge | `RoutingAdminView.tsx:188-230, 474-511` | P2 |
| **`requireValidatedAdmin()` gate** — every mutation is blocked until the token is validated | `RoutingAdminView.tsx:232-242` | P2 |
| **Circuit-breaker threshold presets** (Strict 3 / Default 5 / Lenient 10) | `RoutingAdminView.tsx:44-48` | P3 |

*Already in web1:* provider + rule tables, breaker badge with threshold, enable/disable,
add/edit/delete, matcher JSON editors with live validation, upstream status table.
web1 uses modal dialogs where the old app used inline forms (web1 also disables rather
than deletes built-ins), and web1's rule matchers are summarised more crudely than the
old `prettyJSON` matcher printer (`RoutingAdminView.tsx:74-147`).

---

## 9. Event Log

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Click the request id to filter by that request** | `EventLog.tsx:81-124` | P2 |
| **Stage-family colour coding** (6 families: guardrail / error / done / received / read / proxy) | `EventLog.tsx:6-13` | P3 |
| **Request filter dropdown** (all / by short id) | `EventLog.tsx:68-73` | P3 |
| **Clear-search (×) button** | `EventLog.tsx:54-67` | P3 |

*Already in web1:* free-text filter, status + stage selects, pause/resume, auto-scroll,
count badge, two empty-state variants.

---

## 10. Analytics

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Auto-refresh every 5 s** | `ObservabilityView.tsx:54-58` | P2 |
| **Click a user row to filter the prompt log**, with a removable filter chip and **300 ms debounced search** | `UserAnalytics.tsx:27-30, 110-141` | P2 |

*Vice-versa:* web1's Analytics is materially broader — 7 tabs, 5 ranges, all 9 routing
analytics queries, latency percentile bars, donut, grouped bars. Parity is otherwise
complete.

---

## 11. Keys & Vault

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Optional role selector at provisioning time** (+ the follow-up `POST /admin/users/{id}/roles`, with a partial-failure message) | `ApiKeysView.tsx:30-34, 48-61, 93-109` | P2 |
| **Vault KV-v2 mount-path hint** (`agentplane/`) | `VaultView.tsx:75-78` | P3 |

*Already in web1:* provider dropdown (13 slugs), secret storage with the returned secret
name (web1 better), provision + one-time key reveal, copy-to-clipboard, per-row revoke,
key inventory. web1 additionally has **delete-user**, which the old app never had.

---

## 12. Access Control

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Policy document viewer** — expandable `<details>` rendering the pretty-printed JSON with its version | `AccessControlView.tsx:152-195` | web1 shows a statement count; the document itself is never displayed. | **P1** |
| **Entity pickers everywhere** — attach-policy principal dropdown populated per type; grant-role role **and user** dropdowns; group add-member dropdown (filters existing members); user assign-role dropdown; **permission-boundary policy dropdown** (fires on change) | `AccessControlView.tsx:267-276, 350-397, 624-681, 728-841, 961-1043` | web1 asks the operator to **type a raw UUID / role name** (verified `views/access.js:118-119`). Highest-friction gap in the app. | **P1** |
| **Role → policy attach picker** (per-row dropdown + Attach) | `AccessControlView.tsx:476-577` | web1 can only attach policy → principal, not role → policy from the role row. | P2 |
| **Actionable 401 guidance** — "open Settings ⚙ and paste the admin token" | `AccessControlView.tsx:79-87` | web1 shows a generic error box. | P2 |
| **"no role" warning badge** on users without roles | `AccessControlView.tsx:988` | web1 shows the word "none". | P3 |
| **Per-card refresh buttons** | `AccessControlView.tsx:68-75` | web1 has one view-level refresh. | P3 |

*Vice-versa:* web1 additionally has **delete user** (new endpoint + UI), an inspector
panel for attachments/grants/members, and live JSON validation in the policy editor.

---

## 13. Shared UI primitives & visual system

| Feature | Where in `/web` | Notes | Pri |
|---|---|---|---|
| **Pulsing live indicators** — `pulse-dot` on status dots, in-progress badges, active stage pills, lane dots, scanning health | `styles.css:217` + many uses | web1 pulses only the active pipeline node. | P2 |
| **Bar-fill transitions** — `width 0.5s` easing on every bar/track | `styles.css` (`.bar-fill`, `.t-fill`, `.ov-sess-fill`, `.fp-fill`) | web1's `.barlist-fill` has **no transition** — charts snap. | P2 |
| **Section reveal animation** — `reveal` on every section/setup/health panel | `styles.css:1163` | web1 has no enter animation on section change. | P3 |
| **Typing-dot animation** (`translateY` + staggered delays) | `styles.css:759` | Not in web1. | P3 |
| **Explicit `prefers-reduced-motion` block** covering all animation/transition | `styles.css:135-143` | web1 has a narrow rule for one node only. | P2 |
| **Custom scrollbar styling** (incl. a distinct sidebar thumb) | `styles.css:1157-1161` | web1 uses browser defaults. | P3 |
| **Icon coverage** — 35 named icons; absent from web1: `message`, `layers`, `arrow`, `warning`, `minus`, `info`, `help`, `eye`, `server`, `globe`, `dollar`, `check` | `icons.tsx:3-38` | web1 ships 19 named icons (`ui.js` `ICONS`) plus a few inline (menu/sun/moon/settings). `external` exists in web1 but is unused. | P3 |
| **Branded typography** — Inter + JetBrains Mono via Google Fonts | `index.html:7-12`, `styles.css:10-12` | web1 intentionally uses system fonts (offline-safe, zero-dependency). | P3 — deliberate |
| **Metric tiles with icons + tone-coloured icon** | `ui.tsx:4-19` | web1's stat tiles are text-only. | P3 |

---

## 14. Responsive & motion

| Feature | Where in `/web` | Pri |
|---|---|---|
| **Event table collapses to 3 columns** on narrow screens (hides status / request / duration) | `styles.css:1185-1186` | P2 |
| **Sidebar overlay with scrim** (`box-shadow: 0 0 0 100vmax` technique) | `styles.css:1178-1192` | P3 |
| Breakpoints at 1080 / 900 / 860 / 700 / 640 / 560 with specific reflows | `styles.css:1169-1216` | P3 |
| **`.beforeafter` stacks vertically with a rotated arrow** on mobile | `styles.css:1193-1198` | P3 (only relevant with §7) |
| Chat/overlay width clamping to viewport | `styles.css:1193-1198` | P3 |

web1 relies on Oat's sidebar layout (768 px breakpoint) plus three small rules
(crumb, brand, status) — adequate, but less tuned.

---

## 15. Deliberate differences — do **not** "fix" these

These look like gaps but are intentional in web1; changing them would undo a decision.

| web1 choice | Rationale |
|---|---|
| System font stack instead of Inter / JetBrains Mono | Zero external dependencies; works offline in the container. |
| Oat component library instead of hand-rolled CSS/divs | The brief: use Oat for ~90% of the UI. |
| Modal dialogs + toasts instead of inline forms + `window.confirm` | Better UX; web1 is ahead here. |
| `30d` removed from range selectors | The gateway only accepts `15m/1h/6h/24h/7d` (`observability.ParseAnalyticsRange`); the old UI offers an unsupported range. |
| Hash routing + deep-linkable tabs | The old app has no URL state at all. |
| Vendored Oat 0.8.3 rather than npm | No build step, no Node dependency. |

---

## 16. Where web1 is ahead (for balance)

- **Usage of Oat**: semantic, accessible components — real `<dialog>` modals, toasts, skeletons, tooltips, grid, tag inputs — where the old app had none.
- **Delete user** — new `DELETE /admin/users/{id}` endpoint + UI; the old app could never remove a user.
- **Analytics breadth** — 7 tabs, 5 ranges, all 9 routing queries, SVG latency/donut/bar charts.
- **Live telemetry safety** — SSE dedupe with correct eviction (the old app clears its whole seen-set at 5000, risking replays).
- **Error handling** — HTTP error contract mapped per status/type, HTML proxy errors sanitised.
- **Deployment** — containerised, health-checked, per-request DNS resolution.
- **Dead code**: `charts.js:104 sparkline()` is exported but unused — either use it for §4/§6 trends or delete it.

---

## 17. Suggested backlog

**P1 — closes real capability gaps**
1. Auto-refresh / polling for Overview, Analytics and Routing (§3).
2. Guardrails: findings list + before/after viewer (§7) — needs stream parsing of guardrail detail (§3).
3. Usage section: verify-key + per-key attribution (§2).
4. Access Control: entity pickers replacing raw UUID entry (§12).
5. Playground: Enter-to-send (§5).
6. Pipeline: parallel fire + concurrency Gantt (§6).

**P2 — valuable, mostly UI**
7. Stepped event playback (§3). · 8. Overview health banner + system chips + session summary (§4). · 9. Policy document viewer (§12). · 10. Role selector at provisioning (§11). · 11. Event-log request-id click-to-filter (§9). · 12. Bar transitions + reduced-motion + pulsing indicators (§13–14).

**P3 — polish**
13. Icons, scrollbar styling, section reveal, typing dots, provider-traffic bars on Overview, threshold presets, mount-path hint, per-card refresh buttons.

---

## Appendix — verification notes

- web1 claims were confirmed against `web1/src`: 9 sections (`main.js:19`), **no
  `setInterval` anywhere**, `sparkline` exported but unimported (`charts.js:104`),
  Playground sends only on Ctrl/⌘+Enter (`views/playground.js:213`), Access Control
  takes a typed `principal_id` (`views/access.js:118-119`), no parallel-fire code.
- The old frontend was read in full; file:line references above point at the source of
  each behaviour.
- Known cosmetic defects in the old app (not worth porting): `.live-dot` is referenced
  in `EventLog.tsx:75` and `ChatOverlay.tsx:79` but has no CSS rule; `.is-complete`,
  `.brand-text`, `.dot-status`, `.ds-dot` are unused; `main.tsx` defaults the theme to
  `dark` while `App.tsx` initialises it to `light`.
