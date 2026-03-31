# tinybox sample

End-to-end demo showing **tinykube** + **tinydns** + **tinyotel** + **tinyenvoy** working together.

tinykube manages the pod lifecycle (real Docker containers) and exposes a Service endpoint API. tinydns syncs Running pod IPs into DNS. tinyotel collects OTLP traces, metrics, and logs from instrumented services. tinyenvoy sits in front as the L7 proxy, discovering backends from tinykube and routing traffic with round-robin load balancing.

## What this demonstrates

| Step | Component | What you see |
|---|---|---|
| Deploy | tinykube | Reconciliation loop creates 3 whoami pods |
| Service | tkctl | Service object registers selector; endpoint API returns live pod addresses |
| DNS | tinydns | Syncer polls tinykube; `whoami.default.svc.cluster.local.` resolves to pod IPs |
| Observability | tinyotel | OTLP receiver ingests traces/metrics/logs; query API returns stored data |
| Route | tinyenvoy | Discovers backends via Service API; round-robin load balancing |
| Update | tkctl | Rolling update changes image; DNS + tinyenvoy re-sync backends automatically |
| Metrics | tinyenvoy | Prometheus counters + latency histograms |

## Prerequisites

- Docker Desktop running
- Go 1.23+
- `dig` (macOS default, or install `bind-utils` on Linux)

## Option A — Step by step (recommended for learning)

### 1. Start tinykube

```bash
cd tinykube
go run ./cmd/tinykube/
# INFO  Starting tinykube...
# INFO  API server listening on :8080
```

### 2. Deploy 3 whoami replicas

In a new terminal:

```bash
tkctl apply -f sample/manifests/whoami.yaml
# deployment/whoami created

tkctl get pods
# NAME                STATUS    IP           IMAGE
# whoami-a1b2c        Running   172.19.0.2   traefik/whoami
# whoami-d3e4f        Running   172.19.0.3   traefik/whoami
# whoami-g5h6i        Running   172.19.0.4   traefik/whoami
```

### 3. Create a Service

```bash
tkctl apply -f sample/manifests/whoami-svc.yaml
# service/whoami created

tkctl get services
# NAME     NAMESPACE   PORT   SELECTOR
# whoami   default     80     app=whoami
```

Check the endpoint discovery API directly:

```bash
curl http://localhost:8080/apis/v1/namespaces/default/services/whoami/endpoints
# [{"podName":"whoami-a1b2c","addr":"localhost:52410"},
#  {"podName":"whoami-d3e4f","addr":"localhost:52411"},
#  {"podName":"whoami-g5h6i","addr":"localhost:52412"}]
```

The API returns `localhost:{hostPort}` — host-mapped ports reachable from the macOS host. Pod-to-pod communication inside Docker uses container IPs instead (see step 4).

### 4. Start tinydns

tinydns syncs Running pod IPs from tinykube and serves them as DNS A records:

```bash
cd tinydns
go run ./cmd/tinydns/ -tinykube http://localhost:8080 -namespace default
# [INFO] tinydns listening on :10053
# [INFO] tinydns API listening on :9053
# [INFO] tinydns health on 127.0.0.1:8181
# [INFO] tinydns syncer polling http://localhost:8080 (ns=default)
```

After the first sync cycle (up to 10s), query it:

```bash
dig @127.0.0.1 -p 10053 whoami.default.svc.cluster.local. A +short
# 172.19.0.2
# 172.19.0.3
# 172.19.0.4
```

These are Docker container IPs — suitable for pod-to-pod communication inside the Docker network. Unknown names return NXDOMAIN:

```bash
dig @127.0.0.1 -p 10053 unknown.default.svc.cluster.local. A +short
# (empty — NXDOMAIN)
```

### 5. Start tinyotel (OTLP receiver + query API)

tinyotel collects traces, metrics, and logs from any OTLP-instrumented service:

```bash
cd tinyotel
go run ./cmd/tinyotel/
# tinyotel OTLP receiver  listening on :4318
# tinyotel query API + UI  listening on :4319
```

