DROP INDEX IF EXISTS idx_providers_enabled_group_weight;
ALTER TABLE providers DROP COLUMN IF EXISTS provider_group;
