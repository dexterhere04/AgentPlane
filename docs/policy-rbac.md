# Policy Layer & RBAC

**Location:** `internal/policy/`, `internal/policy/rbac/`, `internal/handlers/roles.go`, `migrations/000003_create_rbac_tables.*`

## 1. Overview

AgentPlane has two distinct kinds of policy:

| Layer | Package | Question answered | Evaluated on |
|-------|---------|-------------------|--------------|
| **Guardrails** | `internal/guardrail/` | *Is this content safe/allowed?* | Request/response **body** |
| **Policy (authorization)** | `internal/policy/` | *Is this principal allowed this action?* | **Identity** + resource |

This document covers the authorization layer. RBAC (role-based access control) is its first — and currently only — implementation.

Authorization is **deny-by-default**: a user with no assigned role is permitted nothing. A newly provisioned user cannot call the gateway until an operator explicitly grants them a role.

## 2. Concepts

### Principal

The authenticated `users.User` resolved by API-key authentication (`internal/auth`). Roles attach to **users**, so every API key belonging to a user shares that user's permissions.

### Permission

An opaque string of the form `<resource>:<action>`:

| Permission | Meaning |
|------------|---------|
| `chat:invoke` | Call `POST /chat` |
| `model:gpt-4o` | Use the `gpt-4o` model |
| `model:*` | Use any model (prefix wildcard) |
| `admin:*` | Reserved for future admin-role enforcement |
| `*` | Superuser — matches everything |

### Matching

`policy.Match(granted, requested)`:

- `*` matches everything.
- An exact string matches only itself.
- A trailing `:*` is a **prefix wildcard**: `model:*` matches `model:gpt-4o`, `model:gpt-4o-mini`, etc.

### Role

A named set of permissions (`roles` + `role_permissions`). Users are granted one or more roles via `user_roles`.

## 3. Architecture

```
internal/policy/                      generic authorization engine
  policy.go        Decision{Allow,Deny}, Request{UserID, Permission},
                   Policy interface, Match / MatchAny
  enforcement.go   EnforcementPoint — runs a chain of policies,
                   first-deny-wins, fail-closed on error
  middleware.go    Require(permission) http middleware + WriteDenied/WriteUnavailable

internal/policy/rbac/                 first Policy implementation
  engine.go        Engine (policy.Policy): loads a user's permissions and matches
  store.go         Store: LoadPermissions, role/permission CRUD, user-role assignment

internal/handlers/roles.go            admin HTTP surface for role management
```

`EnforcementPoint` is intentionally generic. RBAC is one `Policy`; rate limits, quotas, or attribute-based rules can be added as additional policies without changing callers.

## 4. Database schema

Migration `000003_create_rbac_tables`:

```sql
roles            (id UUID PK, name TEXT UNIQUE, description TEXT, created_at)
role_permissions (role_id FK→roles ON DELETE CASCADE, permission TEXT, PK(role_id, permission))
user_roles       (user_id FK→users ON DELETE CASCADE, role_id FK→roles ON DELETE CASCADE,
                  PK(user_id, role_id))
```

The migration seeds two roles but **assigns them to nobody**:

| Role | Permissions | Purpose |
|------|-------------|---------|
| `admin` | `*` | Full access |
| `member` | `chat:invoke`, `model:*` | Standard gateway access |

Because nothing is auto-assigned, existing and new users both start with no access. Grant `member` (or a custom role) explicitly.

## 5. Enforcement points

Authorization runs at two points, both through the same `EnforcementPoint`:

| Check | Where | Why there |
|-------|-------|-----------|
| `chat:invoke` | Middleware on `/chat` (`cmd/server/main.go`) | Static — needs no request body, so it rejects before the body is read |
| `model:<name>` | `handlers.Chat` after body validation (`internal/handlers/chat.go`) | Dynamic — the model name comes from the request body |

Middleware chain for `/chat`:

```
auth.Authenticator.Middleware        # resolves user, 401 on failure
  └── policy.EnforcementPoint.Require("chat:invoke")   # 403 / 503
        └── handlers.Chat            # then authorizes "model:<name>"
              └── guardrails (input) → provider → guardrails (output)
```

If a request has no `model` field, the model check is skipped — `chat:invoke` already gates access and the upstream provider will reject a malformed request.

### Failure semantics

| Condition | Response |
|-----------|----------|
| No authenticated user | `401 Unauthorized` |
| Explicit deny | `403 Forbidden` (`policy_denied`) |
| Policy could not be evaluated (e.g. database error) | `503 Service Unavailable` (`policy_unavailable`) |

A policy error fails **closed** — an authorization outage denies access rather than silently allowing it — and is surfaced as `503` so it is never mistaken for a policy violation.

## 6. Admin API

