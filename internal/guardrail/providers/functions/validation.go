package functions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

func getString(extra map[string]any, key, fallback string) string {
	if extra == nil {
		return fallback
	}
	if v, ok := extra[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func getStringSlice(extra map[string]any, key string) []string {
	if extra == nil {
		return nil
	}
	if v, ok := extra[key]; ok {
		switch arr := v.(type) {
		case []string:
			return arr
		case []any:
			var out []string
			for _, item := range arr {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func getInt(extra map[string]any, key string) *int {
	if extra == nil {
		return nil
	}
	if v, ok := extra[key]; ok {
		switch n := v.(type) {
		case float64:
			i := int(n)
			return &i
		case int:
			return &n
		case int64:
			i := int(n)
			return &i
		}
	}
	return nil
}

func getEnvInt(key string) *int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return &n
		}
	}
	return nil
}

type WordCountGuardrail struct {
	name string
	min  *int
	max  *int
}

func NewWordCount(s guardrail.Strategy) *WordCountGuardrail {
	minV := getInt(s.Extra, "min")
	if minV == nil {
		minV = getEnvInt("WORD_COUNT_MIN")
	}
	maxV := getInt(s.Extra, "max")
	if maxV == nil {
		maxV = getEnvInt("WORD_COUNT_MAX")
	}
	return &WordCountGuardrail{name: s.Name, min: minV, max: maxV}
}

func (g *WordCountGuardrail) Name() string               { return g.name }

func (g *WordCountGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	words := strings.Fields(string(body))
	count := len(words)
	if g.min != nil && count < *g.min {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("word count %d below minimum %d", count, *g.min),
		}, nil
	}
	if g.max != nil && count > *g.max {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("word count %d exceeds maximum %d", count, *g.max),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type SentenceCountGuardrail struct {
	name string
	min  *int
	max  *int
}

func NewSentenceCount(s guardrail.Strategy) *SentenceCountGuardrail {
	minV := getInt(s.Extra, "min")
	if minV == nil {
		minV = getEnvInt("SENTENCE_COUNT_MIN")
	}
	maxV := getInt(s.Extra, "max")
	if maxV == nil {
		maxV = getEnvInt("SENTENCE_COUNT_MAX")
	}
	return &SentenceCountGuardrail{name: s.Name, min: minV, max: maxV}
}

func (g *SentenceCountGuardrail) Name() string               { return g.name }

var sentenceEnd = regexp.MustCompile(`[.!?]+`)

func (g *SentenceCountGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	text := string(body)
	count := len(sentenceEnd.FindAllString(text, -1))
	if count == 0 && len(text) > 0 {
		count = 1
	}
	if g.min != nil && count < *g.min {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("sentence count %d below minimum %d", count, *g.min),
		}, nil
	}
	if g.max != nil && count > *g.max {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("sentence count %d exceeds maximum %d", count, *g.max),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type CharacterCountGuardrail struct {
	name string
	min  *int
	max  *int
}

func NewCharacterCount(s guardrail.Strategy) *CharacterCountGuardrail {
	minV := getInt(s.Extra, "min")
	if minV == nil {
		minV = getEnvInt("CHARACTER_COUNT_MIN")
	}
	maxV := getInt(s.Extra, "max")
	if maxV == nil {
		maxV = getEnvInt("CHARACTER_COUNT_MAX")
	}
	return &CharacterCountGuardrail{name: s.Name, min: minV, max: maxV}
}

func (g *CharacterCountGuardrail) Name() string               { return g.name }

func (g *CharacterCountGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	count := len(body)
	if g.min != nil && count < *g.min {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("character count %d below minimum %d", count, *g.min),
		}, nil
	}
	if g.max != nil && count > *g.max {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   fmt.Sprintf("character count %d exceeds maximum %d", count, *g.max),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type AllUppercaseGuardrail struct {
	name string
}

func NewAllUppercase(s guardrail.Strategy) *AllUppercaseGuardrail {
	return &AllUppercaseGuardrail{name: s.Name}
}

func (g *AllUppercaseGuardrail) Name() string               { return g.name }

func (g *AllUppercaseGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	text := strings.TrimSpace(string(body))
	if len(text) < 3 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	hasLetter := false
	for _, r := range text {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
			}
		}
	}
	if hasLetter {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   "content is all uppercase",
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type AllLowercaseGuardrail struct {
	name string
}

func NewAllLowercase(s guardrail.Strategy) *AllLowercaseGuardrail {
	return &AllLowercaseGuardrail{name: s.Name}
}

func (g *AllLowercaseGuardrail) Name() string               { return g.name }

func (g *AllLowercaseGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	text := strings.TrimSpace(string(body))
	if len(text) < 3 {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	hasLetter := false
	for _, r := range text {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsLower(r) {
				return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
			}
		}
	}
	if hasLetter {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionWarn,
			Message:   "content is all lowercase",
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type JsonKeysGuardrail struct {
	name     string
	required []string
}

func NewJsonKeys(s guardrail.Strategy) *JsonKeysGuardrail {
	keys := getStringSlice(s.Extra, "required")
	if len(keys) == 0 {
		raw := os.Getenv("JSON_KEYS_REQUIRED")
		if raw != "" {
			for _, k := range strings.Split(raw, ",") {
				keys = append(keys, strings.TrimSpace(k))
			}
		}
	}
	return &JsonKeysGuardrail{name: s.Name, required: keys}
}

func (g *JsonKeysGuardrail) Name() string               { return g.name }

func (g *JsonKeysGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.required) == 0 || !json.Valid(body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var doc map[string]any
	json.Unmarshal(body, &doc)
	var missing []string
	for _, key := range g.required {
		if _, ok := doc[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("missing required JSON keys: %v", missing),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type NotNullGuardrail struct {
	name   string
	fields []string
}

func NewNotNull(s guardrail.Strategy) *NotNullGuardrail {
	fields := getStringSlice(s.Extra, "fields")
	if len(fields) == 0 {
		raw := os.Getenv("NOT_NULL_FIELDS")
		if raw != "" {
			for _, f := range strings.Split(raw, ",") {
				fields = append(fields, strings.TrimSpace(f))
			}
		}
	}
	return &NotNullGuardrail{name: s.Name, fields: fields}
}

func (g *NotNullGuardrail) Name() string               { return g.name }

func (g *NotNullGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.fields) == 0 || !json.Valid(body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var doc map[string]any
	json.Unmarshal(body, &doc)
	var nullFields []string
	for _, f := range g.fields {
		v, ok := doc[f]
		if !ok || v == nil {
			nullFields = append(nullFields, f)
		}
	}
	if len(nullFields) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("null or missing fields: %v", nullFields),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}

type RequiredMetadataKeysGuardrail struct {
	name string
	keys []string
}

func NewRequiredMetadataKeys(s guardrail.Strategy) *RequiredMetadataKeysGuardrail {
	keys := getStringSlice(s.Extra, "keys")
	if len(keys) == 0 {
		raw := os.Getenv("REQUIRED_METADATA_KEYS")
		if raw != "" {
			for _, k := range strings.Split(raw, ",") {
				keys = append(keys, strings.TrimSpace(k))
			}
		}
	}
	return &RequiredMetadataKeysGuardrail{name: s.Name, keys: keys}
}

func (g *RequiredMetadataKeysGuardrail) Name() string               { return g.name }

func (g *RequiredMetadataKeysGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	if len(g.keys) == 0 || !json.Valid(body) {
		return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
	}
	var doc map[string]any
	json.Unmarshal(body, &doc)
	meta, _ := doc["metadata"].(map[string]any)
	if meta == nil {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   "metadata object is missing",
		}, nil
	}
	var missing []string
	for _, key := range g.keys {
		if _, ok := meta[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return &guardrail.Result{
			Guardrail: g.name,
			Decision:  guardrail.DecisionBlock,
			Message:   fmt.Sprintf("missing metadata keys: %v", missing),
		}, nil
	}
	return &guardrail.Result{Guardrail: g.name, Decision: guardrail.DecisionPass}, nil
}
