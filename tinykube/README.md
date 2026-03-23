# tinykube

A toy Kubernetes implementation — Deployment controller with pod replica reconciliation and rolling updates. Pods are real Docker containers.

Built as part of [tinybox](../README.md), a collection of simplified infrastructure and tooling implementations for study.

## What it teaches

| Concept | Where |
|---|---|
| Reconciliation loop (desired vs actual state) | `controller/deployment_controller.go` |
| Rolling update with maxSurge / maxUnavailable | `controller/rolling_update.go` |
| CRI abstraction (runtime interface) | `runtime/runtime.go` |
| In-memory store with watch (etcd substitute) | `store/store.go` |
| REST API server for resources | `apiserver/server.go` |
| Readiness probes and pod lifecycle | `runtime/watcher.go` |
| Structured two-level logging across components | `logger/logger.go` |
| HTTP access log middleware | `apiserver/logging.go` |
| kubectl-like CLI tooling | `cmd/tkctl/main.go` |
| Service resource and selector-based endpoint discovery | `api/v1/types.go`, `apiserver/server.go` |
| Host-mapped port for cross-host reachability (macOS Docker) | `runtime/docker_runtime.go` |

## Architecture

```
  tkctl            curl / HTTP client
    │                      │
    └──────────┬───────────┘
               │ HTTP REST (:8080)
               ▼
┌──────────────────────────────────────────────────────────┐
│                        tinykube                          │
│                                                          │
│  ┌──────────────┐     ┌───────────────────────────────┐  │
│  │  API Server  │────▶│           Store               │  │
│  │  (HTTP REST) │     │  (in-memory KV, etcd-like)    │  │
│  └──────────────┘     └────────────────┬──────────────┘  │
│                                        │ watch           │
│                          ┌─────────────▼──────────────┐  │
│                          │   Deployment Controller     │  │
│                          │   (reconciliation loop)     │  │
│                          └─────────────┬──────────────┘  │
│                                        │                 │
│                          ┌─────────────▼──────────────┐  │
│                          │   PodRuntime (interface)    │  │
│                          └─────────────┬──────────────┘  │
│                                        │                 │
│                    ┌───────────────────┴──────────────┐  │
│                    │          DockerRuntime            │  │
│                    │   pod → docker container          │  │
│                    │   namespace → docker network      │  │
│                    │   readiness → HTTP host port      │  │
│                    └──────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

## How pods map to Docker

| tinykube concept | Docker concept |
|---|---|
| Pod | Container |
| Namespace | Docker network (`tinykube-{namespace}`) |
| Pod name | Container name (`tinykube-{pod-name}`) |
| Image | Docker image |
| Port | Exposed container port (host-mapped) |
| PodIP | Container IP on the Docker network |
| Readiness probe | HTTP GET `http://127.0.0.1:{hostPort}{path}` |

## Running it

### Option A — directly (recommended for development)

Requires Docker Desktop running.

```bash
cd tinykube
go run ./cmd/tinykube/
# 2026/03/21 19:00:00 [INFO]  Starting tinykube...
# 2026/03/21 19:00:00 [INFO]  API server listening on :8080
```

### Option B — docker compose

```bash
cd tinykube
docker compose up -d
docker compose logs -f   # stream logs
docker compose down      # tear down
```

The container mounts `/var/run/docker.sock` so `DockerRuntime` manages sibling containers on the host daemon.

## Debug logging

Debug logging is **enabled by default**. Every component emits `[DEBUG]` lines so you can follow exactly what the reconcile loop and runtime are doing:

```
2026/03/21 19:00:05 [DEBUG] controller: reconcile — 1 deployment(s)
2026/03/21 19:00:05 [DEBUG] controller: deployment=default/nginx desired=3 current=0
2026/03/21 19:00:05 [DEBUG] controller: scale up — creating pod nginx-q5lga (nginx:alpine)
2026/03/21 19:00:05 [DEBUG] runtime: CreatePod pod=nginx-q5lga image=nginx:alpine
2026/03/21 19:00:05 [DEBUG] store: put key=pods/default/nginx-q5lga event=Added
2026/03/21 19:00:07 [DEBUG] runtime: IsReady pod=nginx-q5lga url=http://127.0.0.1:52410/ → true (status=200)
2026/03/21 19:00:07 [DEBUG] watcher: pod=nginx-q5lga Pending → Running
2026/03/21 19:00:07 [DEBUG] store: put key=pods/default/nginx-q5lga event=Modified
```

API server requests are logged separately on every call:

