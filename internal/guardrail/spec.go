package guardrail

type GuardrailSpec struct {
	Name     string
	Required bool
	Config   map[string]any
}

type GuardrailSet struct {
	Guards []GuardrailSpec
}

func (gs GuardrailSet) RequiredNames() []string {
	var names []string
	for _, s := range gs.Guards {
		if s.Required {
			names = append(names, s.Name)
		}
	}
	return names
}
