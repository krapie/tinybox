// Package config provides YAML configuration loading for tinyenvoy.
// It mirrors Envoy's static bootstrap structure: listener → route_config → clusters.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// TLSConfig mirrors Envoy's transport_socket / DownstreamTLSContext.
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

// ListenerConfig mirrors Envoy's Listener (static_resources.listeners).
type ListenerConfig struct {
	Addr string    `yaml:"addr"`
	TLS  TLSConfig `yaml:"tls"`
}

// AdminConfig mirrors Envoy's admin endpoint (admin.address).
type AdminConfig struct {
	Addr string `yaml:"addr"`
}

// EndpointConfig mirrors Envoy's LbEndpoint (locality_lb_endpoints).
type EndpointConfig struct {
	Addr string `yaml:"addr"`
}

// HealthCheckConfig mirrors Envoy's health_checks filter config.
type HealthCheckConfig struct {
	Path               string        `yaml:"path"`
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	UnhealthyThreshold int           `yaml:"unhealthy_threshold"`
	HealthyThreshold   int           `yaml:"healthy_threshold"`
}

// ClusterConfig mirrors Envoy's Cluster (static_resources.clusters).
type ClusterConfig struct {
	Name        string            `yaml:"name"`
	LbPolicy    string            `yaml:"lb_policy"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Endpoints   []EndpointConfig  `yaml:"endpoints"`
}

// RouteRule mirrors Envoy's Route (virtual_hosts[].routes[]).
type RouteRule struct {
	Prefix  string `yaml:"prefix"`
	Cluster string `yaml:"cluster"`
}

// VirtualHostConfig mirrors Envoy's VirtualHost (route_config.virtual_hosts).
type VirtualHostConfig struct {
	VirtualHost string      `yaml:"virtual_host"`
	Routes      []RouteRule `yaml:"routes"`
}

// Config is the top-level tinyenvoy configuration, analogous to Envoy's Bootstrap.
type Config struct {
	Listener ListenerConfig      `yaml:"listener"`
	Admin    AdminConfig         `yaml:"admin"`
	Clusters []ClusterConfig     `yaml:"clusters"`
	Routes   []VirtualHostConfig `yaml:"routes"`
}

// Load reads and parses a YAML config file at the given path.
// Returns an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return &cfg, nil
}
