package scheduler

import "sync"

// RoundRobin is a simple round-robin node selector.
// In M1-M4 the scheduler is a stub; the kubelet roadmap upgrades it to
// a full least-loaded scheduler with real Node objects.
type RoundRobin struct {
	mu      sync.Mutex
	counter int
}

// NewRoundRobin creates a new RoundRobin scheduler.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Select picks the next node from the list using round-robin.
// Returns an empty string if nodes is empty.
func (r *RoundRobin) Select(nodes []string) string {
	if len(nodes) == 0 {
		return ""
	}
	r.mu.Lock()
	idx := r.counter % len(nodes)
	r.counter++
	r.mu.Unlock()
	return nodes[idx]
}
