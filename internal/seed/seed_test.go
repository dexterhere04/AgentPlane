package seed

import (
	"math/rand"
	"testing"
	"time"
)

func testUsers() []demoUser {
	return []demoUser{
		{ID: "11111111-1111-1111-1111-111111111111", Username: "ava.chen", Team: "platform", Weight: 5},
		{ID: "22222222-2222-2222-2222-222222222222", Username: "svc-batch-inference", Team: "service-accounts", Weight: 20},
		{ID: "33333333-3333-3333-3333-333333333333", Username: "sara.lindqvist", Team: "ml-research", Weight: 3, EU: true},
	}
}

func TestDistributeAcrossHoursSumsToTotal(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)

	for _, total := range []int{0, 1, 37, 12000} {
		buckets := distributeAcrossHours(start, end, total)
		if got := len(buckets); got != 7*24 {
			t.Fatalf("total=%d: got %d buckets, want %d", total, got, 7*24)
		}
		sum := 0
		for _, b := range buckets {
			if b.count < 0 {
				t.Fatalf("total=%d: negative bucket count", total)
			}
			sum += b.count
		}
		if sum != total {
			t.Fatalf("total=%d: distributed sum=%d", total, sum)
		}
	}
}

func TestDistributePrefersBusinessHours(t *testing.T) {
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // a Monday
	buckets := distributeAcrossHours(start, start.Add(24*time.Hour), 100000)

	business, overnight := 0, 0
	for _, b := range buckets {
		if b.start.Hour() >= 9 && b.start.Hour() < 18 {
			business += b.count
		}
		if b.start.Hour() < 6 {
			overnight += b.count
		}
	}
	if business <= overnight {
		t.Fatalf("business hours (%d) should outweigh overnight (%d)", business, overnight)
	}
}

func TestGenerateIsDeterministicAndConsistent(t *testing.T) {
	opts := Options{Days: 2, TraceCount: 400, RandSeed: 7, OrgID: "test-org"}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	first, err := newGenerator(opts, testUsers()).generate(now)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	second, err := newGenerator(opts, testUsers()).generate(now)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if len(first.traces) != len(second.traces) {
		t.Fatalf("non-deterministic trace count: %d vs %d", len(first.traces), len(second.traces))
	}
	if len(first.traces) == 0 {
		t.Fatal("expected traces to be generated")
	}
	if first.traces[0].TraceID != second.traces[0].TraceID {
		t.Fatalf("non-deterministic trace id: %q vs %q", first.traces[0].TraceID, second.traces[0].TraceID)
	}
}

func TestGeneratedTracesAreWellFormed(t *testing.T) {
	opts := Options{Days: 3, TraceCount: 1500, RandSeed: 3, OrgID: "test-org"}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	ds, err := newGenerator(opts, testUsers()).generate(now)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(ds.usage) != len(ds.traces) {
		t.Fatalf("usage events (%d) must match traces (%d)", len(ds.usage), len(ds.traces))
	}

	windowStart := now.AddDate(0, 0, -opts.Days).Add(-time.Hour)
	for _, tr := range ds.traces {
		switch {
		case tr.Timestamp.After(now):
			t.Fatalf("trace %s timestamp %s is in the future (now=%s)", tr.TraceID, tr.Timestamp, now)
		case tr.Timestamp.Before(windowStart):
			t.Fatalf("trace %s timestamp %s predates window (%s)", tr.TraceID, tr.Timestamp, windowStart)
		}
		if tr.UserID == "" || tr.Username == "" {
			t.Fatalf("trace %s missing user attribution", tr.TraceID)
		}
		if tr.Provider == "" || tr.Model == "" || tr.Route == "" {
			t.Fatalf("trace %s missing routing metadata", tr.TraceID)
		}
		if tr.TotalTokens != tr.InputTokens+tr.OutputTokens {
			t.Fatalf("trace %s token total mismatch", tr.TraceID)
		}
		if tr.EstimatedCost < 0 {
			t.Fatalf("trace %s negative cost", tr.TraceID)
		}
		if tr.Failover == 1 {
			if tr.FailoverFrom == "" || tr.FailoverTo == "" || tr.Attempt < 2 {
				t.Fatalf("trace %s failover metadata inconsistent", tr.TraceID)
			}
		} else if tr.FailoverFrom != "" || tr.FailoverTo != "" {
			t.Fatalf("trace %s has failover parties without failover flag", tr.TraceID)
		}
		switch tr.GuardrailAction {
		case "", "pass", "block", "redact":
		default:
			t.Fatalf("trace %s unexpected guardrail action %q", tr.TraceID, tr.GuardrailAction)
		}
	}

	// Blocked traces must be represented as an effective error via a block event.
	blockedTrace := ""
	for _, tr := range ds.traces {
		if tr.GuardrailAction == "block" {
			blockedTrace = tr.TraceID
			break
		}
	}
	if blockedTrace == "" {
		t.Fatal("expected at least one blocked trace in a 1500-trace sample")
	}
	found := false
	for _, ev := range ds.guardrails {
		if ev.TraceID == blockedTrace && ev.Action == "block" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("blocked trace %s has no block guardrail event", blockedTrace)
	}
}

func TestEstimateCost(t *testing.T) {
	m := modelSpec{InPrice: 2.5, OutPrice: 10}
	got := estimateCost(m, 1_000_000, 1_000_000)
	if got != 12.5 {
		t.Fatalf("estimateCost = %v, want 12.5", got)
	}
	if estimateCost(m, 0, 0) != 0 {
		t.Fatal("zero tokens should cost nothing")
	}
}

func TestWeightedStringRespectsWeights(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	values := []string{"a", "b"}
	weights := []int{99, 1}
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		counts[weightedString(rng, values, weights)]++
	}
	if counts["a"] <= counts["b"] {
		t.Fatalf("expected 'a' to dominate: %v", counts)
	}
	if counts["b"] == 0 {
		t.Fatalf("expected 'b' to appear occasionally: %v", counts)
	}
}
