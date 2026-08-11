package functions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type RegexMatchGuardrail struct {
	name    string
	pattern *regexp.Regexp
}

func NewRegexMatch(s guardrail.Strategy) *RegexMatchGuardrail {
	pat := getString(s.Extra, "pattern", "")
	if pat == "" {
		pat = os.Getenv("REGEX_MATCH_PATTERN")
	}
	var r *regexp.Regexp
	if pat != "" {
		var err error
		r, err = regexp.Compile(pat)
		if err != nil {
			log.Printf("regex_match: invalid pattern %q: %v", pat, err)
		}
	}
	return &RegexMatchGuardrail{name: s.Name, pattern: r}
}

func (g *RegexMatchGuardrail) Name() string              { return g.name }

func (g *RegexMatchGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.pattern == nil {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	if g.pattern.Match(body) {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("regex match: %s", g.pattern.String()),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type ContainsGuardrail struct {
	name  string
	words []string
}

func NewContains(s guardrail.Strategy) *ContainsGuardrail {
	words := getStringSlice(s.Extra, "words")
	if len(words) == 0 {
		raw := os.Getenv("CONTAINS_WORDS")
		if raw != "" {
			for _, w := range strings.Split(raw, ",") {
				words = append(words, strings.TrimSpace(w))
			}
		}
	}
	return &ContainsGuardrail{name: s.Name, words: words}
}

func (g *ContainsGuardrail) Name() string              { return g.name }

func (g *ContainsGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.words) == 0 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	lower := strings.ToLower(string(body))
	var matches []string
	for _, w := range g.words {
		if strings.Contains(lower, strings.ToLower(w)) {
			matches = append(matches, w)
		}
	}
	if len(matches) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("forbidden content matched: %v", matches),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

var codePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:def|function|func|fn|class|struct|interface|enum|impl|trait|module|namespace|package)\s+\w+`),
	regexp.MustCompile(`\b(?:import|require|include|from|use)\s+[\w."'/]`),
	regexp.MustCompile(`\b(?:SELECT|INSERT|UPDATE|DELETE|DROP|ALTER|CREATE)\s+(?:FROM|INTO|SET|TABLE|DATABASE)`),
	regexp.MustCompile(`\b(?:eval|exec|system|subprocess|os\.system|Runtime\.exec|ProcessBuilder)\s*\(`),
	regexp.MustCompile(`(?m)^\s*(?:#!|#!/)`),
	regexp.MustCompile(`\b(?:try|except|catch|finally|throw|raise)\s*[{:]`),
}

type ContainsCodeGuardrail struct {
	name string
}

func NewContainsCode(s guardrail.Strategy) *ContainsCodeGuardrail {
	return &ContainsCodeGuardrail{name: s.Name}
}

func (g *ContainsCodeGuardrail) Name() string              { return g.name }

func (g *ContainsCodeGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	text := string(body)
	var matchedPatterns []string
	for _, p := range codePatterns {
		if p.MatchString(text) {
			matchedPatterns = append(matchedPatterns, p.String())
		}
	}
	if len(matchedPatterns) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("code injection detected (%d patterns matched)", len(matchedPatterns)),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type EndsWithGuardrail struct {
	name    string
	suffix  string
}

func NewEndsWith(s guardrail.Strategy) *EndsWithGuardrail {
	suffix := getString(s.Extra, "suffix", "")
	if suffix == "" {
		suffix = os.Getenv("ENDSWITH_SUFFIX")
	}
	return &EndsWithGuardrail{name: s.Name, suffix: suffix}
}

func (g *EndsWithGuardrail) Name() string              { return g.name }

func (g *EndsWithGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.suffix == "" {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	text := strings.TrimSpace(string(body))
	if strings.HasSuffix(text, g.suffix) {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("content ends with forbidden suffix: %q", g.suffix),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type ValidUrlsGuardrail struct {
	name string
}

func NewValidUrls(s guardrail.Strategy) *ValidUrlsGuardrail {
	return &ValidUrlsGuardrail{name: s.Name}
}

func (g *ValidUrlsGuardrail) Name() string              { return g.name }

var urlPattern = regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)

func (g *ValidUrlsGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	urls := urlPattern.FindAllString(string(body), -1)
	var invalid []string
	for _, raw := range urls {
		if _, err := url.Parse(raw); err != nil {
			invalid = append(invalid, raw)
		}
	}
	if len(invalid) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("invalid URLs detected: %v", invalid),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type JsonSchemaGuardrail struct {
	name   string
	schema map[string]any
}

func NewJsonSchema(s guardrail.Strategy) *JsonSchemaGuardrail {
	var schema map[string]any
	if raw, ok := s.Extra["schema"]; ok {
		if m, ok := raw.(map[string]any); ok {
			schema = m
		}
	}
	if schema == nil {
		schemaRaw := os.Getenv("JSON_SCHEMA")
		if schemaRaw != "" {
			json.Unmarshal([]byte(schemaRaw), &schema)
		}
	}
	return &JsonSchemaGuardrail{name: s.Name, schema: schema}
}

func (g *JsonSchemaGuardrail) Name() string              { return g.name }

func (g *JsonSchemaGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if g.schema == nil || !json.Valid(body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionBlock, Message: "invalid JSON"}, nil
	}
	if errors := validateSchema(doc, g.schema); len(errors) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("schema validation failed: %v", errors),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

func validateSchema(doc any, schema map[string]any) []string {
	var errs []string
	if typ, ok := schema["type"].(string); ok && typ == "object" {
		obj, ok := doc.(map[string]any)
		if !ok {
			return []string{"expected object"}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for key, propSchema := range props {
				if propMap, ok := propSchema.(map[string]any); ok {
					val, exists := obj[key]
					if propType, ok := propMap["type"].(string); ok {
						if !exists {
							errs = append(errs, fmt.Sprintf("missing required field: %s", key))
							continue
						}
						if !typeMatches(val, propType) {
							errs = append(errs, fmt.Sprintf("field %s: expected %s", key, propType))
						}
						if propType == "object" {
							if nestedErrs := validateSchema(val, propMap); len(nestedErrs) > 0 {
								for _, e := range nestedErrs {
									errs = append(errs, fmt.Sprintf("%s.%s", key, e))
								}
							}
						}
					}
				}
			}
		}
	}
	return errs
}

func typeMatches(val any, typ string) bool {
	switch typ {
	case "string":
		_, ok := val.(string)
		return ok
	case "number":
		switch val.(type) {
		case float64, int, int64, json.Number:
			return true
		}
		return false
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "object":
		_, ok := val.(map[string]any)
		return ok
	case "array":
		_, ok := val.([]any)
		return ok
	}
	return true
}
