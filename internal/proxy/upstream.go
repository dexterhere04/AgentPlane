package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/observability"
)

var (
	// ErrUpstreamDisabled is returned when an upstream has been disabled.
	ErrUpstreamDisabled = errors.New("proxy: upstream disabled")
	// ErrCircuitOpen is returned when the breaker is rejecting traffic.
	ErrCircuitOpen = errors.New("proxy: circuit breaker open")
)

// StatusError lets a Provider report an HTTP status from its backend so the
// Upstream can tell retryable / unhealthy failures from client mistakes.
// Providers should return (or wrap) it instead of a bare fmt.Errorf.
type ResilienceConfig struct {
	Timeout       time.Duration
	MaxRetries    int
	BackoffBase   time.Duration
	BackoffMax    time.Duration
	JitterFactor  float64
	MaxRetriesSet bool
	JitterSet     bool
}

func (c ResilienceConfig) normalize(u *Upstream) ResilienceConfig {
	if c.Timeout <= 0 {
		c.Timeout = u.Timeout
	}

	if !c.MaxRetriesSet {
		c.MaxRetries = u.MaxRetries
	}

	if c.BackoffBase <= 0 {
		c.BackoffBase = u.backoffBase
	}

	if c.BackoffMax <= 0 {
		c.BackoffMax = u.backoffMax
	}

	if !c.JitterSet {
		c.JitterFactor = u.backoffJitter
	}

	return c
}

type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("upstream returned status %d: %s", e.Code, e.Body)
}

// classify decides how an error from Provider.Forward is handled.
//
//	retryable      - worth another attempt on the same upstream
//	breakerFailure - counts toward tripping the circuit breaker
//
// Unknown errors (network, timeout, ...) are treated as retryable failures.
// 4xx client errors (bad request etc.) are neither: the upstream is healthy,
// the request is at fault. 401/403 mean the upstream itself is misconfigured,
// so they count as failures but are not retried.
func classify(err error) (retryable, breakerFailure bool) {
	var se *StatusError
	if errors.As(err, &se) {
		switch {
		case se.Code == 408 || se.Code == 429 || se.Code >= 500:
			return true, true
		case se.Code == 401 || se.Code == 403:
			return false, true
		default:
			return false, false
		}
	}
	return true, true
}

// ---------------------------------------------------------------------------
// Circuit breaker
// ---------------------------------------------------------------------------

type BreakerState int32

const (
	BreakerClosed BreakerState = iota
	BreakerOpen
	BreakerHalfOpen
)

func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return "open"
	case BreakerHalfOpen:
		return "half_open"
	}
	return "unknown"
}

type BreakerConfig struct {
	// FailureThreshold is the number of consecutive failures that opens the
	// breaker. Default 5.
	FailureThreshold int
	// Cooldown is how long the breaker stays open before allowing a single
	// probe request (half-open). Default 30s.
	Cooldown time.Duration
}

func (c BreakerConfig) withDefaults() BreakerConfig {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.Cooldown <= 0 {
		c.Cooldown = 30 * time.Second
	}
	return c
}

// CircuitBreaker is a small, concurrency-safe consecutive-failure breaker.
//
// Closed   -> traffic flows; N consecutive failures open it.
// Open     -> traffic rejected until Cooldown elapses.
// HalfOpen -> exactly one probe is admitted; success closes, failure reopens.
type CircuitBreaker struct {
	mu       sync.Mutex
	cfg      BreakerConfig
	state    BreakerState
	failures int
	openedAt time.Time
	probing  bool
	probeAt  time.Time
	now      func() time.Time // injectable for tests
}

func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{cfg: cfg.withDefaults(), now: time.Now}
}

// Allow reports whether a request may proceed and, if the breaker is
// recovering, reserves the single probe slot. Every true return MUST be
// followed by exactly one of RecordSuccess, RecordFailure or Release.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	switch b.state {
	case BreakerClosed:
		return true
	case BreakerOpen:
		if now.Sub(b.openedAt) < b.cfg.Cooldown {
			return false
		}
		b.state = BreakerHalfOpen
		b.probing, b.probeAt = true, now
		return true
	default: // half-open
		// A probe that never reported back is considered lost after Cooldown.
		if b.probing && now.Sub(b.probeAt) < b.cfg.Cooldown {
			return false
		}
		b.probing, b.probeAt = true, now
		return true
	}
}

