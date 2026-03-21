# tinykube

A simplified Kubernetes — implements only the Deployment controller with pod replica
reconciliation and rolling updates. Pods are real Docker containers.

## Goals

- Understand the Kubernetes reconciliation loop (desired state vs actual state)
- Understand rolling update mechanics (`maxSurge`, `maxUnavailable`)
- Understand status subresource reporting
- Understand how the API server, controller, and runtime interact
- Understand the CRI (Container Runtime Interface) abstraction pattern

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         tinykube                             │
│                                                              │
│  ┌──────────────┐     ┌─────────────────────────────────┐   │
│  │  API Server  │────▶│           Store                 │   │
│  │  (HTTP REST) │     │  (in-memory KV, etcd-like)      │   │
│  └──────────────┘     └──────────────┬──────────────────┘   │
│                                      │ watch                 │
│                         ┌────────────▼────────────────────┐ │
│                         │     Deployment Controller        │ │
│                         │     (reconciliation loop)        │ │
│                         └────────────┬────────────────────┘ │
│                                      │                       │
│                         ┌────────────▼────────────────────┐ │
│                         │       PodRuntime (interface)     │ │
│                         └────────────┬────────────────────┘ │
│                                      │                       │
│                         ┌────────────▼────────────────────┐ │
│                         │       DockerRuntime              │ │
│                         │  (docker/docker Go SDK)          │ │
│                         │                                  │ │
│                         │  pod → docker container          │ │
│                         │  namespace → docker network      │ │
│                         │  health check → HTTP /healthz    │ │
│                         └──────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

## How Pods Map to Docker Containers

| tinykube concept | Docker concept |
|-----------------|----------------|
| Pod | Container |
| Namespace | Docker network (`tinykube-{namespace}`) |
| Pod name | Container name |
| Image | Docker image |
| Env vars | Container environment variables |
| Port | Exposed container port |
| Pod IP | Container IP on the Docker network |
| Health check | HTTP GET `http://{containerIP}:{port}/healthz` |

Each pod container is started on a dedicated Docker bridge network per namespace.
Containers in the same namespace can reach each other by container name (Docker DNS).

## Components

### 1. API Types (`api/v1/`)

```go
type Deployment struct {
    Name      string
    Namespace string
    Spec      DeploymentSpec
    Status    DeploymentStatus
}

type DeploymentSpec struct {
    Replicas int
    Selector map[string]string
    Template PodTemplate
    Strategy RollingUpdateStrategy
}

type PodTemplate struct {
    Labels map[string]string
    Spec   PodSpec
}

type PodSpec struct {
    Image           string
    Env             map[string]string
    Port            int
    ReadinessProbe  *HTTPProbe // path + initialDelaySeconds + periodSeconds
}

type HTTPProbe struct {
    Path                string
    InitialDelaySeconds int
    PeriodSeconds       int
    FailureThreshold    int
}

type RollingUpdateStrategy struct {
    MaxSurge       int // extra pods allowed during update
    MaxUnavailable int // pods allowed to be unavailable during update
}

type DeploymentStatus struct {
    Replicas          int
    ReadyReplicas     int
    AvailableReplicas int
    UpdatedReplicas   int
}

type Pod struct {
    Name        string
    Namespace   string
    Labels      map[string]string
    Spec        PodSpec
    Status      PodPhase  // Pending | Running | Terminating | Failed
    PodIP       string    // assigned after container starts
    ContainerID string    // Docker container ID
}

type PodPhase string
const (
    PodPending     PodPhase = "Pending"
    PodRunning     PodPhase = "Running"
    PodTerminating PodPhase = "Terminating"
    PodFailed      PodPhase = "Failed"
)
```

### 2. Store (`store/`)

- In-memory key-value store (etcd substitute)
- Supports `Get`, `Put`, `Delete`, `List`, `Watch`
- `Watch` returns a channel emitting events: `Added | Modified | Deleted`
- Keys: `deployments/{namespace}/{name}`, `pods/{namespace}/{name}`

### 3. API Server (`apiserver/`)

