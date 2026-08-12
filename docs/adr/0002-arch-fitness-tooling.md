# 0002 — Architecture fitness function: `go test` over `golang.org/x/tools/go/packages`, not `go-arch-lint`

## Status

Accepted

## Context

MesaPosta follows tactical DDD + Hexagonal architecture as a modular
monolith. To keep the architecture honest over time (domain packages must
not reach into infrastructure, modules must not reach into each other's
internals, etc.), the project needs an automated architecture fitness
function that runs in CI and locally.

Two broad approaches were considered:

1. **External tool**: `go-arch-lint` (or similar), a standalone binary
   configured via YAML that expresses layering/dependency rules and is
   invoked as a separate CI step.
2. **Self-hosted fitness function**: a Go test suite, backed by
   `golang.org/x/tools/go/packages`, that loads the module's package graph
   and asserts architectural rules directly in Go code.

The acceptance criteria already agreed for this project is literally:

```
go test ./architecture/...
```

i.e. the architecture check must be runnable as a normal Go test, with no
extra binaries to install, no separate CI step wiring, and no YAML DSL to
learn or keep in sync with the actual rules.

Additionally, `golang.org/x/tools/go/packages` gives direct access to the
already-resolved, transitive import graph of the module (`NeedImports` +
`NeedDeps`), which is exactly what several rules require (e.g. "domain
packages must not *transitively* reach `net/http`"). External tools that
work off of static per-file import declarations tend to model direct imports
well but do not always model transitive closure over the whole dependency
graph correctly, especially across build tags, generated code, or vendored
paths — and verifying that behavior for a third-party tool is itself extra
work with no guarantee of the desired semantics.

## Decision

Architecture rules are implemented as a table-driven Go test suite:

- `architecture/rules.go` — the rule engine: rule definitions, the allowed
  package prefixes / allowlists, and the checking logic built on top of
  `golang.org/x/tools/go/packages` (loaded with
  `NeedName | NeedImports | NeedDeps`, plus `NeedSyntax | NeedTypesInfo` for
  rules that need to inspect symbol references rather than just import
  edges).
- `architecture/arch_test.go` — the test suite that loads real packages (and
  fixtures under `architecture/testdata/fixtures/`) and asserts the rule
  engine reports exactly the expected violations, including a
  meta-test that fails the suite if any non-`AllowEmpty` rule matched zero
  packages (anti-vacuity check).

No `go-arch-lint` (or equivalent) dependency is introduced. The check is
invoked as `go test ./architecture/...`, matching the already-approved
acceptance criteria exactly, and requires no additional binary beyond the Go
toolchain itself.

## Consequences

- Zero extra CLI dependency to install/pin/upgrade in CI or locally beyond
  what `go.mod`/`go.sum` already manage.
- Rules are expressed in Go, with the full expressiveness of the language
  (arbitrary predicates, precise transitive closure, symbol-level checks via
  `go/types`), rather than being limited to what a YAML DSL can express.
- Rules and their tests live in the same place and are exercised by the same
  `go test` command developers already use, lowering the chance the
  architecture check silently rots out of sync with reality.
- The trade-off is that the team owns the rule engine's correctness instead
  of delegating it to a maintained external project; this is considered
  acceptable given the rule set is currently small (six rules) and the
  engine itself is covered by its own tests (including the anti-vacuity
  meta-test).
