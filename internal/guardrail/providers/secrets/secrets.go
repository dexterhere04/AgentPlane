package secrets

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/zricethezav/gitleaks/v8/detect"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type SecretsEngine string

const (
	EngineGitleaks SecretsEngine = "gitleaks"
	EngineRegex    SecretsEngine = "regex"
)

var (
	detectorOnce sync.Once
	detectorInst *detect.Detector
	detectorErr  error
)

func getDetector() (*detect.Detector, error) {
	detectorOnce.Do(func() {
		detectorInst, detectorErr = detect.NewDetectorDefaultConfig()
	})
	return detectorInst, detectorErr
}

type SecretsGuardrail struct {
	engine SecretsEngine
}

func resolveEngine(strategy guardrail.Strategy) SecretsEngine {
	if v, ok := strategy.Extra["engine"]; ok {
		if e, ok := v.(string); ok && e == "regex" {
			return EngineRegex
		}
	}
	if env := strings.ToLower(os.Getenv("SECRETS_ENGINE")); env == "regex" {
		return EngineRegex
	}
	return EngineGitleaks
}

func New(strategy guardrail.Strategy) *SecretsGuardrail {
	return &SecretsGuardrail{engine: resolveEngine(strategy)}
}

func (g *SecretsGuardrail) Name() string {
	return "secrets"
}

func (g *SecretsGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	return g.Detect(context.Background(), string(body))
}

func (g *SecretsGuardrail) Detect(ctx context.Context, content string) (*guardrail.Result, error) {
	if g.engine == EngineRegex {
		return g.detectWithRegex(ctx, content)
	}
	return g.detectWithGitleaks(ctx, content)
}

