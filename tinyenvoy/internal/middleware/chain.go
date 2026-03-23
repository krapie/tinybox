package middleware

import (
	"log/slog"
	"net/http"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/metrics"
)

// Chain builds the tinyenvoy HTTP filter chain:
//
//	Downstream → [access-log] → [stats] → next
//
// Analogous to Envoy's http_filters list: access_log + stats are applied before
// the router/cluster filter (next).
func Chain(logger *slog.Logger, reg *metrics.Registry, cluster, route string, next http.Handler) http.Handler {
	// Innermost: stats filter
	h := NewStats(reg, cluster, route, next)
	// Outermost: access log filter
	h = NewAccessLog(logger, h)
	return h
}
