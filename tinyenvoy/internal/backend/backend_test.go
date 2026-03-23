package backend

import (
	"sync/atomic"
	"testing"
)

func TestBackend_DefaultHealthy(t *testing.T) {
	// Go zero value for atomic.Bool is false; backends start unhealthy until confirmed.
	b := &Backend{Addr: "localhost:8081"}
	if b.IsHealthy() {
		t.Error("new Backend zero value should be IsHealthy()=false")
	}
	// Explicitly marking healthy should work.
	b2 := NewBackend("localhost:8082", true)
	if !b2.IsHealthy() {
		t.Error("Backend created with healthy=true should be healthy")
	}
}

func TestBackend_SetHealthy(t *testing.T) {
	b := NewBackend("localhost:8081", true)
	b.SetHealthy(false)
	if b.IsHealthy() {
		t.Error("Backend.IsHealthy() should be false after SetHealthy(false)")
	}
	b.SetHealthy(true)
	if !b.IsHealthy() {
		t.Error("Backend.IsHealthy() should be true after SetHealthy(true)")
	}
}

func TestBackend_ActiveConns(t *testing.T) {
	b := NewBackend("localhost:8081", true)
	atomic.AddInt64(&b.ActiveConns, 1)
	if atomic.LoadInt64(&b.ActiveConns) != 1 {
		t.Error("ActiveConns should be atomic-incrementable")
	}
	atomic.AddInt64(&b.ActiveConns, -1)
	if atomic.LoadInt64(&b.ActiveConns) != 0 {
		t.Error("ActiveConns should be atomic-decrementable")
	}
}

func TestBackend_IncDecConns(t *testing.T) {
	b := NewBackend("localhost:8081", true)
	b.IncConns()
	b.IncConns()
	if b.Conns() != 2 {
		t.Errorf("Conns() = %d, want 2", b.Conns())
	}
	b.DecConns()
	if b.Conns() != 1 {
		t.Errorf("Conns() = %d, want 1", b.Conns())
	}
}