REST endpoints:

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/apis/apps/v1/namespaces/{ns}/deployments` | Create deployment |
| GET    | `/apis/apps/v1/namespaces/{ns}/deployments` | List deployments |
| GET    | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Get deployment |
| PUT    | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Update deployment |
| DELETE | `/apis/apps/v1/namespaces/{ns}/deployments/{name}` | Delete deployment |
| GET    | `/apis/apps/v1/namespaces/{ns}/deployments/{name}/status` | Get status |
| GET    | `/apis/v1/namespaces/{ns}/pods` | List pods |
| GET    | `/apis/v1/namespaces/{ns}/pods/{name}` | Get pod |

Accepts and returns JSON.

### 4. PodRuntime Interface (`runtime/`)

The controller never calls Docker directly — it talks to a `PodRuntime` interface.
This mirrors how Kubernetes uses the CRI to stay decoupled from the container runtime.

```go
// runtime/runtime.go
type PodRuntime interface {
    // CreatePod starts a container for the pod, updates pod.PodIP and pod.ContainerID.
    CreatePod(ctx context.Context, pod *api.Pod) error

    // DeletePod stops and removes the container gracefully.
    DeletePod(ctx context.Context, pod *api.Pod) error

    // PodStatus returns the current phase of the pod by inspecting the container.
    PodStatus(ctx context.Context, pod *api.Pod) (api.PodPhase, error)

    // IsReady probes the pod's readiness endpoint.
    IsReady(ctx context.Context, pod *api.Pod) bool
}
```

### 5. DockerRuntime (`runtime/docker_runtime.go`)

Implements `PodRuntime` using `github.com/docker/docker/client`.

**CreatePod:**
1. Ensure Docker network `tinykube-{namespace}` exists (`NetworkCreate` if not).
2. Pull image if not present locally (`ImagePull`).
3. `ContainerCreate` with:
   - image, env vars, exposed port
   - network: `tinykube-{namespace}`
   - container name: `tinykube-{pod.Name}`
   - labels: `{"tinykube": "true", "namespace": ns, "deployment": deploymentName}`
4. `ContainerStart`
5. `ContainerInspect` to get assigned IP → set `pod.PodIP`
6. Update `pod.Status = Pending` in store; a background goroutine polls readiness.

**DeletePod:**
1. `ContainerStop` with a grace period (default 5s).
2. `ContainerRemove`.
3. Update `pod.Status = Terminating` then remove from store.

**PodStatus:**
1. `ContainerInspect` → check `State.Status`
2. Map: `running → Running`, `exited/dead → Failed`, container not found → deleted

**IsReady:**
1. HTTP GET `http://{pod.PodIP}:{pod.Spec.Port}{probe.Path}`
2. Returns true if response is 2xx within 1s timeout.
3. If no readiness probe configured, return true once container is running.

**FakeRuntime (`runtime/fake_runtime.go`):**
- In-memory implementation for unit tests
- Instantly transitions pods to Running
- Tracks CreatePod/DeletePod call counts for assertions

### 6. Deployment Controller (`controller/`)

Reconciliation loop (runs every `reconcileInterval`, default 5s):

```
for each Deployment in store:
    pods = store.List("pods") filtered by deployment selector

    // Scale up
    while len(pods) < deployment.Spec.Replicas:
        pod = newPod(deployment)
        runtime.CreatePod(pod)
        store.Put(pod)

    // Scale down
    while len(pods) > deployment.Spec.Replicas:
        pod = selectPodToDelete(pods)
        runtime.DeletePod(pod)
        store.Delete(pod)

    // Rolling update (template changed)
    if templateChanged(deployment, pods):
        rollingUpdate(deployment, pods)

    // Update status
    deployment.Status = computeStatus(pods)
    store.Put(deployment)
```

Template change detection: hash `PodSpec` (image + env + port) and compare
against a `templateHash` label on running pods.

**Rolling update algorithm:**
1. Compute `maxSurge` and `maxUnavailable` counts from strategy.
2. Create up to `maxSurge` new pods (new template).
3. Poll `runtime.IsReady()` until new pods are Running+Ready.
4. Delete up to `maxUnavailable` old pods.
5. Repeat until all old pods replaced.
6. Update `UpdatedReplicas` in status at each step.

### 7. Readiness Watcher (`runtime/watcher.go`)

Background goroutine per pod that polls `runtime.IsReady()` and updates
`pod.Status` in the store:

```
Pending → (container running) → Pending
Pending → (readiness probe passes) → Running
Running → (container exited) → Failed
```

This keeps the store eventually consistent with real container state.

### 8. Logger (`logger/`)

A minimal two-level logger (INFO / DEBUG) wrapping stdlib `log`. Debug output is
enabled by default so all internal component activity is visible at startup.

