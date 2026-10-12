DROP INDEX IF EXISTS idx_routing_rules_user_matcher;
DROP INDEX IF EXISTS idx_routing_rules_role_matcher;
DROP INDEX IF EXISTS idx_routing_rules_model_matcher;
DROP INDEX IF EXISTS idx_routing_rules_provider_group;
DROP INDEX IF EXISTS idx_routing_rules_priority;
DROP INDEX IF EXISTS idx_providers_supported_models;

DROP TABLE IF EXISTS routing_rules;
DROP TABLE IF EXISTS providers;
