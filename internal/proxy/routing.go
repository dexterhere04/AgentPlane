package proxy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RouteRequest contains the request attributes used by routing rules.
//
// Roles are supplied by the caller because the authenticated users.User type
// does not contain roles itself.
type RouteRequest struct {
	Model    string
	UserID   string
	Username string
	Roles    []string
}

type TimeoutRetryOverrides struct {
	TimeoutMS     *int `json:"timeout_ms,omitempty"`
	MaxRetries    *int `json:"max_retries,omitempty"`
	BackoffBaseMS *int `json:"backoff_base_ms,omitempty"`
	BackoffMaxMS  *int `json:"backoff_max_ms,omitempty"`
	JitterPercent *int `json:"jitter_percent,omitempty"`
}

// RoutingRule is the in-memory representation of a routing_rules row.
type RoutingRule struct {
	ID                    string
	Name                  string
	Priority              int
	ModelMatcher          json.RawMessage
	RoleMatcher           json.RawMessage
	UserMatcher           json.RawMessage
	Action                string
	ProviderGroup         string
	TimeoutRetryOverrides json.RawMessage
	Enabled               bool
}

// StringMatcher supports exact, set-membership, and prefix matching.
type StringMatcher struct {
	Equals string   `json:"equals,omitempty"`
	In     []string `json:"in,omitempty"`
	Prefix string   `json:"prefix,omitempty"`
}

func (r RoutingRule) ResilienceConfig() (ResilienceConfig, error) {
	if len(r.TimeoutRetryOverrides) == 0 ||
		string(r.TimeoutRetryOverrides) == "null" ||
		string(r.TimeoutRetryOverrides) == "{}" {
		return ResilienceConfig{}, nil
	}

	var overrides TimeoutRetryOverrides
	if err := json.Unmarshal(r.TimeoutRetryOverrides, &overrides); err != nil {
		return ResilienceConfig{}, fmt.Errorf(
			"parse timeout/retry overrides for rule %q: %w",
			r.Name,
			err,
		)
	}

	config := ResilienceConfig{}

	if overrides.TimeoutMS != nil {
		if *overrides.TimeoutMS <= 0 {
			return ResilienceConfig{}, fmt.Errorf(
				"rule %q: timeout_ms must be greater than zero",
				r.Name,
			)
		}
		config.Timeout = time.Duration(*overrides.TimeoutMS) * time.Millisecond
	}

	if overrides.MaxRetries != nil {
		if *overrides.MaxRetries < 0 {
			return ResilienceConfig{}, fmt.Errorf(
				"rule %q: max_retries must be non-negative",
				r.Name,
			)
		}
		config.MaxRetries = *overrides.MaxRetries
		config.MaxRetriesSet = true
	}

	if overrides.BackoffBaseMS != nil {
		if *overrides.BackoffBaseMS <= 0 {
			return ResilienceConfig{}, fmt.Errorf(
				"rule %q: backoff_base_ms must be greater than zero",
				r.Name,
			)
		}
		config.BackoffBase = time.Duration(*overrides.BackoffBaseMS) * time.Millisecond
	}

	if overrides.BackoffMaxMS != nil {
		if *overrides.BackoffMaxMS <= 0 {
			return ResilienceConfig{}, fmt.Errorf(
				"rule %q: backoff_max_ms must be greater than zero",
				r.Name,
			)
		}
		config.BackoffMax = time.Duration(*overrides.BackoffMaxMS) * time.Millisecond
	}

	if overrides.JitterPercent != nil {
		if *overrides.JitterPercent < 0 || *overrides.JitterPercent > 100 {
			return ResilienceConfig{}, fmt.Errorf(
				"rule %q: jitter_percent must be between 0 and 100",
				r.Name,
			)
		}
		config.JitterFactor = float64(*overrides.JitterPercent) / 100
		config.JitterSet = true
	}

	return config, nil
}

func (m StringMatcher) match(value string) bool {
	if m.Equals != "" && strings.EqualFold(value, m.Equals) {
		return true
	}

	for _, candidate := range m.In {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}

	if m.Prefix != "" && strings.HasPrefix(
		strings.ToLower(value),
		strings.ToLower(m.Prefix),
	) {
		return true
	}

	return m.Equals == "" && len(m.In) == 0 && m.Prefix == ""
}

// RoleMatcher matches when at least one user role satisfies the matcher.
type RoleMatcher struct {
	In     []string `json:"in,omitempty"`
	Equals string   `json:"equals,omitempty"`
}

// UserMatcher matches user ID and/or username.
//
// If both IDs and usernames are configured, either one may match. This makes
// the fields alternatives within the user matcher; the routing rule's
// model/role/user matchers remain ANDed together.
type UserMatcher struct {
	IDs       []string `json:"ids,omitempty"`
	Usernames []string `json:"usernames,omitempty"`
}

