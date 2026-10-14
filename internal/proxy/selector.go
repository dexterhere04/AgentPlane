package proxy

import (
	"errors"
	"math/rand"
)

var ErrNoWeightedUpstream = errors.New("proxy: no upstream with positive weight")

// WeightedSelector chooses an upstream according to its configured weight.
type WeightedSelector struct {
	rng *rand.Rand
}

// NewWeightedSelector creates a selector with its own random source.
func NewWeightedSelector(seed int64) *WeightedSelector {
	return &WeightedSelector{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// Select chooses one upstream from the supplied candidates.
//
// Upstreams with weight <= 0 are ignored. If no candidate has a positive
// weight, ErrNoWeightedUpstream is returned.
func (s *WeightedSelector) Select(candidates []*Upstream) (*Upstream, error) {
	total := 0
	for _, u := range candidates {
		if u == nil || u.Weight <= 0 {
			continue
		}
		total += u.Weight
	}

	if total <= 0 {
		return nil, ErrNoWeightedUpstream
	}

	n := s.rng.Intn(total)

	for _, u := range candidates {
		if u == nil || u.Weight <= 0 {
			continue
		}

		if n < u.Weight {
			return u, nil
		}

		n -= u.Weight
	}

	// The loop above must return when total was calculated correctly.
	return nil, ErrNoWeightedUpstream
}
