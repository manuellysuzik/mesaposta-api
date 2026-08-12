# `internal/modules/` — template for mesaposta-api's domain modules

This directory does not contain any module yet. It exists, as of Fase 0
(Bloco F), purely to document the layout every domain module introduced
from Fase 1 onward (starting with `internal/modules/operations`) MUST
follow — so that decision doesn't get improvised, module by module, as the
codebase grows.

This document is deliberately the single source of truth for that layout.
If you are about to create `internal/modules/<name>/`, read this file
first, and follow it exactly; if you think it needs to change, change this
file (and get it reviewed) before or alongside the module that needs the
change, not silently in only one module.

## Why modules exist: the monolith-modular shape

mesaposta-api is a **modular monolith**: one deployable binary
(`cmd/api`), one Postgres database, but internally partitioned into
independent domain modules that own their own data and business rules and
talk to each other only through narrow, explicit contracts — never by
reaching into each other's internals. See
[`docs/adr/0001-module-layout.md`](../../docs/adr/0001-module-layout.md)
for why the module lives at the repo root, and
[`architecture/rules.go`](../../architecture/rules.go) for the automated
fitness function (R1-R6) that keeps this boundary honest over time, not
just at review time.

## Required layout

```
internal/modules/<name>/
├── domain/            # Entities, value objects, domain services,
│                       # repository INTERFACES. Pure business logic.
├── application/        # Use cases / application services: orchestrate
│                       # domain/ objects and ports (repositories, outbox,
│                       # ...) to fulfil one specific use case each.
├── infrastructure/
│   ├── http/           # HTTP adapters: chi handlers, request/response
│   │                   # DTOs, wiring into internal/platform/httpx and
│   │                   # internal/platform/auth. Depends on application/,
│   │                   # never directly on domain/ internals it doesn't
│   │                   # need.
│   └── postgres/       # Postgres adapters: repository IMPLEMENTATIONS
│                       # (satisfying domain/'s repository interfaces),
│                       # built on internal/platform/db.Scope, exactly the
│                       # same pattern internal/platform/outbox/postgres
│                       # and internal/platform/auth/refresh's
│                       # PostgresStore already establish.
└── contracts/          # The ONLY package other modules may import from
                        # this module (see R3, below). Small, stable,
                        # intentionally minimal: types and interfaces other
                        # modules need to integrate with this one, and
                        # nothing else.
```

**Naming choice: `application/`, not `app/`.** Both spellings are common in
the wild; this project uses `application/` specifically to avoid any
confusion with `internal/platform/app` (Fase 0's HTTP composition root,
introduced in Bloco F) — the two have nothing to do with each other, and a
module's own directory should never look like it might be that package.

**Naming choice: `infrastructure/{http,postgres}`, not
`adapters/{in,out}`.** Hexagonal-architecture literature often calls these
"driving"/"primary" (`in`) and "driven"/"secondary" (`out`) adapters. This
project instead names the two adapter kinds directly by what they adapt TO
(`http`, `postgres`) because, in practice, a module's own contributors need
"where does the chi handler for this endpoint live" answered faster than
they need the in/out hexagonal vocabulary reinforced — and because
`internal/platform/outbox/postgres` and
`internal/platform/auth/refresh/postgres_store.go` (Blocos B/C/E) already
established the `postgres` naming convention for a Postgres adapter
elsewhere in this codebase; modules should stay consistent with it rather
than introduce a second convention.

A module that has no HTTP surface of its own (e.g. a module only ever
invoked internally by another module's application layer, through
`contracts/`) may omit `infrastructure/http/`. A module with no
Postgres-backed state may omit `infrastructure/postgres/`. Every module
MUST have `domain/`, `application/`, and `contracts/` even if `contracts/`
starts out nearly empty — the point is that the seam exists from the
module's very first commit, not that every subdirectory is always
populated.

## The architecture rules every module must respect (R1-R6)

These are enforced automatically by `go test ./architecture/...` (see
[`docs/adr/0002-arch-fitness-tooling.md`](../../docs/adr/0002-arch-fitness-tooling.md)) —
a violation fails CI, not just code review — but understanding WHY each
rule exists makes it much easier to write code that never trips them in
the first place:

- **R1 — domain isolation.** Nothing under `**/domain/**` (i.e.
  `internal/modules/<name>/domain/**`) may transitively reach `net/http`,
  `database/sql`, `github.com/jackc/**` (pgx), `github.com/go-chi/**`,
  `github.com/golang-jwt/**`, `github.com/pressly/**` (goose),
  `github.com/alexedwards/**` (argon2id), nor any
  `internal/platform/**` package outside the allowlist
  {`errs`, `tenant`, `outbox`}. Your domain layer describes business rules;
  it must not know Postgres, HTTP, or JWTs exist. Repository behavior your
  domain code needs belongs to an INTERFACE declared in `domain/`, with the
  real implementation living in `infrastructure/postgres/`.
- **R2 — platform must not depend on modules.** `internal/platform/**` can
  never import `internal/modules/**`. The dependency direction is strictly
  one-way: modules depend on the platform, never the reverse. If platform
  code seems to need something module-specific, that's a sign the
  abstraction belongs in the module (or the platform package's scope is
  wrong), not a reason to add the import.
