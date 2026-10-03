CREATE TABLE providers (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    base_url         TEXT NOT NULL,
    secret_ref       TEXT,
    weight           INTEGER NOT NULL DEFAULT 1 CHECK (weight >= 0),
    supported_models JSONB NOT NULL DEFAULT '[]'::jsonb,
    timeout_ms       INTEGER NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
    max_retries      INTEGER NOT NULL DEFAULT 1 CHECK (max_retries >= 0),
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT providers_name_not_blank
        CHECK (btrim(name) <> ''),

    CONSTRAINT providers_base_url_not_blank
        CHECK (btrim(base_url) <> ''),

    CONSTRAINT providers_supported_models_array
        CHECK (jsonb_typeof(supported_models) = 'array')
);

CREATE TABLE routing_rules (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                    TEXT NOT NULL UNIQUE,
    priority                INTEGER NOT NULL DEFAULT 0,
    model_matcher           JSONB,
    role_matcher            JSONB,
    user_matcher            JSONB,
    action                  TEXT NOT NULL,
    provider_group           TEXT,
    timeout_retry_overrides JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT routing_rules_name_not_blank
        CHECK (btrim(name) <> ''),

    CONSTRAINT routing_rules_action_not_blank
        CHECK (btrim(action) <> ''),

    CONSTRAINT routing_rules_model_matcher_object
        CHECK (
            model_matcher IS NULL
            OR jsonb_typeof(model_matcher) = 'object'
        ),

    CONSTRAINT routing_rules_role_matcher_object
        CHECK (
            role_matcher IS NULL
            OR jsonb_typeof(role_matcher) = 'object'
        ),

    CONSTRAINT routing_rules_user_matcher_object
        CHECK (
            user_matcher IS NULL
            OR jsonb_typeof(user_matcher) = 'object'
        ),

    CONSTRAINT routing_rules_overrides_object
        CHECK (jsonb_typeof(timeout_retry_overrides) = 'object')
);

CREATE INDEX idx_providers_enabled
    ON providers(enabled);

CREATE INDEX idx_providers_enabled_weight
    ON providers(enabled, weight);

CREATE INDEX idx_providers_supported_models
    ON providers USING GIN(supported_models);

CREATE INDEX idx_routing_rules_priority
    ON routing_rules(priority, id)
    WHERE enabled = TRUE;

CREATE INDEX idx_routing_rules_provider_group
    ON routing_rules(provider_group)
    WHERE enabled = TRUE;

CREATE INDEX idx_routing_rules_model_matcher
    ON routing_rules USING GIN(model_matcher);

CREATE INDEX idx_routing_rules_role_matcher
    ON routing_rules USING GIN(role_matcher);

CREATE INDEX idx_routing_rules_user_matcher
    ON routing_rules USING GIN(user_matcher);
