package tests

import (
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
)

func newGitleaksGuard() *guardsecrets.SecretsGuardrail {
	return guardsecrets.New(guardrail.Strategy{
		Mode:    guardrail.ModeEnforce,
		Enabled: true,
		Extra:   map[string]any{"engine": "gitleaks"},
	})
}

func newRegexSecretsGuard() *guardsecrets.SecretsGuardrail {
	return guardsecrets.New(guardrail.Strategy{
		Mode:    guardrail.ModeEnforce,
		Enabled: true,
		Extra:   map[string]any{"engine": "regex"},
	})
}

func newGitleaksGuardForChat() *guardsecrets.SecretsGuardrail {
	return guardsecrets.New(guardrail.Strategy{
		Mode:    guardrail.ModeEnforce,
		Enabled: true,
		Extra:   map[string]any{"engine": "gitleaks"},
	})
}

func newRegexSecretsGuardForChat() *guardsecrets.SecretsGuardrail {
	return guardsecrets.New(guardrail.Strategy{
		Mode:    guardrail.ModeEnforce,
		Enabled: true,
		Extra:   map[string]any{"engine": "regex"},
	})
}

var openAIKeyGitleaks = "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"

var githubPAT = "ghp_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH2jK4mN5oP"

var githubOAuth = "gho_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH2jK4mN5oP"

var githubApp = "ghs_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH2jK4mN5oP"

var githubRefresh = "ghr_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH2jK4mN5oP"

var highEntropyKey = "X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH"

var jwtToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

var privateKeyPEM = "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCkL9mN3pQ2rS5t\nV8wYzA1B4cD6eF0gH2jK4mN5oPqR6sT7uV8wX9yZ0aB1cD2eF3gH4iJ5kL6mN7\noP8qR9sT0uV1wX2yZ3aB4cD5eF6gH7iJ8kL9mN0oP1qR2sT3uV4wX5yZ6aB7cD\n-----END PRIVATE KEY-----"

var ecKeyPEM = "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIOkL9mN3pQ2rS5tV8wYzA1B4cD6eF0gH2jK4mN5oPqR6sT7uV8wX9yZ\n0aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4cD5eF6g\n-----END EC PRIVATE KEY-----"

var stripeLiveKey = "sk_live_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH"

var stripeTestKey = "sk_test_X7kL9mN3pQ2rS5tV8wY1aB4cD6eF0gH"


func TestSecrets_Gitleaks_OpenAIKey(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Here is my API key: "+openAIKeyGitleaks))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
	if len(result.Findings) == 0 {
		t.Error("expected findings")
	}
}

func TestSecrets_Gitleaks_GitHubPAT(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("token: "+githubPAT))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_GitHubOAuth(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("oauth: "+githubOAuth))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_GitHubApp(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("app: "+githubApp))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_GitHubRefresh(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("refresh: "+githubRefresh))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_StripeKey(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(stripeLiveKey))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_JWT(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Authorization: Bearer "+jwtToken))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_PrivateKey(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(privateKeyPEM))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_ECPrivateKey(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(ecKeyPEM))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_GenericAPIKey(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("api_key: "+highEntropyKey))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_SlackToken(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("xoxb-123456789012-1234567890123abcdefghijklm"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_NormalText(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("What is the capital of France?"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionPass)
}

func TestSecrets_Gitleaks_OutputDetection(t *testing.T) {
	g := newGitleaksGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionOutput, []byte("My API key is "+openAIKeyGitleaks))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Gitleaks_MultipleFindings(t *testing.T) {
	g := newGitleaksGuard()
	body := "AWS: AKIAX7KL9MN3PQ2RS5TV8\nGitHub: " + githubPAT
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(body))
	assertNoError(t, err)
	if len(result.Findings) < 1 {
		t.Errorf("expected at least 1 finding, got %d", len(result.Findings))
	}
}

func TestSecrets_Gitleaks_FindingsHavePosition(t *testing.T) {
	g := newGitleaksGuard()
	body := []byte("My key: " + openAIKeyGitleaks)
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, body)
	assertNoError(t, err)
	if len(result.Findings) == 0 {
		t.Fatal("expected findings")
	}
	for _, f := range result.Findings {
		if f.Severity == "" {
			t.Error("finding should have severity")
		}
		if f.Start < 0 || f.End <= f.Start {
			t.Errorf("invalid position: start=%d end=%d", f.Start, f.End)
		}
		if f.Value == "" {
			t.Error("finding should have truncated value")
		}
	}
}


