// Package health implements Envoy's active health check model.
// One goroutine per endpoint, controlled by context.Context.
// HTTP GET checks drive HEALTHY/UNHEALTHY state transitions based on consecutive thresholds.
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/config"
)

// HealthPool is the subset of backend.Pool used by the health checker.
type HealthPool interface {
	SetHealthy(addr string, healthy bool)
}

// Checker performs active health checks on a single backend endpoint.
// Mirrors Envoy's health_check filter with consecutive failure/success counters.
type Checker struct {
	backend *backend.Backend
	cfg     config.HealthCheckConfig
	pool    HealthPool
	client  *http.Client
}

// New creates a health Checker for the given backend.
func New(b *backend.Backend, cfg config.HealthCheckConfig, pool HealthPool) *Checker {
	return &Checker{
		backend: b,
		cfg:     cfg,
		pool:    pool,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Run starts the health check loop. Blocks until ctx is cancelled.
// Should be called in a goroutine.
//
// State machine:
//
//	HEALTHY  ──[unhealthy_threshold failures]──► UNHEALTHY
//	UNHEALTHY ──[healthy_threshold successes]──► HEALTHY
func (c *Checker) Run(ctx context.Context) {
	var (
		consecutiveFailures  int
		consecutiveSuccesses int
	)

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok := c.check(ctx)
			if ok {
				consecutiveFailures = 0
				consecutiveSuccesses++
				if !c.backend.IsHealthy() && consecutiveSuccesses >= c.cfg.HealthyThreshold {
					c.pool.SetHealthy(c.backend.Addr, true)
				}
			} else {
				consecutiveSuccesses = 0
				consecutiveFailures++
				if c.backend.IsHealthy() && consecutiveFailures >= c.cfg.UnhealthyThreshold {
					c.pool.SetHealthy(c.backend.Addr, false)
				}
			}
		}
	}
}

// check performs a single HTTP GET health check.
// Returns true if the endpoint returns 2xx.
func (c *Checker) check(ctx context.Context) bool {
	url := fmt.Sprintf("http://%s%s", c.backend.Addr, c.cfg.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
