package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RoutingRuleStore struct {
	pool *pgxpool.Pool
}

func NewRoutingRuleStore(pool *pgxpool.Pool) *RoutingRuleStore {
	return &RoutingRuleStore{pool: pool}
}

func (s *RoutingRuleStore) ListEnabled(ctx context.Context) ([]RoutingRule, error) {
	const query = `
		SELECT
			id::text,
			name,
			priority,
			model_matcher,
			role_matcher,
			user_matcher,
			action,
			COALESCE(provider_group, ''),
			timeout_retry_overrides,
			enabled
		FROM routing_rules
		WHERE enabled = TRUE
		ORDER BY priority ASC, id ASC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query routing rules: %w", err)
	}
	defer rows.Close()

	var rules []RoutingRule

	for rows.Next() {
		var (
			rule                  RoutingRule
			modelMatcher          []byte
			roleMatcher           []byte
			userMatcher           []byte
			timeoutRetryOverrides []byte
		)

		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Priority,
			&modelMatcher,
			&roleMatcher,
			&userMatcher,
			&rule.Action,
			&rule.ProviderGroup,
			&timeoutRetryOverrides,
			&rule.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scan routing rule: %w", err)
		}

		if len(modelMatcher) > 0 && string(modelMatcher) != "null" {
			rule.ModelMatcher = json.RawMessage(modelMatcher)
		}

		if len(roleMatcher) > 0 && string(roleMatcher) != "null" {
			rule.RoleMatcher = json.RawMessage(roleMatcher)
		}

		if len(userMatcher) > 0 && string(userMatcher) != "null" {
			rule.UserMatcher = json.RawMessage(userMatcher)
		}

		if len(timeoutRetryOverrides) > 0 && string(timeoutRetryOverrides) != "null" {
			rule.TimeoutRetryOverrides = json.RawMessage(timeoutRetryOverrides)
		}

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing rules: %w", err)
	}

	return rules, nil
}

func (s *RoutingRuleStore) LoadRuleSet(ctx context.Context) (*RuleSet, error) {
	rules, err := s.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	ruleSet, err := NewRuleSet(rules)
	if err != nil {
		return nil, fmt.Errorf("build routing ruleset: %w", err)
	}

	return ruleSet, nil
}
