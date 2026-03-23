package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_BasicConfig(t *testing.T) {
	yaml := `
listener:
  addr: ":8080"
  tls:
    enabled: false
    cert: "cert.pem"
    key: "key.pem"

admin:
  addr: ":9090"

clusters:
  - name: api
    lb_policy: round-robin
    health_check:
      path: /healthz
      interval: 10s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    endpoints:
      - addr: localhost:8081
      - addr: localhost:8082

routes:
  - virtual_host: "api.example.com"
    routes:
      - prefix: /v1
        cluster: api
      - prefix: /
        cluster: api
  - virtual_host: "*"
    routes:
      - prefix: /
        cluster: api
`
	f, err := os.CreateTemp("", "tinyenvoy-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Listener
	if cfg.Listener.Addr != ":8080" {
		t.Errorf("listener.addr = %q, want :8080", cfg.Listener.Addr)
	}
	if cfg.Listener.TLS.Enabled {
		t.Error("listener.tls.enabled should be false")
	}
	if cfg.Listener.TLS.Cert != "cert.pem" {
		t.Errorf("listener.tls.cert = %q, want cert.pem", cfg.Listener.TLS.Cert)
	}

	// Admin
	if cfg.Admin.Addr != ":9090" {
		t.Errorf("admin.addr = %q, want :9090", cfg.Admin.Addr)
	}

	// Clusters
	if len(cfg.Clusters) != 1 {
		t.Fatalf("len(clusters) = %d, want 1", len(cfg.Clusters))
	}
	cl := cfg.Clusters[0]
	if cl.Name != "api" {
		t.Errorf("clusters[0].name = %q, want api", cl.Name)
	}
	if cl.LbPolicy != "round-robin" {
		t.Errorf("clusters[0].lb_policy = %q, want round-robin", cl.LbPolicy)
	}
	if cl.HealthCheck.Path != "/healthz" {
		t.Errorf("health_check.path = %q, want /healthz", cl.HealthCheck.Path)
	}
	if cl.HealthCheck.Interval != 10*time.Second {
		t.Errorf("health_check.interval = %v, want 10s", cl.HealthCheck.Interval)
	}
	if cl.HealthCheck.Timeout != 2*time.Second {
		t.Errorf("health_check.timeout = %v, want 2s", cl.HealthCheck.Timeout)
	}
	if cl.HealthCheck.UnhealthyThreshold != 3 {
		t.Errorf("health_check.unhealthy_threshold = %d, want 3", cl.HealthCheck.UnhealthyThreshold)
	}
	if cl.HealthCheck.HealthyThreshold != 2 {
		t.Errorf("health_check.healthy_threshold = %d, want 2", cl.HealthCheck.HealthyThreshold)
	}
	if len(cl.Endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2", len(cl.Endpoints))
	}
	if cl.Endpoints[0].Addr != "localhost:8081" {
		t.Errorf("endpoints[0].addr = %q, want localhost:8081", cl.Endpoints[0].Addr)
	}

	// Routes
	if len(cfg.Routes) != 2 {
		t.Fatalf("len(routes) = %d, want 2", len(cfg.Routes))
	}
	if cfg.Routes[0].VirtualHost != "api.example.com" {
		t.Errorf("routes[0].virtual_host = %q, want api.example.com", cfg.Routes[0].VirtualHost)
	}
	if len(cfg.Routes[0].Routes) != 2 {
		t.Errorf("len(routes[0].routes) = %d, want 2", len(cfg.Routes[0].Routes))
	}
	if cfg.Routes[0].Routes[0].Prefix != "/v1" {
		t.Errorf("routes[0].routes[0].prefix = %q, want /v1", cfg.Routes[0].Routes[0].Prefix)
	}
	if cfg.Routes[0].Routes[0].Cluster != "api" {
		t.Errorf("routes[0].routes[0].cluster = %q, want api", cfg.Routes[0].Routes[0].Cluster)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() should return error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "tinyenvoy-bad-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(":: invalid yaml ::")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("Load() should return error for invalid YAML")
	}
}

func TestLoad_RingHashPolicy(t *testing.T) {
	yaml := `
listener:
  addr: ":8080"
admin:
  addr: ":9090"
clusters:
  - name: web
    lb_policy: ring-hash
    endpoints:
      - addr: localhost:9001
routes: []
`
	f, err := os.CreateTemp("", "tinyenvoy-rh-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Clusters[0].LbPolicy != "ring-hash" {
		t.Errorf("lb_policy = %q, want ring-hash", cfg.Clusters[0].LbPolicy)
	}
}
