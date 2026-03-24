package registry_test

import (
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinydns/registry"
)

func TestRegistryRegister(t *testing.T) {
	r := registry.New()
	r.Register(registry.ServiceRecord{
		Name: "whoami.default.svc.cluster.local.",
		IP:   "172.19.0.2",
		Port: 80,
		TTL:  30,
	})

	records := r.Lookup("whoami.default.svc.cluster.local.")
	if len(records) != 1 {
		t.Fatalf("Lookup() = %d records, want 1", len(records))
	}
	if records[0].IP != "172.19.0.2" {
		t.Errorf("IP = %q, want 172.19.0.2", records[0].IP)
	}
}

func TestRegistryLookupMiss(t *testing.T) {
	r := registry.New()
	records := r.Lookup("unknown.default.svc.cluster.local.")
	if len(records) != 0 {
		t.Errorf("Lookup() = %d records, want 0", len(records))
	}
}

func TestRegistryRoundRobin(t *testing.T) {
	r := registry.New()
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.3", TTL: 30})

	seen := make(map[string]bool)
	for i := 0; i < 9; i++ {
		recs := r.Lookup("svc.default.svc.cluster.local.")
		if len(recs) == 0 {
			t.Fatal("Lookup() returned empty")
		}
		seen[recs[0].IP] = true
	}
	if len(seen) < 3 {
		t.Errorf("round-robin hit %d distinct IPs, want 3; seen=%v", len(seen), seen)
	}
}

func TestRegistryDeregister(t *testing.T) {
	r := registry.New()
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	r.Deregister("svc.default.svc.cluster.local.")

	records := r.Lookup("svc.default.svc.cluster.local.")
	if len(records) != 0 {
		t.Errorf("Lookup() after Deregister = %d records, want 0", len(records))
	}
}

func TestRegistryTTLExpiry(t *testing.T) {
	r := registry.New()
	r.RegisterAt(registry.ServiceRecord{
		Name: "ttl.default.svc.cluster.local.",
		IP:   "10.0.0.1",
		TTL:  1,
	}, time.Now().Add(-2*time.Second)) // already expired

	records := r.Lookup("ttl.default.svc.cluster.local.")
	if len(records) != 0 {
		t.Errorf("Lookup() of expired record = %d records, want 0", len(records))
	}
}

func TestRegistryListAll(t *testing.T) {
	r := registry.New()
	r.Register(registry.ServiceRecord{Name: "a.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	r.Register(registry.ServiceRecord{Name: "b.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})

	all := r.ListAll()
	if len(all) != 2 {
		t.Errorf("ListAll() = %d records, want 2", len(all))
	}
}

func TestRegistryDeregisterThenRegister(t *testing.T) {
	r := registry.New()
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.1", TTL: 30})
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.2", TTL: 30})
	r.Deregister("svc.default.svc.cluster.local.")
	r.Register(registry.ServiceRecord{Name: "svc.default.svc.cluster.local.", IP: "10.0.0.3", TTL: 30})

	records := r.Lookup("svc.default.svc.cluster.local.")
	if len(records) != 1 {
		t.Fatalf("Lookup() = %d records, want 1", len(records))
	}
	if records[0].IP != "10.0.0.3" {
		t.Errorf("IP = %q, want 10.0.0.3", records[0].IP)
	}
}