```go
type Logger struct { /* unexported */ }

func New(debug bool) *Logger      // debug=true → emit DEBUG lines
func NewNop() *Logger             // discards all output (for unit tests)

func (l *Logger) Info(format string, args ...interface{})
func (l *Logger) Debug(format string, args ...interface{})
```

Every component that does meaningful work accepts a `*logger.Logger`:

| Component | What is logged at DEBUG |
|---|---|
| Store | `put key=…`, `deleted key=…`, watch event `type=… key=…` |
| DeploymentController | reconcile loop start/end, desired vs actual replicas, scale-up/down pod names |
| Rolling update | wave start, pod created, pod ready, old pod deleted |
| DockerRuntime | `CreatePod image=…`, `DeletePod`, `IsReady url=… → true/false` |
| ReadinessWatcher | pod status transitions (`Pending → Running`, `→ Failed`) |

Example output when tinykube starts and a deployment is created:

```
2026/03/21 19:00:00 [INFO]  API server listening on :8080
2026/03/21 19:00:05 [DEBUG] controller: reconcile — 1 deployment(s)
2026/03/21 19:00:05 [DEBUG] controller: deployment=default/nginx desired=3 current=0
2026/03/21 19:00:05 [DEBUG] controller: scale up — creating pod nginx-abc12 (nginx:alpine)
2026/03/21 19:00:05 [DEBUG] runtime: CreatePod pod=nginx-abc12 image=nginx:alpine
2026/03/21 19:00:05 [DEBUG] store: put key=pods/default/nginx-abc12
2026/03/21 19:00:07 [DEBUG] watcher: pod=nginx-abc12 Pending → Running
```

### 9. API Server Logging (`apiserver/`)

HTTP access log middleware wrapping the existing mux. Logs one line per request:

```
2026/03/21 19:00:00 POST /apis/apps/v1/namespaces/default/deployments 201 312B 1.2ms
```

Fields: timestamp (from stdlib log), method, path, status code, response size (bytes), duration.

Implemented as `loggingMiddleware(next http.Handler) http.Handler` using a
`responseRecorder` that captures the status code and bytes written.
No external logging library — stdlib `log` only.

### 9. CLI — `tkctl` (`cmd/tkctl/`)

A `kubectl`-like command-line tool for tinykube. No external dependency — stdlib `flag` only.

**Commands:**

```
tkctl apply   --name <name> --image <image> --replicas <n> --port <p>
              [--namespace <ns>] [--max-surge <n>] [--max-unavailable <n>]
              [--server <addr>]

tkctl get     deployments|pods [--namespace <ns>] [--server <addr>]

tkctl delete  deployment <name> [--namespace <ns>] [--server <addr>]

tkctl status  deployment <name> [--namespace <ns>] [--server <addr>]
```

- `--server` defaults to `http://localhost:8080`
- `--namespace` defaults to `default`
- Output is human-readable table format (no external table library)
- `apply` creates or updates the deployment (PUT if exists, POST if not)

### 10. Docker Compose (`docker-compose.yml`)

Runs the full tinykube stack as a single `docker compose up`:

```yaml
services:
  tinykube:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

Requires a `Dockerfile` that builds the `tinykube` binary.

The container needs access to the Docker socket so `DockerRuntime` can manage
sibling containers on the host Docker daemon.

## Directory Structure

```
tinykube/
├── cmd/
│   ├── tinykube/
│   │   └── main.go                    ← control plane entry point
│   └── tkctl/
│       └── main.go                    ← CLI client
├── api/
│   └── v1/
│       └── types.go
├── store/
│   ├── store.go
│   └── store_test.go
├── apiserver/
│   ├── server.go
│   ├── server_test.go
│   ├── logging.go                     ← request logging middleware
│   └── logging_test.go
├── controller/
│   ├── deployment_controller.go
│   ├── deployment_controller_test.go  ← uses FakeRuntime
│   ├── rolling_update.go
│   └── rolling_update_test.go         ← uses FakeRuntime
├── runtime/
│   ├── runtime.go                     ← PodRuntime interface
│   ├── docker_runtime.go              ← real Docker implementation
│   ├── docker_runtime_test.go         ← integration test (needs Docker)
│   ├── fake_runtime.go                ← in-memory fake for unit tests
│   ├── fake_runtime_test.go
│   └── watcher.go                     ← readiness watcher goroutine
├── scheduler/
│   ├── scheduler.go
│   └── scheduler_test.go
├── logger/
│   ├── logger.go                      ← two-level logger (Info/Debug)
│   └── logger_test.go
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── SPEC.md
```

## Dependencies

```
github.com/docker/docker/client      ← Docker SDK
github.com/docker/docker/api/types   ← Docker API types
```

No other external dependencies. Standard library for HTTP, JSON, etc.

## Milestones

### M1 — Store + API Server
- [ ] In-memory store with watch support
- [ ] REST API CRUD for Deployments and Pods
- Tests written first for store and API handlers

### M2 — Docker Runtime
- [ ] `PodRuntime` interface defined
- [ ] `FakeRuntime` implemented (for unit tests)
- [ ] `DockerRuntime`: CreatePod, DeletePod, PodStatus, IsReady
- [ ] Network per namespace auto-created
- [ ] Readiness watcher goroutine
- Tests written first: fake runtime unit tests; Docker runtime integration test (tagged `//go:build integration`)

