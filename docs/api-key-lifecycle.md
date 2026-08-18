# AgentPlane API Key Lifecycle

## 1. Overview

This document describes the complete lifecycle of an AgentPlane API key,
from user provisioning and key generation through request authentication,
user resolution, request-context attachment, usage tracking, and revocation.


## 2. User Provisioning and API Key Creation

```text
┌──────────────────────┐
│ 1. User Provisioning │
│ POST /provision/user │
└──────────┬───────────┘
           │
           │ username
           │ email
           │ key_name
           ▼
┌─────────────────────────────┐
│ Provisioning Service        │
│ internal/provisioning       │
└──────────┬──────────────────┘
           │
           │ Creates user
           ▼
┌─────────────────────────────┐
│ PostgreSQL: users            │
│                             │
│ user.id                     │
│ username                    │
│ email                       │
│ status = active             │
└──────────┬──────────────────┘
           │
           │ Generate API key
           ▼
┌───────────────────────────────┐
│ API Key Generator             │
│ internal/api/api_generation.go│
│                               │
│ ap_live_<key_id>_<secret>     │
└──────────┬────────────────────┘
           │
           │ Split into:
           │
           ├── key_id ────────────────┐
           │                          │
           └── secret                 │
                                      ▼
                         ┌─────────────────────────┐
                         │ Hash secret + pepper    │
                         │                         │
                         │ secret_hash             │
                         └────────────┬────────────┘
                                      │
                                      ▼
                         ┌─────────────────────────┐
                         │ PostgreSQL: api_keys    │
                         │                         │
                         │ id                      │
                         │ key_id                  │
                         │ user_id ────────────────┼──► users.id
                         │ name                    │
                         │ secret_hash             │
                         │ status = active         │
                         │ created_at              │
                         │ last_used_at = NULL     │
                         │ expires_at              │
                         │ revoked_at              │
                         └─────────────────────────┘

           IMPORTANT:
           plaintext secret is NOT stored in PostgreSQL.

                         │
                         │ Full key returned ONCE
                         ▼
┌──────────────────────────────────┐
│ Client receives                   │
│ ap_live_<key_id>_<secret>        │
│                                  │
│ Client must store the full key.  │
└──────────────────────────────────┘
```


## 3. Request Authentication Lifecycle

```text
Client
  │
  │ POST /chat
  │ Authorization: Bearer ap_live_<key_id>_<secret>
  ▼
┌─────────────────────────────┐
│ HTTP Server / ServeMux      │
│                             │
│ /chat                       │
│      │                      │
│      ▼                      │
│ Authenticator.Middleware()  │
└─────────────┬───────────────┘
              │
              │ Read Authorization header
              ▼
       ┌──────────────────┐
       │ Parse Bearer key │
       └────────┬─────────┘
                │
                │ Extract
                ├── key_id
                └── secret
                │
                ▼
       ┌─────────────────────────┐
       │ auth.Store              │
       │ FindCredential(key_id)  │
       └────────────┬────────────┘
                    │
                    │ SELECT api_keys
                    ▼
       ┌─────────────────────────┐
       │ PostgreSQL              │
       │                         │
       │ key_id                  │
       │ user_id                 │
       │ secret_hash             │
       │ status                  │
       │ expires_at              │
       │ revoked_at              │
       └────────────┬────────────┘
                    │
                    ▼
       ┌──────────────────────────────┐
       │ Authenticator                │
       │                              │
       │ 1. status == active?         │
       │ 2. revoked_at == NULL?       │
       │ 3. not expired?              │
       │ 4. hash supplied secret      │
       │ 5. constant-time comparison  │
       └──────────────┬───────────────┘
                      │
                 valid │
                      ▼
       ┌─────────────────────────────┐
       │ Resolve user                 │
       │                              │
       │ users.GetUserByID(user_id)  │
       └──────────────┬──────────────┘
                      │
                      ▼
       ┌─────────────────────────────┐
       │ Verify user is active       │
       └──────────────┬──────────────┘
                      │
                      ▼
       ┌─────────────────────────────┐
       │ Update last_used_at          │
       │                             │
       │ api_keys.last_used_at       │
       │ = now()                     │
       └──────────────┬──────────────┘
                      │
                      ▼
       ┌─────────────────────────────┐
       │ Attach user to request      │
       │ context.WithValue(...)      │
       └──────────────┬──────────────┘
                      │
                      ▼
       ┌─────────────────────────────┐
       │ next.ServeHTTP(...)         │
       │                             │
       │ Handler can call:           │
       │ UserFromContext(ctx)        │
       └──────────────┬──────────────┘
                      │
                      ▼
              ┌───────────────┐
              │ Chat Handler  │
              └───────┬───────┘
                      │
                      ▼
              ┌───────────────┐
              │ Proxy/OpenAI  │
              └───────────────┘
```

## 4. Revocation Lifecycle

Document that revocation is performed through the application-level
`RevokeAPIKey` method, which updates `api_keys.revoked_at`.

After revocation:

1. The API key remains in PostgreSQL for audit/history.
2. `revoked_at` is no longer NULL.
3. `Authenticator.Authenticate` detects the revoked key.
4. Authentication fails.
5. The middleware returns HTTP 401.
6. The request does not reach the downstream handler.

## 5. Security Properties

- Plaintext API keys are never stored in PostgreSQL.
- Only the secret hash is persisted.
- Each API key belongs to exactly one user through `api_keys.user_id`.
- A user can have multiple API keys.
- API keys can be revoked independently.
- API-key secrets are compared using constant-time comparison.
- Inactive, expired, and revoked keys cannot authenticate.
- Successful authentication updates `last_used_at`.
- The authenticated user is attached to the request context.

## 6. Relevant Components

| Component | Location | Responsibility |
|---|---|---|
| API-key generation | `internal/api/api_generation.go` | Generates the API key |
| API-key persistence | `internal/api/store.go` | Creates and revokes keys |
| Provisioning | `internal/provisioning/` | Creates users and their API keys |
| Credential lookup | `internal/auth/store.go` | Loads authentication data |
| Authentication | `internal/auth/authenticator.go` | Validates API keys and resolves users |
| Middleware | `internal/auth/middleware.go` | Authenticates HTTP requests and attaches users |
| User store | `internal/users/` | Resolves user records |
| API-key schema | `migrations/` | Defines PostgreSQL API-key storage |

## 7. Validation

The lifecycle is covered by unit and integration tests.

Include the commands that were used to validate the implementation:

```bash
go test ./...
go test -tags=integration ./internal/...
```