All role-management endpoints require the admin token (`Authorization: Bearer <AGENTPLANE_ADMIN_TOKEN>`).

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/admin/roles` | List roles with their permissions |
| `POST` | `/admin/roles` | Create a role (optionally with initial permissions) |
| `POST` | `/admin/roles/{name}/permissions` | Add a permission to a role |
| `DELETE` | `/admin/roles/{name}/permissions/{permission}` | Remove a permission from a role |
| `GET` | `/admin/users/{id}/roles` | List a user's roles |
| `POST` | `/admin/users/{id}/roles` | Assign a role to a user |
| `DELETE` | `/admin/users/{id}/roles/{role}` | Remove a role from a user |

### Example: grant a user access

```bash
# 1. Create a role limited to one model
curl -X POST http://localhost:3001/admin/roles \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"gpt4o-only","description":"gpt-4o access","permissions":["chat:invoke","model:gpt-4o"]}'

# 2. Assign it to a user
curl -X POST http://localhost:3001/admin/users/8b0f.../roles \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role":"gpt4o-only"}'

# 3. Inspect
curl http://localhost:3001/admin/roles \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN"
```

### Control-plane UI

The web dashboard exposes RBAC under the **Access Control** section (sidebar index 08), with two tabs:

- **Roles** — list roles and their permissions; create a role; add/remove individual permissions.
- **Users** — list every user with their roles (`GET /admin/users`); assign or revoke a role per user.

The **Keys & Vault** section's provisioning form also includes an optional **role** selector. Because role assignment is a separate operation from provisioning, the UI performs it as a second step: it provisions the user + key, then calls `POST /admin/users/{id}/roles` with the selected role. If assignment fails, the user and key still exist and the UI surfaces the error so the role can be assigned later from the Access Control → Users tab.

## 7. Adding a role to an existing install

After deploying, existing users have no roles and are denied. Grant the built-in `member` role to restore standard access:

```bash
curl -X POST http://localhost:3001/admin/users/<user_id>/roles \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role":"member"}'
```

## 8. Extending the policy layer

To add a new policy:

1. Implement `policy.Policy` (`Name()` + `Evaluate(ctx, policy.Request) (policy.Decision, error)`).
2. Register it in `cmd/server/main.go`:
   ```go
   policyEP := policy.NewEnforcementPoint(rbac.New(rbacStore), myNewPolicy)
   ```
3. Reuse `policyEP.Require("...")` for static checks or `policyEP.Authorize(...)` for dynamic ones.

Policies are evaluated in order; the first deny wins. A returned error fails closed.

## 9. Files delivered

| File | Purpose |
|------|---------|
| `migrations/000003_create_rbac_tables.up.sql` / `.down.sql` | Roles, permissions, assignments; seeded `admin`/`member` |
| `internal/policy/policy.go` | Core types, `Policy` interface, permission matcher |
| `internal/policy/enforcement.go` | `EnforcementPoint` — chain evaluation, fail-closed |
| `internal/policy/middleware.go` | `Require(permission)`, `WriteDenied`, `WriteUnavailable` |
| `internal/policy/rbac/engine.go` | RBAC `Policy` implementation |
| `internal/policy/rbac/store.go` | Persistence for roles, permissions, assignments |
| `internal/handlers/roles.go` | Admin role-management HTTP handlers |
| `internal/handlers/chat.go` | Per-model authorization after body validation |
| `internal/auth/middleware.go` | `ContextWithUser` (counterpart to `UserFromContext`) |
| `cmd/server/main.go` | Wiring: RBAC store → engine → enforcement point; routes |

## 10. Design decisions

### Why a separate `policy` package instead of extending `guardrail`

Guardrails inspect content; authorization inspects identity and resource. Keeping them separate preserves the single-responsibility split and lets each evolve independently. They share a structural pattern (`Registry`/chain + `Decision` + fail-closed) so the codebase stays consistent without coupling the two.

### Why user-level roles (not per-key scopes)

The authenticated principal is already a user, and `users.User` is what authentication resolves. User-level roles require no change to the API-key schema and cover the common case. Per-key scopes (a user-level baseline with per-key restrictions) remain a future extension.

### Why deny-by-default

Access control should require an explicit grant. A missing role assignment is treated as "no permissions", never as an implicit allow. This is the safe failure mode and makes the effective permission set auditable.

### Why model access is a permission, not a separate table

Expressing model access as `model:<name>` / `model:*` permissions keeps a single, uniform authorization model — the same matcher handles endpoints and resources — instead of introducing a parallel model-allowlist mechanism.

## 11. Future work

- Enforce `admin:*` on the admin endpoints (replacing the static admin token).
- Per-API-key scopes on top of user-level roles.
- Additional `Policy` implementations (rate limiting, quotas, attribute-based rules) behind the same `EnforcementPoint`.
- Permission caching to avoid a database round-trip per request.