Send a test trace span:

```bash
curl -s -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"mysvc"}}]},"scopeSpans":[{"spans":[{"traceId":"aabbccddeeff00112233445566778899","spanId":"aabbccdd11223344","name":"my-op","kind":2,"startTimeUnixNano":1700000000000000000,"endTimeUnixNano":1700000001000000000,"attributes":[],"status":{"code":1}}]}]}]}'
# {"partialSuccess":{}}
```

Query the trace back:

```bash
curl -s http://localhost:4319/api/v1/traces | jq .
# [{"traceID":"aabbccddeeff00112233445566778899","rootSpan":"my-op","spanCount":1,"durationMs":1000,...}]

# Open the web UI (trace waterfall, metrics chart, log viewer)
open http://localhost:4319/ui/
```

### 6. Start tinyenvoy (auto-discovery mode)

```bash
cd tinyenvoy
go run ./cmd/envoy -config ../sample/envoy-config.yaml
# INFO tinyenvoy listening  addr=:8888
# INFO admin listening      addr=:9090
# INFO service discovery started  cluster=whoami service=whoami namespace=default interval=5s
```

tinyenvoy polls the tinykube endpoint API every 5 seconds and keeps its backend pool in sync — no manual configuration of host ports required.

### 7. Route through tinyenvoy

```bash
# Round-robin across all 3 pods
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
# Hostname: tinykube-whoami-a1b2c
# Hostname: tinykube-whoami-d3e4f
# Hostname: tinykube-whoami-g5h6i
# Hostname: tinykube-whoami-a1b2c
# ...
```

### 8. Check Prometheus metrics

```bash
curl http://localhost:9090/metrics | grep tinyenvoy
# tinyenvoy_requests_total{cluster="whoami",route="/",status="200"} 6
# tinyenvoy_request_duration_seconds_sum{...} 0.012
```

### 9. Rolling update (DNS + tinyenvoy auto-resync)

```bash
tkctl apply --name whoami --image traefik/whoami:v1.10 --replicas 3 --port 80
# deployment/whoami updated

# Watch pods flip to v1.10 (tinykube reconciler does the rolling update)
watch -n1 "tkctl get pods"

# tinydns automatically picks up new pod IPs on the next sync cycle
dig @127.0.0.1 -p 10053 whoami.default.svc.cluster.local. A +short
# 172.19.0.5   ← new container IPs after rolling update
# 172.19.0.6
# 172.19.0.7

# tinyenvoy also re-syncs on the next discovery poll
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
```

### 10. Clean up

```bash
tkctl delete deployment whoami
tkctl delete service whoami
```

## Option B — automated e2e test

The e2e test script exercises all the above steps end-to-end:

```bash
cd sample
bash e2e_test.sh
```

