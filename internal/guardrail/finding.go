package guardrail

import "fmt"

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Finding struct {
	Guardrail string   `json:"guardrail"`
	Type      string   `json:"type"`
	Severity  Severity `json:"severity"`

	Start int `json:"start"`
	End   int `json:"end"`

	Entity string `json:"entity"`
	Value  string `json:"value"`
}

func (f Finding) String() string {
	return fmt.Sprintf("[%s] %s %s: %s (pos %d-%d)",
		f.Guardrail, f.Severity, f.Type, f.Entity, f.Start, f.End)
}
