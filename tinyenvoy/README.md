# tinyenvoy

A toy Envoy L7 proxy implementation — virtual-host routing, round-robin + ring-hash load balancing, active health checks, Prometheus metrics, and hot-reload.

Built as part of [tinybox](../README.md), a collection of simplified infrastructure and tooling implementations for study.

## What it teaches

| Concept | Where |
|---|---|
| Listener → Route → Cluster pipeline | `cmd/envoy/main.go` |
| Virtual-host + prefix routing (RouteConfiguration) | `internal/router/router.go` |
| Round-robin LB (`ROUND_ROBIN` policy) | `internal/balancer/roundrobin.go` |
| Consistent-hash LB (`RING_HASH` policy) | `internal/balancer/ringhash.go` |
| Active health checks (HEALTHY/UNHEALTHY state machine) | `internal/health/checker.go` |
| Endpoint pool with dynamic Add/Remove (ClusterLoadAssignment) | `internal/backend/pool.go` |
| EDS-style endpoint discovery (polls tinykube Service API) | `internal/discovery/discovery.go` |
| Prometheus stats sink | `internal/metrics/metrics.go` |
| Access-log filter | `internal/middleware/logging.go` |
| Stats filter | `internal/middleware/metrics.go` |
| Cluster proxy (httputil.ReverseProxy) | `internal/proxy/proxy.go` |
| Hot-reload (xDS file analogue, fsnotify) | `internal/config/watcher.go` |

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                         tinyenvoy                          │
│                                                            │
│  Downstream                                                │
│     │                                                      │
│     ▼                                                      │
│  ┌──────────────────────────────────────────────────────┐  │
│  │               HTTP Filter Chain                      │  │
│  │  [access-log] → [stats] → [router] → [cluster proxy] │  │
│  └──────────────────────────────────────────────────────┘  │
│     │                                                      │
│     ▼                                                      │
│  ┌────────────┐   ┌─────────────────────────────────────┐  │
│  │   Router   │   │         Cluster Manager             │  │
│  │ vhost+path │──▶│  pool + LB policy + health checker  │  │
│  └────────────┘   └──────────────┬──────────────────────┘  │
│                                  │                         │
│                    ┌─────────────▼────────────────┐        │
│                    │  Upstream endpoints           │        │
│                    │  (RoundRobin or RingHash)     │        │
│                    └──────────────────────────────┘        │
└────────────────────────────────────────────────────────────┘
```

## How Envoy concepts map to tinyenvoy

| Envoy concept | tinyenvoy equivalent |
|---|---|
| Listener | `listener.addr` in config |
| RouteConfiguration / VirtualHost | `internal/router.Router` |
| Cluster | Named entry in `clusters:` |
| LbEndpoint | `backend.Backend` |
| ROUND_ROBIN policy | `balancer.RoundRobin` |
| RING_HASH policy | `balancer.RingHash` |
| Active health check | `health.Checker` goroutine per endpoint |
| EDS (Endpoint Discovery Service) | `internal/discovery` — polls tinykube Service API |
| Stats sink (Prometheus) | `metrics.Registry` → `/metrics` |
| HTTP filter chain | `middleware.Chain` |
| xDS / dynamic config | `config.Watcher` (fsnotify on YAML file) |
| Transport socket (TLS) | `crypto/tls` on listener |

## Running it

```bash
cd tinyenvoy

# Start fake upstream endpoints (any HTTP server will do)
python3 -m http.server 8081 &
python3 -m http.server 8082 &
python3 -m http.server 8083 &

# Run tinyenvoy
go run ./cmd/envoy -config config.yaml
# INFO tinyenvoy listening addr=:8080
# INFO admin listening addr=:9090
```

## Testing load balancing

```bash
# Round-robin: each request goes to a different endpoint
for i in {1..6}; do curl -s http://localhost:8080/ -o /dev/null -w "%{http_code}\n"; done

# Ring-hash sticky routing: same X-Forwarded-For IP always hits same endpoint
curl -H "X-Forwarded-For: 1.2.3.4" http://localhost:8080/
curl -H "X-Forwarded-For: 1.2.3.4" http://localhost:8080/
```

## Prometheus metrics

```bash
curl http://localhost:9090/metrics | grep tinyenvoy
```

| Metric | Envoy equivalent | Labels |
|---|---|---|
| `tinyenvoy_requests_total` | `cluster.upstream_rq_total` | cluster, route, status |
| `tinyenvoy_request_duration_seconds` | `cluster.upstream_rq_time` | cluster, route |
| `tinyenvoy_endpoint_healthy` | `cluster.membership_healthy` | cluster, endpoint |
| `tinyenvoy_active_connections` | `cluster.upstream_cx_active` | cluster, endpoint |

## Hot-reload

Edit `config.yaml` while tinyenvoy is running. The fsnotify watcher detects the change and logs `config reloaded`. In a full implementation this would atomically swap the router and cluster pools (analogous to Envoy's xDS CDS/RDS updates).

## Config schema

```yaml
listener:
  addr: ":8080"
  tls:
    enabled: false
    cert: "cert.pem"
    key:  "key.pem"

