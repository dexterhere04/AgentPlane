# IAM (Identity & Access Management)

**Location:** `internal/iam/`, `internal/handlers/iam.go`, `internal/policy/` (engine spine), `migrations/000009_*`, `migrations/000010_*`

## 1. Overview

AgentPlane authorizes requests with an AWS-style IAM layer. Three kinds of policy
exist, deliberately separated:

| Layer | Package | Question | Evaluated on |
|-------|---------|----------|--------------|
| **Guardrails** | `internal/guardrail/` | *Is this content safe/allowed?* | request/response **body** |
| **Authorization (IAM)** | `internal/iam/` | *Is this principal allowed this action on this resource?* | **identity** + action + resource |
| **Enforcement spine** | `internal/policy/` | *Run the policy chain and fail closed* | — |

`internal/policy` is the generic spine (`Policy` interface, `EnforcementPoint`,
`Require` middleware). `internal/iam.Engine` implements `policy.Policy`, so it
drops into the existing middleware without changing callers.

This layer replaces the earlier RBAC implementation (`roles` + flat
`<resource>:<action>` permission strings). The key upgrade is **JSON policy
documents with an explicit `Deny` that overrides any `Allow`**, plus resource
scoping, groups, roles, and permission boundaries.

## 2. Concepts

### Actions
An action is a verb with the `ap:` service prefix, mirroring AWS's
`service:ActionName` (e.g. `s3:GetObject`). Wildcards `*` and `?` are legal
anywhere.

| Action | Meaning |
|--------|---------|
| `ap:InvokeChat` | Call `POST /chat` |
| `ap:InvokeModel` | Use a model (resource = the model ARN) |
| `ap:ListProviders`, `ap:CreateProvider`, … | Provider administration |
| `ap:ListRoutingRules`, `ap:CreateRoutingRule`, … | Routing administration |
| `ap:ProvisionUser`, `ap:RevokeApiKey`, `ap:ListApiKeys`, `ap:DeleteUser` | User/API-key administration |
| `ap:ManageSecrets`, `ap:ViewMetrics` | Secrets and observability |
| `ap:ListPolicies`, `ap:CreatePolicy`, `ap:AttachPolicy`, `ap:ListGroups`, `ap:ListRoles`, `ap:GrantRole`, `ap:SetPermissionsBoundary`, … | IAM administration |
| `ap:AssumeRole` | Reserved (v2) |

The canonical list is `iam.KnownActions`. Actions are validated at policy-write
time, so a typo cannot silently grant nothing (or, with wildcards, too much).

### Resources (ARN)
Resources use a simplified ARN, `arn:ap:{type}/{id}`:

```
arn:ap:model/gpt-4o      arn:ap:provider/*      arn:ap:*      *
```

Wildcards `*` (any run, including `/` and `:`) and `?` (one character) are legal
anywhere. `arn:ap:model/*` matches every model.

### Policy document
A policy is a JSON document, stored as `JSONB` and validated before storage:

```json
{
  "Version": "2026-01-01",
  "Statement": [
    { "Sid": "AllowInvoke", "Effect": "Allow",
      "Action": ["ap:InvokeChat", "ap:InvokeModel"],
      "Resource": ["arn:ap:*"] },
    { "Sid": "DenyExpensive", "Effect": "Deny",
      "Action": "ap:InvokeModel",
      "Resource": "arn:ap:model/gpt-4o" }
  ]
}
```

Statement fields in v1: `Sid`, `Effect` (`Allow`/`Deny`), `Action`,
`NotAction`, `Resource`, `NotResource`. `Action`/`Resource` accept a string or
an array; `Statement` accepts an object or an array. `Condition` is modelled but
**rejected at write time in v1** — an un-evaluatable `Deny` must never be stored,
because the evaluator would treat it as "not applicable" and silently drop it.
Conditions are the first v2 feature.

### Identities
- **User** — the `users` table; API keys resolve to a user.
- **Group** — a collection of users.
- **Role** — an assumable identity carrying policies, an optional assume-role
  trust policy (stored/validated now; session issuance is v2), and an optional
  permission boundary.

A policy attaches to any of user, group, or role. A **role grant** gives a user
or group the role's policies. Effective permissions of a principal =
direct policy attachments ∪ group policies ∪ granted-role policies, all capped
by the principal's permission boundary.

