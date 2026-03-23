package balancer

import (
	"fmt"
	"testing"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
)

func TestRingHash_StickyRouting(t *testing.T) {
	rh := NewRingHash(150)
	rh.Add(backend.NewBackend("a:8081", true))
	rh.Add(backend.NewBackend("b:8082", true))
	rh.Add(backend.NewBackend("c:8083", true))

	// Same key should always route to same backend
	key := "1.2.3.4"
	first := rh.Pick(key)
	if first == nil {
		t.Fatal("Pick() returned nil")
	}
	for i := 0; i < 10; i++ {
		b := rh.Pick(key)
		if b == nil {
			t.Fatal("Pick() returned nil")
		}
		if b.Addr != first.Addr {
			t.Errorf("pick[%d] = %q, want %q (sticky)", i, b.Addr, first.Addr)
		}
	}
}

func TestRingHash_DifferentKeys_DifferentBackends(t *testing.T) {
	rh := NewRingHash(150)
	rh.Add(backend.NewBackend("a:8081", true))
	rh.Add(backend.NewBackend("b:8082", true))
	rh.Add(backend.NewBackend("c:8083", true))

	// With many different keys, all 3 backends should be reachable
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		b := rh.Pick(fmt.Sprintf("192.168.1.%d", i))
		if b != nil {
			seen[b.Addr] = true
		}
	}
	if len(seen) < 2 {
		t.Errorf("ring hash hit only %d backends out of 3 with 1000 keys", len(seen))
	}
}

func TestRingHash_OnlyHealthy(t *testing.T) {
	rh := NewRingHash(150)
	rh.Add(backend.NewBackend("a:8081", false))
	rh.Add(backend.NewBackend("b:8082", true))

	for i := 0; i < 20; i++ {
		b := rh.Pick(fmt.Sprintf("key-%d", i))
		if b == nil {
			t.Fatal("Pick() returned nil")
		}
		if b.Addr != "b:8082" {
			t.Errorf("got unhealthy backend %q", b.Addr)
		}
	}
}

func TestRingHash_NilWhenEmpty(t *testing.T) {
	rh := NewRingHash(150)
	if b := rh.Pick("key"); b != nil {
		t.Errorf("Pick() on empty ring = %v, want nil", b)
	}
}

func TestRingHash_NilWhenAllUnhealthy(t *testing.T) {
	rh := NewRingHash(150)
	rh.Add(backend.NewBackend("a:8081", false))
	if b := rh.Pick("key"); b != nil {
		t.Errorf("Pick() all unhealthy = %v, want nil", b)
	}
}

func TestRingHash_Remove(t *testing.T) {
	rh := NewRingHash(150)
	rh.Add(backend.NewBackend("a:8081", true))
	rh.Add(backend.NewBackend("b:8082", true))
	rh.Remove("a:8081")

	for i := 0; i < 20; i++ {
		b := rh.Pick(fmt.Sprintf("key-%d", i))
		if b != nil && b.Addr == "a:8081" {
			t.Error("removed backend should not be picked")
		}
	}
}
