package plugins_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinydns/plugins"
)

func TestHealthEndpointReturns200(t *testing.T) {
	hp := plugins.NewHealth("127.0.0.1:0")
	addr, err := hp.Start()
	if err != nil {
		t.Fatalf("health.Start: %v", err)
	}
	t.Cleanup(func() { _ = hp.Stop() })

	// Give the HTTP server a moment to bind.
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
