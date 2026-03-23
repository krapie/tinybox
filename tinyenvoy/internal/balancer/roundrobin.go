package balancer

import (
	"sync"
	"sync/atomic"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
)

// RoundRobin implements Envoy's ROUND_ROBIN lb_policy.
// Uses an atomic counter mod len(healthy endpoints). No lock required for Pick.
type RoundRobin struct {
	mu       sync.RWMutex
	backends []*backend.Backend
	counter  atomic.Uint64
}

// NewRoundRobin creates a new RoundRobin balancer.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Add adds a backend to the pool. Duplicate addresses are ignored.
func (rr *RoundRobin) Add(b *backend.Backend) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	for _, existing := range rr.backends {
		if existing.Addr == b.Addr {
			return
		}
	}
	rr.backends = append(rr.backends, b)
}

// Remove removes the backend with the given address from the pool.
func (rr *RoundRobin) Remove(addr string) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	filtered := rr.backends[:0]
	for _, b := range rr.backends {
		if b.Addr != addr {
			filtered = append(filtered, b)
		}
	}
	rr.backends = filtered
}

// Pick selects the next healthy backend using round-robin rotation.
// The key parameter is ignored (Envoy ROUND_ROBIN behavior).
// Returns nil if no healthy backends are available.
func (rr *RoundRobin) Pick(_ string) *backend.Backend {
	rr.mu.RLock()
	// Snapshot healthy backends under read lock
	healthy := make([]*backend.Backend, 0, len(rr.backends))
	for _, b := range rr.backends {
		if b.IsHealthy() {
			healthy = append(healthy, b)
		}
	}
	rr.mu.RUnlock()

	if len(healthy) == 0 {
		return nil
	}
	idx := rr.counter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))]
}
