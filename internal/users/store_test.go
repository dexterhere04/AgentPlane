package users

import "testing"

func strPtr(s string) *string { return &s }

func TestValidateCreateUserParams_RequiresUsername(t *testing.T) {
	_, err := validateCreateUserParams(CreateUserParams{Username: ""})
	if err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
}

func TestValidateCreateUserParams_DefaultsStatus(t *testing.T) {
	params, err := validateCreateUserParams(CreateUserParams{Username: "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Status != StatusActive {
		t.Errorf("expected default status %q, got %q", StatusActive, params.Status)
	}
}

func TestValidateCreateUserParams_KeepsExplicitStatus(t *testing.T) {
	params, err := validateCreateUserParams(CreateUserParams{Username: "alice", Status: "suspended"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Status != "suspended" {
		t.Errorf("expected explicit status to be preserved, got %q", params.Status)
	}
}

func TestValidateCreateUserParams_AllowsNilEmail(t *testing.T) {
	_, err := validateCreateUserParams(CreateUserParams{Username: "alice", Email: nil})
	if err != nil {
		t.Fatalf("unexpected error for nil email: %v", err)
	}
}

func TestValidateCreateUserParams_RejectsBadEmail(t *testing.T) {
	_, err := validateCreateUserParams(CreateUserParams{Username: "alice", Email: strPtr("not-an-email")})
	if err == nil {
		t.Fatal("expected error for malformed email, got nil")
	}
}

func TestValidateCreateUserParams_AcceptsGoodEmail(t *testing.T) {
	_, err := validateCreateUserParams(CreateUserParams{Username: "alice", Email: strPtr("alice@example.com")})
	if err != nil {
		t.Fatalf("unexpected error for valid email: %v", err)
	}
}

func TestIsValidEmail(t *testing.T) {
	cases := []struct {
		email string
		want  bool
	}{
		{"alice@example.com", true},
		{"a.b+c@sub.example.co", true},
		{"", false},
		{"no-at-sign", false},
		{"@example.com", false},
		{"alice@", false},
		{"alice@b", false},       // domain has no dot
		{"alice@b@c.com", false}, // second '@'
		{"alice@.com", true},     // local/domain non-empty, domain has a dot — accepted by this lightweight check
	}

	for _, c := range cases {
		got := isValidEmail(c.email)
		if got != c.want {
			t.Errorf("isValidEmail(%q) = %v, want %v", c.email, got, c.want)
		}
	}
}
