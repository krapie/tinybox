# tinybox

A collection of simplified, study-purpose implementations of core CNCF projects.
Each project implements only the essential feature set to understand how the real
system works under the hood.

## Projects

| Project   | Models After      | Core Feature                              |
|-----------|------------------|-------------------------------------------|
| tinykube  | Kubernetes        | Deployment reconciliation + rolling update |
| tinyargo  | ArgoCD            | GitOps sync from a git repository         |
| tinydns   | CoreDNS           | Service discovery + plugin chain          |
| tinyotel  | OpenTelemetry     | OTLP receiver + traces + metrics + logs   |
| tinyenvoy | Envoy             | L7 proxy with routing, retries, observability |

## Principles

- **Test first**: write tests before implementation for every component.
- **Minimal scope**: implement only what the spec describes — no extras.
- **Go**: all projects are written in Go, mirroring real CNCF project language choices.
- **No external dependencies** beyond the standard library and one or two focused libs
  (e.g. `miekg/dns` for tinydns, `go-git` for tinyargo, `net/http` reverse proxy for tinyenvoy).

## Build Order

```
1. tinykube  — foundation; others integrate with it
2. tinyotel  — independent; observability pipeline immediately useful
3. tinydns   — independent; DNS layer
4. tinyargo  — ties everything together; syncs manifests to tinykube
5. tinyenvoy — sits in front of tinykube pods; exports OTLP spans to tinyotel
```

## Demo

A `docker-compose` in the root will wire all five together:
- tinyargo syncs a Deployment manifest to tinykube
- tinydns resolves the service name
- tinyenvoy proxies inbound traffic to the pods, applying routing rules
- tinyotel receives OTLP traces and metrics from all components (tinyenvoy exports spans natively)
