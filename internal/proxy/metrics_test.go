package proxy

import (
	"testing"
)

func TestMetricsRecordsTotalsAndPerUpstream(t *testing.T) {
	metrics := &Metrics{byName: make(map[string]*upstreamCounters)}

	metrics.RecordDecision("a")
	metrics.RecordDecision("b")
	metrics.RecordDecision("b")
	metrics.RecordFailover("a", "b")
	metrics.RecordRetry("b")
	metrics.RecordBreakerTrip("a")
	metrics.RecordBreakerRecovery("a")
	metrics.RecordNoProvider()
	metrics.RecordDenied()

	snap := metrics.Snapshot()

	if snap.Decisions != 3 {
		t.Errorf("Decisions = %d, want 3", snap.Decisions)
	}
	if snap.Failovers != 1 {
		t.Errorf("Failovers = %d, want 1", snap.Failovers)
	}
	if snap.Retries != 1 {
		t.Errorf("Retries = %d, want 1", snap.Retries)
	}
	if snap.BreakerTrips != 1 || snap.BreakerRecovers != 1 {
		t.Errorf("Breaker trips/recoveries = %d/%d, want 1/1", snap.BreakerTrips, snap.BreakerRecovers)
	}
	if snap.NoProvider != 1 || snap.Denied != 1 {
		t.Errorf("NoProvider/Denied = %d/%d, want 1/1", snap.NoProvider, snap.Denied)
	}
	if len(snap.Upstreams) != 2 {
		t.Fatalf("Upstreams len = %d, want 2", len(snap.Upstreams))
	}

	// Snapshot is sorted by name: a, b.
	a, b := snap.Upstreams[0], snap.Upstreams[1]
	if a.Name != "a" || b.Name != "b" {
		t.Fatalf("upstream order = %q,%q want a,b", a.Name, b.Name)
	}

	if a.Decisions != 1 || a.Failovers != 1 || a.Retries != 0 || a.BreakerTrips != 1 || a.BreakerRecovers != 1 {
		t.Errorf("upstream a counters = %+v, want decisions=1 failovers=1 breaker_trips=1 recoveries=1", a)
	}
	if b.Decisions != 2 || b.Failovers != 0 || b.Retries != 1 || b.BreakerTrips != 0 {
		t.Errorf("upstream b counters = %+v, want decisions=2 retries=1", b)
	}
}

func TestMetricsNilSafe(t *testing.T) {
	var metrics *Metrics
	metrics.RecordDecision("a")
	metrics.RecordFailover("a", "b")
	metrics.RecordRetry("a")
	metrics.RecordBreakerTrip("a")
	metrics.RecordBreakerRecovery("a")
	metrics.RecordNoProvider()
	metrics.RecordDenied()
	snap := metrics.Snapshot()
	if snap.Decisions != 0 || snap.Upstreams != nil {
		t.Fatalf("nil metrics snapshot = %+v, want empty", snap)
	}
}

func TestMetricsSnapshotIsStable(t *testing.T) {
	metrics := &Metrics{byName: make(map[string]*upstreamCounters)}
	metrics.RecordDecision("c")
	metrics.RecordDecision("a")
	metrics.RecordDecision("b")

	first := metrics.Snapshot()
	second := metrics.Snapshot()

	for i := range first.Upstreams {
		if first.Upstreams[i].Name != second.Upstreams[i].Name {
			t.Fatalf("unstable upstream order: %v vs %v", first.Upstreams, second.Upstreams)
		}
	}
}
