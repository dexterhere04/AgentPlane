package policy

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		name      string
		granted   string
		requested string
		want      bool
	}{
		{"wildcard matches any", "*", "chat:invoke", true},
		{"wildcard matches model", "*", "model:gpt-4o", true},
		{"exact match", "chat:invoke", "chat:invoke", true},
		{"exact mismatch", "chat:invoke", "chat:other", false},
		{"model prefix wildcard", "model:*", "model:gpt-4o", true},
		{"model prefix wildcard multi-segment", "model:*", "model:gpt-4o-mini", true},
		{"model wildcard does not cross resource", "model:*", "chat:invoke", false},
		{"admin prefix wildcard", "admin:*", "admin:roles:manage", true},
		{"exact model does not match sibling", "model:gpt-4o", "model:gpt-4o-mini", false},
		{"empty granted", "", "chat:invoke", false},
		{"empty requested", "chat:invoke", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.granted, tc.requested); got != tc.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tc.granted, tc.requested, got, tc.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	granted := []string{"chat:invoke", "model:gpt-4o"}

	if !MatchAny(granted, "chat:invoke") {
		t.Fatal("expected chat:invoke to match")
	}
	if !MatchAny(granted, "model:gpt-4o") {
		t.Fatal("expected model:gpt-4o to match")
	}
	if MatchAny(granted, "model:gpt-4o-mini") {
		t.Fatal("did not expect model:gpt-4o-mini to match")
	}
	if MatchAny(nil, "chat:invoke") {
		t.Fatal("expected no match for empty permission set")
	}
}
