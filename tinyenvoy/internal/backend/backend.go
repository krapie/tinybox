// Package backend models Envoy's Endpoint concept: an individual upstream address
// with health status and active connection tracking.
package backend

import "sync/atomic"

// Backend represents a single upstream endpoint.
// Analogous to Envoy's LbEndpoint with HealthStatus.
// Both Healthy and ActiveConns are accessed atomically for safe concurrent access.
type Backend struct {
	Addr        string
	healthy     atomic.Bool
	ActiveConns int64 // accessed via sync/atomic
}

// NewBackend creates a Backend with the given address and initial health state.
func NewBackend(addr string, healthy bool) *Backend {
	b := &Backend{Addr: addr}
	b.healthy.Store(healthy)
	return b
}

// IsHealthy returns the current health status atomically.
func (b *Backend) IsHealthy() bool {
	return b.healthy.Load()
}

// SetHealthy sets the health status atomically.
func (b *Backend) SetHealthy(v bool) {
	b.healthy.Store(v)
}

// IncConns atomically increments the active connection count.
func (b *Backend) IncConns() {
	atomic.AddInt64(&b.ActiveConns, 1)
}

// DecConns atomically decrements the active connection count.
func (b *Backend) DecConns() {
	atomic.AddInt64(&b.ActiveConns, -1)
}

// Conns returns the current active connection count.
func (b *Backend) Conns() int64 {
	return atomic.LoadInt64(&b.ActiveConns)
}