// Ready is a read-only version of Allow: it never reserves the probe slot.
// Use it when choosing among upstreams; use Allow when actually sending.
func (b *CircuitBreaker) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	switch b.state {
	case BreakerClosed:
		return true
	case BreakerOpen:
		return now.Sub(b.openedAt) >= b.cfg.Cooldown
	default:
		return !b.probing || now.Sub(b.probeAt) >= b.cfg.Cooldown
	}
}

func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == BreakerOpen {
		return // late result from a request that predates the trip
	}
	b.state = BreakerClosed
	b.failures = 0
	b.probing = false
}

func (b *CircuitBreaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case BreakerHalfOpen:
		b.trip()
	case BreakerClosed:
		b.failures++
		if b.failures >= b.cfg.FailureThreshold {
			b.trip()
		}
	}
}

// Release frees a reserved probe slot without changing state. Used when the
// outcome says nothing about upstream health (caller cancelled, client error).
func (b *CircuitBreaker) Release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == BreakerHalfOpen {
		b.probing = false
	}
}

func (b *CircuitBreaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// trip must be called with mu held.
func (b *CircuitBreaker) trip() {
	b.state = BreakerOpen
	b.openedAt = b.now()
	b.failures = 0
	b.probing = false
}

// ---------------------------------------------------------------------------
// Upstream
// ---------------------------------------------------------------------------

// Upstream binds a Provider to the routing metadata and resilience policy the
// router needs. It only ever sees the Provider interface, so routing stays
// independent of concrete provider types.
//
// Exported config fields are read-only after construction. Runtime changes go
// through SetEnabled (atomic) or the breaker (self-locking).
type Upstream struct {
	Name          string
	Provider      Provider
	ProviderGroup string
	Weight        int
	Models        []string
	Timeout       time.Duration
	MaxRetries    int
	Breaker       *CircuitBreaker

	enabled       atomic.Bool
	backoffBase   time.Duration
	backoffMax    time.Duration
	backoffJitter float64
}

type UpstreamOption func(*Upstream)

func WithWeight(w int) UpstreamOption {
	return func(u *Upstream) {
		if w < 0 {
			w = 0
		}
		u.Weight = w
	}
}

func WithProviderGroup(group string) UpstreamOption {
	return func(u *Upstream) {
		u.ProviderGroup = group
	}
}

func WithModels(models ...string) UpstreamOption {
	return func(u *Upstream) { u.Models = append([]string(nil), models...) }
}

func WithTimeout(d time.Duration) UpstreamOption {
	return func(u *Upstream) { u.Timeout = d }
}

func WithMaxRetries(n int) UpstreamOption {
	return func(u *Upstream) {
		if n < 0 {
			n = 0
		}
		u.MaxRetries = n
	}
}

func WithBreaker(cfg BreakerConfig) UpstreamOption {
	return func(u *Upstream) { u.Breaker = NewCircuitBreaker(cfg) }
}

// NewUpstream wraps p. An empty name defaults to p.Name(). Defaults: weight 1,
// all models, 60s timeout, 1 retry, breaker at 5 failures / 30s, enabled.
func NewUpstream(name string, p Provider, opts ...UpstreamOption) *Upstream {
	if name == "" {
		name = p.Name()
	}
	u := &Upstream{
		Name:          name,
		Provider:      p,
		Weight:        1,
		Timeout:       60 * time.Second,
		MaxRetries:    1,
		Breaker:       NewCircuitBreaker(BreakerConfig{}),
		backoffBase:   100 * time.Millisecond,
		backoffMax:    2 * time.Second,
		backoffJitter: 0.20,
	}
	u.enabled.Store(true)
	for _, opt := range opts {
		opt(u)
	}
	return u
}

func (u *Upstream) Enabled() bool      { return u.enabled.Load() }
func (u *Upstream) SetEnabled(on bool) { u.enabled.Store(on) }

// SupportsModel reports whether this upstream serves model. An empty Models
// list means "everything". Patterns are exact matches, "*", or a trailing
// wildcard such as "gpt-*". Matching is case-insensitive.
func (u *Upstream) SupportsModel(model string) bool {
	if len(u.Models) == 0 {
		return true
	}
	model = strings.ToLower(model)
	for _, pat := range u.Models {
		pat = strings.ToLower(pat)
		switch {
		case pat == "*":
			return true
		case strings.HasSuffix(pat, "*"):
			if strings.HasPrefix(model, strings.TrimSuffix(pat, "*")) {
				return true
			}
		case pat == model:
			return true
		}
	}
	return false
}

// Available reports whether the router may consider this upstream right now:
// enabled AND the breaker would admit traffic. It does not reserve a probe.
func (u *Upstream) Available() bool {
	return u.Enabled() && u.Breaker.Ready()
}

// Forward sends the request through the upstream's timeout, retry and
// circuit-breaker policy. It satisfies the Provider signature, so an Upstream
// can be used anywhere a Provider is expected.
// Forward sends the request through the upstream's default timeout,
// retry and circuit-breaker policy.
func (u *Upstream) Forward(ctx context.Context, body []byte, requestID string) ([]byte, error) {
	return u.ForwardWithConfig(ctx, body, requestID, ResilienceConfig{})
}

// ForwardWithConfig sends the request using per-request resilience settings.
// The supplied config is not written back to the shared Upstream.
func (u *Upstream) ForwardWithConfig(
	ctx context.Context,
	body []byte,
	requestID string,
	config ResilienceConfig,
) ([]byte, error) {
	if !u.Enabled() {
		return nil, ErrUpstreamDisabled
	}

	config = config.normalize(u)

	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoffWithJitter(
				attempt,
				config.BackoffBase,
				config.BackoffMax,
				config.JitterFactor,
			)); err != nil {
				return nil, err
			}
		}

		if !u.Breaker.Allow() {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ErrCircuitOpen
		}

		resp, err := u.attemptWithTimeout(
			ctx,
			body,
			requestID,
			config.Timeout,
		)
		if err == nil {
			u.Breaker.RecordSuccess()
			return resp, nil
		}

		lastErr = err

		// Caller cancellation/deadline is not an upstream failure.
		if ctx.Err() != nil {
			u.Breaker.Release()
			return nil, err
		}

		retryable, failure := classify(err)

		if failure {
			u.Breaker.RecordFailure()
		} else {
			u.Breaker.Release()
		}

		if !retryable {
			return nil, err
		}

		if attempt < config.MaxRetries {
			observability.DefaultBus.Publish(
				observability.NewDataEvent(
					requestID,
					observability.StageRetry,
					"started",
					map[string]any{
						"upstream": u.Name,
						"attempt":  attempt + 1,
						"reason":   err.Error(),
					},
				),
			)
		}
	}

	return nil, lastErr
}

