// Package vmodelrouting implements load-balancing strategies for virtual models.
package vmodelrouting

import (
	"math/rand"
	"sync"

	"omnimodel/internal/database"
)

// roundRobinState holds the per-virtual-model cursor for round-robin selection.
var (
	rrMu    sync.Mutex
	rrState = make(map[string]int)
)

// SelectUpstream picks one upstream from the list according to the given
// load-balancing strategy. Returns nil when the upstream list is empty.
func SelectUpstream(
	upstreams []database.VirtualModelUpstreamRecord,
	strategy database.LbStrategy,
	virtualModelID string,
) *database.VirtualModelUpstreamRecord {
	if len(upstreams) == 0 {
		return nil
	}

	switch strategy {
	case database.LbStrategyRoundRobin:
		rrMu.Lock()
		idx := rrState[virtualModelID] % len(upstreams)
		rrState[virtualModelID] = idx + 1
		rrMu.Unlock()
		return &upstreams[idx]

	case database.LbStrategyRandom:
		idx := rand.Intn(len(upstreams))
		return &upstreams[idx]

	case database.LbStrategyPriority:
		// upstreams are already sorted by priority ASC from the DB query
		return &upstreams[0]

	case database.LbStrategyWeighted:
		totalWeight := 0
		for _, u := range upstreams {
			w := u.Weight
			if w < 1 {
				w = 1
			}
			totalWeight += w
		}
		roll := rand.Intn(totalWeight)
		for i, u := range upstreams {
			w := u.Weight
			if w < 1 {
				w = 1
			}
			roll -= w
			if roll < 0 {
				return &upstreams[i]
			}
		}
		// Fallback (floating-point edge case)
		return &upstreams[len(upstreams)-1]

	default:
		return &upstreams[0]
	}
}
