// Package registry is an in-memory DNS service record store.
// It maps fully-qualified DNS names to one or more IP addresses,
// supports TTL-based expiry, and rotates returned records (round-robin).
package registry

import (
	"sync"
	"sync/atomic"
	"time"
)

// ServiceRecord is one resolvable entry in the registry.
type ServiceRecord struct {
	Name string // FQDN with trailing dot, e.g. "whoami.default.svc.cluster.local."
	IP   string // IPv4 address, e.g. "172.19.0.2"
	Port int    // informational; not included in DNS A records
	TTL  uint32 // seconds
}

// timedRecord wraps a ServiceRecord with an expiry timestamp.
type timedRecord struct {
	ServiceRecord
	expiresAt time.Time
}

// nameEntry holds all records for a single DNS name plus a round-robin counter.
type nameEntry struct {
	records []timedRecord
	counter uint64
}

// Registry stores ServiceRecords indexed by FQDN.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*nameEntry
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*nameEntry)}
}

// Register adds a record using time.Now() as the start of the TTL window.
func (r *Registry) Register(rec ServiceRecord) {
	r.RegisterAt(rec, time.Now())
}

// RegisterAt adds a record with an explicit registration time (useful in tests).
func (r *Registry) RegisterAt(rec ServiceRecord, registeredAt time.Time) {
	expiresAt := registeredAt.Add(time.Duration(rec.TTL) * time.Second)
	tr := timedRecord{ServiceRecord: rec, expiresAt: expiresAt}

	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[rec.Name]
	if !ok {
		e = &nameEntry{}
		r.entries[rec.Name] = e
	}
	e.records = append(e.records, tr)
}

// Deregister removes all records for the given FQDN.
func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, name)
}

// Lookup returns live (non-expired) records for name, rotated by round-robin.
// The first element of the returned slice is the "next" record for this caller.
func (r *Registry) Lookup(name string) []ServiceRecord {
	r.mu.RLock()
	e, ok := r.entries[name]
	r.mu.RUnlock()
	if !ok {
		return nil
	}

	now := time.Now()

	// Collect live records.
	var live []timedRecord
	r.mu.RLock()
	for _, tr := range e.records {
		if now.Before(tr.expiresAt) {
			live = append(live, tr)
		}
	}
	r.mu.RUnlock()

	if len(live) == 0 {
		return nil
	}

	// Rotate: pick starting index via atomic counter, then build result.
	idx := int(atomic.AddUint64(&e.counter, 1)-1) % len(live)
	out := make([]ServiceRecord, len(live))
	for i, tr := range live {
		out[(i+len(live)-idx)%len(live)] = tr.ServiceRecord
	}
	return out
}

// ListAll returns all non-expired records across every registered name.
func (r *Registry) ListAll() []ServiceRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var out []ServiceRecord
	for _, e := range r.entries {
		for _, tr := range e.records {
			if now.Before(tr.expiresAt) {
				out = append(out, tr.ServiceRecord)
			}
		}
	}
	return out
}
