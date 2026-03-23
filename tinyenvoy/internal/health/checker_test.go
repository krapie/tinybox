package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/config"
)

func TestChecker_MarksUnhealthyAfterThreshold(t *testing.T) {
	// Serve 503 to simulate unhealthy endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	b := backend.NewBackend(srv.Listener.Addr().String(), true)
	pool := backend.NewPool([]*backend.Backend{b})

	hc := config.HealthCheckConfig{
		Path:               "/healthz",
		Interval:           20 * time.Millisecond,
		Timeout:            100 * time.Millisecond,
		UnhealthyThreshold: 2,
		HealthyThreshold:   2,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	checker := New(b, hc, pool)
	go checker.Run(ctx)

	// Wait for unhealthy threshold to be reached
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !b.IsHealthy() {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("backend should have been marked unhealthy after threshold failures")
}

func TestChecker_MarksHealthyAfterRecovery(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(false)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	b := backend.NewBackend(srv.Listener.Addr().String(), true)
	pool := backend.NewPool([]*backend.Backend{b})

	hc := config.HealthCheckConfig{
		Path:               "/healthz",
		Interval:           20 * time.Millisecond,
		Timeout:            100 * time.Millisecond,
		UnhealthyThreshold: 2,
		HealthyThreshold:   2,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	checker := New(b, hc, pool)
	go checker.Run(ctx)

	// Wait to become unhealthy
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !b.IsHealthy() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b.IsHealthy() {
		t.Fatal("backend should have become unhealthy")
	}

	// Now recover
	healthy.Store(true)

	// Wait to become healthy again
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if b.IsHealthy() {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("backend should have recovered to healthy after threshold successes")
}

func TestChecker_StopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := backend.NewBackend(srv.Listener.Addr().String(), true)
	pool := backend.NewPool([]*backend.Backend{b})

	hc := config.HealthCheckConfig{
		Path:               "/healthz",
		Interval:           50 * time.Millisecond,
		Timeout:            100 * time.Millisecond,
		UnhealthyThreshold: 3,
		HealthyThreshold:   2,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	checker := New(b, hc, pool)
	go func() {
		checker.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success: goroutine stopped
	case <-time.After(500 * time.Millisecond):
		t.Error("checker goroutine did not stop after context cancel")
	}
}

func TestChecker_StaysHealthyWhenEndpointOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := backend.NewBackend(srv.Listener.Addr().String(), true)
	pool := backend.NewPool([]*backend.Backend{b})

	hc := config.HealthCheckConfig{
		Path:               "/healthz",
		Interval:           20 * time.Millisecond,
		Timeout:            100 * time.Millisecond,
		UnhealthyThreshold: 3,
		HealthyThreshold:   2,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	checker := New(b, hc, pool)
	go checker.Run(ctx)

	<-ctx.Done()
	if !b.IsHealthy() {
		t.Error("backend should remain healthy when endpoint returns 200")
	}
}
