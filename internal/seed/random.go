package seed

import (
	"math"
	"math/rand"
)

// weightedTable is a precomputed cumulative-weight sampler over a slice.
type weightedTable[T any] struct {
	items []T
	cum   []int
	total int
}

func newWeightedTable[T any](items []T, weight func(T) int) *weightedTable[T] {
	t := &weightedTable[T]{items: items, cum: make([]int, len(items))}
	running := 0
	for i, item := range items {
		w := weight(item)
		if w < 0 {
			w = 0
		}
		running += w
		t.cum[i] = running
	}
	t.total = running
	return t
}

// pick returns a weighted-random item. If every weight is zero it falls back to
// a uniform choice so callers never receive a zero value by accident.
func (t *weightedTable[T]) pick(rng *rand.Rand) T {
	if len(t.items) == 0 {
		var zero T
		return zero
	}
	if t.total <= 0 {
		return t.items[rng.Intn(len(t.items))]
	}

	roll := rng.Intn(t.total)
	// Binary search for the first cumulative weight strictly greater than roll.
	lo, hi := 0, len(t.cum)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if t.cum[mid] <= roll {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return t.items[lo]
}

// lognormal multiplies base by a positive random factor with the given sigma,
// producing the long right tail real latency and token distributions show.
func lognormal(rng *rand.Rand, base float64, sigma float64) float64 {
	return base * math.Exp(rng.NormFloat64()*sigma)
}

// clampInt bounds v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// pickOne returns a uniform random element, or "" for an empty slice.
func pickOne(rng *rand.Rand, items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[rng.Intn(len(items))]
}
