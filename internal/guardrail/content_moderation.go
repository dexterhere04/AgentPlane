package guardrail

import (
	"context"
	"regexp"
	"strings"
)

var contentModerationCategories = []struct {
	name     string
	patterns []*regexp.Regexp
}{
	{
		name: "hate_speech",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(hate|racist|bigot|supremacist|xenophob|antisemit|homophob|transphob)\w*\b`),
			regexp.MustCompile(`(?i)\b(racial\s+slur|ethnic\s+cleansing|white\s+power|aryan)\b`),
			regexp.MustCompile(`(?i)\b(kill\s+(all\s+)?(the\s+)?(jews|muslims|blacks|gays|immigrants))\b`),
		},
	},
	{
		name: "violence",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(murder|torture|massacre|genocide|terroris[mt]|behead|dismember)\w*\b`),
			regexp.MustCompile(`(?i)\b(bomb\s+(making|instructions?|recipe)|how\s+to\s+(build|make|create)\s+(a\s+)?(bomb|weapon|explosive))\b`),
			regexp.MustCompile(`(?i)\b(shoot(ing)?\s+(up|at)\s+|school\s+shooting|mass\s+shooting)\b`),
		},
	},
	{
		name: "sexual_content",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(porn|pornograph|explicit\s+(sexual|content)|hardcore|xxx)\w*\b`),
			regexp.MustCompile(`(?i)\b(sexual\s+(abuse|assault|violence)|rape|molest|pedophil)\w*\b`),
			regexp.MustCompile(`(?i)\b(child\s+(porn|abuse|exploitation)|csam)\b`),
		},
	},
	{
		name: "self_harm",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(suicide|self[\s-]harm|cut\s+(myself|yourself)|kill\s+(myself|yourself))\b`),
			regexp.MustCompile(`(?i)\b(methods?\s+(of|for|to)\s+(suicide|self[\s-]harm)|ways?\s+to\s+(die|kill\s+(myself|yourself)))\b`),
			regexp.MustCompile(`(?i)\b(eating\s+disorder|anorexi|bulimi)\w*\b`),
		},
	},
	{
		name: "abuse_harassment",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(harass(ment|ing)?|bully(ing)?|stalk(ing|er)?|dox(xing|x)?)\b`),
			regexp.MustCompile(`(?i)\b(cyber\s*(stalking|bullying|harassment)|online\s+harassment)\b`),
			regexp.MustCompile(`(?i)\b(threat(en|ing)?\s+(to\s+)?(kill|harm|hurt|attack))\b`),
		},
	},
}

type ContentModerationGuardrail struct{}

func NewContentModerationGuardrail(_ Strategy) *ContentModerationGuardrail {
	return &ContentModerationGuardrail{}
}

func (g *ContentModerationGuardrail) Name() string {
	return "content_moderation"
}

func (g *ContentModerationGuardrail) Type() GuardrailType {
	return TypePolicy
}

func (g *ContentModerationGuardrail) Evaluate(_ context.Context, _ Direction, body []byte) (*Result, error) {
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	var findings []Finding
	totalMatches := 0

	for _, cat := range contentModerationCategories {
		for _, pat := range cat.patterns {
			matches := pat.FindAllStringIndex(bodyLower, -1)
			for _, loc := range matches {
				start, end := loc[0], loc[1]
				findings = append(findings, Finding{
					Guardrail: g.Name(),
					Type:      cat.name,
					Severity:  SeverityLow,
					Start:     start,
					End:       end,
					Entity:    "content_moderation",
					Value:     bodyStr[max(0, start):min(len(bodyStr), end)],
				})
				totalMatches++
			}
		}
	}

	if len(findings) == 0 {
		return &Result{Guardrail: g.Name(), Decision: DecisionPass}, nil
	}

	severity := SeverityLow
	if totalMatches > 10 {
		severity = SeverityHigh
	} else if totalMatches > 3 {
		severity = SeverityMedium
	}

	for i := range findings {
		findings[i].Severity = severity
	}

	categoryCounts := make(map[string]int)
	for _, f := range findings {
		categoryCounts[f.Type]++
	}
	details := map[string]any{}
	for cat, count := range categoryCounts {
		details[cat] = count
	}
	details["severity"] = string(severity)
	details["matched_count"] = len(findings)

	return &Result{
		Guardrail: g.Name(),
		Decision:  DecisionWarn,
		Message:   "potentially harmful content detected",
		Details:   details,
		Findings:  findings,
	}, nil
}