```
2026/03/21 19:00:05 POST /apis/apps/v1/namespaces/default/deployments 201 401B 0.374ms
```

## CLI — tkctl

`tkctl` is a `kubectl`-like CLI for tinykube. Build it once and use it instead of raw curl:

```bash
go build -o tkctl ./cmd/tkctl/
```

### Create / update a deployment

**From a YAML manifest (recommended — used by tinyargo for GitOps):**

```bash
tkctl apply -f manifests/nginx.yaml
# deployment/nginx created

# Edit the manifest (e.g. change image to nginx:1.27) then re-apply
tkctl apply -f manifests/nginx.yaml
# deployment/nginx updated
```

**From flags (quick one-liners):**

```bash
tkctl apply --name nginx --image nginx:alpine --replicas 3 --port 80
# deployment/nginx created

tkctl apply --name nginx --image nginx:1.27 --replicas 3 --port 80
# deployment/nginx updated
```

`-f` and `--name` are mutually exclusive.

### List resources

```bash
tkctl get deployments
# NAME    REPLICAS   READY   IMAGE
# nginx   3          3       nginx:alpine

tkctl get pods
# NAME           STATUS    IP           IMAGE
# nginx-q5lga   Running   172.19.0.2   nginx:alpine
# nginx-j80sl   Running   172.19.0.3   nginx:alpine
# nginx-abc12   Running   172.19.0.4   nginx:alpine
```

### Check status

```bash
tkctl status deployment nginx
# FIELD                VALUE
# Replicas             3
# ReadyReplicas        3
# AvailableReplicas    3
# UpdatedReplicas      3
```

### Delete

```bash
tkctl delete deployment nginx
# deployment/nginx deleted
```

### Service commands

```bash
# Apply a Service manifest
tkctl apply -f manifests/whoami-svc.yaml
# service/whoami created

# List services
tkctl get services
# NAME     NAMESPACE   PORT   SELECTOR
# whoami   default     80     app=whoami

# Delete a service
tkctl delete service whoami
# service/whoami deleted
```

### Get live endpoints for a service

This calls the endpoint discovery API — returns only Running pods matching the selector:

```bash
curl http://localhost:8080/apis/v1/namespaces/default/services/whoami/endpoints
# [{"podName":"whoami-a1b2c","addr":"localhost:54321"},
#  {"podName":"whoami-d3e4f","addr":"localhost:54322"}]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--server` | `http://localhost:8080` | tinykube API server address |
| `--namespace` | `default` | namespace |
| `--replicas` | `1` | number of replicas (apply only) |
| `--port` | `80` | container port (apply only) |
| `--max-surge` | `1` | max extra pods during rolling update |
| `--max-unavailable` | `1` | max unavailable pods during rolling update |

## Raw API

For scripting or direct inspection, all endpoints accept and return JSON.

### Create a deployment

```bash
curl -X POST http://localhost:8080/apis/apps/v1/namespaces/default/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "nginx",
    "namespace": "default",
    "spec": {
      "replicas": 3,
      "selector": {"app": "nginx"},
      "template": {
        "labels": {"app": "nginx"},
        "spec": {
          "image": "nginx:alpine",
          "port": 80,
          "readinessProbe": {
            "path": "/",
            "initialDelaySeconds": 2,
            "periodSeconds": 2,
            "failureThreshold": 3
          }
        }
      },
      "strategy": {"maxSurge": 1, "maxUnavailable": 1}
    }
  }'
```

### Rolling update (change image)

```bash
curl -X PUT http://localhost:8080/apis/apps/v1/namespaces/default/deployments/nginx \
  -H 'Content-Type: application/json' \
  -d '{ ... same body with "image": "nginx:1.27" ... }'
```

### API reference

| Method | Path | Description |
|--------|------|-------------|
| POST | `/apis/apps/v1/namespaces/{ns}/deployments` | Create deployment |
| GET | `/apis/apps/v1/namespaces/{ns}/deployments` | List deployments |
| GET | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Get deployment |
| PUT | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Update deployment |
| DELETE | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Delete deployment |
| GET | `/apis/apps/v1/namespaces/{ns}/deployments/{name}/status` | Get status |
| GET | `/apis/v1/namespaces/{ns}/pods` | List pods |
| GET | `/apis/v1/namespaces/{ns}/pods/{name}` | Get pod |
| POST | `/apis/v1/namespaces/{ns}/services` | Create service |
| GET | `/apis/v1/namespaces/{ns}/services` | List services |
| GET | `/apis/v1/namespaces/{ns}/services/{name}` | Get service |
| PUT | `/apis/v1/namespaces/{ns}/services/{name}` | Update service |
| DELETE | `/apis/v1/namespaces/{ns}/services/{name}` | Delete service |
| GET | `/apis/v1/namespaces/{ns}/services/{name}/endpoints` | Get live endpoints |

