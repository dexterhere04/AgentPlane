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
