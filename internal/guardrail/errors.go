package guardrail

import "fmt"

type GuardrailError struct {
	Guardrail string
	Err       error
}

func (e *GuardrailError) Error() string {
	return fmt.Sprintf("guardrail %q failed: %v", e.Guardrail, e.Err)
}

func (e *GuardrailError) Unwrap() error {
	return e.Err
}

var ErrGuardrailUnavailable = fmt.Errorf("request could not be evaluated by mandatory security controls")