## Testing

```bash
# Unit tests (no Docker needed)
go test ./...

# Integration tests (Docker required)
go test -tags=integration ./runtime/...
```

Unit tests use `FakeRuntime` — an in-memory runtime that instantly transitions pods to `Running` and tracks `CreatePod`/`DeletePod` call counts. All components accept `logger.NewNop()` in tests so no output noise.

## Manifest format

Manifests live in `manifests/` and are applied with `tkctl apply -f`. They are also what tinyargo will sync from a git repo in a GitOps workflow.

```yaml
kind: Deployment
name: nginx
namespace: default
spec:
  replicas: 3
  selector:
    app: nginx
  template:
    labels:
      app: nginx
    spec:
      image: nginx:alpine
      port: 80
      readinessProbe:
        path: /
        initialDelaySeconds: 2
        periodSeconds: 2
        failureThreshold: 3
  strategy:
    maxSurge: 1
    maxUnavailable: 1
```

| Field | Required | Description |
|---|---|---|
| `kind` | yes | `Deployment` or `Service` |
| `name` | yes | Resource name |
| `namespace` | no | Defaults to `default` |
| `spec.replicas` | yes (Deployment) | Desired pod count |
| `spec.strategy.maxSurge` | yes (Deployment) | Extra pods allowed during rolling update |
| `spec.strategy.maxUnavailable` | yes (Deployment) | Pods allowed unavailable during rolling update |
| `spec.template.spec.readinessProbe` | no | Omit to mark ready as soon as container starts |
| `serviceSpec.selector` | yes (Service) | Label selector — must match pod labels |
| `serviceSpec.port` | yes (Service) | Service port (informational) |
| `serviceSpec.targetPort` | yes (Service) | Container port to reach |

**Service manifest example:**

```yaml
kind: Service
name: whoami
namespace: default
serviceSpec:
  selector:
    app: whoami
  port: 80
  targetPort: 80
```

## Directory structure

```
tinykube/
├── cmd/
│   ├── tinykube/       — control plane entry point (wires everything together)
│   └── tkctl/          — kubectl-like CLI client
├── api/v1/             — type definitions with yaml + json tags (Deployment, Pod, Service, Manifest ...)
├── store/              — in-memory KV store with watch (etcd substitute)
├── apiserver/
│   ├── server.go       — HTTP REST handlers
│   └── logging.go      — request logging middleware
├── controller/         — reconciliation loop + rolling update
├── runtime/
│   ├── runtime.go      — PodRuntime interface
│   ├── docker_runtime.go — real Docker implementation
│   ├── fake_runtime.go — in-memory fake for unit tests
│   └── watcher.go      — background readiness polling goroutine
├── logger/             — two-level logger (Info/Debug), NewNop for tests
├── scheduler/          — round-robin pod scheduler (stub for M1–M6)
├── manifests/          — example YAML manifests (nginx.yaml, whoami.yaml)
├── Dockerfile          — multi-stage build (golang:1.23 + GOTOOLCHAIN=auto)
└── docker-compose.yml  — single-service stack with Docker socket mount
```

## Key design decisions

**Why a PodRuntime interface?**
The controller never calls Docker directly — it talks through `PodRuntime`. This mirrors Kubernetes' CRI (Container Runtime Interface) and makes the controller fully unit-testable with `FakeRuntime`.

**Why poll instead of watch for readiness?**
`StartReadinessWatcher` runs a goroutine per pod that polls `IsReady` on a timer. This mirrors how the kubelet works: periodic health checks rather than event-driven transitions.

**Template hash for rolling updates**
A hash of `PodSpec` (image + env + port) is stored as a `template-hash` label on each pod. The controller compares this against the current deployment template to detect when a rolling update is needed — the same mechanism Kubernetes uses.

**Logger injected at construction time**
Every component (`Store`, `DeploymentController`, `DockerRuntime`, `ReadinessWatcher`) accepts a `*logger.Logger`. Production code passes `logger.New(true)` (debug on by default); tests pass `logger.NewNop()` so test output stays clean. No global logger state.

**macOS readiness probe**
Docker on macOS runs containers inside a Linux VM, so internal container IPs (`172.x.x.x`) are not reachable from the host. `IsReady` calls `ContainerInspect` to find the host-mapped port and probes `127.0.0.1:{hostPort}` instead.
