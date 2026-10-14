package proxy

import (
	"errors"
	"testing"
)

func TestWeightedSelectorSelectsByWeight(t *testing.T) {
	selector := NewWeightedSelector(1)

	a := NewUpstream("a", &stubProvider{}, WithWeight(5))
	b := NewUpstream("b", &stubProvider{}, WithWeight(3))
	c := NewUpstream("c", &stubProvider{}, WithWeight(2))

	for i := 0; i < 10000; i++ {
		got, err := selector.Select([]*Upstream{a, b, c})
		if err != nil {
			t.Fatal(err)
		}

		if got != a && got != b && got != c {
			t.Fatalf("unexpected upstream %q", got.Name)
		}
	}
}

// TestWeightedSelectorDistributionMatchesWeights verifies the acceptance
// criterion that the weighted split matches the configured weights. The seed
// makes the run deterministic, and the tolerance (absolute 1.5% per weight)
// allows for normal rounding without masking a broken selector.
func TestWeightedSelectorDistributionMatchesWeights(t *testing.T) {
	const samples = 200000
	const tolerance = 0.015 // absolute

	selector := NewWeightedSelector(42)

	type want struct {
		name   string
		weight int
		expect float64
	}
	table := []want{
		{name: "a", weight: 5, expect: 50000},
		{name: "b", weight: 3, expect: 30000},
		{name: "c", weight: 2, expect: 20000},
	}

	upstreams := make([]*Upstream, 0, len(table))
	for _, w := range table {
		upstreams = append(upstreams, NewUpstream(w.name, &stubProvider{}, WithWeight(w.weight)))
	}

	counts := map[string]int{}
	for i := 0; i < samples; i++ {
		got, err := selector.Select(upstreams)
		if err != nil {
			t.Fatal(err)
		}
		counts[got.Name]++
	}

	totalWeight := 0
	for _, w := range table {
		totalWeight += w.weight
	}

	for _, w := range table {
		got := counts[w.name]
		expected := float64(samples) * float64(w.weight) / float64(totalWeight)
		if diff := absFloat(float64(got) - expected); diff > tolerance*samples {
			t.Errorf(
				"%s: got %d selections, want ~%.0f (weight %d/%d), diff %.0f exceeds tolerance %.0f",
				w.name, got, expected, w.weight, totalWeight, diff, tolerance*samples,
			)
		}
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestWeightedSelectorIgnoresZeroWeight(t *testing.T) {
	selector := NewWeightedSelector(1)

	zero := NewUpstream("zero", &stubProvider{}, WithWeight(0))
	positive := NewUpstream("positive", &stubProvider{}, WithWeight(1))

	for i := 0; i < 100; i++ {
		got, err := selector.Select([]*Upstream{zero, positive})
		if err != nil {
			t.Fatal(err)
		}

		if got != positive {
			t.Fatalf("got %q, want positive", got.Name)
		}
	}
}

func TestWeightedSelectorRejectsNoPositiveWeight(t *testing.T) {
	selector := NewWeightedSelector(1)

	a := NewUpstream("a", &stubProvider{}, WithWeight(0))
	b := NewUpstream("b", &stubProvider{}, WithWeight(0))

	_, err := selector.Select([]*Upstream{a, b})
	if !errors.Is(err, ErrNoWeightedUpstream) {
		t.Fatalf("got %v, want ErrNoWeightedUpstream", err)
	}
}

func TestWeightedSelectorRejectsEmptyCandidates(t *testing.T) {
	selector := NewWeightedSelector(1)

	_, err := selector.Select(nil)
	if !errors.Is(err, ErrNoWeightedUpstream) {
		t.Fatalf("got %v, want ErrNoWeightedUpstream", err)
	}
}
