# tinyargo

A simplified ArgoCD — implements GitOps sync for a single application: poll a git
repository, detect manifest drift, and apply changes to tinykube.

## Goals

- Understand the GitOps loop: git → diff → sync
- Understand drift detection between desired state (git) and live state (tinykube)
- Understand Application CR lifecycle: `OutOfSync → Syncing → Synced`
- Understand self-heal (auto-sync on drift)

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                       tinyargo                           │
│                                                          │
│  ┌──────────────┐     ┌──────────────────────────────┐  │
│  │  API Server  │────▶│         Store                │  │
│  │  (HTTP REST) │     │  (Application CRs)           │  │
│  └──────────────┘     └──────────────┬───────────────┘  │
│                                      │ watch             │
│                         ┌────────────▼────────────────┐ │
│                         │    Application Controller   │ │
│                         └────┬──────────┬─────────────┘ │
│                              │          │                │
│                   ┌──────────▼──┐  ┌───▼────────────┐  │
│                   │ Git Poller  │  │  Sync Engine   │  │
│                   │(go-git)     │  │ (→ tinykube)   │  │
│                   └─────────────┘  └────────────────┘  │
│                              │                          │
│                   ┌──────────▼──┐                       │
│                   │ Diff Engine │                        │
│                   │(desired vs  │                        │
│                   │  live)      │                        │
│                   └─────────────┘                       │
└──────────────────────────────────────────────────────────┘
```

## Components

### 1. API Types (`api/v1/`)

```go
type Application struct {
    Name      string
    Namespace string
    Spec      ApplicationSpec
    Status    ApplicationStatus
}

type ApplicationSpec struct {
    Source      GitSource
    Destination Destination
    SyncPolicy  SyncPolicy
}

type GitSource struct {
    RepoURL  string // git repo URL or local path
    Revision string // branch, tag, or commit SHA
    Path     string // directory within repo containing manifests
}

type Destination struct {
    Server    string // tinykube API server URL
    Namespace string
}

type SyncPolicy struct {
    AutoSync  bool // automatically sync on drift
    SelfHeal  bool // re-sync if live state drifts after sync
}

type ApplicationStatus struct {
    Sync   SyncStatus   // OutOfSync | Synced | Syncing | Failed
    Health HealthStatus // Healthy | Degraded | Progressing | Unknown
    Revision string     // current synced git commit SHA
}
```

### 2. Git Poller (`gitpoller/`)

- Clone or open a local git repo using `go-git`
- Poll every N seconds (default: 30s) for new commits on the tracked revision
- On change: extract manifests from the configured path
- Emit events: `ManifestChanged{oldRevision, newRevision, manifests}`

### 3. Diff Engine (`diffengine/`)

- Parse YAML manifests from git into typed structs (Deployment, etc.)
- Fetch live resources from tinykube API
- Produce a diff: `[]DiffResult{resource, action}` where action is `Create | Update | Delete | NoChange`
- Drive `SyncStatus`: if any diff exists → `OutOfSync`

### 4. Sync Engine (`syncengine/`)

- Consume diff results from the diff engine
- Apply each change to tinykube via HTTP:
  - `Create` → `POST /apis/apps/v1/namespaces/{ns}/deployments`
  - `Update` → `PUT /apis/apps/v1/namespaces/{ns}/deployments/{name}`
  - `Delete` → `DELETE /apis/apps/v1/namespaces/{ns}/deployments/{name}`
- Update Application status after sync completes

### 5. Application Controller (`controller/`)

```
for each Application:
    manifests = gitpoller.GetManifests(app.Spec.Source)
    diff      = diffengine.Diff(manifests, tinykube.LiveState())

    if diff is empty:
        app.Status.Sync = Synced
        continue

    app.Status.Sync = OutOfSync

    if app.Spec.SyncPolicy.AutoSync:
        app.Status.Sync = Syncing
        syncengine.Apply(diff)
        app.Status.Sync = Synced | Failed
        app.Status.Revision = currentGitSHA
```

### 6. API Server (`apiserver/`)

REST endpoints:

| Method | Path | Description |
|--------|------|-------------|
| POST   | `/apis/argoproj.io/v1/applications` | Create application |
| GET    | `/apis/argoproj.io/v1/applications` | List applications |
| GET    | `/apis/argoproj.io/v1/applications/{name}` | Get application |
| POST   | `/apis/argoproj.io/v1/applications/{name}/sync` | Trigger manual sync |
| GET    | `/apis/argoproj.io/v1/applications/{name}/status` | Get sync + health status |

## Directory Structure

```
tinyargo/
├── cmd/tinyargo/
│   └── main.go
├── api/
│   └── v1/
│       └── types.go
├── store/
│   ├── store.go
│   └── store_test.go
├── apiserver/
│   ├── server.go
│   └── server_test.go
├── gitpoller/
│   ├── poller.go
│   └── poller_test.go
├── diffengine/
│   ├── diff.go
│   └── diff_test.go
├── syncengine/
│   ├── sync.go
│   └── sync_test.go
├── controller/
│   ├── app_controller.go
│   └── app_controller_test.go
├── go.mod
└── SPEC.md
```

## Milestones

### M1 — Git Poller
- [ ] Clone/open repo, read manifests from a path
- [ ] Detect new commits on a branch
- [ ] Emit change events
- Tests written first with a local bare git repo fixture

### M2 — Diff Engine
- [ ] Parse YAML manifests into Deployment structs
- [ ] Fetch live state from tinykube API
- [ ] Produce Create/Update/Delete/NoChange diff
- Tests written first with fixture manifests and mock tinykube client

### M3 — Sync Engine
- [ ] Apply diff to tinykube via HTTP
- [ ] Report sync success/failure per resource
- Tests written first with mock tinykube API server

### M4 — Application Controller + API
- [ ] Application CR CRUD
- [ ] Reconciliation loop: poll → diff → auto-sync
- [ ] SyncStatus and HealthStatus updated
- [ ] Manual sync trigger via API
- Tests written first for controller with fakes

## Test Strategy

- **Unit tests**: poller, diff engine, sync engine tested independently with fakes
- **Integration tests**: controller wired to a local git repo + mock tinykube
- **E2E test**: real local git repo → tinyargo → tinykube; verify deployment created/updated
- All test files created before their corresponding implementation files
