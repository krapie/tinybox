package balancer

import (
	"testing"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
)

func TestRoundRobin_Pick_Cycles(t *testing.T) {
	backends := []*backend.Backend{
		backend.NewBackend("a:8081", true),
		backend.NewBackend("b:8082", true),
		backend.NewBackend("c:8083", true),
	}
	rr := NewRoundRobin()
	for _, b := range backends {
		rr.Add(b)
	}

	// Should cycle through all backends in order
	seen := make([]string, 6)
	for i := range seen {
		b := rr.Pick("")
		if b == nil {
			t.Fatal("Pick() returned nil")
		}
		seen[i] = b.Addr
	}

	// First 3 should cover all 3 backends exactly once (in some order)
	first3 := map[string]bool{seen[0]: true, seen[1]: true, seen[2]: true}
	if len(first3) != 3 {
		t.Errorf("first 3 picks should cover all 3 backends, got %v", seen[:3])
	}
	// Second 3 should be the same as first 3 (cycling)
	for i := 0; i < 3; i++ {
		if seen[i] != seen[i+3] {
			t.Errorf("pick[%d]=%q should equal pick[%d]=%q (cycling)", i, seen[i], i+3, seen[i+3])
		}
	}
}

func TestRoundRobin_Pick_OnlyHealthy(t *testing.T) {
	rr := NewRoundRobin()
	rr.Add(backend.NewBackend("a:8081", false))
	rr.Add(backend.NewBackend("b:8082", true))

	for i := 0; i < 5; i++ {
		b := rr.Pick("")
		if b == nil {
			t.Fatal("Pick() returned nil")
		}
		if b.Addr != "b:8082" {
			t.Errorf("Pick() = %q, want b:8082 (only healthy)", b.Addr)
		}
	}
}

func TestRoundRobin_Pick_NilWhenEmpty(t *testing.T) {
	rr := NewRoundRobin()
	if b := rr.Pick(""); b != nil {
		t.Errorf("Pick() on empty = %v, want nil", b)
	}
}

func TestRoundRobin_Pick_NilWhenAllUnhealthy(t *testing.T) {
	rr := NewRoundRobin()
	rr.Add(backend.NewBackend("a:8081", false))
	if b := rr.Pick(""); b != nil {
		t.Errorf("Pick() with all unhealthy = %v, want nil", b)
	}
}

func TestRoundRobin_Remove(t *testing.T) {
	rr := NewRoundRobin()
	rr.Add(backend.NewBackend("a:8081", true))
	rr.Add(backend.NewBackend("b:8082", true))
	rr.Remove("a:8081")

	for i := 0; i < 5; i++ {
		b := rr.Pick("")
		if b == nil {
			t.Fatal("Pick() returned nil after remove")
		}
		if b.Addr == "a:8081" {
			t.Error("removed backend should not be picked")
		}
	}
}

func TestRoundRobin_Add_DuplicateIgnored(t *testing.T) {
	rr := NewRoundRobin()
	b := backend.NewBackend("a:8081", true)
	rr.Add(b)
	rr.Add(b) // duplicate

	// Should still cycle correctly (only one entry)
	p1 := rr.Pick("")
	p2 := rr.Pick("")
	if p1 == nil || p2 == nil {
		t.Fatal("Pick() returned nil")
	}
	if p1.Addr != "a:8081" || p2.Addr != "a:8081" {
		t.Error("with single backend, both picks should return same addr")
	}
}
