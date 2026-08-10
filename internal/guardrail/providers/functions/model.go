package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type JWTGuardrail struct {
	name string
}

func NewJWT(s guardrail.Strategy) *JWTGuardrail {
	return &JWTGuardrail{name: s.Name}
}

func (g *JWTGuardrail) Name() string               { return g.name }
func (g *JWTGuardrail) Type() guardrail.GuardrailType { return guardrail.TypePolicy }

func (g *JWTGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	jwts := jwtPattern.FindAllString(string(body), -1)
	if len(jwts) == 0 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var findings []guardrail.Finding
	for i, token := range jwts {
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			findings = append(findings, guardrail.Finding{
				Guardrail: g.name,
				Type:      "invalid_jwt_structure",
				Severity:  guardrail.SeverityHigh,
				Entity:    "jwt",
				Start:     -1,
				End:       -1,
				Value:     token[:intMin(20, len(token))],
			})
			continue
		}
		_ = i
		for _, part := range parts[:2] {
			if !isBase64URL(part) {
				findings = append(findings, guardrail.Finding{
					Guardrail: g.name,
					Type:      "invalid_jwt_encoding",
					Severity:  guardrail.SeverityMedium,
					Entity:    "jwt",
					Start:     -1,
					End:       -1,
					Value:     part[:intMin(20, len(part))],
				})
			}
		}
	}
	if len(findings) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("%d JWT token(s) failed validation", len(findings)),
			Findings:  findings,
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

func isBase64URL(s string) bool {
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return len(s) > 0
}

type ModelWhitelistGuardrail struct {
	name      string
	whitelist []string
}

func NewModelWhitelist(s guardrail.Strategy) *ModelWhitelistGuardrail {
	list := getStringSlice(s.Extra, "models")
	if len(list) == 0 {
		raw := os.Getenv("MODEL_WHITELIST")
		if raw != "" {
			for _, m := range strings.Split(raw, ",") {
				list = append(list, strings.TrimSpace(m))
			}
		}
	}
	return &ModelWhitelistGuardrail{name: s.Name, whitelist: list}
}

func (g *ModelWhitelistGuardrail) Name() string               { return g.name }
func (g *ModelWhitelistGuardrail) Type() guardrail.GuardrailType { return guardrail.TypeMandatory }

func (g *ModelWhitelistGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.whitelist) == 0 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	model := extractModel(body)
	if model == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	for _, allowed := range g.whitelist {
		if strings.EqualFold(model, allowed) {
			return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
		}
	}
	return &guardrail.Result{
		Guardrail: g.name,
		Decision:  guardrail.DecisionBlock,
		Message:   fmt.Sprintf("model %q is not in the approved whitelist", model),
	}, nil
}

