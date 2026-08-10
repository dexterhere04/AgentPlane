package tests

import (
	"strings"
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	guardpii "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/pii"
)

func TestPIIDetectsEmail(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`Contact me at john.doe@example.com`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)

	redacted := string(result.Redacted)
	if strings.Contains(redacted, "john.doe@example.com") {
		t.Error("email should have been redacted")
	}
	if !strings.Contains(redacted, "[EMAIL REDACTED]") {
		t.Error("expected [EMAIL REDACTED] placeholder")
	}
}

func TestPIIDetectsPhone(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`Call me at 555-123-4567`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)

	redacted := string(result.Redacted)
	if strings.Contains(redacted, "555-123-4567") {
		t.Error("phone should have been redacted")
	}
	if !strings.Contains(redacted, "[PHONE REDACTED]") {
		t.Error("expected [PHONE REDACTED] placeholder")
	}
}

func TestPIIDetectsSSN(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`My SSN is 123-45-6789`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)
}

func TestPIIDetectsCreditCard(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`CC: 4111111111111111`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)
}

func TestPIIDetectsIPAddress(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`IP: 192.168.1.1`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)
}

func TestPIIAllowsCleanText(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`What is the weather like today?`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionPass)
}

func TestPIIOutputRedaction(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionOutput, []byte(`The user's email is test@example.com`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionRedact)
}

func TestPIIIsMandatory(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	if g.Type() != guardrail.TypeMandatory {
		t.Error("pii should be TypeMandatory")
	}
}

func TestPIIFindingsHavePositions(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`Email: test@example.com`))
	assertNoError(t, err)

	for _, f := range result.Findings {
		if f.Start < 0 || f.End <= f.Start {
			t.Errorf("invalid position: start=%d end=%d", f.Start, f.End)
		}
		if f.Severity == "" {
			t.Error("finding should have severity")
		}
	}
}

func TestPIIMultipleFindings(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	body := `Email: alice@example.com, Phone: 555-123-4567, SSN: 123-45-6789`
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(body))
	assertNoError(t, err)
	if len(result.Findings) < 3 {
		t.Errorf("expected at least 3 findings, got %d", len(result.Findings))
	}
}

func TestPIIFindingsDetails(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`Email: alice@example.com`))
	assertNoError(t, err)
	if result.Details == nil {
		t.Fatal("expected details")
	}
	if result.Details["total_findings"].(int) == 0 {
		t.Error("expected total_findings > 0")
	}
	if count, ok := result.Details["email"]; !ok || count.(int) != 1 {
		t.Errorf("expected email count 1, got %v", result.Details["email"])
	}
}

func TestPIIMultipleEmailFormats(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	emails := []string{
		"alice@example.com",
		"bob.smith@company.co.uk",
		"test+filter@domain.org",
	}
	for _, email := range emails {
		result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(email))
		assertNoError(t, err)
		if result.Decision != guardrail.DecisionRedact {
			t.Errorf("expected redact for %s, got %s", email, result.Decision)
		}
		if !strings.Contains(string(result.Redacted), "[EMAIL REDACTED]") {
			t.Errorf("email %s not redacted", email)
		}
	}
}

func TestPIIMultiplePhoneFormats(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	phones := []string{"555-123-4567", "(800) 555-0199", "1-555-867-5309"}
	for _, phone := range phones {
		result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(phone))
		assertNoError(t, err)
		if result.Decision != guardrail.DecisionRedact {
			t.Errorf("expected redact for %s, got %s", phone, result.Decision)
		}
	}
}

func TestPIIMultipleCreditCards(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	cards := map[string]string{
		"visa":       "4111111111111111",
		"mastercard": "5500000000000004",
		"amex":       "340000000000009",
	}
	for label, num := range cards {
		result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(num))
		assertNoError(t, err)
		if result.Decision != guardrail.DecisionRedact {
			t.Errorf("expected redact for %s (%s), got %s", label, num, result.Decision)
		}
	}
}

func TestPIIRedactPreservesNonPII(t *testing.T) {
	g := guardpii.New(guardrail.Strategy{})
	body := "My name is John and my email is alice@example.com. Please reply."
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(body))
	assertNoError(t, err)
	redacted := string(result.Redacted)
	if !strings.Contains(redacted, "My name is John") {
		t.Error("non-PII should be preserved")
	}
	if !strings.Contains(redacted, "Please reply") {
		t.Error("non-PII should be preserved")
	}
	if !strings.Contains(redacted, "[EMAIL REDACTED]") {
		t.Error("email should be redacted")
	}
}
