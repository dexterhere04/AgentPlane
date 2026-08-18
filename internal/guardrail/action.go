package guardrail

type Action int

const (
	ActionDetect  Action = iota
	ActionBlock
	ActionRedact
	ActionWarn
)

func (a Action) String() string {
	switch a {
	case ActionDetect:
		return "detect"
	case ActionBlock:
		return "block"
	case ActionRedact:
		return "redact"
	case ActionWarn:
		return "warn"
	default:
		return "unknown"
	}
}

func DecisionToAction(d Decision) Action {
	switch d {
	case DecisionPass:
		return ActionDetect
	case DecisionBlock:
		return ActionBlock
	case DecisionRedact:
		return ActionRedact
	case DecisionLogOnly:
		return ActionDetect
	case DecisionWarn:
		return ActionWarn
	default:
		return ActionDetect
	}
}
