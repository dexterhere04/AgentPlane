package guardrail

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	guards map[string]Guardrail
}

func NewRegistry() *Registry {
	return &Registry{guards: make(map[string]Guardrail)}
}

func (r *Registry) Register(g Guardrail) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.guards[g.Name()] = g
}

func (r *Registry) Get(name string) (Guardrail, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.guards[name]
	return g, ok
}

func (r *Registry) Resolve(spec GuardrailSpec) (Guardrail, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.guards[spec.Name]
	if !ok {
		return nil, fmt.Errorf("guardrail %q not registered", spec.Name)
	}
	return g, nil
}