func (u *Upstream) attemptWithTimeout(
	ctx context.Context,
	body []byte,
	requestID string,
	timeout time.Duration,
) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return u.Provider.Forward(ctx, body, requestID)
}

func backoffWithJitter(
	attempt int,
	base time.Duration,
	max time.Duration,
	jitterFactor float64,
) time.Duration {
	if attempt <= 0 {
		return 0
	}

	if base <= 0 {
		base = 100 * time.Millisecond
	}

	if max <= 0 {
		max = 2 * time.Second
	}

	// 2^(attempt-1) * base.
	delay := base

	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			delay = max
			break
		}
		delay *= 2
	}

	if delay > max {
		delay = max
	}

	if jitterFactor <= 0 {
		return delay
	}

	// Add bounded positive jitter:
	// delay .. delay * (1 + jitterFactor)
	jitter := time.Duration(
		rand.Float64() * float64(delay) * jitterFactor,
	)

	if delay+jitter > max {
		return max
	}

	return delay + jitter
}

func WithBackoffBase(d time.Duration) UpstreamOption {
	return func(u *Upstream) {
		u.backoffBase = d
	}
}

func WithBackoffMax(d time.Duration) UpstreamOption {
	return func(u *Upstream) {
		u.backoffMax = d
	}
}

func WithBackoffJitter(factor float64) UpstreamOption {
	return func(u *Upstream) {
		u.backoffJitter = factor
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// UpstreamStatus is a point-in-time snapshot for admin/metrics endpoints.
type UpstreamStatus struct {
	Name    string       `json:"name"`
	Enabled bool         `json:"enabled"`
	Weight  int          `json:"weight"`
	Models  []string     `json:"models"`
	Breaker BreakerState `json:"-"`
	State   string       `json:"breaker"`
}

func (u *Upstream) Status() UpstreamStatus {
	st := u.Breaker.State()
	return UpstreamStatus{
		Name:    u.Name,
		Enabled: u.Enabled(),
		Weight:  u.Weight,
		Models:  append([]string(nil), u.Models...),
		Breaker: st,
		State:   st.String(),
	}
}
