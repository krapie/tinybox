# CLAUDE.md — tinybox

Project-level instructions for Claude. These override any default behavior.

## Project Overview

tinybox is a collection of toy implementations of modern infrastructure and developer
tooling, built for study and potentially used for a home server. Each project is a
simplified clone of a real-world system, written in Go.

| Project   | Models After      | SPEC |
|-----------|------------------|------|
| tinykube  | Kubernetes        | [tinykube/SPEC.md](./tinykube/SPEC.md) |
| tinyotel  | OpenTelemetry     | [tinyotel/SPEC.md](./tinyotel/SPEC.md) |
| tinydns   | CoreDNS           | [tinydns/SPEC.md](./tinydns/SPEC.md) |
| tinyargo  | ArgoCD            | [tinyargo/SPEC.md](./tinyargo/SPEC.md) |
| tinyenvoy | Envoy             | [tinyenvoy/SPEC.md](./tinyenvoy/SPEC.md) |

## Before Starting Any Task

1. Read the relevant `SPEC.md` for the project being worked on.
2. Never implement anything not described in the spec — scope is intentionally minimal.
3. Architectural decisions belong to the human, not Claude.

## Implementation Workflow

```
update SPEC.md → write tests → write implementation → verify tests pass → update README.md → commit
```

**TDD is strict — no exceptions:**
- Create the `_test.go` file first, with all test cases written out.
- Only then create the implementation file.
- Never write implementation code before the corresponding test exists.

## Documentation Sync — Mandatory

**Every feature addition or bug fix must update all of the following before committing:**

| Document | What to keep current |
|---|---|
| `SPEC.md` | Add/renumber component sections; fix any outdated API/behavior descriptions; mark milestones `[x]` when complete |
| `README.md` | Usage examples, CLI commands, directory structure, "What it teaches" table, key design decisions |
| `sample/e2e_test.sh` | Add/update test sections for any new component or feature; keep assertion count in sync |
| `sample/README.md` | Update step-by-step guide, architecture diagram, and "What this demonstrates" table |

Never commit implementation changes without updating all four files in the same commit.

## Code Conventions

- Language: Go for all projects.
- Each project is its own Go module (`go.mod` inside each project directory).
- No external dependencies beyond the standard library and the one or two libs
  approved per project in its SPEC.md.
- Use `//go:build integration` tag for tests that require Docker or a network.
- Unit tests must run with `go test ./...` and no external services.

## Commit Workflow

- Commit after each milestone or logical unit of work.
- Commit message format: `type(project): short description`
  - e.g. `feat(tinykube): add in-memory store with watch support`
  - Types: `feat`, `fix`, `test`, `refactor`, `docs`, `chore`
- Always push to `main` after committing.

## Bash Commands

- All bash commands freely allowed.
- `rm` / `rm -rf` require human approval before executing.

## Build Order

If unsure which project to work on next, follow this order:

```
1. tinykube  → 2. tinyotel  → 3. tinydns  → 4. tinyargo  → 5. tinyenvoy
```
