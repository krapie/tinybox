package router

import (
	"testing"

	"github.com/krapi0314/tinybox/tinyenvoy/internal/config"
)

func makeRouter(routes []config.VirtualHostConfig) *Router {
	return New(routes)
}

func TestRouter_ExactVirtualHost(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "api.example.com",
			Routes: []config.RouteRule{
				{Prefix: "/v1", Cluster: "api-v1"},
				{Prefix: "/", Cluster: "api-default"},
			},
		},
	})

	cluster, ok := r.Match("api.example.com", "/v1/users")
	if !ok {
		t.Fatal("Match() should find route for api.example.com /v1/users")
	}
	if cluster != "api-v1" {
		t.Errorf("cluster = %q, want api-v1", cluster)
	}
}

func TestRouter_PrefixMatching_LongestFirst(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "api.example.com",
			Routes: []config.RouteRule{
				{Prefix: "/v1/users", Cluster: "users-cluster"},
				{Prefix: "/v1", Cluster: "api-v1"},
				{Prefix: "/", Cluster: "fallback"},
			},
		},
	})

	tests := []struct {
		path    string
		want    string
	}{
		{"/v1/users/123", "users-cluster"},
		{"/v1/orders", "api-v1"},
		{"/health", "fallback"},
	}

	for _, tc := range tests {
		cluster, ok := r.Match("api.example.com", tc.path)
		if !ok {
			t.Errorf("Match(api.example.com, %q) not found", tc.path)
			continue
		}
		if cluster != tc.want {
			t.Errorf("Match(api.example.com, %q) = %q, want %q", tc.path, cluster, tc.want)
		}
	}
}

func TestRouter_WildcardVirtualHost(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "api.example.com",
			Routes: []config.RouteRule{
				{Prefix: "/", Cluster: "api"},
			},
		},
		{
			VirtualHost: "*",
			Routes: []config.RouteRule{
				{Prefix: "/", Cluster: "catchall"},
			},
		},
	})

	// Exact host matches first
	cluster, ok := r.Match("api.example.com", "/anything")
	if !ok || cluster != "api" {
		t.Errorf("exact host: Match() = %q/%v, want api/true", cluster, ok)
	}

	// Unknown host falls through to wildcard
	cluster, ok = r.Match("unknown.host.com", "/anything")
	if !ok || cluster != "catchall" {
		t.Errorf("wildcard host: Match() = %q/%v, want catchall/true", cluster, ok)
	}
}

func TestRouter_NoMatch(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "api.example.com",
			Routes: []config.RouteRule{
				{Prefix: "/v1", Cluster: "api"},
			},
		},
	})

	_, ok := r.Match("other.com", "/v1")
	if ok {
		t.Error("Match() should return false for unknown virtual host")
	}
}

func TestRouter_EmptyRoutes(t *testing.T) {
	r := makeRouter(nil)
	_, ok := r.Match("any.host", "/any/path")
	if ok {
		t.Error("Match() on empty router should return false")
	}
}

func TestRouter_TableDriven(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "svc.internal",
			Routes: []config.RouteRule{
				{Prefix: "/api/v2", Cluster: "v2"},
				{Prefix: "/api", Cluster: "v1"},
				{Prefix: "/", Cluster: "root"},
			},
		},
		{
			VirtualHost: "*",
			Routes: []config.RouteRule{
				{Prefix: "/", Cluster: "wildcard-root"},
			},
		},
	})

	tests := []struct {
		host    string
		path    string
		cluster string
		found   bool
	}{
		{"svc.internal", "/api/v2/resource", "v2", true},
		{"svc.internal", "/api/v1/resource", "v1", true},
		{"svc.internal", "/health", "root", true},
		{"other.svc", "/health", "wildcard-root", true},
		{"other.svc", "/api/v2/anything", "wildcard-root", true}, // wildcard only has / prefix
	}

	for _, tc := range tests {
		cluster, ok := r.Match(tc.host, tc.path)
		if ok != tc.found {
			t.Errorf("Match(%q, %q) found=%v, want %v", tc.host, tc.path, ok, tc.found)
			continue
		}
		if tc.found && cluster != tc.cluster {
			t.Errorf("Match(%q, %q) = %q, want %q", tc.host, tc.path, cluster, tc.cluster)
		}
	}
}

func TestRouter_HostWithPort(t *testing.T) {
	r := makeRouter([]config.VirtualHostConfig{
		{
			VirtualHost: "api.example.com",
			Routes: []config.RouteRule{
				{Prefix: "/", Cluster: "api"},
			},
		},
	})

	// Host header often includes port; should strip port for matching
	cluster, ok := r.Match("api.example.com:8080", "/path")
	if !ok || cluster != "api" {
		t.Errorf("Match with port: got %q/%v, want api/true", cluster, ok)
	}
}