Output:
```
=== 0. Build ===
  ✓ tkctl built
  ✓ tinydns built
  ✓ tinyenvoy built
  ✓ tinyotel built

=== 2. Deploy whoami (3 replicas) ===
  ✓ deployment created
  ✓ 3 pods Running

=== 3. Service resource + endpoint discovery API ===
  ✓ service created via tkctl
  ✓ tkctl get services shows whoami
  ✓ endpoint API returns 3 endpoints (want ≥3)
  ✓ endpoint addrs are localhost:{port} (host-mapped, macOS)

=== 4. Start tinydns (tinykube syncer) ===
  ✓ DNS resolves whoami.default.svc.cluster.local. to 3 IPs (want ≥3)
  ✓ DNS returns container IPs (172.x.x.x) for pod-to-pod communication
  ✓ tinydns health endpoint returns 200
  ✓ unknown name returns NXDOMAIN

=== 5. Start tinyotel (OTLP receiver + query API) ===
  ✓ tinyotel health endpoint returns 200
  ✓ OTLP traces endpoint accepts spans (200)
  ✓ OTLP metrics endpoint accepts data points (200)
  ✓ OTLP logs endpoint accepts records (200)
  ✓ tinyotel trace API returns 1 trace(s)
  ✓ tinyotel services API lists e2e-svc
  ✓ tinyotel metric-names API lists e2e.counter
  ✓ tinyotel log API returns 1 record(s)

=== 6. Start tinyenvoy (discovery mode) ===
  ✓ tinyenvoy proxy ready

=== 7. Round-robin routing ===
  ✓ round-robin hit 3 distinct backends

=== 8. Prometheus metrics ===
  ✓ tinyenvoy_requests_total present
  ✓ tinyenvoy_request_duration_seconds present
  ✓ request counter ≥9

=== 9. Rolling update ===
  ✓ rolling update triggered
  ✓ rolling update complete — 3 pods on v1.10
  ✓ endpoint API returns 3 endpoints after rolling update
  ✓ DNS still resolves 3 IPs after rolling update

=== 10. Delete deployment + service ===
  ✓ deployment deleted
  ✓ service deleted via tkctl
  ✓ all pods removed after delete
  ✓ all Docker containers removed
  ✓ endpoint API returns 0 or 404 after service deletion

════════════════════════════════
  Results: 36 passed, 0 failed
════════════════════════════════
```

## Architecture

```
  OTLP clients (curl / instrumented services)
       │ POST /v1/traces   /v1/metrics   /v1/logs
       ▼ :4318
┌──────────────────────────────────────┐
│             tinyotel                 │
│  OTLP/HTTP receiver                  │
│  processor: batch→attributes→sample  │
│  stores: traces / metrics / logs     │
│  query API + web UI → :4319          │
└──────────────────────────────────────┘

  dig / pod-to-pod DNS
       │ :10053
       ▼
┌──────────────────────────────────────┐
│             tinydns                  │
│  plugin chain: log→cache→registry→  │
│  forward                             │
│  health → :8181                      │
│                                      │
│  ┌────────────────────────────────┐  │
│  │ syncer                         │  │
│  │ polls tinykube /pods+/services │  │
│  │ registers pod IPs into DNS     │  │
│  └──────────────┬─────────────────┘  │
└─────────────────│────────────────────┘
                  │
  curl / browser  │ /apis/v1/namespaces/default/pods
       │          │ /apis/v1/namespaces/default/services
       ▼ :8888    │
┌──────────────────────────────────────┐
│             tinyenvoy                │
│  round-robin load balancer           │
│  Prometheus metrics → :9090          │
│                                      │
│  ┌────────────────────────────────┐  │
│  │ discovery (EDS analogue)       │  │
│  │ polls tinykube /endpoints      │  │
│  └──────────────┬─────────────────┘  │
└─────────────────│────────────────────┘
                  │ /apis/v1/.../endpoints
                  ▼ :8080
┌──────────────────────────────────────┐
│             tinykube                 │
│  reconciliation loop                 │
│  readiness probes                    │
│  rolling update                      │
│  Service + endpoint API              │
└──────────────────────────────────────┘
       │         │         │
       ▼         ▼         ▼
┌──────────┐ ┌──────────┐ ┌──────────┐
│whoami-1  │ │whoami-2  │ │whoami-3  │
│(Docker)  │ │(Docker)  │ │(Docker)  │
└──────────┘ └──────────┘ └──────────┘
```

**Endpoint paths:**
- **tinydns** → container IPs (`172.x.x.x`) — for pod-to-pod DNS inside Docker
- **tinyenvoy** → `localhost:{hostPort}` — for host-level traffic routing (macOS)
- **tinyotel** → collects OTLP telemetry; query API at `:4319/ui/`

## Manifest files

| File | Kind | Purpose |
|---|---|---|
| `manifests/whoami.yaml` | Deployment | 3-replica whoami deployment with readiness probe |
| `manifests/whoami-svc.yaml` | Service | Selector `app=whoami`, port 80 |
| `envoy-config.yaml` | tinyenvoy config | Discovery mode — polls tinykube for `whoami` endpoints |
