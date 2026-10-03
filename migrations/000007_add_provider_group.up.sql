ALTER TABLE providers
    ADD COLUMN provider_group TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_providers_enabled_group_weight
    ON providers(provider_group, enabled, weight);
