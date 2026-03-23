package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/balancer"
)

func TestProxy_ForwardsToBackend(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from upstream"))
	}))
	defer upstream.Close()

	b := backend.NewBackend(upstream.Listener.Addr().String(), true)
	lb := balancer.NewRoundRobin()
	lb.Add(b)

	p := New(lb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if rr.Body.String() != "hello from upstream" {
		t.Errorf("body = %q, want hello from upstream", rr.Body.String())
	}
}

func TestProxy_Returns502WhenNoBackend(t *testing.T) {
	lb := balancer.NewRoundRobin() // empty — no backends
	p := New(lb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestProxy_Returns502WhenAllUnhealthy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	b := backend.NewBackend(upstream.Listener.Addr().String(), false) // unhealthy
	lb := balancer.NewRoundRobin()
	lb.Add(b)

	p := New(lb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestProxy_PreservesPath(t *testing.T) {
	var receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	b := backend.NewBackend(upstream.Listener.Addr().String(), true)
	lb := balancer.NewRoundRobin()
	lb.Add(b)

	p := New(lb)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if receivedPath != "/api/v1/users" {
		t.Errorf("upstream received path %q, want /api/v1/users", receivedPath)
	}
}
