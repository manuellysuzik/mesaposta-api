# 0004 — CI pipeline: two jobs, and a coverage gate that doesn't punish DB-backed code

## Status

Accepted

## Context

`make check` (Bloco A) already codifies mesaposta-api's local pre-commit
gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, and
`go test -race -cover -count=1 ./...`. None of that requires Docker, so it
deliberately excludes every `-tags=integration` test (Bloco B-F's
Postgres-backed suites, built on `internal/platform/db.TestPostgres` /
testcontainers-go) -- a contributor without Docker running can still get a
fast, meaningful local signal.

CI needs to do everything `make check` does, PLUS:

- run the `-tags=integration` suites against a real Postgres, including
  T19's full end-to-end proof slice (`0003-composition-root-and-e2e-proof-slice.md`);
  the fitness function that catches architecture regressions, and this task's own
  own acceptance criterion is that both suites are actually exercised, not
  just compiled;
- enforce this project's committed-to 80% coverage floor
  (`rules/common/testing.md`) as an automated gate, not just a number
  someone eyeballs in a PR;
- run `gosec` and `govulncheck`, per `rules/golang/security.md` and this
  project's own `security.md`/`code-review.md` rules, as automated,
  CI-enforced security scanning rather than a manually-remembered step.

The coverage-gate design has one real subtlety, already visible in Bloco E
and reconfirmed by Bloco F (see `0003-composition-root-and-e2e-proof-slice.md`,
"Consequences"): several packages
(`internal/platform/db`, `internal/platform/outbox/postgres`,
`internal/platform/auth/refresh`, `internal/platform/app`, `cmd/api`) have
large swaths of code that ONLY execute against a real Postgres connection.
Measuring coverage from `go test ./...` alone (no `-tags=integration`, no
`-coverpkg`) reports these packages far below 80% even though the code is
thoroughly tested -- just not by tests compiled into that package's own
`go test` binary. A coverage gate built on that number would either have to
carve out permanent per-package exceptions (drifting out of sync as new
packages are added) or be silently ignored/disabled the first time it
blocks a legitimate PR.

## Decision

### Two jobs: `check` and `integration`

- **`check`** (no Docker): `gofmt -l .`, `go vet ./...`, `golangci-lint
  run`, `go test -race -count=1 ./...` -- i.e. exactly `make check`, run as
  discrete steps so a failure is immediately attributable to the specific
  tool that caught it. Also runs `gosec ./...` and `govulncheck ./...` here
  (both are static/dependency scans; neither needs a database).
- **`integration`** (Docker available on GitHub's `ubuntu-latest` runners
  out of the box -- testcontainers-go talks to the runner's existing Docker
  daemon with no extra services block or Docker-in-Docker setup required):
  `go test -tags=integration -race -count=1 ./...`. This single command
  already runs BOTH the plain unit-style tests (the `-tags=integration`
  build tag is additive, not exclusive -- see `internal/platform/db`'s own
  `testsupport_integration.go`) AND every `_integration_test.go` suite
  across Blocos B-F, including T19's E2E proof slice.

Splitting into two jobs (rather than one job running only the
`-tags=integration` superset) keeps the fast, Docker-free signal
(`check`) reportable on every push/PR independently of Postgres/container
infrastructure availability or flakiness, matching `make check`'s existing
promise to local contributors.

### Coverage gate measures the COMBINED profile, in the `integration` job

The 80% gate is computed from:

```
go test -tags=integration -race -count=1 -coverpkg=./... -coverprofile=coverage.out ./...
go tool cover -func=coverage.out   # last line: "total: (statements) NN.N%"
```

`-coverpkg=./...` is the deliberate fix for the subtlety above: without it,
Go's coverage instrumentation only attributes a test's coverage to the
package that test file itself lives in, so `internal/platform/httpx`'s own
E2E test (which calls deep into `internal/platform/app`, `internal/platform/db`,
and `internal/platform/outbox/postgres` over real HTTP) would report ZERO
coverage credit to any of those packages. With `-coverpkg=./...`, every
statement executed by ANY test in the run -- regardless of which package's
test file triggered it -- counts toward that statement's own package's
coverage. This is what lets a package like `internal/platform/app`, whose
own `_test.go` files intentionally avoid the database (see
`0003-composition-root-and-e2e-proof-slice.md`), still be credited for the
database-touching code paths that T19's E2E test exercises through it.

The gate is a dedicated step (not a golangci-lint linter, not inline in the
`go test` step) so its pass/fail reason -- "total coverage NN.N%, floor is
80%" -- is unambiguous in CI output, independent of whichever specific
package(s) happen to be under/over the floor in a given PR.

### `gosec` and `govulncheck`

Both run as their own `check`-job steps, `./...`-wide, non-optional (a
finding fails the job, matching this project's `security.md`,
"Mandatory Security Checks"). `.golangci.yml` already runs `gosec` as one
of `golangci-lint`'s enabled linters (see `.golangci.yml`); the standalone
`gosec ./...` CI step is intentionally redundant with that -- it uses
`gosec`'s own, more actively maintained ruleset/CLI directly rather than
golangci-lint's (sometimes lagging) vendored copy, so a real finding cannot
slip through solely because golangci-lint's bundled version is stale.

## Consequences

- A contributor never needs to reason about per-package coverage
  exceptions: the single number CI reports and gates on
  (`go tool cover -func`'s `total:` line, from the `-coverpkg=./...`
  combined profile) is honest about what is and isn't tested across the
  WHOLE repository, DB-backed code included.
- The `integration` job requires Docker and takes meaningfully longer than
  `check` (multiple ephemeral Postgres containers across Blocos B-F's
  suites plus T19's own). It is still required to pass before merge --
  this project has no "integration tests are best-effort" exception -- but
  running it as a separate job means a contributor iterating on a
  Docker-free change gets `check`'s fast feedback first.
- If a future package genuinely cannot reach 80% combined coverage (e.g. a
  thin adapter around a third-party SDK with no reasonable way to exercise
  every branch), the right fix is a `//go:build integration`-only test that
  exercises it against a real dependency, or a documented, reviewed
  golangci-lint `nolint`-style exception at the specific line/function --
  never lowering the global 80% floor.
