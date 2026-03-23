# tinyenvoy — L7 Proxy

Study-grade L7 proxy in Go, mirroring core Envoy concepts.
Goal is learning through clean, well-structured code — not production hardening.

## Envoy Concepts Modelled

| Envoy concept | tinyenvoy equivalent |
|---|---|
| Listener | `server.addr` — accepts inbound connections |
| Route Configuration | `routes` — virtual host + path prefix matching |
| Cluster | `upstreams` — named group of endpoints |
| Endpoint | `backends[].addr` — individual upstream address |
| Load Balancing Policy | `balancer: round-robin \| consistent-hash` |
| Health Discovery Service (HDS) | `health_check` — active HTTP health checks |
| Access Log | `middleware/logging.go` — structured slog output |
| Stats / Admin | `metrics_addr` — Prometheus `/metrics` endpoint |
| Transport Socket (TLS) | `server.tls` — TLS termination via `crypto/tls` |

## Goals

- Understand how Envoy maps listeners → routes → clusters → endpoints
- Implement the two canonical LB policies Envoy ships with (RR + ring hash)
- Model Envoy's active health check state machine (healthy/unhealthy thresholds)
- Practice hot-reload semantics similar to Envoy's xDS dynamic config

## Features

- Listener: single HTTP/HTTPS listener (Envoy ListenerManager)
- Route Configuration: virtual-host matching (exact + wildcard) with path-prefix rules
- Cluster Manager: named upstream clusters, each with its own LB policy
- Endpoint Discovery: static endpoints in config **or** dynamic via tinykube Service API (EDS analogue)
- Load Balancing: Round Robin + Ring Hash (Envoy's `RING_HASH` policy)
- Health Checks: active HTTP health checks mirroring Envoy's health_check filter
- Access Logging: structured per-request log (method, path, cluster, status, latency)
- Stats: Prometheus counters + histograms, analogous to Envoy's stats sink
- Dynamic Config: fsnotify-based hot reload (analogous to xDS/file-based config)

## Directory Layout

```
tinyenvoy/
├── cmd/envoy/main.go         # Entrypoint: load config, wire, serve
├── config.yaml               # Example config
├── go.mod
├── internal/
│   ├── config/
│   │   ├── config.go         # YAML structs + Load()
│   │   ├── config_test.go
│   │   ├── watcher.go        # fsnotify → callback on change (xDS analogue)
│   │   └── watcher_test.go
│   ├── backend/
│   │   ├── backend.go        # Endpoint struct: addr, healthy bool, activeConns int64
│   │   ├── backend_test.go
│   │   ├── pool.go           # Cluster endpoint pool: []Backend + RWMutex + Healthy()
│   │   └── pool_test.go
│   ├── balancer/
│   │   ├── balancer.go       # LbPolicy interface (mirrors Envoy LbPolicy enum)
│   │   ├── roundrobin.go     # ROUND_ROBIN: atomic counter mod len
│   │   ├── roundrobin_test.go
│   │   ├── ringhash.go       # RING_HASH: crc32 ring, 150 virtual nodes per endpoint
│   │   └── ringhash_test.go
│   ├── health/
│   │   ├── checker.go        # Active health check — goroutine per endpoint, context-controlled
│   │   └── checker_test.go
│   ├── router/
│   │   ├── router.go         # Route config: Match(host, path) → cluster name
│   │   └── router_test.go
│   ├── metrics/
│   │   ├── metrics.go        # Prometheus registry — analogous to Envoy stats sink
│   │   └── metrics_test.go
│   ├── middleware/
│   │   ├── chain.go          # Filter chain: access-log → stats → route → cluster
│   │   ├── logging.go        # Access log filter (slog)
│   │   ├── logging_test.go
│   │   ├── metrics.go        # Stats filter (Prometheus per-cluster counters)
│   │   └── metrics_test.go
│   ├── proxy/
│       ├── proxy.go          # Cluster proxy: httputil.ReverseProxy per cluster
│       └── proxy_test.go
│   └── discovery/
│       ├── discovery.go      # EDS analogue: polls tinykube /endpoints, diffs pool
│       └── discovery_test.go
```

## Config Schema

The config mirrors Envoy's static bootstrap structure: listener → route_config → clusters.

```yaml
# Listener (Envoy: static_resources.listeners)
listener:
  addr: ":8080"
  tls:
    enabled: false
    cert: "cert.pem"
    key:  "key.pem"

# Admin / stats (Envoy: admin + stats_sinks)
admin:
  addr: ":9090"   # serves /metrics (Prometheus)

# Clusters (Envoy: static_resources.clusters)
clusters:
  - name: api
    lb_policy: round-robin   # or: ring-hash
    health_check:
      path: /healthz          # Envoy: health_checks[].http_health_check.path
      interval: 10s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    endpoints:
      - addr: localhost:8081
      - addr: localhost:8082
      - addr: localhost:8083

  # Optional: EDS-style dynamic discovery via tinykube Service API.
  # When set, the static endpoints list is ignored.
  - name: whoami
    lb_policy: round-robin
    health_check:
      path: /health
      interval: 10s
      timeout: 2s
      unhealthy_threshold: 3
      healthy_threshold: 2
    discovery:
      tinykube_addr: http://localhost:8080   # tinykube API server
      service: whoami                        # tinykube Service name
      namespace: default
      interval: 5s                           # how often to poll /endpoints

# Route configuration (Envoy: route_config.virtual_hosts)
routes:
  - virtual_host: "api.example.com"
    routes:
      - prefix: /v1
        cluster: api
      - prefix: /
        cluster: api
  - virtual_host: "*"          # catch-all virtual host
    routes:
      - prefix: /
        cluster: api
```

## Core Interfaces

### LbPolicy (mirrors Envoy `cluster.lb_policy`)
```go
type LbPolicy interface {
    Pick(key string) *backend.Backend  // key = client IP for ring-hash, ignored for RR
    Add(b *backend.Backend)
    Remove(addr string)
}
```

### Router
```go
type Router interface {
    // Match finds the cluster name for a given virtual host + path.
    // Envoy analogue: RouteConfiguration.virtual_hosts[].routes[].match
    Match(host, path string) (cluster string, ok bool)
}
```

## Algorithms

### Round Robin (`ROUND_ROBIN`)
Mirrors Envoy's default `ROUND_ROBIN` policy. Atomic counter mod len(healthy endpoints). No lock required.

### Ring Hash (`RING_HASH`)
Mirrors Envoy's `RING_HASH` policy:
- Hash ring via `crc32.ChecksumIEEE`
- 150 virtual nodes per endpoint (Envoy default minimum_ring_size = 1024; simplified here)
- Key: client IP from `X-Forwarded-For` or `RemoteAddr`
- Ring stored as sorted `[]uint32` + map to endpoint
- Binary search for successor node (consistent sticky sessions)

## Health Check State Machine

Mirrors Envoy's active health check model:

```
HEALTHY ──[unhealthy_threshold failures]──► UNHEALTHY
UNHEALTHY ──[healthy_threshold successes]──► HEALTHY
```

- One goroutine per endpoint, controlled by `context.Context`
- HTTP GET to `endpoint.addr + health_check.path`
- Consecutive failure/success counters drive state transitions
- Unhealthy endpoints excluded from LB pool (Envoy: `health_status: UNHEALTHY`)

## Dynamic Config (xDS analogue)

Envoy uses xDS APIs (LDS/RDS/CDS/EDS) for dynamic config. tinyenvoy uses fsnotify on a static YAML file as a simplified stand-in:

- `config.Watcher` watches the config file for WRITE events
- On change: re-parse YAML → diff clusters and routes
- Atomically swap router and cluster pools via `sync/atomic`
- Cancel old health-check contexts, start new ones for added/changed clusters

## Filter Chain

Analogous to Envoy's HTTP filter chain (`http_filters`):

```
Downstream → [access-log filter] → [stats filter] → [router filter] → Cluster proxy → Upstream
```

Implemented as Go `http.Handler` wrappers in `internal/middleware`.

## Transport Socket (TLS)

- `tls.LoadX509KeyPair` from config paths
- `http.Server.TLSConfig` with TLS 1.2 minimum (Envoy default)
- Plaintext if `tls.enabled: false`

## Cluster Proxy

- One `httputil.ReverseProxy` per cluster
- `Director` func: calls LbPolicy.Pick() to select endpoint, rewrites request URL
- `ModifyResponse`: captures upstream status code for access log + stats
- `ErrorHandler`: returns 502 on upstream connection failure (Envoy: `UF,URX` response flags)

## Metrics (Envoy stats analogue)

Prometheus metrics, mirroring Envoy's cluster stats:

| Metric | Envoy equivalent | Type | Labels |
|---|---|---|---|
| `tinyenvoy_requests_total` | `cluster.upstream_rq_total` | Counter | `cluster`, `route`, `status` |
| `tinyenvoy_request_duration_seconds` | `cluster.upstream_rq_time` | Histogram | `cluster`, `route` |
| `tinyenvoy_endpoint_healthy` | `cluster.membership_healthy` | Gauge | `cluster`, `endpoint` |
| `tinyenvoy_active_connections` | `cluster.upstream_cx_active` | Gauge | `cluster`, `endpoint` |

## Implementation Order (TDD)

1. [x] `internal/config` — parse YAML, unit test struct fields
2. [x] `internal/backend` — Endpoint + Pool, test Healthy() filtering; `Add`/`Remove` for EDS
3. [x] `internal/balancer` — RR + ring-hash, table-driven tests
4. [x] `internal/health` — active health check with httptest.Server mock
5. [x] `internal/router` — virtual-host + path matching, table tests
6. [x] `internal/metrics` — Prometheus registry, test metric registration
7. [x] `internal/middleware` — access-log + stats filters, test via httptest
8. [x] `internal/proxy` — cluster proxy wiring, integration test with mock endpoints
9. [x] `internal/config/watcher` — hot reload, test config swap
10. [x] `cmd/envoy/main.go` — wire all packages, SIGINT/SIGTERM handling
11. [x] `internal/discovery` — EDS analogue: poll tinykube `/endpoints` + diff pool (S3)

## Verification

```bash
# Start 3 fake upstream endpoints
python3 -m http.server 8081 &
python3 -m http.server 8082 &
python3 -m http.server 8083 &

# Run tinyenvoy
go run ./cmd/envoy -config config.yaml

# Verify round-robin across 3 endpoints
for i in {1..6}; do curl -s http://localhost:8080/ -o /dev/null -w "%{http_code}\n"; done

# Verify ring-hash sticky routing (same IP → same endpoint)
curl -H "X-Forwarded-For: 1.2.3.4" http://localhost:8080/
curl -H "X-Forwarded-For: 1.2.3.4" http://localhost:8080/

# Verify admin / stats endpoint (Envoy admin analogue)
curl http://localhost:9090/metrics | grep tinyenvoy

# Verify dynamic config reload
echo "# bump" >> config.yaml && sleep 2   # watcher triggers reload

# Verify health check failover: kill one endpoint, wait for threshold
kill %1 && sleep 15 && curl http://localhost:8080/
```

## Dependencies

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v3` | YAML config parsing |
| `github.com/fsnotify/fsnotify` | File watch (xDS analogue) |
| `github.com/prometheus/client_golang` | Stats sink (Prometheus) |
| `crypto/tls` (stdlib) | Transport socket / TLS |
| `log/slog` (stdlib) | Access log |
| `net/http/httputil` (stdlib) | Cluster proxy (ReverseProxy) |
