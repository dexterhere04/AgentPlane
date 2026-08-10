package guardrail

import "context"

type Detector interface {
	Name() string
	Detect(ctx context.Context, content string) ([]Finding, error)
}