func extractModel(body []byte) string {
	if !json.Valid(body) {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	if m, ok := doc["model"].(string); ok {
		return m
	}
	return ""
}

type ModelRulesGuardrail struct {
	name  string
	rules map[string]map[string]any
}

func NewModelRules(s guardrail.Strategy) *ModelRulesGuardrail {
	var rules map[string]map[string]any
	if raw, ok := s.Extra["rules"]; ok {
		if m, ok := raw.(map[string]any); ok {
			rules = make(map[string]map[string]any)
			for k, v := range m {
				if inner, ok := v.(map[string]any); ok {
					rules[k] = inner
				}
			}
		}
	}
	if rules == nil {
		raw := os.Getenv("MODEL_RULES")
		if raw != "" {
			json.Unmarshal([]byte(raw), &rules)
		}
	}
	return &ModelRulesGuardrail{name: s.Name, rules: rules}
}

func (g *ModelRulesGuardrail) Name() string               { return g.name }
func (g *ModelRulesGuardrail) Type() guardrail.GuardrailType { return guardrail.TypeMandatory }

func (g *ModelRulesGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.rules) == 0 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	model := extractModel(body)
	if model == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	modelRules, ok := g.rules[model]
	if !ok {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	if disabled, _ := modelRules["disabled"].(bool); disabled {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("model %q is disabled by rules", model),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type AllowedRequestTypesGuardrail struct {
	name  string
	types []string
}

func NewAllowedRequestTypes(s guardrail.Strategy) *AllowedRequestTypesGuardrail {
	types := getStringSlice(s.Extra, "types")
	if len(types) == 0 {
		raw := os.Getenv("ALLOWED_REQUEST_TYPES")
		if raw != "" {
			for _, t := range strings.Split(raw, ",") {
				types = append(types, strings.TrimSpace(t))
			}
		}
	}
	return &AllowedRequestTypesGuardrail{name: s.Name, types: types}
}

func (g *AllowedRequestTypesGuardrail) Name() string               { return g.name }
func (g *AllowedRequestTypesGuardrail) Type() guardrail.GuardrailType { return guardrail.TypeMandatory }

func (g *AllowedRequestTypesGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.types) == 0 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	if !json.Valid(body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var doc map[string]any
	json.Unmarshal(body, &doc)
	endpoint, _ := doc["endpoint"].(string)
	if endpoint == "" {
		endpoint, _ = doc["type"].(string)
	}
	if endpoint == "" {
		endpoint, _ = doc["object"].(string)
	}
	if endpoint == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	for _, allowed := range g.types {
		if strings.EqualFold(endpoint, allowed) {
			return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
		}
	}
	return &guardrail.Result{
		Guardrail: g.name,
		Decision:  guardrail.DecisionBlock,
		Message:   fmt.Sprintf("request type %q is not allowed", endpoint),
	}, nil
}

type WebhookGuardrail struct {
	name       string
	webhookURL string
	httpClient *http.Client
}

func NewWebhook(s guardrail.Strategy) *WebhookGuardrail {
	url := getString(s.Extra, "url", "")
	if url == "" {
		url = os.Getenv("WEBHOOK_GUARD_URL")
	}
	return &WebhookGuardrail{
		name:       s.Name,
		webhookURL: url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (g *WebhookGuardrail) Name() string               { return g.name }
func (g *WebhookGuardrail) Type() guardrail.GuardrailType { return guardrail.TypePolicy }

func (g *WebhookGuardrail) Evaluate(ctx context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.webhookURL == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	reqBody := map[string]any{"content": string(body)}
	reqBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.webhookURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: send request: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Blocked bool   `json:"blocked"`
		Reason  string `json:"reason,omitempty"`
	}
	json.Unmarshal(respBytes, &result)
	if result.Blocked {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("webhook blocked: %s", result.Reason),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type LogGuardrail struct {
	name string
}

func NewLog(s guardrail.Strategy) *LogGuardrail {
	return &LogGuardrail{name: s.Name}
}

func (g *LogGuardrail) Name() string               { return g.name }
func (g *LogGuardrail) Type() guardrail.GuardrailType { return guardrail.TypePolicy }

func (g *LogGuardrail) Evaluate(_ context.Context, dir guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(body) > 500 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionPass,
			Message:   fmt.Sprintf("[%s] logged %d bytes", dir, len(body)),
		}, nil
	}
	return &guardrail.Result{
		Guardrail: g.name,
		Decision:  guardrail.DecisionPass,
		Message:   fmt.Sprintf("[%s] %s", dir, string(body)),
	}, nil
}

type AddPrefixGuardrail struct {
	name   string
	prefix string
}

func NewAddPrefix(s guardrail.Strategy) *AddPrefixGuardrail {
	prefix := getString(s.Extra, "prefix", "")
	if prefix == "" {
		prefix = os.Getenv("ADD_PREFIX_TEXT")
	}
	return &AddPrefixGuardrail{name: s.Name, prefix: prefix}
}

func (g *AddPrefixGuardrail) Name() string               { return g.name }
func (g *AddPrefixGuardrail) Type() guardrail.GuardrailType { return guardrail.TypePolicy }

func (g *AddPrefixGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.prefix == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	newBody := append([]byte(g.prefix), body...)
	return &guardrail.Result{
		Guardrail: g.name,
		Decision:  guardrail.DecisionRedact,
		Message:   "added prefix",
		Redacted:  newBody,
	}, nil
}

type RegexReplaceGuardrail struct {
	name        string
	searchPat   *regexp.Regexp
	replacement string
}

func NewRegexReplace(s guardrail.Strategy) *RegexReplaceGuardrail {
	pat := getString(s.Extra, "pattern", "")
	if pat == "" {
		pat = os.Getenv("REGEX_REPLACE_PATTERN")
	}
	repl := getString(s.Extra, "replacement", "")
	if repl == "" {
		repl = os.Getenv("REGEX_REPLACE_REPLACEMENT")
	}
	var r *regexp.Regexp
	if pat != "" {
		r = regexp.MustCompile(pat)
	}
	return &RegexReplaceGuardrail{name: s.Name, searchPat: r, replacement: repl}
}

func (g *RegexReplaceGuardrail) Name() string               { return g.name }
func (g *RegexReplaceGuardrail) Type() guardrail.GuardrailType { return guardrail.TypePolicy }

func (g *RegexReplaceGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.searchPat == nil {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	result := g.searchPat.ReplaceAll(body, []byte(g.replacement))
	if bytes.Equal(result, body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	return &guardrail.Result{
		Guardrail: g.name,
		Decision:  guardrail.DecisionRedact,
		Message:   fmt.Sprintf("regex replacement applied: %s → %q", g.searchPat.String(), g.replacement),
		Redacted:  result,
	}, nil
}
