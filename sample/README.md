# tinybox sample

End-to-end demo showing **tinykube** + **tinyenvoy** working together.

tinykube manages the pod lifecycle (real Docker containers) and exposes a Service endpoint API. tinyenvoy sits in front as the L7 proxy — it discovers live backends automatically from the tinykube Service API (EDS analogue) and routes traffic across them with round-robin load balancing.

## What this demonstrates

| Step | Component | What you see |
|---|---|---|
| Deploy | tinykube | Reconciliation loop creates 3 whoami pods |
| Service | tkctl | Service object registers selector; endpoint API returns live pod addresses |
| Route | tinyenvoy | Discovers backends via Service API; round-robin load balancing |
| Update | tkctl | Rolling update changes image; tinyenvoy re-syncs backends automatically |
| Metrics | tinyenvoy | Prometheus counters + latency histograms |

## Prerequisites

- Docker Desktop running
- Go 1.23+
- `tkctl` built: `cd tinykube && go build -o tkctl ./cmd/tkctl/`

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

Create the Service so tinyenvoy can discover pod endpoints by label selector:

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

The API returns only Running pods matching the selector — `localhost:{hostPort}` addresses that are reachable from the host (needed on macOS where container IPs are inside the Docker VM).

### 4. Start tinyenvoy (auto-discovery mode)

```bash
cd tinyenvoy
go run ./cmd/envoy -config ../sample/envoy-config.yaml
# INFO tinyenvoy listening  addr=:8888
# INFO admin listening      addr=:9090
# INFO service discovery started  cluster=whoami service=whoami namespace=default interval=5s
```

tinyenvoy polls the tinykube endpoint API every 5 seconds and keeps its backend pool in sync — no manual configuration of host ports required.

### 5. Route through tinyenvoy

```bash
# Round-robin across all 3 pods
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
# Hostname: tinykube-whoami-a1b2c
# Hostname: tinykube-whoami-d3e4f
# Hostname: tinykube-whoami-g5h6i
# Hostname: tinykube-whoami-a1b2c
# ...
```

### 6. Check Prometheus metrics

```bash
curl http://localhost:9090/metrics | grep tinyenvoy
# tinyenvoy_requests_total{cluster="whoami",route="/",status="200"} 6
# tinyenvoy_request_duration_seconds_sum{...} 0.012
```

### 7. Rolling update (auto-resync)

```bash
tkctl apply --name whoami --image traefik/whoami:v1.10 --replicas 3 --port 80
# deployment/whoami updated

# Watch pods flip to v1.10 (tinykube reconciler does the rolling update)
watch -n1 "tkctl get pods"

# tinyenvoy automatically picks up the new pod addresses on the next discovery poll
# No config change required — just keep sending traffic
for i in {1..6}; do curl -s http://localhost:8888/ | grep Hostname; done
```

### 8. Clean up

```bash
tkctl delete deployment whoami
# deployment/whoami deleted

tkctl delete service whoami
# service/whoami deleted
```

## Option B — automated e2e test

The e2e test script exercises all the above steps end-to-end and verifies 22 assertions:

```bash
cd sample
bash e2e_test.sh
```

Output:
```
=== 0. Build ===
  ✓ tkctl built
  ✓ tinyenvoy built

=== 2. Deploy whoami (3 replicas) ===
  ✓ 3 pods Running

=== 3. Service resource + endpoint discovery API (S1/S2) ===
  ✓ service created via tkctl
  ✓ tkctl get services shows whoami
  ✓ endpoint API returns 3 endpoints (want ≥3)
  ✓ endpoint addrs are localhost:{port}

=== 4. Start tinyenvoy (discovery mode, S3) ===
  ✓ tinyenvoy proxy ready

=== 5. Round-robin routing ===
  ✓ round-robin hit 3 distinct backends

...

  Results: 22 passed, 0 failed
```

## Architecture

```
  curl / browser
       │
       ▼ :8888
┌──────────────────────────────────────┐
│             tinyenvoy                │
│  round-robin load balancer           │
│  Prometheus metrics → :9090          │
│                                      │
│  ┌────────────────────────────────┐  │
│  │ discovery (EDS analogue)       │  │
│  │ polls tinykube /endpoints      │  │
│  │ every 5s, diffs backend pool   │  │
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

tinyenvoy never talks directly to the Docker daemon — it discovers backend addresses through the tinykube Service API, the same way Envoy EDS works in production (but via HTTP polling instead of gRPC streaming).

## Manifest files

| File | Kind | Purpose |
|---|---|---|
| `manifests/whoami.yaml` | Deployment | 3-replica whoami deployment with readiness probe |
| `manifests/whoami-svc.yaml` | Service | Selector `app=whoami`, port 80 |
| `envoy-config.yaml` | tinyenvoy config | Discovery mode — polls tinykube for `whoami` endpoints |
