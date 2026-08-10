package proxy

import "context"

type Provider interface {
	Name() string
	Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
}
