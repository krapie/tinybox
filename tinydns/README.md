# tinydns

A toy CoreDNS — a DNS server with service discovery and a middleware plugin chain.

Built as part of [tinybox](../README.md), a collection of simplified infrastructure and tooling implementations for study.

## What it teaches

| Concept | Where |
|---|---|
| DNS wire protocol (A records, query/response) | `server/server.go` |
| Plugin chain / middleware pattern | `plugins/plugin.go`, `plugins/chain.go` |
| In-memory service registry with round-robin and TTL | `registry/registry.go` |
| TTL-based DNS cache | `plugins/cache.go` |
| Upstream DNS forwarding | `plugins/forward.go` |
| Pull-model service discovery (tinykube integration) | `syncer/syncer.go` |
| Config file parsing | `config/config.go` |
| REST API for registry management | `apiserver/server.go` |

## Architecture

```
  DNS client (UDP/TCP :5353)
         │
         ▼
┌──────────────────────────────────────┐
│             Plugin Chain             │
│                                      │
│  Log → Cache → Registry → Forward   │
└──────────────────────────────────────┘
         │                │
         │ miss           │ upstream
         ▼                ▼
   [Service Registry]  [8.8.8.8:53]
         ▲
         │ sync (poll)
┌────────┴────────────┐
│  Syncer             │
│  polls tinykube     │
│  /pods + /services  │
└─────────────────────┘
         │
   tinykube API (:8080)
```

### Plugin chain

`Log → Cache → RegistryPlugin → Forward`

Each plugin implements:

```go
type Plugin interface {
    Name() string
    ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error)
}
```

| Plugin | Behaviour |
|---|---|
| **log** | Logs query name, type, rcode, latency |
| **cache** | Caches successful responses; evicts on TTL expiry |
| **registry** | Resolves from in-memory registry; calls next on miss |
| **forward** | Forwards to upstream resolver (default `8.8.8.8:53`); returns SERVFAIL on timeout |
| **health** | HTTP `GET /health` → 200 OK |

## Running it

```bash
cd tinydns

# Build
go build -o tinydns ./cmd/tinydns/

# Run with defaults (:5353 DNS, :9053 REST API, :8080 health)
./tinydns

# Run with tinykube sync (registers pod IPs into DNS automatically)
./tinydns -tinykube http://localhost:8080 -namespace default

# Run with a config file
./tinydns -config tinydns.conf
```

## Config file

```
# tinydns config
listen :5353
upstream 8.8.8.8:53

plugins {
  log
  cache ttl=30
  registry
  forward
  health :8080
}
```

## REST API

Manage the service registry directly (used by tinykube integration or manual registration):

| Method | Path | Description |
|--------|------|-------------|
| POST | `/registry/services` | Register a service record |
| DELETE | `/registry/services/{name}` | Deregister all records for a name |
| GET | `/registry/services` | List all non-expired records |
| GET | `/health` | Health check |

### Register a service

```bash
curl -X POST http://localhost:9053/registry/services \
  -H 'Content-Type: application/json' \
  -d '{"name":"whoami.default.svc.cluster.local.","ip":"172.19.0.2","ttl":30}'
```

### Query it

```bash
dig @127.0.0.1 -p 5353 whoami.default.svc.cluster.local. A
# whoami.default.svc.cluster.local. 30 IN A 172.19.0.2
```

### List registered services

```bash
curl http://localhost:9053/registry/services
```

## Syncer (tinykube integration)

When started with `-tinykube`, tinydns polls tinykube's pod and service APIs every 10 seconds and automatically builds DNS A records:

- Only `Running` pods are registered
- Pod's `podIP` (container IP, e.g. `172.x.x.x`) is used — suitable for pod-to-pod DNS within Docker
- Records are named `{service}.{namespace}.svc.cluster.local.`
- Each sync cycle deregisters stale records and re-registers live ones

> **macOS note:** tinydns uses `pod.PodIP` (Docker container IPs), which are reachable from other containers on the same Docker network but **not** from the macOS host. This is intentional — pod-to-pod DNS should use container IPs, while host-level consumers (e.g. tinyenvoy) use `localhost:{hostPort}` from tinykube's endpoint API.

## Testing

```bash
go test ./...
```

All tests use in-memory fakes — no external services required.

## Directory structure

```
tinydns/
├── cmd/tinydns/
│   └── main.go             — entry point, wires all components
├── server/
│   ├── server.go           — UDP+TCP DNS server (miekg/dns)
│   └── server_test.go
├── registry/
│   ├── registry.go         — in-memory store, round-robin, TTL expiry
│   └── registry_test.go
├── plugins/
│   ├── plugin.go           — Plugin interface
│   ├── log.go              — query logging
│   ├── cache.go            — TTL-based response cache
│   ├── registry_plugin.go  — registry lookup plugin
│   ├── forward.go          — upstream DNS forwarding
│   ├── health.go           — HTTP health endpoint
│   └── *_test.go
├── config/
│   ├── config.go           — config file parser
│   └── config_test.go
├── apiserver/
│   ├── server.go           — REST API for registry management
│   └── server_test.go
├── syncer/
│   ├── syncer.go           — tinykube poll loop
│   └── syncer_test.go
├── go.mod
└── SPEC.md
```

## Key design decisions

**Plugin interface instead of hard-coded handler**
Each processing step (log, cache, lookup, forward) is a `Plugin` that calls the next one — identical to CoreDNS's middleware pattern. New plugins can be inserted without touching existing ones.

**Pull model for service discovery**
tinydns polls tinykube rather than tinykube pushing events to tinydns. This keeps tinykube unaware of tinydns, avoids coupling, and makes the syncer independently testable with a fake HTTP server.

**pod.PodIP for DNS (not hostPort)**
DNS A records use Docker container IPs (`172.x.x.x`) so pod-to-pod communication inside Docker works. The tinykube endpoint API returns `localhost:{hostPort}` for host-level consumers — tinydns deliberately uses a different path.

**Lazy TTL eviction**
The registry evicts expired records lazily on the next `Lookup` rather than running a background reaper goroutine. Simple and correct for the query rates expected in a toy system.
