# tinybox sample

End-to-end demo showing **tinykube** + **tinyenvoy** working together.

tinykube manages the pod lifecycle (real Docker containers). tinyenvoy sits in front as the L7 proxy, routing and load-balancing across the pods.

## What this demonstrates

| Step | Component | What you see |
|---|---|---|
| Deploy | tinykube | Reconciliation loop creates 3 whoami pods |
| Inspect | tkctl | `get pods` shows pods Running with IP addresses |
| Route | tinyenvoy | Round-robin load balancing across the 3 pods |
| Update | tkctl | Rolling update changes image, zero downtime |
| Metrics | tinyenvoy | Prometheus counters + latency histograms |

## Prerequisites

- Docker Desktop running
- Go 1.23+
- `tkctl` built: `go build -o tkctl ./tinykube/cmd/tkctl/`

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

### 3. Find the host ports

Tinykube pods are exposed on random host ports. Find them:

```bash
docker ps --filter "label=tinykube=true" --format "table {{.Names}}\t{{.Ports}}"
# tinykube-whoami-a1b2c   0.0.0.0:52410->80/tcp
# tinykube-whoami-d3e4f   0.0.0.0:52411->80/tcp
# tinykube-whoami-g5h6i   0.0.0.0:52412->80/tcp
```

Test a pod directly:

```bash
curl http://localhost:52410/
# Hostname: tinykube-whoami-a1b2c
# IP: 172.19.0.2
# ...
```

### 4. Start tinyenvoy (routing to the pods)

Update `sample/envoy-config.yaml` with the host ports from above, then:

```bash
cd tinyenvoy
go run ./cmd/envoy -config ../sample/envoy-config.yaml
# INFO tinyenvoy listening addr=:8080
# INFO admin listening addr=:9090
```

> **Note:** On macOS, tinykube uses host-mapped ports because Docker containers run inside a VM and their IPs (172.x.x.x) are not reachable from the host. Update `envoy-config.yaml` to use `localhost:{hostPort}` for each endpoint.

### 5. Route through tinyenvoy

```bash
# Round-robin across all 3 pods
for i in {1..6}; do curl -s http://localhost:8080/ | grep Hostname; done
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

### 7. Rolling update (zero downtime)

```bash
# Edit sample/manifests/whoami.yaml to bump the image version, then:
tkctl apply -f sample/manifests/whoami.yaml
# deployment/whoami updated

# Watch the rolling update — tinyenvoy keeps serving during the update
watch -n1 "tkctl get pods"
```

### 8. Scale down

```bash
tkctl delete deployment whoami
# deployment/whoami deleted
```

## Option B — docker compose

```bash
cd sample
docker compose up -d
docker compose logs -f
```

After the stack is up, deploy the whoami manifest:

```bash
# Apply manifest via the tinykube API container
curl -X POST http://localhost:8090/apis/apps/v1/namespaces/default/deployments \
  -H 'Content-Type: application/json' \
  -d "$(cat manifests/whoami.yaml | python3 -c '
import sys, yaml, json
m = yaml.safe_load(sys.stdin)
d = {"name": m["name"], "namespace": m.get("namespace","default"), "spec": m["spec"]}
print(json.dumps(d))
')"
```

## Architecture

```
                    ┌─────────────────────────────────┐
                    │          tinyenvoy              │
  :8080 ──────────▶ │  round-robin load balancer      │
                    │  Prometheus metrics → :9090     │
                    └──────────────┬──────────────────┘
                                   │
               ┌───────────────────┼───────────────────┐
               ▼                   ▼                   ▼
        ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
        │ whoami pod 1 │  │ whoami pod 2 │  │ whoami pod 3 │
        │ (Docker ctr) │  │ (Docker ctr) │  │ (Docker ctr) │
        └──────────────┘  └──────────────┘  └──────────────┘
               ▲                   ▲                   ▲
               └───────────────────┼───────────────────┘
                                   │
                    ┌──────────────┴──────────────────┐
                    │          tinykube               │
                    │  reconciliation loop            │
                    │  readiness probes               │
                    │  rolling update                 │
                    └─────────────────────────────────┘
```
