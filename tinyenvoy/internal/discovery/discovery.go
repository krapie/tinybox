// Package discovery polls the tinykube endpoint API and keeps a backend pool in sync.
// It mirrors Envoy's EDS (Endpoint Discovery Service) — the xDS variant that provides
// dynamic endpoint sets — but uses simple HTTP polling instead of gRPC streaming.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/balancer"
)

// Config holds the parameters for a single cluster's discovery.
type Config struct {
	// Service is the tinykube Service name to query.
	Service string `yaml:"service"`
	// Namespace is the Kubernetes namespace of the service.
	Namespace string `yaml:"namespace"`
	// Interval is how often to poll the endpoint API.
	Interval time.Duration `yaml:"interval"`
}

// serviceEndpoint mirrors tinykube's ServiceEndpoint type.
type serviceEndpoint struct {
	PodName string `json:"podName"`
	Addr    string `json:"addr"`
}

// Client fetches live endpoints from a tinykube API server.
type Client struct {
	addr string // tinykube API server base URL, e.g. "http://localhost:8080"
	hc   *http.Client
}

// NewClient creates a Client that queries the given tinykube address.
func NewClient(addr string) *Client {
	return &Client{
		addr: addr,
		hc:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Endpoints fetches the current live endpoints for a Service.
// Returns a slice of "localhost:{hostPort}" addresses.
func (c *Client) Endpoints(ns, svc string) ([]string, error) {
	url := fmt.Sprintf("%s/apis/v1/namespaces/%s/services/%s/endpoints", c.addr, ns, svc)
	resp, err := c.hc.Get(url)
	if err != nil {
		return nil, fmt.Errorf("discovery: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery: GET %s: status %d", url, resp.StatusCode)
	}

	var eps []serviceEndpoint
	if err := json.NewDecoder(resp.Body).Decode(&eps); err != nil {
		return nil, fmt.Errorf("discovery: decode: %w", err)
	}

	addrs := make([]string, len(eps))
	for i, ep := range eps {
		addrs[i] = ep.Addr
	}
	return addrs, nil
}

// Start launches a background goroutine that polls the endpoint API at cfg.Interval
// and diffs the result against pool and lb, adding new backends and removing stale ones.
// New backends are marked healthy=true by default; the health checker updates them later.
// The goroutine stops when ctx is cancelled.
//
// Both pool and lb are kept in sync so that:
//   - pool.SetHealthy(addr, false) (called by health checkers) also affects lb.Pick()
//     because the same *Backend pointer is shared between pool and lb.
//   - lb.Pick() sees dynamically discovered backends, not just static config entries.
func Start(ctx context.Context, c *Client, pool *backend.Pool, lb balancer.LbPolicy, cfg *Config) {
	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		// Poll immediately on startup.
		syncOnce(c, pool, lb, cfg)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncOnce(c, pool, lb, cfg)
			}
		}
	}()
}

// syncOnce fetches the current endpoint set and reconciles pool and lb.
func syncOnce(c *Client, pool *backend.Pool, lb balancer.LbPolicy, cfg *Config) {
	addrs, err := c.Endpoints(cfg.Namespace, cfg.Service)
	if err != nil {
		// Transient error — keep existing state.
		return
	}

	// Build a set of desired addresses.
	desired := make(map[string]bool, len(addrs))
	for _, addr := range addrs {
		desired[addr] = true
	}

	// Remove backends no longer in the endpoint list from both pool and lb.
	for _, b := range pool.All() {
		if !desired[b.Addr] {
			pool.Remove(b.Addr)
			lb.Remove(b.Addr)
		}
	}

	// Add new backends to both pool and lb (shared pointer so health updates affect both).
	for addr := range desired {
		b := backend.NewBackend(addr, true)
		pool.Add(b)
		lb.Add(b)
	}
}
