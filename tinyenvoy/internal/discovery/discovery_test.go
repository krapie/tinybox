package discovery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/balancer"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/discovery"
)

// endpoint mirrors the tinykube ServiceEndpoint type.
type endpoint struct {
	PodName string `json:"podName"`
	Addr    string `json:"addr"`
}

func makeEndpointServer(endpoints []endpoint) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/v1/namespaces/default/services/web-svc/endpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(endpoints)
	})
	return httptest.NewServer(mux)
}

func TestClient_Endpoints(t *testing.T) {
	eps := []endpoint{
		{PodName: "web-abc", Addr: "localhost:54321"},
		{PodName: "web-def", Addr: "localhost:54322"},
	}
	srv := makeEndpointServer(eps)
	defer srv.Close()

	c := discovery.NewClient(srv.URL)
	addrs, err := c.Endpoints("default", "web-svc")
	if err != nil {
		t.Fatalf("Endpoints() error: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("Endpoints() = %d addrs, want 2", len(addrs))
	}
	if addrs[0] != "localhost:54321" {
		t.Errorf("addrs[0] = %q, want localhost:54321", addrs[0])
	}
}

func TestClient_EndpointsEmpty(t *testing.T) {
	srv := makeEndpointServer(nil)
	defer srv.Close()

	c := discovery.NewClient(srv.URL)
	addrs, err := c.Endpoints("default", "web-svc")
	if err != nil {
		t.Fatalf("Endpoints() error: %v", err)
	}
	if len(addrs) != 0 {
		t.Fatalf("Endpoints() = %d addrs, want 0", len(addrs))
	}
}

func TestClient_EndpointsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := discovery.NewClient(srv.URL)
	_, err := c.Endpoints("default", "missing-svc")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestStartDiscovery_AddsEndpoints(t *testing.T) {
	eps := []endpoint{
		{PodName: "web-abc", Addr: "localhost:54321"},
	}
	srv := makeEndpointServer(eps)
	defer srv.Close()

	pool := backend.NewPool(nil)
	lb := balancer.NewRoundRobin()
	c := discovery.NewClient(srv.URL)

	cfg := &discovery.Config{
		Service:   "web-svc",
		Namespace: "default",
		Interval:  20 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery.Start(ctx, c, pool, lb, cfg)

	// Give goroutine time to poll.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pool.All()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	all := pool.All()
	if len(all) != 1 {
		t.Fatalf("pool.All() = %d, want 1", len(all))
	}
	if all[0].Addr != "localhost:54321" {
		t.Errorf("backend addr = %q, want localhost:54321", all[0].Addr)
	}

	// Verify lb also has the backend
	b := lb.Pick("")
	if b == nil {
		t.Fatal("lb.Pick() returned nil — backend not added to lb")
	}
	if b.Addr != "localhost:54321" {
		t.Errorf("lb.Pick().Addr = %q, want localhost:54321", b.Addr)
	}
}

func TestStartDiscovery_RemovesStaleEndpoints(t *testing.T) {
	// Start with one endpoint, then return empty.
	current := []endpoint{{PodName: "web-abc", Addr: "localhost:54321"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/v1/namespaces/default/services/web-svc/endpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(current)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pool := backend.NewPool(nil)
	lb := balancer.NewRoundRobin()
	c := discovery.NewClient(srv.URL)
	cfg := &discovery.Config{
		Service:   "web-svc",
		Namespace: "default",
		Interval:  20 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discovery.Start(ctx, c, pool, lb, cfg)

	// Wait for initial endpoint to appear.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pool.All()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pool.All()) != 1 {
		t.Fatalf("initial pool size = %d, want 1", len(pool.All()))
	}

	// Remove the endpoint from the server response.
	current = nil

	// Wait for removal from both pool and lb.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(pool.All()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(pool.All()) != 0 {
		t.Fatalf("after removal, pool size = %d, want 0", len(pool.All()))
	}
	if lb.Pick("") != nil {
		t.Error("lb.Pick() should return nil after removal")
	}
}
