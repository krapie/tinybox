// Package middleware provides HTTP filter chain components analogous to Envoy's http_filters.
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseRecorder captures the status code written by the inner handler.
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

// NewAccessLog wraps next with an access-log filter.
// Analogous to Envoy's access_log filter: logs method, path, status, and latency.
func NewAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("access",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}