func (g *SecretsGuardrail) detectWithGitleaks(ctx context.Context, content string) (*guardrail.Result, error) {
	det, err := getDetector()
	if err != nil {
		return nil, fmt.Errorf("gitleaks detector init: %w", err)
	}

	rawFindings := det.DetectString(content)

	if len(rawFindings) == 0 {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	findings := make([]guardrail.Finding, 0, len(rawFindings))
	searchOffset := 0

	for _, gf := range rawFindings {
		secretSnippet := gf.Secret
		if len(secretSnippet) > 32 {
			secretSnippet = secretSnippet[:16] + "..." + secretSnippet[len(secretSnippet)-8:]
		}

		matchStr := strings.TrimSpace(gf.Match)
		start, end := findMatch(content, matchStr, searchOffset)
		if start < 0 {
			searchOffset = end
			start = searchOffset
			end = searchOffset
		} else {
			searchOffset = end
		}

		severity := classifyGitleaksSeverity(gf.RuleID, gf.Tags)

		findings = append(findings, guardrail.Finding{
			Guardrail: g.Name(),
			Type:      gf.RuleID,
			Severity:  severity,
			Start:     start,
			End:       end,
			Entity:    "secret",
			Value:     secretSnippet,
		})
	}

	return &guardrail.Result{
		Guardrail: g.Name(),
		Decision:  guardrail.DecisionBlock,
		Message:   fmt.Sprintf("detected %d potential secret(s)", len(findings)),
		Findings:  findings,
	}, nil
}

func findMatch(content, match string, from int) (int, int) {
	if match == "" {
		return -1, from
	}
	idx := strings.Index(content[from:], match)
	if idx < 0 {
		return -1, from
	}
	absIdx := from + idx
	return absIdx, absIdx + len(match)
}

func classifyGitleaksSeverity(ruleID string, tags []string) guardrail.Severity {
	highRules := map[string]bool{
		"aws-access-key":           true,
		"aws-secret-access-key":    true,
		"openai-api-key":           true,
		"github-pat":               true,
		"github-oauth":             true,
		"github-app-token":         true,
		"github-refresh-token":     true,
		"github-fine-grained-pat":  true,
		"google-api-key":           true,
		"google-cloud-platform-service-account": true,
		"stripe-access-token":      true,
		"slack-bot-token":          true,
		"slack-user-token":         true,
		"slack-app-token":          true,
		"slack-webhook-url":        true,
		"private-key":              true,
		"pgp-private-key":          true,
		"generic-api-key":          true,
		"azure-storage-account-key":     true,
		"sendgrid-api-key":         true,
		"heroku-api-key":           true,
		"password-in-url":          true,
		"discord-bot-token":        true,
		"gitlab-pat":               true,
		"jwt":                      true,
		"auth0-oauth2-client-secret": true,
		"microsoft-teams-webhook":  true,
		"twilio-api-key":           true,
	}

	for _, t := range tags {
		if t == "critical" {
			return guardrail.SeverityCritical
		}
	}

	if highRules[ruleID] {
		return guardrail.SeverityCritical
	}

	mediumRules := map[string]bool{
		"discord-api-key":     true,
		"discord-client-secret": true,
	}
	if mediumRules[ruleID] {
		return guardrail.SeverityMedium
	}

	return guardrail.SeverityHigh
}

var gitleaksPatterns = []struct {
	name     string
	severity guardrail.Severity
	pattern  *regexp.Regexp
}{
	{
		name:     "openai_api_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9-_]{20,}`),
	},
	{
		name:     "github_pat",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`),
	},
	{
		name:     "github_oauth",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`gho_[A-Za-z0-9_]{36}`),
	},
	{
		name:     "github_app_token",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(ghu|ghs)_[A-Za-z0-9_]{36}`),
	},
	{
		name:     "github_refresh",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`ghr_[A-Za-z0-9_]{36}`),
	},
	{
		name:     "aws_access_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?:A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`),
	},
	{
		name:     "aws_secret_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?i)aws.{0,20}(?:key|pwd|pw|password|pass|token).{0,20}["\s:=]+['"]?([0-9a-zA-Z/+]{40})['"]?`),
	},
	{
		name:     "google_api_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
	},
	{
		name:     "google_oauth_id",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\.apps\.googleusercontent\.com`),
	},
	{
		name:     "slack_token",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`xox[baprs]-[0-9A-Za-z\-_]{10,}`),
	},
	{
		name:     "slack_webhook",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`https://hooks\.slack\.com/services/T[0-9A-Za-z_]{8,}/B[0-9A-Za-z_]{8,}/[0-9A-Za-z_]{23}`),
	},
	{
		name:     "jwt_token",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	},
	{
		name:     "private_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`-----BEGIN\s+(?:RSA\s+|EC\s+|DSA\s+|OPENSSH\s+)?PRIVATE\s+KEY-----`),
	},
	{
		name:     "pgp_private_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`-----BEGIN\s+PGP\s+PRIVATE\s+KEY\s+BLOCK-----`),
	},
	{
		name:     "generic_api_key",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`(?i)(api[-_]?key|apikey|api[-_]?secret)\s*[:=]\s*['"]?[A-Za-z0-9+/=_-]{16,}['"]?`),
	},
	{
		name:     "azure_storage_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?i)(DefaultEndpointsProtocol=https;AccountName=[^;]+;AccountKey=[^;]+)|\b(AccountKey=[A-Za-z0-9+/=]{60,88})\b`),
	},
	{
		name:     "azure_sas_token",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`(?i)(sig=[0-9a-z%]{40,})`),
	},
	{
		name:     "heroku_api_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?i)heroku.{0,20}(?:key|token).{0,20}[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`),
	},
	{
		name:     "stripe_live_key",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?:sk|rk)_live_[0-9a-zA-Z]{24,}`),
	},
	{
		name:     "stripe_test_key",
		severity: guardrail.SeverityLow,
		pattern:  regexp.MustCompile(`(?:sk|rk)_test_[0-9a-zA-Z]{24,}`),
	},
	{
		name:     "bearer_token_in_header",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]+=*`),
	},
	{
		name:     "basic_auth",
		severity: guardrail.SeverityMedium,
		pattern:  regexp.MustCompile(`(?i)basic\s+[A-Za-z0-9+/=]{20,}`),
	},
	{
		name:     "password_in_uri",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?i)(?:mongodb|postgres|mysql|redis|http|ftp)://[^:]+:[^@]+@`),
	},
	{
		name:     "discord_webhook",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`https://discord(?:app)?\.com/api/webhooks/[0-9]+/[A-Za-z0-9_-]+`),
	},
	{
		name:     "sendgrid_api_key",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
	},
	{
		name:     "twilio_sid",
		severity: guardrail.SeverityMedium,
		pattern:  regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
	},
	{
		name:     "twilio_auth_token",
		severity: guardrail.SeverityCritical,
		pattern:  regexp.MustCompile(`(?i)twilio.{0,20}(?:auth.?token|key).{0,20}["\s:=]+['"]?([0-9a-fA-F]{32})['"]?`),
	},
	{
		name:     "password_assignment",
		severity: guardrail.SeverityHigh,
		pattern:  regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token)\s*[:=]\s*["'][^"'\s]{8,}["']`),
	},
}

func (g *SecretsGuardrail) detectWithRegex(ctx context.Context, content string) (*guardrail.Result, error) {
	var findings []guardrail.Finding

	for _, sp := range gitleaksPatterns {
		matches := sp.pattern.FindAllStringIndex(content, -1)
		for _, loc := range matches {
			start, end := loc[0], loc[1]
			matchStr := content[start:end]
			snippet := matchStr
			if len(snippet) > 32 {
				snippet = matchStr[:16] + "..." + matchStr[len(matchStr)-8:]
			}
			findings = append(findings, guardrail.Finding{
				Guardrail: g.Name(),
				Type:      sp.name,
				Severity:  sp.severity,
				Start:     start,
				End:       end,
				Entity:    "secret",
				Value:     snippet,
			})
		}
	}

	if len(findings) == 0 {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	return &guardrail.Result{
		Guardrail: g.Name(),
		Decision:  guardrail.DecisionBlock,
		Message:   fmt.Sprintf("detected %d potential secret(s)", len(findings)),
		Findings:  findings,
	}, nil
}
