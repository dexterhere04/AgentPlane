package proxy

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Metrics records routing activity in-process so /metrics can report it
// without depending on ClickHouse. Counters are cumulative for the life of the
// process and are updated synchronously at each routing decision, which makes
// them cheap, always available, and resilient to the tracing pipeline being
// disabled.
//
// The per-upstream breakdown is keyed by name and guarded by a mutex because
// the set of names is dynamic (providers are registered/reloaded at runtime);
// each individual counter is atomic so snapshotting never blocks recording for
// long.
type Metrics struct {
	Decisions       atomic.Int64
	Failovers       atomic.Int64
	Retries         atomic.Int64
	BreakerTrips    atomic.Int64
	BreakerRecovers atomic.Int64
	NoProvider      atomic.Int64
	Denied          atomic.Int64

	mu     sync.Mutex
	byName map[string]*upstreamCounters
}

type upstreamCounters struct {
	decisions       atomic.Int64
	failovers       atomic.Int64
	retries         atomic.Int64
	breakerTrips    atomic.Int64
	breakerRecovers atomic.Int64
}

// DefaultMetrics is the process-global routing metrics instance. The router
// and upstreams record into it; the /metrics handler snapshots it.
var DefaultMetrics = &Metrics{byName: make(map[string]*upstreamCounters)}

func (m *Metrics) counters(name string) *upstreamCounters {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byName[name]
	if !ok {
		c = &upstreamCounters{}
		m.byName[name] = c
	}
	return c
}

// RecordDecision counts a primary upstream selection (the initial attempt for
// a request, before any failover).
func (m *Metrics) RecordDecision(upstream string) {
	if m == nil {
		return
	}
	m.Decisions.Add(1)
	if upstream != "" {
		m.counters(upstream).decisions.Add(1)
	}
}

// RecordFailover counts a switch away from a failing upstream. It is
// attributed to the upstream being left, so a per-upstream failover count
// answers "how often did traffic move off this provider".
func (m *Metrics) RecordFailover(from, to string) {
	if m == nil {
		return
	}
	m.Failovers.Add(1)
	if from != "" {
		m.counters(from).failovers.Add(1)
	}
}

// RecordRetry counts a retry on the same upstream.
func (m *Metrics) RecordRetry(upstream string) {
	if m == nil {
		return
	}
	m.Retries.Add(1)
	if upstream != "" {
		m.counters(upstream).retries.Add(1)
	}
}

// RecordBreakerTrip counts a circuit-breaker transition into the open state.
func (m *Metrics) RecordBreakerTrip(upstream string) {
	if m == nil {
		return
	}
	m.BreakerTrips.Add(1)
	if upstream != "" {
		m.counters(upstream).breakerTrips.Add(1)
	}
}

// RecordBreakerRecovery counts a circuit-breaker transition back to closed.
func (m *Metrics) RecordBreakerRecovery(upstream string) {
	if m == nil {
		return
	}
	m.BreakerRecovers.Add(1)
	if upstream != "" {
		m.counters(upstream).breakerRecovers.Add(1)
	}
}

// RecordNoProvider counts a request that found no eligible upstream.
func (m *Metrics) RecordNoProvider() {
	if m == nil {
		return
	}
	m.NoProvider.Add(1)
}

// RecordDenied counts a request rejected by a deny routing rule.
func (m *Metrics) RecordDenied() {
	if m == nil {
		return
	}
	m.Denied.Add(1)
}

// UpstreamRoutingMetrics is the per-upstream slice of a snapshot.
type UpstreamRoutingMetrics struct {
	Name            string `json:"name"`
	Decisions       int64  `json:"decisions"`
	Failovers       int64  `json:"failovers"`
	Retries         int64  `json:"retries"`
	BreakerTrips    int64  `json:"breaker_trips"`
	BreakerRecovers int64  `json:"breaker_recoveries"`
}

// RoutingMetricsSnapshot is the JSON shape merged into /metrics. Field names
// are prefixed with routing_ so they never collide with the guardrail
// counters served from the same endpoint.
type RoutingMetricsSnapshot struct {
	Decisions       int64                    `json:"routing_decisions_total"`
	Failovers       int64                    `json:"routing_failovers_total"`
	Retries         int64                    `json:"routing_retries_total"`
	BreakerTrips    int64                    `json:"routing_breaker_trips_total"`
	BreakerRecovers int64                    `json:"routing_breaker_recoveries_total"`
	NoProvider      int64                    `json:"routing_no_provider_total"`
	Denied          int64                    `json:"routing_denied_total"`
	Upstreams       []UpstreamRoutingMetrics `json:"routing_upstreams"`
}

// Snapshot returns a point-in-time copy. Upstreams are sorted by name so the
// output is stable across polls.
func (m *Metrics) Snapshot() RoutingMetricsSnapshot {
	if m == nil {
		return RoutingMetricsSnapshot{}
	}

	m.mu.Lock()
	names := make([]string, 0, len(m.byName))
	counters := make(map[string]*upstreamCounters, len(m.byName))
	for name, c := range m.byName {
		names = append(names, name)
		counters[name] = c
	}
	m.mu.Unlock()

	sort.Strings(names)

	upstreams := make([]UpstreamRoutingMetrics, 0, len(names))
	for _, name := range names {
		c := counters[name]
		upstreams = append(upstreams, UpstreamRoutingMetrics{
			Name:            name,
			Decisions:       c.decisions.Load(),
			Failovers:       c.failovers.Load(),
			Retries:         c.retries.Load(),
			BreakerTrips:    c.breakerTrips.Load(),
			BreakerRecovers: c.breakerRecovers.Load(),
		})
	}

	return RoutingMetricsSnapshot{
		Decisions:       m.Decisions.Load(),
		Failovers:       m.Failovers.Load(),
		Retries:         m.Retries.Load(),
		BreakerTrips:    m.BreakerTrips.Load(),
		BreakerRecovers: m.BreakerRecovers.Load(),
		NoProvider:      m.NoProvider.Load(),
		Denied:          m.Denied.Load(),
		Upstreams:       upstreams,
	}
}
