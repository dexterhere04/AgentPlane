package guardrail

import (
	"context"
	"os"
	"strings"
)

type Decision int

const (
	DecisionPass    Decision = iota
	DecisionBlock
	DecisionRedact
	DecisionLogOnly
	DecisionWarn
)

func (d Decision) String() string {
	switch d {
	case DecisionPass:
		return "pass"
	case DecisionBlock:
		return "block"
	case DecisionRedact:
		return "redact"
	case DecisionLogOnly:
		return "log_only"
	case DecisionWarn:
		return "warn"
	default:
		return "unknown"
	}
}

type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeLogOnly Mode = "log_only"
	ModeWarn    Mode = "warn"
	ModeOff     Mode = "off"
)

type GuardrailType int

const (
	TypeMandatory GuardrailType = iota
	TypePolicy
)

func (t GuardrailType) String() string {
	switch t {
	case TypeMandatory:
		return "mandatory"
	case TypePolicy:
		return "policy"
	default:
		return "unknown"
	}
}

type Strategy struct {
	Name    string
	Mode    Mode
	Enabled bool
	Extra   map[string]any
}

type Result struct {
	Guardrail string         `json:"guardrail"`
	Decision  Decision       `json:"decision"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Findings  []Finding      `json:"findings,omitempty"`
	Redacted  []byte         `json:"-"`
}

func (r *Result) AddFinding(f Finding) {
	r.Findings = append(r.Findings, f)
}

type Guardrail interface {
	Name() string
	Type() GuardrailType
	Evaluate(ctx context.Context, dir Direction, body []byte) (*Result, error)
}

type Config struct {
	Strategies map[string]Strategy
}

func (c Config) Strategy(name string) Strategy {
	if s, ok := c.Strategies[name]; ok {
		return s
	}
	return Strategy{Name: name, Mode: ModeOff, Enabled: false}
}

func LoadConfig() Config {
	return Config{
		Strategies: map[string]Strategy{
			"prompt_injection":   loadStrategy("GUARDRAIL_PROMPT_INJECTION", "PROMPT_INJECTION"),
			"secrets":            loadStrategy("GUARDRAIL_SECRETS", "SECRETS"),
			"pii":                loadStrategy("GUARDRAIL_PII", "PII"),
			"content_moderation": loadStrategy("GUARDRAIL_CONTENT_MODERATION", "CONTENT_MODERATION"),
		},
	}
}

func loadStrategy(envKey, legacySuffix string) Strategy {
	mode := os.Getenv(envKey)
	if mode == "" {
		mode = os.Getenv("GUARDRAIL_" + legacySuffix + "_MODE")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))

	s := Strategy{Name: strings.ToLower(strings.TrimPrefix(envKey, "GUARDRAIL_")), Enabled: true}

	switch Mode(mode) {
	case ModeEnforce, ModeLogOnly, ModeWarn:
		s.Mode = Mode(mode)
	case ModeOff, "":
		s.Mode = ModeOff
		s.Enabled = false
	default:
		s.Mode = ModeEnforce
	}

	return s
}