func TestSecrets_Regex_OpenAIKey(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Here is my API key: sk-proj-abcdefghijklmnopqrstuvwxyz123456"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
	if len(result.Findings) == 0 {
		t.Error("expected findings")
	}
	if result.Findings[0].Type != "openai_api_key" {
		t.Errorf("expected openai_api_key, got %s", result.Findings[0].Type)
	}
}

func TestSecrets_Regex_GitHubToken(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("token: ghp_1234567890abcdefghijklmnopqrstuvwxyz1234"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_AWSKey(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_GoogleAPIKey(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("AIzaSyB-abcdefghijklmnopqrstuvwxyz12345"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_BasicAuth(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Basic dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODkwYWJjZA=="))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_BearerToken(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Bearer abcdefghijklmnopqrstuvwxyz1234567890abcdefgh"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_PasswordInURI(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("mongodb://admin:secret123@localhost:27017"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_AzureConnectionString(t *testing.T) {
	g := newRegexSecretsGuard()
	conn := "DefaultEndpointsProtocol=https;AccountName=test;AccountKey=abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOP1234567890=="
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(conn))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_PasswordAssignment(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("password: 'MySecretPassword123'"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_HerokuKey(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("heroku key: 12345678-90ab-cdef-1234-567890abcdef"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_DiscordWebhook(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz012345"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecrets_Regex_StripeTestKey(t *testing.T) {
	g := newRegexSecretsGuard()
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("sk_test_123456789012345678901234567890"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
	for _, f := range result.Findings {
		if f.Type == "stripe_test_key" && f.Severity != guardrail.SeverityLow {
			t.Errorf("expected low severity for stripe test key, got %s", f.Severity)
		}
	}
}

func TestSecrets_Regex_MultipleFindings(t *testing.T) {
	g := newRegexSecretsGuard()
	body := "AWS: AKIAIOSFODNN7EXAMPLE\nGitHub: ghp_1234567890abcdefghijklmnopqrstuvwxyz1234"
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(body))
	assertNoError(t, err)
	if len(result.Findings) < 2 {
		t.Errorf("expected at least 2 findings, got %d", len(result.Findings))
	}
}


func TestSecrets_E2E_Gitleaks_OpenAIKey(t *testing.T) {
	w := doChat(wrapInChat("OpenAI key: "+openAIKeyGitleaks), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_GitHubPAT(t *testing.T) {
	w := doChat(wrapInChat("GitHub token: "+githubPAT), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_StripeKey(t *testing.T) {
	w := doChat(wrapInChat("Stripe: "+stripeLiveKey), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_SlackToken(t *testing.T) {
	w := doChat(wrapInChat("Slack: xoxb-123456789012-1234567890123abcdefghijklm"), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_JWT(t *testing.T) {
	w := doChat(wrapInChat("Bearer "+jwtToken), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_PrivateKey(t *testing.T) {
	w := doChat(wrapInChat("Key:\n"+privateKeyPEM), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Gitleaks_CleanText(t *testing.T) {
	g := newGitleaksGuardForChat()
	result, _ := g.Evaluate(nil, guardrail.DirectionInput, wrapInChat("What is the capital of France?"))
	if result == nil || result.Decision != guardrail.DecisionPass {
		t.Error("clean text should pass secrets detection")
	}
}

func TestSecrets_E2E_Gitleaks_GenericAPIKey(t *testing.T) {
	w := doChat(wrapInChat("api_key: "+highEntropyKey), newGitleaksGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Regex_OpenAIKey(t *testing.T) {
	w := doChat(wrapInChat("OpenAI key: sk-proj-abcdefghijklmnopqrstuvwxyz123456"), newRegexSecretsGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Regex_AWSKey(t *testing.T) {
	w := doChat(wrapInChat("AWS key: AKIA1234567890ABCDEF"), newRegexSecretsGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Regex_PasswordInURI(t *testing.T) {
	w := doChat(wrapInChat("Connect to mongodb://admin:secretpassword@localhost:27017"), newRegexSecretsGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Regex_BasicAuth(t *testing.T) {
	w := doChat(wrapInChat("Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODkw"), newRegexSecretsGuardForChat())
	assertBlocked(t, w)
}

func TestSecrets_E2E_Regex_BearerToken(t *testing.T) {
	w := doChat(wrapInChat("Authorization: Bearer abcdefghijklmnopqrstuvwxyz1234567890"), newRegexSecretsGuardForChat())
	assertBlocked(t, w)
}
