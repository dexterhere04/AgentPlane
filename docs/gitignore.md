# .gitignore

**File:** `.gitignore`

## Content

```
.env
build/
bin/
logs/
```

## Exclusion Rules

| Pattern | Reason |
|---------|--------|
| `.env` | Contains secrets (API keys). Must never be committed. |
| `build/` | Build output directory. Generated artifacts. |
| `bin/` | Binary output directory. Compiled executables. |
| `logs/` | Runtime log files. Environment-specific noise. |

## Important

The `.env` file stores the `OPENAI_API_KEY` used for local development. It is excluded from version control to prevent accidental secret leaks. In production, the API key is loaded from environment variables set by the deployment platform (Docker, Kubernetes, etc.).
