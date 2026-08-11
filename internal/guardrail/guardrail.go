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
		"aim":                loadStrategy("GUARDRAIL_AIM", "AIM"),
		"lakera":             loadStrategy("GUARDRAIL_LAKERA", "LAKERA"),
		"lumigator":          loadStrategy("GUARDRAIL_LUMIGATOR", "LUMIGATOR"),
		"prisma_airs":        loadStrategy("GUARDRAIL_PRISMA_AIRS", "PRISMA_AIRS"),
		"nvidia_content":     loadStrategy("GUARDRAIL_NVIDIA_CONTENT", "NVIDIA_CONTENT"),
		"openai_moderation":  loadStrategy("GUARDRAIL_OPENAI_MODERATION", "OPENAI_MODERATION"),
		"zscaler":            loadStrategy("GUARDRAIL_ZSCALER", "ZSCALER"),
		"regex_match":        loadStrategy("GUARDRAIL_REGEX_MATCH", "REGEX_MATCH"),
		"contains":           loadStrategy("GUARDRAIL_CONTAINS", "CONTAINS"),
		"contains_code":      loadStrategy("GUARDRAIL_CONTAINS_CODE", "CONTAINS_CODE"),
		"ends_with":          loadStrategy("GUARDRAIL_ENDS_WITH", "ENDS_WITH"),
		"valid_urls":         loadStrategy("GUARDRAIL_VALID_URLS", "VALID_URLS"),
		"json_schema":        loadStrategy("GUARDRAIL_JSON_SCHEMA", "JSON_SCHEMA"),
		"json_keys":          loadStrategy("GUARDRAIL_JSON_KEYS", "JSON_KEYS"),
		"jwt":                loadStrategy("GUARDRAIL_JWT", "JWT"),
		"word_count":         loadStrategy("GUARDRAIL_WORD_COUNT", "WORD_COUNT"),
		"sentence_count":     loadStrategy("GUARDRAIL_SENTENCE_COUNT", "SENTENCE_COUNT"),
		"character_count":    loadStrategy("GUARDRAIL_CHARACTER_COUNT", "CHARACTER_COUNT"),
		"all_uppercase":      loadStrategy("GUARDRAIL_ALL_UPPERCASE", "ALL_UPPERCASE"),
		"all_lowercase":      loadStrategy("GUARDRAIL_ALL_LOWERCASE", "ALL_LOWERCASE"),
		"model_whitelist":    loadStrategy("GUARDRAIL_MODEL_WHITELIST", "MODEL_WHITELIST"),
		"model_rules":        loadStrategy("GUARDRAIL_MODEL_RULES", "MODEL_RULES"),
		"required_metadata_keys": loadStrategy("GUARDRAIL_REQUIRED_METADATA_KEYS", "REQUIRED_METADATA_KEYS"),
		"allowed_request_types":  loadStrategy("GUARDRAIL_ALLOWED_REQUEST_TYPES", "ALLOWED_REQUEST_TYPES"),
		"not_null":           loadStrategy("GUARDRAIL_NOT_NULL", "NOT_NULL"),
		"webhook":            loadStrategy("GUARDRAIL_WEBHOOK", "WEBHOOK"),
		"log":                loadStrategy("GUARDRAIL_LOG", "LOG"),
		"add_prefix":         loadStrategy("GUARDRAIL_ADD_PREFIX", "ADD_PREFIX"),
		"regex_replace":      loadStrategy("GUARDRAIL_REGEX_REPLACE", "REGEX_REPLACE"),
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