### M3 — Reconciliation Loop
- [ ] Controller scale-up / scale-down using FakeRuntime
- [ ] Template hash detection
- [ ] DeploymentStatus updated after each reconcile
- Tests written first using FakeRuntime

### M4 — Rolling Update
- [ ] Rolling update with maxSurge + maxUnavailable
- [ ] UpdatedReplicas tracked in status
- [ ] End-to-end test: update image → verify old containers replaced
- Tests written first using FakeRuntime; integration test with Docker

## Test Strategy

- **Unit tests**: store, API handlers, controller, rolling update — all use `FakeRuntime`
- **Integration tests** (`//go:build integration`): `DockerRuntime` tests that actually
  start/stop containers; require Docker daemon
- **E2E scenario tests**: deploy nginx with 3 replicas → update image → verify rolling
  update completes with no downtime window
- All test files created before their corresponding implementation files

---

## Roadmap: Kubelet + Control Plane / Worker Node Split

> This is a future milestone after M1–M4 are complete. It fundamentally restructures
> tinykube from a single-process toy into a two-process distributed system, teaching
> the real Kubernetes node architecture.

### The Problem with the Current Design

Right now the Deployment Controller calls `DockerRuntime` directly — it creates and
deletes containers itself. This means everything runs in one process on one machine.
There is no concept of a "node."

In real Kubernetes the controller never touches containers. It only writes to etcd.
A separate agent — the **kubelet** — runs on every worker node, watches for pods
assigned to it, and manages containers locally. This is the key architectural insight.

### Target Architecture

```
┌─────────────────────────────────┐       ┌──────────────────────────────┐
│         Control Plane           │       │         Worker Node 1        │
│                                 │       │                              │
│  ┌───────────┐  ┌────────────┐  │  HTTP │  ┌──────────────────────┐   │
│  │ APIServer │  │ Controller │  │◀─────▶│  │       kubelet        │   │
│  │  :8080    │  │  (loop)    │  │       │  │                      │   │
│  └─────┬─────┘  └────────────┘  │       │  │  watches API server  │   │
│        │                        │       │  │  for pods where      │   │
│  ┌─────▼──────────────────────┐ │       │  │  NodeName == "node1" │   │
│  │          Store             │ │       │  │                      │   │
│  │   (pods, deployments,      │ │       │  │  calls DockerRuntime │   │
│  │    nodes)                  │ │       │  │  locally             │   │
│  └────────────────────────────┘ │       │  └──────────────────────┘   │
└─────────────────────────────────┘       │                              │
                                          │  ┌──────────────────────┐   │
                                          │  │    Docker Daemon      │   │
                                          │  │  (actual containers)  │   │
                                          │  └──────────────────────┘   │
                                          └──────────────────────────────┘
```

Multiple worker nodes can run, each with its own kubelet process and Docker daemon.

### What Changes

| Current (M1–M4) | After Kubelet Roadmap |
|---|---|
| Controller calls `DockerRuntime` directly | Controller only writes `pod.NodeName` to store |
| One process: everything | Two binaries: `tinykube` (control plane) + `tinykubelet` (node agent) |
| No node concept | `Node` object registered in store by each kubelet |
| Scheduler is a stub | Scheduler assigns `pod.NodeName` to a registered node |
| Single machine | Multiple machines (or processes) can join as nodes |

