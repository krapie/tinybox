package backend

import (
	"testing"
)

func TestPool_Healthy_FiltersUnhealthy(t *testing.T) {
	p := NewPool([]*Backend{
		NewBackend("a:8081", true),
		NewBackend("b:8082", false),
		NewBackend("c:8083", true),
	})

	healthy := p.Healthy()
	if len(healthy) != 2 {
		t.Fatalf("Healthy() returned %d backends, want 2", len(healthy))
	}
	for _, b := range healthy {
		if !b.IsHealthy() {
			t.Errorf("Healthy() returned unhealthy backend %q", b.Addr)
		}
	}
}

func TestPool_Healthy_AllUnhealthy(t *testing.T) {
	p := NewPool([]*Backend{
		NewBackend("a:8081", false),
	})
	if len(p.Healthy()) != 0 {
		t.Error("Healthy() should return empty slice when all are unhealthy")
	}
}

func TestPool_Healthy_Empty(t *testing.T) {
	p := NewPool(nil)
	if len(p.Healthy()) != 0 {
		t.Error("Healthy() should return empty slice for empty pool")
	}
}

func TestPool_SetHealthy(t *testing.T) {
	p := NewPool([]*Backend{
		NewBackend("a:8081", true),
		NewBackend("b:8082", true),
	})

	p.SetHealthy("a:8081", false)
	healthy := p.Healthy()
	if len(healthy) != 1 {
		t.Fatalf("after SetHealthy(false), Healthy() = %d, want 1", len(healthy))
	}
	if healthy[0].Addr != "b:8082" {
		t.Errorf("remaining healthy = %q, want b:8082", healthy[0].Addr)
	}

	p.SetHealthy("a:8081", true)
	if len(p.Healthy()) != 2 {
		t.Error("after SetHealthy(true), should have 2 healthy backends")
	}
}

func TestPool_All(t *testing.T) {
	p := NewPool([]*Backend{
		NewBackend("a:8081", true),
		NewBackend("b:8082", false),
	})
	all := p.All()
	if len(all) != 2 {
		t.Fatalf("All() = %d, want 2", len(all))
	}
}

func TestPool_ConcurrentSetHealthy(t *testing.T) {
	p := NewPool([]*Backend{
		NewBackend("a:8081", true),
	})

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			p.SetHealthy("a:8081", i%2 == 0)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// Just verifying no race — result doesn't matter
	_ = p.Healthy()
}