- **R3 — modules don't reach into each other's internals.**
  `internal/modules/<a>/**` may only import
  `internal/modules/<b>/**` through `internal/modules/<b>/contracts`
  — never `<b>/domain`, `<b>/application`, or `<b>/infrastructure`
  directly. If module A genuinely needs something from module B, add it to
  B's `contracts/` package (as a type, interface, or small function) and
  have A depend on that. This is what keeps modules independently
  understandable, testable, and (eventually, if ever needed) extractable
  into separate services.
- **R4 — `errs`/`tenant` stay dependency-free.** Not module-specific, but
  relevant background: `internal/platform/errs` and `internal/platform/tenant`
  (the two packages every module's `domain/` is allowed to import per R1)
  only ever import the standard library between them. That's exactly what
  makes it safe for R1 to allow domain code to depend on them.
- **R5 — `cmd/**` is exempt.** Composition-root code (`cmd/api`, and this
  project's own `internal/platform/app`) may import anything, including
  every module directly, to wire the final `http.Handler` together — that
  is composition roots' entire purpose. Never use this as an excuse to put
  actual module logic in `cmd/api` or `internal/platform/app`, though:
  R5's exemption is about wiring, not about where behavior lives.
- **R6 — `tenant.Parse` is reserved.** Only `internal/platform/auth/**`
  code (and `_test.go` files anywhere) may call `tenant.Parse`. Every other
  package — every module included — receives an already-validated
  `tenant.TenantID` from the request context, via
  `internal/platform/tenant.From(ctx)`, after
  `internal/platform/auth.Authenticate` has already run. A module's HTTP
  handlers and application services should never parse a raw tenant id
  string themselves; if you find yourself wanting to, you're probably
  missing a `tenant.TenantID` that should already be flowing through from
  the request context.

## What a module reuses from `internal/platform/**`

Every module's `infrastructure/postgres/**` code is expected to be built on
top of `internal/platform/db.Pool.Scope` — the same tenant-scoped,
row-level-security-enforcing pattern
`internal/platform/outbox/postgres` and
`internal/platform/auth/refresh.PostgresStore` already establish (see
Bloco E). Domain events a module needs to publish go through
`internal/platform/outbox.DomainEventPublisher` (R1 allowlists `outbox` for
exactly this reason). Every module's `infrastructure/http/**` handlers are
expected to be mounted into the router `internal/platform/app.NewApp`
builds (see
[`docs/adr/0003-composition-root-and-e2e-proof-slice.md`](../../docs/adr/0003-composition-root-and-e2e-proof-slice.md)),
behind `internal/platform/auth.Authenticate`/`RequireRole` exactly like
Bloco F's own `/v1/_probe` proof-of-wiring route, and responding through
`internal/platform/httpx`'s standard `Envelope`
(`httpx.RespondData`/`httpx.RespondError`) — never a hand-rolled JSON
response shape.

## Testing expectations

Follow this project's testing standard (80%+ coverage; unit, integration,
and — for a module's own critical flows — end-to-end tests) and this
repository's own established split:

- `domain/` and `application/`: plain `go test`, no build tag, no Docker.
  Repository interfaces are exercised against hand-written in-memory fakes
  (see `internal/platform/auth/refresh`'s own `RefreshStore` interface and
  its fake-backed unit tests for the pattern this project already uses).
- `infrastructure/postgres/**`: `//go:build integration` tests, built on
  `internal/platform/db.TestPostgres` (the same testcontainers-go bootstrap
  every Bloco B-F Postgres suite already shares) — see
  `internal/platform/outbox/postgres/postgres_publisher_integration_test.go`
  for the reference shape.
- `infrastructure/http/**`: ordinary `httptest`-based handler tests
  (no Docker) for request/response shape, validation, and error mapping;
  reserve real end-to-end, real-database proof for a SMALL number of
  critical flows, in the same spirit as
  `internal/platform/httpx/e2e_integration_test.go` (T19) — not one
  full-stack E2E test per endpoint.

## Checklist for a new module's first commit

- [ ] `domain/`, `application/`, `contracts/` exist (even if `contracts/`
      is nearly empty); `infrastructure/{http,postgres}/` exist if/when the
      module needs them.
- [ ] `domain/` imports nothing beyond the standard library plus
      `internal/platform/{errs,tenant,outbox}` (R1).
- [ ] Nothing in the module imports another module's `domain/`,
      `application/`, or `infrastructure/**` directly — only
      `<other-module>/contracts` (R3).
- [ ] No `tenant.Parse` call anywhere outside a `_test.go` file (R6) — the
      module receives `tenant.TenantID` from context, already validated.
- [ ] Postgres access goes through `internal/platform/db.Pool.Scope`; HTTP
      responses go through `internal/platform/httpx`'s `Envelope` helpers.
- [ ] `go test ./architecture/...` still passes with the new module in the
      tree.
- [ ] `make check` is green; combined coverage (see
      [`docs/adr/0004-ci-pipeline-and-coverage-gate.md`](../../docs/adr/0004-ci-pipeline-and-coverage-gate.md))
      stays at or above 80%.
