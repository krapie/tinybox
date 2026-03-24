# tinydns

A simplified CoreDNS — a DNS server with service discovery and a plugin chain
(middleware pattern).

## Goals

- Understand DNS wire protocol (A records, query/response)
- Understand the plugin chain / middleware pattern
- Understand service discovery: name → IP resolution
- Understand TTL-based caching and upstream forwarding

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                        tinydns                           │
│                                                          │
│   DNS Query (UDP/TCP :5353)                             │
│         │                                               │
│         ▼                                               │
│   ┌─────────────────────────────────────────────────┐  │
│   │              Plugin Chain                       │  │
│   │                                                 │  │
│   │  ┌────────┐  ┌────────┐  ┌──────────────────┐  │  │
│   │  │  Log   │→ │ Cache  │→ │    Registry      │  │  │
│   │  │plugin  │  │plugin  │  │    plugin        │  │  │
│   │  └────────┘  └────────┘  └────────┬─────────┘  │  │
│   │                                   │ miss        │  │
│   │                          ┌────────▼─────────┐  │  │
│   │                          │  Forward plugin  │  │  │
│   │                          │  (upstream DNS)  │  │  │
│   │                          └──────────────────┘  │  │
│   └─────────────────────────────────────────────────┘  │
│                                                          │
│   ┌──────────────────────────────────────────────────┐  │
│   │  Service Registry  (name → IP, REST API)         │  │
│   └──────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

## Components

### 1. DNS Server (`server/`)

- Listen on UDP and TCP port 5353 (configurable)
- Parse DNS wire-format messages using `miekg/dns`
- Pass each query through the plugin chain
- Write the response back to the client

### 2. Service Registry (`registry/`)

```go
type ServiceRecord struct {
    Name      string // e.g. "my-service.default.svc.cluster.local"
    IP        string // e.g. "10.0.0.5"
    Port      int
    TTL       uint32
}
```

- In-memory map: `name → []ServiceRecord`
- REST API for registration/deregistration (used by tinykube to register pods/services)
- Supports multiple IPs per name (round-robin on response)

### 3. Plugin Chain (`plugins/`)

Plugins implement a common interface:

```go
type Plugin interface {
    Name() string
    ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error)
}
```

Each plugin calls `Next.ServeDNS(...)` to pass the query down the chain.

**Plugins to implement:**

#### `log` plugin
- Log every query: timestamp, client IP, query name, query type, response code, latency

#### `cache` plugin
- Cache DNS responses keyed by `(name, type)`
- Respect TTL from the record; evict expired entries
- On cache hit: serve immediately without calling downstream plugins

#### `registry` plugin
- Look up query name in the Service Registry
- On hit: build an A record response and return
- On miss: call next plugin

#### `forward` plugin
- Forward the query to a configured upstream resolver (default: `8.8.8.8:53`)
- Cache the upstream response (delegated to the cache plugin on the way back)
- Timeout: 2s per upstream query

#### `health` plugin
- Expose `GET /health` on an HTTP port (default: 8080)
- Returns 200 OK if the server is running

### 4. Config (`config/`)

Heraldfile-inspired config (simple text format):

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

Parsed into a `Config` struct at startup.

### 5. REST API for Registry (`apiserver/`)

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/registry/services` | Register a service |
| DELETE | `/registry/services/{name}` | Deregister a service |
| GET    | `/registry/services` | List all registered services |
| GET    | `/health` | Health check |

## Directory Structure

```
tinydns/
├── cmd/tinydns/
│   └── main.go
├── server/
│   ├── server.go
│   └── server_test.go
├── registry/
│   ├── registry.go
│   └── registry_test.go
├── plugins/
│   ├── plugin.go            # Plugin interface
│   ├── chain.go             # chain wiring
│   ├── log.go
│   ├── log_test.go
│   ├── cache.go
│   ├── cache_test.go
│   ├── registry_plugin.go
│   ├── registry_plugin_test.go
│   ├── forward.go
│   ├── forward_test.go
│   ├── health.go
│   └── health_test.go
├── config/
│   ├── config.go
│   └── config_test.go
├── apiserver/
│   ├── server.go
│   └── server_test.go
├── go.mod
└── SPEC.md
```

## Milestones

### M1 — DNS Server + Registry Plugin
- [x] UDP + TCP DNS server (configurable address)
- [x] Service registry (in-memory, round-robin, TTL expiry)
- [x] Registry plugin: resolve registered names to A records
- Tests written first: registry lookup, DNS response format

### M2 — Plugin Chain
- [x] Plugin interface (`plugins/plugin.go`)
- [x] Log plugin (query name, type, rcode, latency)
- [x] Cache plugin with TTL eviction
- [x] Registry plugin
- [x] Forward plugin with timeout
- [x] Health plugin (HTTP GET /health)
- Tests written first for each plugin in isolation

### M3 — Syncer (tinykube integration)
- [x] Poll tinykube /pods + /services APIs
- [x] Match running pods to service selectors
- [x] Rebuild registry on each sync cycle
- Tests written first with fake httptest server

### M4 — Config Parser + REST API + Main
- [x] Parse config file into Config struct
- [x] REST API for registry (POST/DELETE/GET /registry/services)
- [x] Health endpoint
- [x] cmd/tinydns/main.go wiring all components
- Tests written first for config parsing and API handlers

## Test Strategy

- **Unit tests**: each plugin tested with a fake `dns.ResponseWriter`
- **Integration tests**: full plugin chain tested with real UDP queries
- **Round-robin test**: multiple IPs registered for one name, verify rotation
- All test files created before their corresponding implementation files
