package guardrail

import (
	"context"
	"regexp"
	"strings"
)

var promptInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above|before)\s+(instructions?|commands?|prompts?|messages?)`),
	regexp.MustCompile(`(?i)reveal\s+(your|the)\s+(system\s+)?(prompt|instructions?|directives?)`),
	regexp.MustCompile(`(?i)(hidden|secret)\s+(developer\s+)?(instructions?|prompts?|rules?)`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+(a\s+)?(DAN|jailbreak|unfiltered|unrestricted)`),
	regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be)\b`),
	regexp.MustCompile(`(?i)(override|bypass|disable)\s+(your\s+)?(safety|guidelines|restrictions|filters?|rules?)`),
	regexp.MustCompile(`(?i)(new|updated|revised)\s+(system\s+)?(instructions?|directives?)\s*(:|are|as\s+follows)`),
	regexp.MustCompile(`(?i)(from\s+now\s+on|starting\s+now)\s*[,:]\s*(you\s+(are|must|will|should))`),
	regexp.MustCompile(`(?i)act\s+as\s+(if\s+)?(you\s+are|a\b)`),
	regexp.MustCompile(`(?i)(forget|erase|clear|wipe)\s+(your|all|everything\s+about)\s+(previous\s+)?(conversation|context|history|memory)`),
	regexp.MustCompile(`(?i)do\s+not\s+follow\s+(your\s+)?(instructions?|guidelines?|rules?)`),
	regexp.MustCompile(`(?i)(tell|show|print|output|display)\s+me\s+(your|the)\s+(system\s+)?(prompt|instructions?|rules?)`),
	regexp.MustCompile(`(?i)respond\s+(as|like)\s+(you\s+are\s+)?(an?\s+)?(unfiltered|uncensored|evil|dark|malicious)`),
	regexp.MustCompile(`(?i)(you\s+(must|will|should|have\s+to)\s+(obey|follow|comply))`),
	regexp.MustCompile(`(?i)(I\s+(command|order|instruct|direct)\s+you)`),
}

type PromptInjectionGuardrail struct{}

func NewPromptInjectionGuardrail(_ Strategy) *PromptInjectionGuardrail {
	return &PromptInjectionGuardrail{}
}

func (g *PromptInjectionGuardrail) Name() string {
	return "prompt_injection"
}

func (g *PromptInjectionGuardrail) Evaluate(_ context.Context, dir Direction, body []byte) (*Result, error) {
	if dir == DirectionOutput {
		return &Result{Guardrail: g.Name(), Decision: DecisionPass}, nil
	}

	contents, err := ExtractContent(body)
	if err != nil {
		contents = []Content{{Type: ContentTypeUser, Text: string(body)}}
	}

	var findings []Finding
	for _, content := range contents {
		textLower := strings.ToLower(content.Text)
		for _, pat := range promptInjectionPatterns {
			if !pat.MatchString(textLower) {
				continue
			}
			if loc := pat.FindStringIndex(content.Text); loc != nil {
				start, end := loc[0], loc[1]
				finding := Finding{
					Guardrail: g.Name(),
					Type:      "prompt_injection",
					Severity:  SeverityHigh,
					Start:     start,
					End:       end,
					Entity:    "injection_attempt",
					Value:     content.Text[max(0, start):min(len(content.Text), end)],
				}
				findings = append(findings, finding)
			}
		}
	}

	if len(findings) == 0 {
		return &Result{Guardrail: g.Name(), Decision: DecisionPass}, nil
	}

	return &Result{
		Guardrail: g.Name(),
		Decision:  DecisionBlock,
		Message:   "potential prompt injection detected",
		Findings:  findings,
	}, nil
}
