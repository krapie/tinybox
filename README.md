# tinybox

Toy implementations of modern cloud native infrastructure — maybe used for my home server.

Each project implements only the essential feature set to understand how the real system works under the hood. Written in Go, test-first.

## Projects

| Project | Models After | Core Feature |
|---------|-------------|--------------|
| [tinykube](./tinykube/SPEC.md) | Kubernetes | Deployment reconciliation + rolling update via real Docker containers |
| [tinyotel](./tinyotel/SPEC.md) | OpenTelemetry | OTLP receiver + traces + metrics + logs |
| [tinydns](./tinydns/SPEC.md) | CoreDNS | Service discovery + plugin chain (log, cache, forward) |
| [tinyargo](./tinyargo/SPEC.md) | ArgoCD | GitOps sync from a git repo to tinykube |
| [tinyenvoy](./tinyenvoy/SPEC.md) | Envoy | L7 proxy with routing, retries, and observability |

## Build Order

```
tinykube → tinyotel → tinydns → tinyargo → tinyenvoy
```

Each project builds on the previous — tinykube is the foundation everything else integrates with.

## Principles

- **Test first** — test files written before implementation, no exceptions
- **Minimal scope** — only what the spec describes
- **No magic** — standard library + one or two focused external libs per project

## Structure

```
tinybox/
├── tinykube/    # mini Kubernetes
├── tinyotel/    # mini OpenTelemetry Collector
├── tinydns/     # mini CoreDNS
├── tinyargo/    # mini ArgoCD
├── tinyenvoy/   # mini Envoy
└── SPEC.md      # collection-level spec
```

Each project contains its own `SPEC.md` with architecture, components, milestones, and test strategy.
