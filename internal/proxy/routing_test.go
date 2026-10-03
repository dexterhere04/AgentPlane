package proxy

import (
	"encoding/json"
	"testing"
)

func rawJSON(t *testing.T, value string) json.RawMessage {
	t.Helper()
	return json.RawMessage(value)
}

func TestRuleSetMatchByModel(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:           "1",
			Name:         "gpt4",
			Priority:     10,
			ModelMatcher: rawJSON(t, `{"equals":"gpt-4o"}`),
			Action:       "route",
			Enabled:      true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	rule, ok, err := set.Match(RouteRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected rule to match")
	}
	if rule.Name != "gpt4" {
		t.Fatalf("matched %q, want gpt4", rule.Name)
	}

	_, ok, err = set.Match(RouteRequest{Model: "gpt-3.5"})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected rule not to match")
	}
}

func TestRuleSetMatchByRole(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:          "1",
			Name:        "premium",
			Priority:    10,
			RoleMatcher: rawJSON(t, `{"in":["premium","admin"]}`),
			Action:      "route",
			Enabled:     true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	_, ok, err := set.Match(RouteRequest{
		Roles: []string{"user", "premium"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected role rule to match")
	}
}

func TestRuleSetMatchByUser(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:          "1",
			Name:        "specific-user",
			Priority:    10,
			UserMatcher: rawJSON(t, `{"usernames":["alice","bob"]}`),
			Action:      "route",
			Enabled:     true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	_, ok, err := set.Match(RouteRequest{
		Username: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected user rule to match")
	}

	_, ok, err = set.Match(RouteRequest{
		Username: "charlie",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected user rule not to match")
	}
}

func TestRuleSetUsesANDAcrossMatchers(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:           "1",
			Name:         "premium-gpt4",
			Priority:     10,
			ModelMatcher: rawJSON(t, `{"equals":"gpt-4o"}`),
			RoleMatcher:  rawJSON(t, `{"equals":"premium"}`),
			UserMatcher:  rawJSON(t, `{"usernames":["alice"]}`),
			Action:       "route",
			Enabled:      true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		req  RouteRequest
		want bool
	}{
		{
			name: "all match",
			req: RouteRequest{
				Model:    "gpt-4o",
				Roles:    []string{"premium"},
				Username: "alice",
			},
			want: true,
		},
		{
			name: "wrong model",
			req: RouteRequest{
				Model:    "gpt-3.5",
				Roles:    []string{"premium"},
				Username: "alice",
			},
			want: false,
		},
		{
			name: "wrong role",
			req: RouteRequest{
				Model:    "gpt-4o",
				Roles:    []string{"user"},
				Username: "alice",
			},
			want: false,
		},
		{
			name: "wrong user",
			req: RouteRequest{
				Model:    "gpt-4o",
				Roles:    []string{"premium"},
				Username: "bob",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got, err := set.Match(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("matched=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestRuleSetPriorityWins(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:       "2",
			Name:     "lower-priority-number",
			Priority: 5,
			Action:   "route-a",
			Enabled:  true,
		},
		{
			ID:       "1",
			Name:     "higher-priority-number",
			Priority: 10,
			Action:   "route-b",
			Enabled:  true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	rule, ok, err := set.Match(RouteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a rule to match")
	}

	if rule.Name != "lower-priority-number" {
		t.Fatalf("matched %q, want lower-priority-number", rule.Name)
	}
}

func TestRuleSetIgnoresDisabledRules(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:       "1",
			Name:     "disabled",
			Priority: 1,
			Action:   "disabled",
			Enabled:  false,
		},
		{
			ID:       "2",
			Name:     "enabled",
			Priority: 2,
			Action:   "enabled",
			Enabled:  true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	rule, ok, err := set.Match(RouteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected enabled rule to match")
	}

	if rule.Name != "enabled" {
		t.Fatalf("matched %q, want enabled", rule.Name)
	}
}

func TestRuleSetEqualPriorityIsDeterministic(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:       "b",
			Name:     "second",
			Priority: 10,
			Action:   "b",
			Enabled:  true,
		},
		{
			ID:       "a",
			Name:     "first",
			Priority: 10,
			Action:   "a",
			Enabled:  true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	rule, ok, err := set.Match(RouteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a rule to match")
	}

	if rule.ID != "a" {
		t.Fatalf("matched ID %q, want a", rule.ID)
	}
}

func TestRuleSetPrefixMatcher(t *testing.T) {
	rules := []RoutingRule{
		{
			ID:           "1",
			Name:         "gpt-family",
			Priority:     1,
			ModelMatcher: rawJSON(t, `{"prefix":"gpt-"}`),
			Action:       "route",
			Enabled:      true,
		},
	}

	set, err := NewRuleSet(rules)
	if err != nil {
		t.Fatal(err)
	}

	_, ok, err := set.Match(RouteRequest{Model: "GPT-4.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected case-insensitive prefix match")
	}
}

func TestRuleSetRejectsInvalidMatcher(t *testing.T) {
	_, err := NewRuleSet([]RoutingRule{
		{
			ID:           "1",
			Name:         "invalid",
			Priority:     1,
			ModelMatcher: rawJSON(t, `{"unknown":"value"}`),
			Action:       "route",
			Enabled:      true,
		},
	})
	if err != nil {
		// Construction currently validates the JSON shape. Semantic matcher
		// validation occurs when the rule is evaluated.
		return
	}

	_, _, err = (&RuleSet{
		rules: []RoutingRule{
			{
				ID:           "1",
				Name:         "invalid",
				Priority:     1,
				ModelMatcher: rawJSON(t, `{"unknown":"value"}`),
				Action:       "route",
				Enabled:      true,
			},
		},
	}).Match(RouteRequest{Model: "gpt-4o"})

	if err == nil {
		t.Fatal("expected invalid matcher error")
	}
}
