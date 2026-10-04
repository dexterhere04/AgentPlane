package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
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

func (s *RoutingRuleStore) List(ctx context.Context) ([]RoutingRule, error) {
	rows, err := s.pool.Query(ctx, `
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
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RoutingRule

	for rows.Next() {
		var rule RoutingRule

		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Priority,
			&rule.ModelMatcher,
			&rule.RoleMatcher,
			&rule.UserMatcher,
			&rule.Action,
			&rule.ProviderGroup,
			&rule.TimeoutRetryOverrides,
			&rule.Enabled,
		); err != nil {
			return nil, err
		}

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rules, nil
}

func (s *RoutingRuleStore) Get(
	ctx context.Context,
	id string,
) (*RoutingRule, error) {
	var rule RoutingRule

	err := s.pool.QueryRow(ctx, `
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
		WHERE id = $1::uuid
	`, id).Scan(
		&rule.ID,
		&rule.Name,
		&rule.Priority,
		&rule.ModelMatcher,
		&rule.RoleMatcher,
		&rule.UserMatcher,
		&rule.Action,
		&rule.ProviderGroup,
		&rule.TimeoutRetryOverrides,
		&rule.Enabled,
	)
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

func (s *RoutingRuleStore) Create(
	ctx context.Context,
	rule RoutingRule,
) (*RoutingRule, error) {
	if _, err := rule.ResilienceConfig(); err != nil {
		return nil, err
	}

	if _, err := NewRuleSet([]RoutingRule{rule}); err != nil {
		return nil, err
	}

	var id string

	err := s.pool.QueryRow(ctx, `
		INSERT INTO routing_rules (
			name,
			priority,
			model_matcher,
			role_matcher,
			user_matcher,
			action,
			provider_group,
			timeout_retry_overrides,
			enabled
		)
		VALUES (
			$1,
			$2,
			$3::jsonb,
			$4::jsonb,
			$5::jsonb,
			$6,
			NULLIF($7, ''),
			$8::jsonb,
			$9
		)
		RETURNING id::text
	`,
		rule.Name,
		rule.Priority,
		jsonOrNull(rule.ModelMatcher),
		jsonOrNull(rule.RoleMatcher),
		jsonOrNull(rule.UserMatcher),
		rule.Action,
		rule.ProviderGroup,
		jsonOrNull(rule.TimeoutRetryOverrides),
		rule.Enabled,
	).Scan(&id)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

func jsonOrNull(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return string(value)
}

func jsonOrObject(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return "{}"
	}
	return string(value)
}

func (s *RoutingRuleStore) Update(
	ctx context.Context,
	id string,
	rule RoutingRule,
) (*RoutingRule, error) {
	if _, err := rule.ResilienceConfig(); err != nil {
		return nil, err
	}

	if _, err := NewRuleSet([]RoutingRule{rule}); err != nil {
		return nil, err
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE routing_rules
		SET
			name = $1,
			priority = $2,
			model_matcher = $3::jsonb,
			role_matcher = $4::jsonb,
			user_matcher = $5::jsonb,
			action = $6,
			provider_group = NULLIF($7, ''),
			timeout_retry_overrides = $8::jsonb,
			enabled = $9,
			updated_at = now()
		WHERE id = $10::uuid
	`,
		rule.Name,
		rule.Priority,
		jsonOrNull(rule.ModelMatcher),
		jsonOrNull(rule.RoleMatcher),
		jsonOrNull(rule.UserMatcher),
		rule.Action,
		rule.ProviderGroup,
		jsonOrObject(rule.TimeoutRetryOverrides),
		rule.Enabled,
		id,
	)
	if err != nil {
		return nil, err
	}

	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}

	return s.Get(ctx, id)
}

func (s *RoutingRuleStore) SetEnabled(
	ctx context.Context,
	id string,
	enabled bool,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE routing_rules
		SET enabled = $1, updated_at = now()
		WHERE id = $2::uuid
	`, enabled, id)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

func (s *RoutingRuleStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM routing_rules
		WHERE id = $1::uuid
	`, id)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
