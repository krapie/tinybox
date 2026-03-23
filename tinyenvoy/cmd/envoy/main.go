// Command envoy is the tinyenvoy L7 proxy entry point.
// Wires listener, router, cluster manager, load balancers, health checks,
// Prometheus metrics, and hot-reload into a running proxy.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/balancer"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/config"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/health"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/metrics"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/middleware"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/proxy"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/router"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Metrics registry (Prometheus, analogous to Envoy stats sink)
	reg := metrics.NewRegistry()

	// Build cluster manager: pool + balancer + health checker per cluster
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clusters := buildClusters(ctx, cfg, reg, logger)

	// Router: virtual-host + path-prefix matching
	rt := router.New(cfg.Routes)

	// Top-level handler: route → cluster proxy
	mux := buildMux(rt, clusters, reg, logger)

	// Admin / metrics server (Prometheus /metrics endpoint)
	adminAddr := cfg.Admin.Addr
	if adminAddr == "" {
		adminAddr = ":9090"
	}
	adminMux := http.NewServeMux()
	adminMux.Handle("/metrics", promhttp.HandlerFor(reg.Gatherer(), promhttp.HandlerOpts{}))
	adminSrv := &http.Server{Addr: adminAddr, Handler: adminMux}
	go func() {
		logger.Info("admin listening", "addr", adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("admin server error", "err", err)
		}
	}()

	// Config hot-reload (xDS analogue)
	watcher, err := config.NewWatcher(*configPath, func() {
		newCfg, err := config.Load(*configPath)
		if err != nil {
			logger.Warn("config reload failed", "err", err)
			return
		}
		logger.Info("config reloaded")
		_ = newCfg // in a full implementation: diff and swap router/pools
	})
	if err != nil {
		logger.Warn("config watcher failed", "err", err)
	} else {
		defer watcher.Close()
	}

	// Main proxy server
	listenerAddr := cfg.Listener.Addr
	if listenerAddr == "" {
		listenerAddr = ":8080"
	}

	srv := &http.Server{
		Addr:    listenerAddr,
		Handler: mux,
	}

	// TLS termination
	if cfg.Listener.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(cfg.Listener.TLS.Cert, cfg.Listener.TLS.Key)
		if err != nil {
			logger.Error("TLS key pair load failed", "err", err)
			os.Exit(1)
		}
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	// Graceful shutdown on SIGTERM / SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		logger.Info("shutting down")
		cancel()
		_ = srv.Shutdown(context.Background())
		_ = adminSrv.Shutdown(context.Background())
	}()

	logger.Info("tinyenvoy listening", "addr", listenerAddr)
	if cfg.Listener.TLS.Enabled {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}

// clusterEntry holds the LB policy and pool for one cluster.
type clusterEntry struct {
	lb   balancer.LbPolicy
	pool *backend.Pool
}

// buildClusters constructs a pool + LB policy + health checkers for every cluster in cfg.
func buildClusters(ctx context.Context, cfg *config.Config, reg *metrics.Registry, logger *slog.Logger) map[string]*clusterEntry {
	entries := make(map[string]*clusterEntry, len(cfg.Clusters))

	for _, cc := range cfg.Clusters {
		backends := make([]*backend.Backend, len(cc.Endpoints))
		for i, ep := range cc.Endpoints {
			backends[i] = backend.NewBackend(ep.Addr, true)
		}
		pool := backend.NewPool(backends)

		var lb balancer.LbPolicy
		if strings.EqualFold(cc.LbPolicy, "ring-hash") {
			lb = balancer.NewRingHash(150)
		} else {
			lb = balancer.NewRoundRobin()
		}
		for _, b := range backends {
			lb.Add(b)
		}

		// Start health checkers if configured
		if cc.HealthCheck.Path != "" {
			for _, b := range backends {
				b := b // capture
				checker := health.New(b, cc.HealthCheck, pool)
				go checker.Run(ctx)
				// Seed endpoint_healthy gauge
				reg.EndpointHealthy.WithLabelValues(cc.Name, b.Addr).Set(1)
			}
		}

		entries[cc.Name] = &clusterEntry{lb: lb, pool: pool}
	}

	return entries
}

// buildMux returns an http.Handler that routes requests to the correct cluster proxy.
func buildMux(rt *router.Router, clusters map[string]*clusterEntry, reg *metrics.Registry, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if host == "" {
			host = r.Header.Get("Host")
		}
		// Strip port for matching
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		clusterName, ok := rt.Match(host, r.URL.Path)
		if !ok {
			http.Error(w, "no route matched", http.StatusNotFound)
			return
		}

		entry, ok := clusters[clusterName]
		if !ok {
			http.Error(w, "unknown cluster: "+clusterName, http.StatusBadGateway)
			return
		}

		// Build per-request handler: stats → proxy
		p := proxy.New(entry.lb)
		h := middleware.NewStats(reg, clusterName, r.URL.Path, p)
		h = middleware.NewAccessLog(logger, h)
		h.ServeHTTP(w, r)
	})
}
