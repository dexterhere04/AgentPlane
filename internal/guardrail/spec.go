package guardrail

type GuardrailSpec struct {
	Name   string
	Config map[string]any
}

type GuardrailSet struct {
	Guards []GuardrailSpec
}
