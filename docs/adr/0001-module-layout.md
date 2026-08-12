# 0001 — Module layout: repo root as the service, not `services/api/`

## Status

Accepted

## Context

MesaPosta's backend could have been laid out in (at least) two ways:

1. A monorepo-style layout where the Go service lives under a subdirectory
   such as `services/api/`, leaving the repo root free for other services,
   infra code, or documentation that might be added later.
2. A layout where the repository root *is* the service — `go.mod` at the
   root, `cmd/`, `internal/`, `architecture/` etc. directly under the repo
   root.

This repository (`mesaposta-api`) was created specifically to hold the
`mesaposta-api` backend service and nothing else. There is no multi-service
monorepo requirement at this time, and no other service is expected to be
added to this same repository — additional services (if any) would get their
own repositories.

## Decision

The Go module lives at the repository root:

- `go.mod`: `module github.com/manuellysuzik/mesaposta-api`
- `go 1.24.5`

No `services/api/` (or similar) subdirectory wrapper is used. `cmd/`,
`internal/`, `architecture/`, `docs/`, etc. sit directly at the repo root.

The module path and this layout decision were made by the project owner
before the first `.go` file was written, so there is zero migration cost
associated with this choice — it is the layout from day one.

## Consequences

- Simpler tooling: `go build ./...`, `go vet ./...`, `go test ./...` and
  `golangci-lint run` operate on the whole repo without needing a `-C`/`cd`
  step or a nested go.mod.
- If, in the future, this repository needs to host more than one Go module
  or service, that will require a deliberate restructuring (and a new ADR
  superseding this one). Given the current scope (single backend service),
  this is considered acceptable and unlikely in the near term.
- CI/CD, Docker build contexts, and local developer workflows can all assume
  the repository root is the Go module root.
