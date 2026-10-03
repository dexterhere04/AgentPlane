package proxy

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrUpstreamNotFound is returned when a name is not in the registry.
	ErrUpstreamNotFound = errors.New("proxy: upstream not found")
	// ErrDuplicateUpstream is returned when registering an existing name.
	ErrDuplicateUpstream = errors.New("proxy: upstream already registered")
)

// Registry is the set of upstreams the gateway can route to. It is safe for
// concurrent use. Enumeration order is registration order, which keeps
// routing and admin output deterministic.
//
// The registry stores *Upstream values and never inspects the concrete
// Provider type behind them.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]*Upstream
	order  []string
}

func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]*Upstream)}
}

// Register adds u. It fails on a nil upstream/provider, an empty name, or a
// name that is already registered.
func (r *Registry) Register(u *Upstream) error {
	if u == nil || u.Provider == nil {
		return errors.New("proxy: upstream and its provider must be non-nil")
	}
	if u.Name == "" {
		return errors.New("proxy: upstream name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[u.Name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateUpstream, u.Name)
	}
	r.byName[u.Name] = u
	r.order = append(r.order, u.Name)
	return nil
}

// RegisterProvider wraps p in a new Upstream named p.Name() and registers it.
func (r *Registry) RegisterProvider(p Provider, opts ...UpstreamOption) (*Upstream, error) {
	if p == nil {
		return nil, errors.New("proxy: provider must be non-nil")
	}
	u := NewUpstream("", p, opts...)
	if err := r.Register(u); err != nil {
		return nil, err
	}
	return u, nil
}

// Unregister removes an upstream. In-flight requests that already hold the
// *Upstream are unaffected. It reports whether the name existed.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[name]; !ok {
		return false
	}
	delete(r.byName, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return true
}

// Get looks up an upstream by name.
func (r *Registry) Get(name string) (*Upstream, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byName[name]
	return u, ok
}

// SetEnabled enables or disables an upstream at runtime.
func (r *Registry) SetEnabled(name string, enabled bool) error {
	u, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUpstreamNotFound, name)
	}
	u.SetEnabled(enabled)
	return nil
}

// Len returns the number of registered upstreams.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// Names returns upstream names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// List returns all upstreams (enabled or not) in registration order.
func (r *Registry) List() []*Upstream {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Upstream, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.byName[n])
	}
	return out
}

// Statuses returns a snapshot of every upstream for admin/metrics endpoints.
func (r *Registry) Statuses() []UpstreamStatus {
	ups := r.List()
	out := make([]UpstreamStatus, 0, len(ups))
	for _, u := range ups {
		out = append(out, u.Status())
	}
	return out
}

// Candidates returns the upstreams that could serve model right now: enabled,
// supporting the model, and with a breaker that would admit traffic. The
// result is in registration order and is a fresh slice the caller may reorder.
// Selection (weighting, fallback order) is the router's job, not the
// registry's. An empty result means nothing can serve the model.
func (r *Registry) Candidates(model, providerGroup string) []*Upstream {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Upstream, 0, len(r.order))
	for _, n := range r.order {
		u := r.byName[n]
		if !u.Available() || !u.SupportsModel(model) {
			continue
		}

		if providerGroup != "" && u.ProviderGroup != providerGroup {
			continue
		}

		out = append(out, u)
	}
	return out
}