### Permission boundary
A per-identity ceiling: it can only restrict, never grant. Even a policy
granting `ap:*` cannot exceed the boundary.

## 3. Evaluation algorithm

Given `(principal, action, resource)`:

```
1. If any applicable statement is an explicit Deny          → DENY
   (in the identity policies OR the permission boundary)
2. If a permission boundary is set and does not allow        → DENY
3. If any applicable statement is an Allow                   → ALLOW
4. Otherwise                                                 → DENY (implicit)
```

Explicit deny always wins. This is the core difference from the old RBAC chain,
which was allow-only: a single `Deny` statement overrides every `Allow`.

## 4. Database schema

Migration `000009_create_iam_tables`:

```
iam_policies            (id, name UNIQUE, description, document JSONB, version, is_builtin, …)
iam_policy_versions     (policy_id, version, document, is_default, …)   -- max 5, one default
iam_groups              (id, name UNIQUE, description, …)
iam_roles               (id, name UNIQUE, description, assume_role_policy_document, permissions_boundary_policy_id, …)
iam_policy_attachments  (policy_id, principal_type IN (user|group|role), principal_id)
iam_group_memberships   (group_id, user_id)
iam_role_assignments    (role_id, principal_type IN (user|group), principal_id)
users.permissions_boundary_policy_id
```

Migration `000010_migrate_rbac_to_iam` migrates the legacy RBAC data and drops
the old tables, mapping a legacy flat permission to an IAM statement:

| Legacy permission | Action | Resource |
|---|---|---|
| `*`, `admin:*` | `ap:*` | `arn:ap:*` |
| `chat:invoke` | `ap:InvokeChat` | `*` |
| `model:*` | `ap:InvokeModel` | `arn:ap:model/*` |
| `model:<name>` | `ap:InvokeModel` | `arn:ap:model/<name>` |
| anything else | `ap:<sanitized>` | `arn:ap:*` (opaque) |

Each legacy role becomes an IAM role with a single attached policy; legacy
user→role assignments become role grants. Access stays deny-by-default.

## 5. Built-in policies

`Store.SeedBuiltins` (run at startup) idempotently creates the recommended
policies and the `admin` / `member` roles:

| Policy | Grants |
|--------|--------|
| `APAdmin` | `ap:*` on `arn:ap:*` |
| `APMember` | `ap:InvokeChat` on `*`, `ap:InvokeModel` on `arn:ap:model/*` |
| `APDeveloper` | invoke chat/models + read providers, routes, metrics |
| `APReadOnly` | read providers, routes, users, keys, metrics |
| `APModelUser` | invoke chat + any model, no administration |

`admin` carries `APAdmin`; `member` carries `APMember`.

## 6. Enforcement points

| Check | Where |
|-------|-------|
| `ap:InvokeChat` on `arn:ap:*` | Middleware on `/chat` |
| `ap:InvokeModel` on `arn:ap:model/<name>` | `handlers.Chat` after body validation |
| one action per admin route | `authorize(action, …)` wrapper in `cmd/server/main.go` |

The root `AGENTPLANE_ADMIN_TOKEN` is the bootstrap superuser and bypasses policy
evaluation. Every other caller — including the `admin` role — is evaluated as
`ap:*`-style policies, not a hard-coded role-name string.

Failure semantics (unchanged from the spine):

| Condition | Response |
|-----------|----------|
| No authenticated user | `401 Unauthorized` |
| No resolvable model | `400 Bad Request` (`model_required`) |
| Denied (explicit or implicit) | `403 Forbidden` (`policy_denied`) |
| Policy could not be evaluated | `503 Service Unavailable` (`policy_unavailable`) |

## 7. Admin API

All endpoints require the root admin token or an API key permitted the relevant
action.

### Policies
| Method | Path | Action |
|--------|------|--------|
| `GET` | `/admin/policies` | `ap:ListPolicies` |
| `POST` | `/admin/policies` | `ap:CreatePolicy` |
| `GET` | `/admin/policies/{name}` | `ap:GetPolicy` |
| `PUT` | `/admin/policies/{name}` | `ap:UpdatePolicy` |
| `DELETE` | `/admin/policies/{name}` | `ap:DeletePolicy` |
| `POST` | `/admin/policies/{name}/attach` | `ap:AttachPolicy` |
| `POST` | `/admin/policies/{name}/detach` | `ap:DetachPolicy` |
| `GET` | `/admin/policies/{name}/attachments` | `ap:GetPolicy` |

