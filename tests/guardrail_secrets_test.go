package tests

import (
	"testing"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
	guardsecrets "github.com/dexterhere04/AgentPlane/internal/guardrail/providers/secrets"
)

func TestSecretsDetectsOpenAIKey(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{Mode: guardrail.ModeEnforce, Enabled: true})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`Here is my API key: sk-proj-abcdefghijklmnopqrstuvwxyz123456`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
	if len(result.Findings) == 0 {
		t.Error("expected findings")
	}
	if result.Findings[0].Type != "openai_api_key" {
		t.Errorf("expected openai_api_key, got %s", result.Findings[0].Type)
	}
}

func TestSecretsDetectsGitHubToken(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`token: ghp_1234567890abcdefghijklmnopqrstuvwxyz1234`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsGitHubOAuth(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`oauth: gho_1234567890abcdefghijklmnopqrstuvwxyz1234`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsGitHubApp(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`app: ghs_1234567890abcdefghijklmnopqrstuvwxyz1234`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsAWSKey(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsStripeKey(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`sk_live_123456789012345678901234567890`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsJWT(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	tok := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Authorization: Bearer "+tok))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsPrivateKey(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsPasswordInURI(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("mongodb://admin:secret123@localhost:27017"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsGoogleAPIKey(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("AIzaSyB-abcdefghijklmnopqrstuvwxyz12345"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsSlackToken(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("xoxb-123456789012-abcdefghijklmnopqrst"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsBasicAuth(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Basic dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODkwYWJjZA=="))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsDetectsBearerToken(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte("Bearer abcdefghijklmnopqrstuvwxyz1234567890abcdefgh"))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsAllowsNormalText(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`What is the capital of France?`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionPass)
}

func TestSecretsOutputDetection(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionOutput, []byte(`My API key is sk-proj-test123456789012345678901234`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsFindingSeverities(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`API: sk-proj-abcdefghijklmnopqrstuvwxyz123456`))
	assertNoError(t, err)
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

func TestSecretsDetectsMultipleFindings(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	body := "AWS: AKIAIOSFODNN7EXAMPLE\nGitHub: ghp_1234567890abcdefghijklmnopqrstuvwxyz1234"
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(body))
	assertNoError(t, err)
	if len(result.Findings) < 2 {
		t.Errorf("expected at least 2 findings, got %d", len(result.Findings))
	}
}

func TestSecretsGenericAPIKeyDetection(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(`api_key: abcdefghijklmnopqrstuvwxyz123456`))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}

func TestSecretsAzureConnectionString(t *testing.T) {
	g := guardsecrets.New(guardrail.Strategy{})
	conn := "DefaultEndpointsProtocol=https;AccountName=test;AccountKey=abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOP1234567890=="
	result, err := g.Evaluate(ctx(), guardrail.DirectionInput, []byte(conn))
	assertNoError(t, err)
	assertDecision(t, result, guardrail.DecisionBlock)
}
