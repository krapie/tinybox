// Package metrics provides a Prometheus registry with tinyenvoy metrics.
// Analogous to Envoy's stats sink, exposing cluster-level request counters,
// latency histograms, endpoint health gauges, and active connection gauges.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Registry holds all tinyenvoy Prometheus metrics.
// Each metric mirrors an Envoy cluster stat as described in the SPEC.
type Registry struct {
	// prometheus is the underlying registry (isolated from default global registry).
	prometheus *prometheus.Registry

	// RequestsTotal mirrors Envoy's cluster.upstream_rq_total.
	// Labels: cluster, route, status
	RequestsTotal *prometheus.CounterVec

	// RequestDuration mirrors Envoy's cluster.upstream_rq_time.
	// Labels: cluster, route
	RequestDuration *prometheus.HistogramVec

	// EndpointHealthy mirrors Envoy's cluster.membership_healthy.
	// Labels: cluster, endpoint
	EndpointHealthy *prometheus.GaugeVec

	// ActiveConnections mirrors Envoy's cluster.upstream_cx_active.
	// Labels: cluster, endpoint
	ActiveConnections *prometheus.GaugeVec
}

// NewRegistry creates and registers all tinyenvoy metrics in an isolated Prometheus registry.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "tinyenvoy_requests_total",
		Help: "Total number of requests proxied, by cluster, route, and HTTP status code.",
	}, []string{"cluster", "route", "status"})

	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tinyenvoy_request_duration_seconds",
		Help:    "Upstream request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"cluster", "route"})

	endpointHealthy := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tinyenvoy_endpoint_healthy",
		Help: "1 if the endpoint is currently healthy, 0 otherwise.",
	}, []string{"cluster", "endpoint"})

	activeConnections := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tinyenvoy_active_connections",
		Help: "Number of active connections to the upstream endpoint.",
	}, []string{"cluster", "endpoint"})

	reg.MustRegister(requestsTotal, requestDuration, endpointHealthy, activeConnections)

	return &Registry{
		prometheus:        reg,
		RequestsTotal:     requestsTotal,
		RequestDuration:   requestDuration,
		EndpointHealthy:   endpointHealthy,
		ActiveConnections: activeConnections,
	}
}

// Gatherer returns the underlying prometheus.Gatherer for use with promhttp.HandlerFor.
func (r *Registry) Gatherer() prometheus.Gatherer {
	return r.prometheus
}

// Reg returns the underlying *prometheus.Registry (for testing and handler setup).
func (r *Registry) Reg() *prometheus.Registry {
	return r.prometheus
}
