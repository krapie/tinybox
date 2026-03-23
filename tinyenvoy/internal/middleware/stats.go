package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/metrics"
)

// NewStats wraps next with a stats filter that records request counts and latency.
// Analogous to Envoy's stats filter: increments tinyenvoy_requests_total and
// observes tinyenvoy_request_duration_seconds for the given cluster and route.
func NewStats(reg *metrics.Registry, cluster, route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Seconds()

		statusStr := fmt.Sprintf("%d", rec.status)
		reg.RequestsTotal.WithLabelValues(cluster, route, statusStr).Inc()
		reg.RequestDuration.WithLabelValues(cluster, route).Observe(elapsed)
	})
}