admin:
  addr: ":9090"

clusters:
  - name: api
    lb_policy: round-robin   # or: ring-hash
    health_check:
      path: /healthz
      interval: 10s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    endpoints:                 # static endpoints
      - addr: localhost:8081
      - addr: localhost:8082

  - name: whoami
    lb_policy: round-robin
    health_check:
      path: /health
      interval: 5s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    discovery:                           # EDS-style dynamic discovery
      tinykube_addr: http://localhost:8080  # tinykube API server
      service: whoami                    # tinykube Service name
      namespace: default
      interval: 5s                       # poll interval for /endpoints

routes:
  - virtual_host: "api.example.com"
    routes:
      - prefix: /v1
        cluster: api
  - virtual_host: "*"
    routes:
      - prefix: /
        cluster: api
```

## Testing

```bash
# Unit tests (no network required)
go test ./...
```

## Directory structure

```
tinyenvoy/
├── cmd/envoy/main.go         — entry point: load config, wire, serve
├── config.yaml               — example config
├── go.mod
└── internal/
    ├── config/
    │   ├── config.go         — YAML structs + Load()
    │   ├── config_test.go
    │   ├── watcher.go        — fsnotify → callback on change (xDS analogue)
    │   └── watcher_test.go
    ├── backend/
    │   ├── backend.go        — Endpoint: addr, healthy (atomic), activeConns
    │   ├── backend_test.go
    │   ├── pool.go           — Cluster endpoint pool with SetHealthy()
    │   └── pool_test.go
    ├── balancer/
    │   ├── balancer.go       — LbPolicy interface
    │   ├── roundrobin.go     — ROUND_ROBIN: atomic counter mod len
    │   ├── roundrobin_test.go
    │   ├── ringhash.go       — RING_HASH: crc32 ring, 150 virtual nodes
    │   └── ringhash_test.go
    ├── health/
    │   ├── checker.go        — active health check goroutine per endpoint
    │   └── checker_test.go
    ├── router/
    │   ├── router.go         — Match(host, path) → cluster name
    │   └── router_test.go
    ├── metrics/
    │   ├── metrics.go        — Prometheus registry (4 metrics)
    │   └── metrics_test.go
    ├── middleware/
    │   ├── chain.go          — Chain() wires access-log + stats filters
    │   ├── logging.go        — access-log filter (slog)
    │   ├── logging_test.go
    │   ├── metrics.go        — stats filter (Prometheus per-cluster counters)
    │   └── metrics_test.go
    ├── proxy/
    │   ├── proxy.go          — cluster proxy: LB pick → httputil.ReverseProxy
    │   └── proxy_test.go
    └── discovery/
        ├── discovery.go      — EDS analogue: polls tinykube /endpoints, diffs pool+lb
        └── discovery_test.go
```

## Key design decisions

**Why `LbPolicy` interface?**
Decouples the proxy from specific load-balancing algorithms. `RoundRobin` and `RingHash` are interchangeable. The controller never calls a specific implementation — mirrors Envoy's `lb_policy` enum.

**Ring hash key selection**
Uses `X-Forwarded-For` header if present, falls back to `RemoteAddr`. This mirrors how Envoy uses `hash_policy` with the `connection_properties` source.

**Health checker state machine**
`health.Checker` tracks consecutive successes and failures, transitioning only after hitting the configured threshold — exactly matching Envoy's `healthy_threshold`/`unhealthy_threshold` semantics.

**Isolated Prometheus registry**
`metrics.NewRegistry()` creates a fresh `prometheus.Registry` rather than using the global one. This makes tests hermetic and avoids metric naming conflicts.

**fsnotify as xDS analogue**
Envoy uses xDS APIs (LDS/RDS/CDS/EDS) for dynamic config. tinyenvoy uses `fsnotify` on a YAML file as a file-based stand-in — same semantic (detect change, trigger reload), much simpler plumbing.

**EDS-style endpoint discovery**
When a cluster has a `discovery:` block, `internal/discovery` polls the tinykube Service `/endpoints` API at a configurable interval. Each poll diffs the response against the current pool: new addresses are added to both `pool` and `lb` (same `*Backend` pointer so health-checker state is shared); removed addresses are deleted from both. This mirrors Envoy's EDS — the same `ROUND_ROBIN`/`RING_HASH` load balancer works identically whether backends come from static config or dynamic discovery.
