# API Key Generation — Design & Decisions

Location: `internal/api/`, `internal/config/apikey_pepper.go`

## 1. What was built

A secure API key **generation** system for AgentPlane, matching the `ap_live_<key_id>_<secret>`
format and `api_keys` schema.

This covers **generation** — creating a new key, returning the full key to show the
user once, and producing the `key_id` / `secret_hash` values to persist. 

## 2. Files delivered

| File | Purpose |
|---|---|
| `internal/api/api_generation.go` | Core generation logic: random `key_id`/secret generation, base64url encoding, SHA-256+pepper hashing, full key assembly |
| `internal/api/apikey_test.go` | Unit tests covering key format, uniqueness, hash determinism/sensitivity, and encoded length |
| `internal/config/apikey_pepper.go` | `config.KeyPepper()`, which retrieves the pepper from the configured `SecretStore` |

The implementation was verified using Go's testing tools (`go test -v ./internal/api`), with all seven unit tests passing successfully.

## 3. Decisions and rationale

### 3.1 Hashing algorithm: SHA-256 + server-side pepper

SHA-256 is fast enough for a gateway that needs to authenticate a high volume of requests
per second, unlike Argon2id or bcrypt, which are deliberately slow and would add latency
to every single request hitting the gateway. The tradeoff — SHA-256 alone is fast for
attackers to brute-force too — is mitigated by mixing in a **server-side pepper**
(`secret_hash = SHA256(secret + pepper)`). Unlike a per-row salt, the pepper is never
stored in the database, so a stolen `api_keys` table dump alone isn't enough to brute-force
secrets offline; the attacker also needs the pepper, which is stored separately in the configured secret management backend and is never persisted alongside the database.

### 3.2 Encoding: base64url

Both `key_id` and `secret` are generated as raw random bytes and then base64url-encoded
(`RawURLEncoding`, no padding). This is more compact than hex (which would roughly double
the string length) and is safe to embed directly in headers/URLs without escaping, unlike
standard base64 (which uses `+`, `/`, and `=`).

### 3.3 Byte lengths: 8 bytes for `key_id`, 32 bytes for `secret`

- **`key_id` — 8 random bytes (11-char base64url string).** `key_id` is not secret; the
  design doc is explicit that it's "used only for fast lookup, not authorization." Its job
  is to be a good index key with a low collision probability, not to resist brute force, so
  it doesn't need anywhere near the entropy of the secret.
- **`secret` — 32 random bytes (43-char base64url string, 256 bits of entropy).** This is
  the actual authorization-bearing value that gets hashed and compared. 256 bits gives a
  wide security margin against brute-force/guessing attacks for the lifetime of a key,
  in line with what comparable API platforms use for bearer secrets.

### 3.4 Pepper source: centralized secret store via `config`

Added `config.KeyPepper()` following the same configuration pattern already established for `config.OpenAIKey()`. Rather than reading secrets directly, the rest of the application retrieves the pepper through the centralized `config` package, which delegates to the configured `SecretStore`.

This means callers never interact with a specific secret backend. Whether the pepper is stored in Vault/OpenBao, AWS Secrets Manager, Azure Key Vault, a local file, or environment variables is determined by the configured `SecretStore`, allowing the backend to be changed without modifying `api` or any other consumer.

`KeyPepper()` returns an error if the secret cannot be retrieved or is empty, causing the application to fail during startup rather than silently hashing API keys with an empty or missing pepper. This ensures deployment or configuration issues are detected immediately.

The implementation lives in its own file (`internal/config/apikey_pepper.go`) alongside the existing configuration package. Since Go packages can span multiple files, this extends the `config` package without requiring changes to the existing `config.go`.

## 4. Secret required

A server-side secret named:

```text
API_KEY_PEPPER
```

must be provisioned in the configured `SecretStore`. It should be a long, cryptographically random value generated once, stored securely in the chosen secret management backend (for example, Vault/OpenBao, AWS Secrets Manager, or Azure Key Vault), and never committed to source control.