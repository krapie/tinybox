# tinybox sample

End-to-end demo showing **tinykube** + **tinydns** + **tinyenvoy** working together.

tinykube manages the pod lifecycle (real Docker containers) and exposes a Service endpoint API. tinydns runs alongside it, syncing Running pod IPs into DNS so services can be discovered by name. tinyenvoy sits in front as the L7 proxy, discovering backends from the tinykube Service API and routing traffic with round-robin load balancing.

## What this demonstrates

| Step | Component | What you see |
|---|---|---|
| Deploy | tinykube | Reconciliation loop creates 3 whoami pods |
| Service | tkctl | Service object registers selector; endpoint API returns live pod addresses |
| DNS | tinydns | Syncer polls tinykube; `whoami.default.svc.cluster.local.` resolves to pod IPs |
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
# [INFO] tinydns listening on :5353
# [INFO] tinydns API listening on :9053
# [INFO] tinydns health on 127.0.0.1:8181
# [INFO] tinydns syncer polling http://localhost:8080 (ns=default)
```

After the first sync cycle (up to 10s), query it:

```bash
dig @127.0.0.1 -p 5353 whoami.default.svc.cluster.local. A +short
# 172.19.0.2
# 172.19.0.3
# 172.19.0.4
```

These are Docker container IPs — suitable for pod-to-pod communication inside the Docker network. Unknown names return NXDOMAIN:

```bash
dig @127.0.0.1 -p 5353 unknown.default.svc.cluster.local. A +short
# (empty — NXDOMAIN)
```

### 5. Start tinyenvoy (auto-discovery mode)

```bash
cd tinyenvoy
go run ./cmd/envoy -config ../sample/envoy-config.yaml
# INFO tinyenvoy listening  addr=:8888
# INFO admin listening      addr=:9090
# INFO service discovery started  cluster=whoami service=whoami namespace=default interval=5s
```

tinyenvoy polls the tinykube endpoint API every 5 seconds and keeps its backend pool in sync — no manual configuration of host ports required.

### 6. Route through tinyenvoy

```bash
# Round-robin across all 3 pods
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
# Hostname: tinykube-whoami-a1b2c
# Hostname: tinykube-whoami-d3e4f
# Hostname: tinykube-whoami-g5h6i
# Hostname: tinykube-whoami-a1b2c
# ...
```

### 7. Check Prometheus metrics

```bash
curl http://localhost:9090/metrics | grep tinyenvoy
# tinyenvoy_requests_total{cluster="whoami",route="/",status="200"} 6
# tinyenvoy_request_duration_seconds_sum{...} 0.012
```

### 8. Rolling update (DNS + tinyenvoy auto-resync)

```bash
tkctl apply --name whoami --image traefik/whoami:v1.10 --replicas 3 --port 80
# deployment/whoami updated

# Watch pods flip to v1.10 (tinykube reconciler does the rolling update)
watch -n1 "tkctl get pods"

# tinydns automatically picks up new pod IPs on the next sync cycle
dig @127.0.0.1 -p 5353 whoami.default.svc.cluster.local. A +short
# 172.19.0.5   ← new container IPs after rolling update
# 172.19.0.6
# 172.19.0.7

# tinyenvoy also re-syncs on the next discovery poll
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
```

### 9. Clean up

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

=== 5. Start tinyenvoy (discovery mode) ===
  ✓ tinyenvoy proxy ready

=== 6. Round-robin routing ===
  ✓ round-robin hit 3 distinct backends

=== 7. Prometheus metrics ===
  ✓ tinyenvoy_requests_total present
  ✓ tinyenvoy_request_duration_seconds present
  ✓ request counter ≥9

=== 8. Rolling update ===
  ✓ rolling update triggered
  ✓ rolling update complete — 3 pods on v1.10
  ✓ endpoint API returns 3 endpoints after rolling update
  ✓ DNS still resolves 3 IPs after rolling update

=== 9. Delete deployment + service ===
  ✓ deployment deleted
  ✓ service deleted via tkctl
  ✓ all pods removed after delete
  ✓ all Docker containers removed
  ✓ endpoint API returns 0 or 404 after service deletion

════════════════════════════════
  Results: 27 passed, 0 failed
════════════════════════════════
```

## Architecture

```
  dig / pod-to-pod DNS
       │ :5353
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

**Two endpoint paths, two use cases:**
- **tinydns** → container IPs (`172.x.x.x`) — for pod-to-pod DNS inside Docker
- **tinyenvoy** → `localhost:{hostPort}` — for host-level traffic routing (macOS)

## Manifest files

| File | Kind | Purpose |
|---|---|---|
| `manifests/whoami.yaml` | Deployment | 3-replica whoami deployment with readiness probe |
| `manifests/whoami-svc.yaml` | Service | Selector `app=whoami`, port 80 |
| `envoy-config.yaml` | tinyenvoy config | Discovery mode — polls tinykube for `whoami` endpoints |