### New API Types

```go
type Node struct {
    Name       string
    Labels     map[string]string
    Status     NodeStatus
}

type NodeStatus struct {
    Phase      NodePhase   // Ready | NotReady
    Conditions []NodeCondition
    LastHeartbeat time.Time
}
```

Pods gain two new fields:

```go
type Pod struct {
    // ... existing fields ...
    NodeName string    // set by scheduler; empty = unscheduled
}
```

### New Components

#### Scheduler (upgraded from stub)

Watches for pods where `NodeName == ""` and assigns them to a registered ready node:

```
for each unscheduled pod:
    nodes = store.List("nodes") where status == Ready
    node  = leastLoadedNode(nodes)  // or round-robin
    pod.NodeName = node.Name
    store.Put(pod)
```

#### Kubelet (`cmd/tinykubelet/`)

A separate binary that runs on each worker node:

```
startup:
    register self as a Node in the API server (POST /apis/v1/nodes)

loop every 5s:
    // Heartbeat — keep node status fresh
    PATCH /apis/v1/nodes/{nodeName}/status  { LastHeartbeat: now }

    // Sync pods assigned to this node
    pods = GET /apis/v1/pods?nodeName={nodeName}

    for each pod in pods:
        if pod not running locally:
            dockerRuntime.CreatePod(pod)
            PATCH pod.Status = Running

    for each locally running container:
        if no matching pod in API server:
            dockerRuntime.DeletePod(...)   // orphan cleanup

    // Report readiness
    for each running pod:
        ready = dockerRuntime.IsReady(pod)
        PATCH pod.Status accordingly
```

#### Node Controller (new, in control plane)

Watches node heartbeats and marks nodes `NotReady` if heartbeat is missed:

```
for each Node:
    if now - node.LastHeartbeat > nodeTimeout (default 30s):
        node.Status.Phase = NotReady
        // evict pods on this node back to unscheduled
```

### New Directory Structure (additions only)

```
tinykube/
├── cmd/
│   ├── tinykube/          ← control plane (existing)
│   │   └── main.go
│   └── tinykubelet/       ← NEW: worker node agent
│       └── main.go
├── kubelet/               ← NEW
│   ├── kubelet.go         ← main kubelet loop
│   ├── kubelet_test.go
│   ├── node_registrar.go  ← registers Node object on startup
│   └── node_registrar_test.go
├── nodecontroller/        ← NEW
│   ├── node_controller.go ← heartbeat monitor, eviction
│   └── node_controller_test.go
├── scheduler/             ← upgraded from round-robin stub
│   ├── scheduler.go       ← least-loaded node assignment
│   └── scheduler_test.go
└── ...
```

### Roadmap Milestones

#### R1 — Node Registration
- [ ] `Node` type added to API types
- [ ] `POST /apis/v1/nodes` and `GET /apis/v1/nodes` endpoints
- [ ] Kubelet registers itself on startup with node name + labels
- [ ] Heartbeat PATCH every N seconds
- Tests written first

#### R2 — Scheduler: Pod Assignment
- [ ] Scheduler watches for pods with empty `NodeName`
- [ ] Assigns `NodeName` to a ready node (round-robin or least-loaded)
- [ ] Controller no longer calls `DockerRuntime` directly
- Tests written first with fake node registry

#### R3 — Kubelet: Pod Sync Loop
- [ ] Kubelet polls API server for pods assigned to its node
- [ ] Calls `DockerRuntime` locally to create/delete containers
- [ ] PATCHes pod status back to API server
- [ ] Orphan cleanup: delete containers with no matching pod
- Tests written first with mock API server and FakeRuntime

#### R4 — Node Controller: Heartbeat + Eviction
- [ ] Node controller monitors heartbeat timestamps
- [ ] Marks node `NotReady` after timeout
- [ ] Evicts pods from `NotReady` nodes (clears `NodeName` → back to scheduler)
- Tests written first for heartbeat timeout and eviction logic

### What You Learn

| Concept | Where |
|---|---|
| Control plane / data plane split | tinykube vs tinykubelet as separate processes |
| Node registration and heartbeats | kubelet → API server |
| Pod scheduling onto nodes | Scheduler setting `pod.NodeName` |
| Kubelet as the real container manager | DockerRuntime moves into kubelet |
| Node failure and pod eviction | Node controller + heartbeat timeout |
