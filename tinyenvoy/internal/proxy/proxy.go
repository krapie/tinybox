// Package proxy implements a cluster proxy using httputil.ReverseProxy.
// Analogous to Envoy's cluster manager: selects an endpoint via LB policy
// and forwards the request, returning 502 if no healthy endpoint is available.
package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/balancer"
	"github.com/krapi0314/tinybox/tinyenvoy/internal/backend"
)

// Proxy forwards HTTP requests to a cluster endpoint selected by an LbPolicy.
// Analogous to Envoy's cluster proxy with httputil.ReverseProxy as the transport.
type Proxy struct {
	lb balancer.LbPolicy
}

// New creates a Proxy backed by the given load-balancing policy.
func New(lb balancer.LbPolicy) *Proxy {
	return &Proxy{lb: lb}
}

// ServeHTTP implements http.Handler. Picks a backend via the LB policy,
// rewrites the request URL to point at the backend, and reverse-proxies it.
// Returns 502 Bad Gateway if no healthy backend is available.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Use client IP as the ring-hash key (X-Forwarded-For or RemoteAddr).
	key := r.Header.Get("X-Forwarded-For")
	if key == "" {
		key = r.RemoteAddr
	}

	b := p.lb.Pick(key)
	if b == nil {
		http.Error(w, "no healthy upstream", http.StatusBadGateway)
		return
	}

	rp := newReverseProxy(b)
	rp.ServeHTTP(w, r)
}

// newReverseProxy creates an httputil.ReverseProxy targeting the given backend.
func newReverseProxy(b *backend.Backend) *httputil.ReverseProxy {
	target := &url.URL{
		Scheme: "http",
		Host:   b.Addr,
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
	}
	return rp
}
