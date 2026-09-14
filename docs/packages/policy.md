# Package: `policy`

**Files:** `internal/policy/policy.go`, `enforcement.go`, `middleware.go` + `internal/policy/rbac/engine.go`, `store.go`

**Packages:** `policy`, `policy/rbac`

## Overview

The identity-based authorization layer. It answers *"is this principal allowed this permission?"* — separate from `internal/guardrail`, which evaluates request/response content. RBAC (`internal/policy/rbac`) is the first implementation of the generic `policy.Policy` interface.

See [Policy Layer & RBAC](../policy-rbac.md) for the end-to-end design, schema, and admin API.

## Types

### `Decision`

```go
type Decision int

const (
    DecisionAllow Decision = iota
    DecisionDeny
)
```

### `Request`

```go
type Request struct {
    UserID     uuid.UUID
    Permission string   // "chat:invoke", "model:gpt-4o", ...
}
```

### `Policy`

```go
type Policy interface {
    Name() string
    Evaluate(ctx context.Context, req Request) (Decision, error)
}
```

An error means the policy could not reach a decision; the `EnforcementPoint` treats it as a denial (fail-closed).

## Functions

### `Match(granted, requested string) bool`

**File:** `internal/policy/policy.go`

Reports whether a granted permission satisfies a requested one. `*` matches everything; a trailing `:*` is a prefix wildcard (`model:*` matches `model:gpt-4o`); otherwise an exact match is required.

### `MatchAny(granted []string, requested string) bool`

Returns true if any granted permission matches the requested one.

### `NewEnforcementPoint(policies ...Policy) *EnforcementPoint`

Builds an enforcement point from an ordered list of policies.

### `(*EnforcementPoint) Authorize(ctx, req) (Decision, error)`

Evaluates every policy. Returns `(DecisionAllow, nil)` only if all allow; `(DecisionDeny, nil)` on an explicit deny; `(DecisionDeny, err)` if a policy errors (fail-closed). Callers distinguish the last two as `403` vs `503`.

### `(*EnforcementPoint) Require(permission string) func(http.Handler) http.Handler`

Middleware that authorizes the authenticated user (from `auth.UserFromContext`) for `permission`. Must be mounted **after** `auth.Authenticator.Middleware`. Responds `401` (no user), `403` (`policy_denied`), or `503` (`policy_unavailable`).

### `WriteDenied(w, permission)` / `WriteUnavailable(w, permission)`

Exported response writers so handlers authorizing dynamic resources (e.g. per-model access in `handlers.Chat`) return the same JSON shape as the middleware.

## RBAC (`internal/policy/rbac`)

### `Engine`

```go
func New(loader PermissionLoader) *Engine
```

A `policy.Policy` named `"rbac"`. It loads the user's effective permissions and allows when any matches (`policy.MatchAny`). A user with no roles has no permissions and is denied everything. `PermissionLoader` is satisfied by `*Store`; the interface exists so the engine can be unit tested without a database.

### `Store`

```go
func NewStore(pool *pgxpool.Pool) *Store
```

Persistence for roles, permissions, and assignments:

| Method | Purpose |
|--------|---------|
| `LoadPermissions(ctx, userID) ([]string, error)` | Distinct permissions from all of a user's roles |
| `ListRoles(ctx) ([]Role, error)` | All roles with their permissions |
| `CreateRole(ctx, name, description, permissions)` | Insert a role |
| `AddPermission(ctx, roleName, permission)` | Grant a permission |
| `RemovePermission(ctx, roleName, permission)` | Revoke a permission |
| `AssignRole(ctx, userID, roleName)` | Grant a role to a user |
| `RevokeRole(ctx, userID, roleName)` | Remove a role from a user |
| `ListUserRoles(ctx, userID) ([]string, error)` | Role names assigned to a user |

Sentinel errors: `ErrDuplicateRole`, `ErrRoleNotFound`, `ErrInvalidRoleName`.

## Wiring

```go
rbacStore := rbac.NewStore(pool)
policyEP := policy.NewEnforcementPoint(rbac.New(rbacStore))

mux.Handle("/chat",
    authenticator.Middleware(
        policyEP.Require("chat:invoke")(chatHandler),
    ),
)
```

`handlers.Chat` receives `policyEP` and additionally authorizes `model:<name>` after validating the request body.

## Design note

`EnforcementPoint` runs an ordered chain and denies on the first explicit denial; policy errors fail closed. This mirrors the guardrail `EnforcementPoint`'s fail-closed behavior for `Required` guardrails, but operates on identity rather than content. New policies (rate limits, quotas, ABAC) can be appended to the chain without changing callers.