`attach`/`detach` take `{"principal_type":"user|group|role","principal_id":"<uuid>"}`.

### Groups
| Method | Path | Action |
|--------|------|--------|
| `GET` | `/admin/groups` | `ap:ListGroups` |
| `POST` | `/admin/groups` | `ap:CreateGroup` |
| `DELETE` | `/admin/groups/{name}` | `ap:DeleteGroup` |
| `GET` | `/admin/groups/{name}/members` | `ap:GetGroup` |
| `POST` | `/admin/groups/{name}/members` | `ap:AddUserToGroup` |
| `DELETE` | `/admin/groups/{name}/members/{id}` | `ap:RemoveUserFromGroup` |

### Roles
| Method | Path | Action |
|--------|------|--------|
| `GET` | `/admin/roles` | `ap:ListRoles` |
| `POST` | `/admin/roles` | `ap:CreateRole` |
| `DELETE` | `/admin/roles/{name}` | `ap:DeleteRole` |
| `GET` | `/admin/roles/{name}/grants` | `ap:ListRoles` |
| `POST` | `/admin/roles/{name}/grant` | `ap:GrantRole` |
| `POST` | `/admin/roles/{name}/revoke` | `ap:RevokeRole` |

### Users
| Method | Path | Action |
|--------|------|--------|
| `GET` | `/admin/users` | `ap:ListUsers` |
| `GET` | `/admin/users/{id}/roles` | `ap:ListRoles` |
| `POST` | `/admin/users/{id}/roles` | `ap:GrantRole` |
| `DELETE` | `/admin/users/{id}/roles/{role}` | `ap:RevokeRole` |
| `POST` | `/admin/users/{id}/permissions-boundary` | `ap:SetPermissionsBoundary` |
| `DELETE` | `/admin/users/{id}` | `ap:DeleteUser` |

### Example: least-privilege model access

```bash
# 1. A policy that only allows one model.
curl -X POST http://localhost:3001/admin/policies \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"gpt4o-only","document":{"Version":"2026-01-01","Statement":[
        {"Effect":"Allow","Action":["ap:InvokeChat","ap:InvokeModel"],"Resource":["arn:ap:model/gpt-4o"]}]}}'

# 2. Attach it directly to a user.
curl -X POST http://localhost:3001/admin/policies/gpt4o-only/attach \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"principal_type":"user","principal_id":"<uuid>"}'

# 3. Or grant the built-in admin/member role.
curl -X POST http://localhost:3001/admin/users/<uuid>/roles \
  -H "Authorization: Bearer $AGENTPLANE_ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"role":"member"}'
```

## 8. Least-privilege features

1. **Implicit deny** — nothing is granted unless allowed.
2. **Explicit deny wins** — a safe override for over-broad allows.
3. **Permission boundaries** — a hard ceiling per identity.
4. **Write-time validation** — malformed or over-broad documents are rejected.
5. **Policy versioning** — up to 5 versions, one default, for safe tightening + rollback.
6. **Recommended policies** — `APAdmin`/`APMember`/`APDeveloper`/`APReadOnly`/`APModelUser`.

## 9. Extending the policy layer

`internal/policy` remains generic. To add a sibling policy (rate limit, quota):

1. Implement `policy.Policy` (`Name()` + `Evaluate(ctx, policy.Request)`).
2. Register it in the chain in `cmd/server/main.go`:
   ```go
   policyEP := policy.NewEnforcementPoint(iam.New(iamStore), myNewPolicy)
   ```
3. Reuse `policyEP.Require(action, resource)` or `policyEP.Authorize(...)`.

## 10. Future work (v2)

- **Conditions** — operators (`StringEquals`, `IpAddress`, …) and context keys.
- **Assume-role / STS** — temporary credentials via the stored trust policy.
- **Policy simulator** — dry-run an identity's effective permissions.
- **SCPs** — an organization-level permission ceiling.
- **Access advisor** — surface unused grants to prune toward least privilege.

See `docs/policy-rbac.md` for the superseded RBAC design.
