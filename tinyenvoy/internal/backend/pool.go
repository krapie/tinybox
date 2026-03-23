package backend

import "sync"

// Pool manages a cluster's set of endpoints.
// Analogous to Envoy's ClusterLoadAssignment (EDS).
type Pool struct {
	mu       sync.RWMutex
	backends []*Backend
}

// NewPool creates a Pool from a slice of backends.
func NewPool(backends []*Backend) *Pool {
	if backends == nil {
		backends = []*Backend{}
	}
	return &Pool{backends: backends}
}

// Healthy returns all backends currently marked healthy.
// Analogous to Envoy's lb_endpoints filtered by health_status == HEALTHY.
func (p *Pool) Healthy() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.IsHealthy() {
			out = append(out, b)
		}
	}
	return out
}

// All returns all backends regardless of health status.
func (p *Pool) All() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Backend, len(p.backends))
	copy(out, p.backends)
	return out
}

// SetHealthy updates the health status of the backend with the given address.
func (p *Pool) SetHealthy(addr string, healthy bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, b := range p.backends {
		if b.Addr == addr {
			b.SetHealthy(healthy)
			return
		}
	}
}

// Add appends b to the pool if no backend with the same address already exists.
// Analogous to EDS endpoint addition in Envoy's xDS.
func (p *Pool) Add(b *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, existing := range p.backends {
		if existing.Addr == b.Addr {
			return // idempotent
		}
	}
	p.backends = append(p.backends, b)
}

// Remove removes the backend with the given address from the pool.
// It is a no-op if the address is not present.
func (p *Pool) Remove(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.backends[:0]
	for _, b := range p.backends {
		if b.Addr != addr {
			out = append(out, b)
		}
	}
	p.backends = out
}
