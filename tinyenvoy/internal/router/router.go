// Package router implements Envoy's RouteConfiguration concept.
// Matches HTTP requests to cluster names based on virtual host and path prefix.
package router

import (
	"net"
	"sort"
	"strings"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/config"
)

// Router matches (host, path) pairs to upstream cluster names.
// Analogous to Envoy's RouteConfiguration with virtual_hosts.
type Router struct {
	// vhosts holds non-wildcard virtual hosts
	vhosts map[string][]config.RouteRule
	// wildcard holds the catch-all virtual host routes (virtual_host: "*")
	wildcard []config.RouteRule
}

// New creates a Router from the given virtual host configurations.
// Routes within each virtual host are sorted longest-prefix-first for correct matching.
func New(routes []config.VirtualHostConfig) *Router {
	r := &Router{
		vhosts: make(map[string][]config.RouteRule),
	}
	for _, vh := range routes {
		sorted := sortedRules(vh.Routes)
		if vh.VirtualHost == "*" {
			r.wildcard = sorted
		} else {
			r.vhosts[vh.VirtualHost] = sorted
		}
	}
	return r
}

// sortedRules returns a copy of rules sorted by prefix length descending
// (longest prefix first, for correct longest-match semantics).
func sortedRules(rules []config.RouteRule) []config.RouteRule {
	out := make([]config.RouteRule, len(rules))
	copy(out, rules)
	sort.Slice(out, func(i, j int) bool {
		return len(out[i].Prefix) > len(out[j].Prefix)
	})
	return out
}

// Match finds the cluster name for the given virtual host and request path.
// Implements Envoy's virtual host matching: exact match first, then wildcard.
// Within a virtual host, longest-prefix match wins.
// Returns ("", false) if no match is found.
func (r *Router) Match(host, path string) (string, bool) {
	// Strip port from host header (e.g. "api.example.com:8080" → "api.example.com")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Try exact virtual host match first
	if rules, ok := r.vhosts[host]; ok {
		if cluster, found := matchRules(rules, path); found {
			return cluster, true
		}
	}

	// Fall back to wildcard virtual host
	if r.wildcard != nil {
		if cluster, found := matchRules(r.wildcard, path); found {
			return cluster, true
		}
	}

	return "", false
}

// matchRules finds the first (longest-prefix) rule matching the path.
func matchRules(rules []config.RouteRule, path string) (string, bool) {
	for _, rule := range rules {
		if strings.HasPrefix(path, rule.Prefix) {
			return rule.Cluster, true
		}
	}
	return "", false
}
