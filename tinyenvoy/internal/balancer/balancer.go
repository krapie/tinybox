// Package balancer implements Envoy's LbPolicy enum as a Go interface.
// Supports ROUND_ROBIN and RING_HASH policies.
package balancer

import "github.com/krapi0314/tinybox/tinyenvoy/internal/backend"

// LbPolicy mirrors Envoy's cluster.lb_policy enum.
// Pick selects a healthy backend for the given key.
// For RoundRobin, key is ignored. For RingHash, key is the client IP.
type LbPolicy interface {
	Pick(key string) *backend.Backend
	Add(b *backend.Backend)
	Remove(addr string)
}
