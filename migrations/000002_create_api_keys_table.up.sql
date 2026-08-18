CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_id       TEXT NOT NULL UNIQUE,
    user_id      UUID NOT NULL REFERENCES users(id),
    name         TEXT NOT NULL,
    secret_hash  TEXT NOT NULL,
    status       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

-- No separate index on key_id: the UNIQUE constraint above already creates
-- a btree index on it, which is what the "indexed" note in the schema
-- calls for. A second explicit index would just be redundant.
