package syncer_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinydns/registry"
	"github.com/krapi0314/tinybox/tinydns/syncer"
)

// fakeTinykube is an httptest server that returns fake pod and service lists.
type fakeTinykube struct {
	pods     []map[string]interface{}
	services []map[string]interface{}
}

func (f *fakeTinykube) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/v1/namespaces/default/pods", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.pods)
	})
	mux.HandleFunc("/apis/v1/namespaces/default/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.services)
	})
	return mux
}

func TestSyncerPopulatesRegistry(t *testing.T) {
	fake := &fakeTinykube{
		pods: []map[string]interface{}{
			{"name": "whoami-abc", "namespace": "default", "podIP": "172.19.0.2", "status": "Running",
				"labels": map[string]interface{}{"app": "whoami"}},
		},
		services: []map[string]interface{}{
			{"name": "whoami", "namespace": "default",
				"spec": map[string]interface{}{"selector": map[string]interface{}{"app": "whoami"}, "port": 80}},
		},
	}
	ts := httptest.NewServer(fake.handler())
	defer ts.Close()

	reg := registry.New()
	s := syncer.New(reg, ts.URL, "default", 50*time.Millisecond)
	s.Start()
	defer s.Stop()

	// Wait for at least one sync cycle.
	time.Sleep(150 * time.Millisecond)

	records := reg.Lookup("whoami.default.svc.cluster.local.")
	if len(records) == 0 {
		t.Fatal("registry has no records after sync")
	}
	if records[0].IP != "172.19.0.2" {
		t.Errorf("IP = %q, want 172.19.0.2", records[0].IP)
	}
}

func TestSyncerSkipsNonRunningPods(t *testing.T) {
	fake := &fakeTinykube{
		pods: []map[string]interface{}{
			{"name": "svc-abc", "namespace": "default", "podIP": "172.19.0.3", "status": "Pending",
				"labels": map[string]interface{}{"app": "svc"}},
		},
		services: []map[string]interface{}{
			{"name": "svc", "namespace": "default",
				"spec": map[string]interface{}{"selector": map[string]interface{}{"app": "svc"}, "port": 80}},
		},
	}
	ts := httptest.NewServer(fake.handler())
	defer ts.Close()

	reg := registry.New()
	s := syncer.New(reg, ts.URL, "default", 50*time.Millisecond)
	s.Start()
	defer s.Stop()

	time.Sleep(150 * time.Millisecond)

	records := reg.Lookup("svc.default.svc.cluster.local.")
	if len(records) != 0 {
		t.Errorf("expected no records for Pending pod, got %d", len(records))
	}
}

func TestSyncerMultiplePodsOnePodIP(t *testing.T) {
	fake := &fakeTinykube{
		pods: []map[string]interface{}{
			{"name": "api-1", "namespace": "default", "podIP": "10.0.0.1", "status": "Running",
				"labels": map[string]interface{}{"app": "api"}},
			{"name": "api-2", "namespace": "default", "podIP": "10.0.0.2", "status": "Running",
				"labels": map[string]interface{}{"app": "api"}},
		},
		services: []map[string]interface{}{
			{"name": "api", "namespace": "default",
				"spec": map[string]interface{}{"selector": map[string]interface{}{"app": "api"}, "port": 8080}},
		},
	}
	ts := httptest.NewServer(fake.handler())
	defer ts.Close()

	reg := registry.New()
	s := syncer.New(reg, ts.URL, "default", 50*time.Millisecond)
	s.Start()
	defer s.Stop()

	time.Sleep(150 * time.Millisecond)

	records := reg.Lookup("api.default.svc.cluster.local.")
	if len(records) != 2 {
		t.Errorf("expected 2 records (one per Running pod), got %d", len(records))
	}
}
