# 0003 — Composition root layout and the Phase-0 end-to-end proof slice

## Status

Accepted

## Context

Blocos A-E (Fase 0) delivered every platform building block mesaposta-api's
HTTP API needs -- config loading, a tenant-scoped Postgres pool with
row-level security, the error taxonomy, the outbox pattern, JWT
issuing/verification, and the `Authenticate`/`RequireRole` HTTP middleware
-- but none of them were ever wired together into an actual running
`cmd/api` binary, nor exercised together against a real Postgres database
over real HTTP. Bloco F (T18-T20) closes that gap.

Two design questions needed a decision:

1. **Where does composition logic live, and how is it kept testable?**
   `main()` cannot reasonably be unit-tested (it calls `os.Exit`, sets up
   real OS signal handlers, and has no return value) but the graceful
   shutdown behavior and the config-error-to-exit-code path absolutely need
   test coverage.
2. **How do we prove the whole chain works, given Fase 1's first real
   domain module (`internal/modules/operations`) does not exist yet?**
   Nothing in the repository yet exercises `Authenticate` + `RequireRole` +
   `tenant.From` + `db.Scope` + row-level security + the `httpx.Envelope`
   shape all together, end-to-end, over real HTTP, against a real
   database.

## Decision

### Composition root split: `internal/platform/app` + `cmd/api`

- **`internal/platform/app`** (`NewApp`, `BuildVerifier`, `BuildIssuer`) is
  a plain, fully unit-testable package: `NewApp(cfg, pool, logger,
  verifier) http.Handler` takes every dependency as an explicit parameter
  and returns an `http.Handler` with no side effects of its own (no
  listening socket, no signal handling). Because `internal/platform/db.Pool`
  never eagerly dials the database (see `db.Open`'s own doc comment),
  `NewApp` can be exercised with a real `*db.Pool` in ordinary `go test`
  runs -- no Docker, no `-tags=integration` -- for every code path that
  never actually reaches the database (health checks, and every request
  `Authenticate`/`RequireRole` rejects before it reaches a handler).
- **`cmd/api`** stays a thin process shell: `main.go` is two lines
  (`os.Exit(run(...))`); `run.go` holds `run` (config -> pool -> optional
  boot migration -> `app.NewApp` -> `httpx.NewServer` -> `serve`) and
  `serve` (start the server, block until `ctx` is cancelled, then a
  bounded graceful `Shutdown`). Both `run` and `serve` are ordinary
  functions taking their dependencies (`getenv`, `stderr`/`stdout` writers,
  a cancellable `context.Context`, and -- for `serve` -- an optional
  `net.Listener`) as parameters, specifically so `cmd/api`'s own tests can
  drive a real graceful-shutdown scenario (a slow in-flight request, a
  simulated `SIGTERM` via `context` cancellation instead of a real OS
  signal, a bounded shutdown timeout that is or is not exceeded) without
  spawning a subprocess or touching a real port 8080.

### Fixed token identity: `app.TokenKID` / `TokenIssuer` / `TokenAudience`

Fase 0 ships no login endpoint, so nothing yet mints a "real" access token
in production. `internal/platform/app` nonetheless fixes a single kid/
issuer/audience triple (`"primary"` / `"mesaposta-api"` / `"mesaposta-api"`)
and exposes both `BuildVerifier` (used by `NewApp`) and `BuildIssuer` (used
today only by this package's own tests and the T19 E2E test, later by a
real login handler) built from the exact same triple. This guarantees a
token minted via `BuildIssuer(cfg)` is always exactly what
`BuildVerifier(cfg)` accepts, for any valid `cfg`, without a test (or a
future login handler) having to duplicate or guess the constants
`Authenticate`'s middleware was wired with.

### The `/v1/_probe` route

`NewApp` registers exactly one non-health route: `POST /v1/_probe/events`
(role `manager` only) writes a single row via
`internal/platform/outbox/postgres` (Bloco E's Postgres outbox adapter);
`GET /v1/_probe/events/{aggregateID}` (any authenticated role) reads it
back through a tenant-scoped `db.Scope`. This is deliberately **not** a
real domain endpoint -- it carries no business meaning, lives entirely
inside `internal/platform/app` (not `internal/modules/**`), and is expected
to be deleted once Fase 1 introduces a real authenticated route that can
serve the same proof purpose. Its only job is to give T19's end-to-end test
something real to write and read through the full JWT -> tenant context ->
RLS -> envelope chain.

### T19: one E2E test, real Postgres, real HTTP, `-tags=integration`

`internal/platform/httpx/e2e_integration_test.go` (package `httpx_test`,
`//go:build integration`) builds the complete app via `app.NewApp` against
a real, ephemeral Postgres container (`db.TestPostgres`, the same
testcontainers bootstrap Bloco E's own suites already use), serves it via
`httptest.NewServer`, and drives it purely over HTTP with
`net/http.Client` -- never calling any handler or middleware function
directly. It proves, as a single suite of subtests: tenant A can create and
read back its own data; tenant B can never observe tenant A's data (404,
**not** 403 -- reusing `httpx.NotFoundError`'s already-established rule
that a cross-tenant resource must be indistinguishable from a nonexistent
one); an expired token is rejected with 401; a role lacking permission is
rejected with 403; every response (success or failure) carries the
standard envelope shape and a request id.

This test intentionally lives under `internal/platform/httpx` (not
`internal/platform/app`) so it reads, from a test-suite-location
standpoint, as "proof that httpx's envelope/routing/middleware contract
holds all the way through a real request" -- the same spirit as `httpx`'s
own (non-E2E) test suite, just against a fully assembled application
instead of a bare router.

## Consequences

- `go test ./...` (no Docker required) already exercises every code path
  that does not touch the database, including the entire
  auth-rejection surface (missing header, malformed scheme, expired token,
  insufficient role) of the composed app -- not just of the underlying
  `internal/platform/auth` package in isolation.
- `go test -tags=integration ./...` (Docker required) additionally proves
  the full chain against a real database over real HTTP. This is the
  authoritative, no-code-changes-required command any future contributor
  (or CI, see `0004-ci-pipeline-and-coverage-gate.md`) runs to get the same
  guarantee this ADR describes.
- Coverage for `internal/platform/app` and `cmd/api`'s own database-facing
  code paths (`createProbeEventHandler`, `getProbeEventHandler`, `run`'s
  happy path) is necessarily low when measured by `go test` alone (no
  `-tags=integration`, no `-coverpkg`): those paths only execute inside the
  T19 E2E test and `cmd/api`'s own integration-only coverage, running in a
  DIFFERENT package's test binary. This mirrors the exact same
  already-accepted pattern Bloco E established for
  `internal/platform/outbox/postgres` and
  `internal/platform/auth/refresh`'s `PostgresStore` (low `go test`-only
  coverage, high combined `go test -tags=integration -coverpkg=./...`
  coverage) -- see `0004-ci-pipeline-and-coverage-gate.md` for how CI
  measures and gates on the combined number instead of penalizing
  DB-backed code for not being unit-testable without a database.
- `internal/modules/operations` (Fase 1's first real domain module) is
  expected to follow the same shape this ADR establishes for `/v1/_probe`
  (a thin HTTP adapter over `db.Scope` and, where relevant,
  `outbox.DomainEventPublisher`) but as a real `internal/modules/**`
  package per `internal/modules/README.md`, wired into `app.NewApp` instead
  of living inside `internal/platform/app` itself.
