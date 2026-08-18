# .gitignore

**File:** `.gitignore`

## Content

```
.env
build/
bin/
logs/

# Compiled binaries (anchored to the repo root so they don't also match
# the cmd/server/ source directory).
/server
/agentplane
```

## Exclusion Rules

| Pattern | Reason |
|---------|--------|
| `.env` | Contains secrets (API keys, pepper, admin token). Must never be committed. |
| `build/` | Build output directory. Generated artifacts. |
| `bin/` | Binary output directory. Compiled executables. |
| `logs/` | Runtime log files. Environment-specific noise. |
| `/server` | Compiled server binary (root-anchored). |
| `/agentplane` | Compiled gateway binary (root-anchored). |

## Important

The binary patterns are anchored to the repository root (`/server`, `/agentplane`) so they don't accidentally match the `cmd/server/` source directory. The `.env` file stores local development secrets — it is excluded from version control to prevent accidental leaks. In production, secrets are loaded from environment variables or a secret store (see `docs/secrets-backends.md`), never from a committed file.
