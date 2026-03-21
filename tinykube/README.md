# tinykube

A toy Kubernetes implementation — Deployment controller with pod replica reconciliation and rolling updates. Pods are real Docker containers.

Built as part of [tinybox](../README.md), a collection of simplified CNCF project implementations for study.

## What it teaches

| Concept | Where |
|---|---|
| Reconciliation loop (desired vs actual state) | `controller/deployment_controller.go` |
| Rolling update with maxSurge / maxUnavailable | `controller/rolling_update.go` |
| CRI abstraction (runtime interface) | `runtime/runtime.go` |
| In-memory store with watch (etcd substitute) | `store/store.go` |
| REST API server for resources | `apiserver/server.go` |
| Readiness probes and pod lifecycle | `runtime/watcher.go` |

## Architecture

```
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
│                    │   health check → HTTP /healthz    │  │
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

Requires Docker Desktop running.

```bash
# Start the control plane
go run ./cmd/tinykube/
# API server now listening on :8080
```

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

The controller reconciles every 5 seconds. Within ~15s, three `nginx:alpine` containers appear in Docker and pods transition `Pending → Running`.

### Watch pods

```bash
curl http://localhost:8080/apis/v1/namespaces/default/pods | jq '[.[] | {name: .Name, status: .Status}]'
```

### Check deployment status

```bash
curl http://localhost:8080/apis/apps/v1/namespaces/default/deployments/nginx/status
# {"Replicas":3,"ReadyReplicas":3,"AvailableReplicas":3,"UpdatedReplicas":3}
```

### Rolling update (change image)

```bash
curl -X PUT http://localhost:8080/apis/apps/v1/namespaces/default/deployments/nginx \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "nginx",
    "namespace": "default",
    "spec": {
      "replicas": 3,
      "selector": {"app": "nginx"},
      "template": {
        "labels": {"app": "nginx"},
        "spec": {"image": "nginx:1.27", "port": 80}
      },
      "strategy": {"maxSurge": 1, "maxUnavailable": 1}
    }
  }'
```

The controller detects the template hash change and performs a rolling update: new pods are created, waited on for readiness, then old pods are terminated — in batches bounded by `maxSurge` and `maxUnavailable`.

### Scale

```bash
# Update replicas field in a PUT request (same as above, change "replicas": 5)
```

### Delete a deployment

```bash
curl -X DELETE http://localhost:8080/apis/apps/v1/namespaces/default/deployments/nginx
```

## API reference

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

## Testing

```bash
# Unit tests (no Docker needed)
go test ./...

# Integration tests (Docker required)
go test -tags=integration ./runtime/...
```

Unit tests use `FakeRuntime` — an in-memory runtime that instantly transitions pods to `Running` and tracks `CreatePod`/`DeletePod` call counts. The controller, rolling update, and store are fully tested without Docker.

## Directory structure

```
tinykube/
├── cmd/tinykube/       — main entry point; wires everything together
├── api/v1/             — type definitions (Deployment, Pod, PodSpec, ...)
├── store/              — in-memory KV store with watch (etcd substitute)
├── apiserver/          — HTTP REST handlers
├── controller/         — reconciliation loop + rolling update
├── runtime/
│   ├── runtime.go      — PodRuntime interface
│   ├── docker_runtime.go — real Docker implementation
│   ├── fake_runtime.go — in-memory fake for unit tests
│   └── watcher.go      — background readiness polling goroutine
└── scheduler/          — round-robin pod scheduler (stub for M1–M4)
```

## Key design decisions

**Why a PodRuntime interface?**
The controller never calls Docker directly — it talks through `PodRuntime`. This mirrors Kubernetes' CRI (Container Runtime Interface) and makes the controller fully unit-testable with `FakeRuntime`.

**Why poll instead of watch for readiness?**
`StartReadinessWatcher` runs a goroutine per pod that polls `IsReady` on a timer. This mirrors how the kubelet works: periodic health checks rather than event-driven transitions.

**Template hash for rolling updates**
A hash of `PodSpec` (image + env + port) is stored as a `template-hash` label on each pod. The controller compares this against the current deployment template to detect when a rolling update is needed — the same mechanism Kubernetes uses.

**macOS note**
Docker on macOS runs containers inside a Linux VM, so internal container IPs (`172.x.x.x`) are not reachable from the host. Readiness probes use the host-mapped port (`127.0.0.1:{hostPort}`) obtained via `ContainerInspect`.