func (m UserMatcher) match(req RouteRequest) bool {
	if len(m.IDs) == 0 && len(m.Usernames) == 0 {
		return false
	}

	for _, id := range m.IDs {
		if req.UserID != "" && strings.EqualFold(req.UserID, id) {
			return true
		}
	}

	for _, username := range m.Usernames {
		if req.Username != "" && strings.EqualFold(req.Username, username) {
			return true
		}
	}

	return false
}

// RuleSet evaluates routing rules in deterministic priority order.
type RuleSet struct {
	rules []RoutingRule
}

// NewRuleSet validates and sorts the supplied rules.
func NewRuleSet(rules []RoutingRule) (*RuleSet, error) {
	copied := append([]RoutingRule(nil), rules...)

	for _, rule := range copied {
		if err := validateRule(rule); err != nil {
			return nil, err
		}
	}

	sort.SliceStable(copied, func(i, j int) bool {
		if copied[i].Priority != copied[j].Priority {
			return copied[i].Priority < copied[j].Priority
		}

		if copied[i].ID != copied[j].ID {
			return copied[i].ID < copied[j].ID
		}

		return copied[i].Name < copied[j].Name
	})

	return &RuleSet{rules: copied}, nil
}

// Match returns the first enabled rule matching the request.
func (s *RuleSet) Match(req RouteRequest) (RoutingRule, bool, error) {
	for _, rule := range s.rules {
		if !rule.Enabled {
			continue
		}

		matches, err := ruleMatches(rule, req)
		if err != nil {
			return RoutingRule{}, false, fmt.Errorf(
				"routing rule %q: %w",
				rule.Name,
				err,
			)
		}

		if matches {
			return rule, true, nil
		}
	}

	return RoutingRule{}, false, nil
}

func validateRule(rule RoutingRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("routing rule name is required")
	}

	if strings.TrimSpace(rule.Action) == "" {
		return fmt.Errorf("routing rule %q: action is required", rule.Name)
	}

	if err := validateMatcherObject(rule.ModelMatcher, "model_matcher"); err != nil {
		return fmt.Errorf("routing rule %q: %w", rule.Name, err)
	}

	if err := validateMatcherObject(rule.RoleMatcher, "role_matcher"); err != nil {
		return fmt.Errorf("routing rule %q: %w", rule.Name, err)
	}

	if err := validateMatcherObject(rule.UserMatcher, "user_matcher"); err != nil {
		return fmt.Errorf("routing rule %q: %w", rule.Name, err)
	}

	return nil
}

func validateMatcherObject(raw json.RawMessage, field string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s must be a JSON object: %w", field, err)
	}

	if value == nil {
		return fmt.Errorf("%s must be a JSON object", field)
	}

	return nil
}

func ruleMatches(rule RoutingRule, req RouteRequest) (bool, error) {
	modelOK, err := matchModel(rule.ModelMatcher, req.Model)
	if err != nil {
		return false, err
	}
	if !modelOK {
		return false, nil
	}

	roleOK, err := matchRoles(rule.RoleMatcher, req.Roles)
	if err != nil {
		return false, err
	}
	if !roleOK {
		return false, nil
	}

	userOK, err := matchUser(rule.UserMatcher, req)
	if err != nil {
		return false, err
	}
	if !userOK {
		return false, nil
	}

	return true, nil
}

func matchModel(raw json.RawMessage, model string) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return true, nil
	}

	var matcher StringMatcher
	if err := json.Unmarshal(raw, &matcher); err != nil {
		return false, fmt.Errorf("invalid model_matcher: %w", err)
	}

	if matcher.Equals == "" && len(matcher.In) == 0 && matcher.Prefix == "" {
		return false, fmt.Errorf(
			"model_matcher must contain equals, in, or prefix",
		)
	}

	return matcher.match(model), nil
}

func matchRoles(raw json.RawMessage, roles []string) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return true, nil
	}

	var matcher RoleMatcher
	if err := json.Unmarshal(raw, &matcher); err != nil {
		return false, fmt.Errorf("invalid role_matcher: %w", err)
	}

	if matcher.Equals == "" && len(matcher.In) == 0 {
		return false, fmt.Errorf(
			"role_matcher must contain equals or in",
		)
	}

	for _, role := range roles {
		if matcher.Equals != "" && strings.EqualFold(role, matcher.Equals) {
			return true, nil
		}

		for _, candidate := range matcher.In {
			if strings.EqualFold(role, candidate) {
				return true, nil
			}
		}
	}

	return false, nil
}

func matchUser(raw json.RawMessage, req RouteRequest) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return true, nil
	}

	var matcher UserMatcher
	if err := json.Unmarshal(raw, &matcher); err != nil {
		return false, fmt.Errorf("invalid user_matcher: %w", err)
	}

	if len(matcher.IDs) == 0 && len(matcher.Usernames) == 0 {
		return false, fmt.Errorf(
			"user_matcher must contain ids or usernames",
		)
	}

	return matcher.match(req), nil
}
