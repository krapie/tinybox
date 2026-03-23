package balancer

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
)

// RingHash implements Envoy's RING_HASH lb_policy.
// Uses crc32 with 150 virtual nodes per endpoint (Envoy default minimum).
// Provides sticky sessions: same key always routes to same healthy endpoint.
type RingHash struct {
	mu          sync.RWMutex
	vnodes      int
	ring        []uint32             // sorted hash positions
	nodeMap     map[uint32]string    // hash → backend addr
	backendMap  map[string]*backend.Backend
}

// NewRingHash creates a RingHash balancer with the given number of virtual nodes per endpoint.
func NewRingHash(vnodes int) *RingHash {
	if vnodes <= 0 {
		vnodes = 150
	}
	return &RingHash{
		vnodes:     vnodes,
		nodeMap:    make(map[uint32]string),
		backendMap: make(map[string]*backend.Backend),
	}
}

// Add adds a backend to the ring. Duplicate addresses are ignored.
func (rh *RingHash) Add(b *backend.Backend) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	if _, exists := rh.backendMap[b.Addr]; exists {
		return
	}
	rh.backendMap[b.Addr] = b
	rh.addVnodes(b.Addr)
}

// addVnodes adds virtual nodes for the given address. Must be called with lock held.
func (rh *RingHash) addVnodes(addr string) {
	for i := 0; i < rh.vnodes; i++ {
		key := fmt.Sprintf("%s#%d", addr, i)
		hash := crc32.ChecksumIEEE([]byte(key))
		rh.ring = append(rh.ring, hash)
		rh.nodeMap[hash] = addr
	}
	sort.Slice(rh.ring, func(i, j int) bool {
		return rh.ring[i] < rh.ring[j]
	})
}

// Remove removes the backend with the given address from the ring.
func (rh *RingHash) Remove(addr string) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	delete(rh.backendMap, addr)

	// Rebuild ring without the removed backend's vnodes
	newRing := rh.ring[:0]
	for _, h := range rh.ring {
		if rh.nodeMap[h] != addr {
			newRing = append(newRing, h)
		} else {
			delete(rh.nodeMap, h)
		}
	}
	rh.ring = newRing
}

// Pick selects a backend using consistent hashing on the given key (client IP).
// Walks the ring clockwise to find the first healthy backend.
// Returns nil if no healthy backends exist.
func (rh *RingHash) Pick(key string) *backend.Backend {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	if len(rh.ring) == 0 {
		return nil
	}

	hash := crc32.ChecksumIEEE([]byte(key))

	// Binary search for the successor node on the ring
	idx := sort.Search(len(rh.ring), func(i int) bool {
		return rh.ring[i] >= hash
	})
	if idx == len(rh.ring) {
		idx = 0 // wrap around
	}

	// Walk the ring to find a healthy backend (at most full circle)
	for i := 0; i < len(rh.ring); i++ {
		pos := (idx + i) % len(rh.ring)
		addr := rh.nodeMap[rh.ring[pos]]
		b, ok := rh.backendMap[addr]
		if ok && b.IsHealthy() {
			return b
		}
	}
	return nil
}
