package pii

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/dexterhere04/AgentPlane/internal/guardrail"
)

type Detector interface {
	Name() string
	Detect(ctx context.Context, content string) ([]guardrail.Finding, error)
}

type RegexDetector struct{}

func (d *RegexDetector) Name() string {
	return "regex"
}

var piiRules = []struct {
	name      string
	severity  guardrail.Severity
	pattern   *regexp.Regexp
	entity    string
	redactStr string
}{
	{
		name:      "email",
		severity:  guardrail.SeverityHigh,
		pattern:   regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		entity:    "email_address",
		redactStr: "[EMAIL REDACTED]",
	},
	{
		name:      "credit_card",
		severity:  guardrail.SeverityCritical,
		pattern:   regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|6(?:011|5[0-9]{2})[0-9]{12}|(?:2131|1800|35\d{3})\d{11})\b`),
		entity:    "credit_card_number",
		redactStr: "[CREDIT CARD REDACTED]",
	},
	{
		name:      "phone_us",
		severity:  guardrail.SeverityMedium,
		pattern:   regexp.MustCompile(`\b(\+?1[-\s.]?)?\(?\d{3}\)?[-\s.]?\d{3}[-\s.]?\d{4}\b`),
		entity:    "phone_number",
		redactStr: "[PHONE REDACTED]",
	},
	{
		name:      "ssn",
		severity:  guardrail.SeverityCritical,
		pattern:   regexp.MustCompile(`\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b`),
		entity:    "us_ssn",
		redactStr: "[SSN REDACTED]",
	},
	{
		name:      "ip_address",
		severity:  guardrail.SeverityLow,
		pattern:   regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),
		entity:    "ip_address",
		redactStr: "[IP REDACTED]",
	},

	{
		name:      "street_address",
		severity:  guardrail.SeverityMedium,
		pattern:   regexp.MustCompile(`\b\d{1,5}\s+[A-Z][a-z]+(?:\s+(?:St(?:reet)?|Ave(?:nue)?|Rd|Road|Blvd|Boulevard|Ln|Lane|Dr|Drive|Ct|Court|Pl|Place|Way|Cir|Circle|Hwy|Highway))\b`),
		entity:    "street_address",
		redactStr: "[ADDRESS REDACTED]",
	},
	{
		name:      "datetime_personal",
		severity:  guardrail.SeverityLow,
		pattern:   regexp.MustCompile(`\b(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+\d{1,2},?\s+\d{4}\b`),
		entity:    "date_with_context",
		redactStr: "[DATE REDACTED]",
	},
}

func (d *RegexDetector) Detect(ctx context.Context, content string) ([]guardrail.Finding, error) {
	var findings []guardrail.Finding

	for _, rule := range piiRules {
		matches := rule.pattern.FindAllStringIndex(content, -1)
		for _, loc := range matches {
			start, end := loc[0], loc[1]
			matchStr := content[start:end]
			snippet := matchStr
			if len(snippet) > 20 {
				snippet = matchStr[:8] + "..." + matchStr[len(matchStr)-4:]
			}
			findings = append(findings, guardrail.Finding{
				Guardrail: "pii",
				Type:      rule.name,
				Severity:  rule.severity,
				Start:     start,
				End:       end,
				Entity:    rule.entity,
				Value:     snippet,
			})
		}
	}

	return findings, nil
}

func redactContent(content string, findings []guardrail.Finding) string {
	if len(findings) == 0 {
		return content
	}

	validated := validateFindings(findings)

	sorted := make([]guardrail.Finding, len(validated))
	copy(sorted, validated)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start > sorted[j].Start
	})

	result := []byte(content)
	for _, f := range sorted {
		if f.Start < 0 || f.End > len(result) || f.Start >= f.End {
			continue
		}
		for _, rule := range piiRules {
			if rule.name == f.Type {
				prefix := result[:f.Start]
				suffix := result[f.End:]
				result = append(prefix, append([]byte(rule.redactStr), suffix...)...)
				break
			}
		}
	}
	return string(result)
}

func validateFindings(findings []guardrail.Finding) []guardrail.Finding {
	if len(findings) <= 1 {
		return findings
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start < findings[j].Start
	})

	valid := make([]guardrail.Finding, 0, len(findings))
	for i, f := range findings {
		if f.Start < 0 || f.End <= f.Start {
			continue
		}
		if i > 0 && f.Start < valid[len(valid)-1].End {
			continue
		}
		valid = append(valid, f)
	}
	return valid
}

type PIIGuardrail struct {
	detectors []Detector
	mu        sync.RWMutex
}

func New(_ guardrail.Strategy) *PIIGuardrail {
	return &PIIGuardrail{
		detectors: []Detector{&RegexDetector{}},
	}
}

func NewWithDetectors(_ guardrail.Strategy, detectors ...Detector) *PIIGuardrail {
	return &PIIGuardrail{
		detectors: detectors,
	}
}

func (g *PIIGuardrail) Name() string {
	return "pii"
}

func (g *PIIGuardrail) RegisterDetector(d Detector) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.detectors = append(g.detectors, d)
}

func (g *PIIGuardrail) Evaluate(_ context.Context, _ guardrail.Direction, body []byte) (*guardrail.Result, error) {
	return g.check(body)
}

func (g *PIIGuardrail) check(body []byte) (*guardrail.Result, error) {
	content := string(body)

	g.mu.RLock()
	detSnap := make([]Detector, len(g.detectors))
	copy(detSnap, g.detectors)
	g.mu.RUnlock()

	var allFindings []guardrail.Finding
	for _, d := range detSnap {
		findings, err := d.Detect(context.Background(), content)
		if err != nil {
			continue
		}
		for i := range findings {
			findings[i].Guardrail = g.Name()
		}
		allFindings = append(allFindings, findings...)
	}

	if len(allFindings) == 0 {
		return &guardrail.Result{Guardrail: g.Name(), Decision: guardrail.DecisionPass}, nil
	}

	redacted := redactContent(content, allFindings)

	categoryCounts := make(map[string]int)
	for _, f := range allFindings {
		categoryCounts[f.Type]++
	}
	details := make(map[string]any)
	for cat, count := range categoryCounts {
		details[cat] = count
	}
	details["total_findings"] = len(allFindings)

	names := make([]string, 0, len(detSnap))
	for _, d := range detSnap {
		names = append(names, d.Name())
	}

	return &guardrail.Result{
		Guardrail: g.Name(),
		Decision:  guardrail.DecisionRedact,
		Message:   fmt.Sprintf("PII detected and redacted via %s", strings.Join(names, ", ")),
		Details:   details,
		Findings:  allFindings,
		Redacted:  []byte(redacted),
	}, nil
}
