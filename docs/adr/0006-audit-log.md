# ADR-0006: Audit Log — Shared-Kernel Port + Staged Rollout

**Status**: Accepted
**Date**: 2026-08-13
**Author**: Manuelly Suzik
**Context**: Cross-cutting concern flagged in `BACKLOG.md` item 7 and `docs/ARCHITECTURE.md` §8 ("o que não está implementado ainda") since Fase 1.

## Problem

Sensitive mutations (receiving a purchase order, applying a discount, deleting a resource) leave no trail of *who* did *what*, *when*, to *which entity*, or what the state transition was. This is a real gap for compliance/debugging in production, and gets harder to retrofit the longer real mutation code paths exist without it.

**The question**: how do we add an audit trail that is actually trustworthy — i.e. that can never disagree with the mutation it describes — without coupling every module's domain layer to Postgres, and without a big-bang rewrite touching all five domain modules at once?

## Decision

**Mirror `internal/platform/outbox` exactly**, because it already solves the identical shape of problem: a side effect that MUST commit atomically with the business mutation it describes, from code that must not know Postgres exists.

- `internal/platform/audit` — pure port: `Entry` (tenant-scoped, validated at construction via `NewEntry`) and `Recorder` (`Record(ctx, entries...) error`). No `pgx`/`database/sql`/`net/http` anywhere in its build graph.
- `internal/platform/audit/postgres` — the Postgres adapter, in its own subpackage for the same reason `outbox/postgres` is separate from `outbox` (see that package's own doc comment): keeping infrastructure dependencies out of the port package is what lets `audit` join R1's domain allowlist without dragging `pgx` into every module's domain build graph. Exposes both `Recorder` (opens its own scope) and the free function `WithScope(ctx, scope, entries...)`, for callers that need the entry to commit inside a transaction they already control.
- `audit_log` table: `tenant_id`, `actor_id` (nullable), `action`, `entity_type`, `entity_id`, `before_data`/`after_data` (jsonb), `occurred_at`. Same `tenant_isolation` RLS policy shape as every other table since `0001_baseline.sql`. Append-only by design — `mesaposta_app` gets `INSERT`+`SELECT` only, never `UPDATE`/`DELETE` — matching the precedent already set by `stock_movements`/`transactions` (an audit trail that can be edited after the fact is not an audit trail).
- `architecture/rules.go`'s R1 domain-platform allowlist grows from `{errs, tenant, outbox}` to `{errs, tenant, outbox, audit}`. `audit/postgres` is deliberately **not** added — a domain package importing it would still be caught by R1's `github.com/jackc/**` forbidden-prefix check, exactly as `outbox/postgres` already is.

### ActorID is a free-form, optional string

There is no identity module yet (`docs/ARCHITECTURE.md` §8 lists this as a separate, known gap) — the only "who" this system can observe today is the authenticated JWT's `sub` claim, via `auth.ClaimsFromContext`, which is not guaranteed to be a UUID and is not always present (a scheduled job, an internal replay has no human actor at all). `Entry.ActorID` is therefore a plain `string`, persisted as SQL `NULL` when empty rather than as a placeholder — "the system did this" must stay distinct from "we forgot to record who did it". Revisit this as a typed reference once a real identity module exists.

## Staged rollout: `operations` only, deliberately

This ADR ships the full port + adapter + migration, **wired into exactly one use case**: `operations.ReceivePurchaseOrder` — the flow BACKLOG.md's original gap named by example ("receive-PO"), and one of the module's two version-conflict-sensitive critical paths (see ADR-0005). `catalog`, `crm`, `community`, and `ordering` are **not** touched by this change.

This is a deliberate scope cut, not an oversight, for the same reason `BACKLOG.md` already tracks other cross-cutting items as staged work: wiring five modules' worth of use cases in one change multiplies the surface for a mistake (each module's own `domain/ports.go`, Postgres `UnitOfWork`, application-layer call sites, and every test fake implementing `domain.Repositories` all need the same mechanical-but-error-prone update) without making any single wiring more correct. `operations.ReceivePurchaseOrder` proves the pattern end-to-end, against a real transaction, with a real RLS-scoped table — the remaining modules are now a known-shape, low-risk follow-up: add `Audit() audit.Recorder` to that module's `Repositories` interface, wire it in that module's `UnitOfWork`, call `audit.NewEntry` + `r.Audit().Record` at the one or two mutation sites that matter for that module, update fakes.

**Not started in this ADR**: `catalog.ApplyDiscount`, `crm` loyalty adjustments, `community` moderation actions (delete post/comment), `ordering`'s order lifecycle. Track as follow-up items in `BACKLOG.md`.

## Rationale: why last, inside the same `Do` call

The audit entry is recorded as the **last** write inside `ReceivePurchaseOrder`'s `uow.Do` callback, after the `PurchaseOrder`/`StockItem`/`StockMovement`/`Transaction` writes and the outbox `Events().Publish` call — mirroring the ordering rationale ADR-0005 already established for the aggregate writes themselves (the version-guarded `PurchaseOrder` update goes first, so a concurrent-conflict is detected as early as possible). Because everything runs inside the same `db.Scope`/transaction, a failure at ANY point — including the audit write itself — rolls back the whole unit of work: there is no code path where the receipt is persisted but unaudited, nor one where the trail claims a receipt that never committed.

## Related Files

- `internal/platform/audit/entry.go`, `internal/platform/audit/recorder.go` — the port
- `internal/platform/audit/postgres/postgres_recorder.go` — the adapter
- `internal/platform/db/migrations/0010_audit_log.sql` — schema
- `internal/modules/operations/domain/ports.go` — `Repositories.Audit()`
- `internal/modules/operations/infrastructure/postgres/unit_of_work.go` — `Audit()` wiring
- `internal/modules/operations/application/receive_purchase_order.go` — the one wired use case
- `internal/modules/operations/infrastructure/http/handlers.go` — extracts `actorID` from `auth.ClaimsFromContext`

## See Also

- [ADR-0002: Architecture Fitness Tooling](0002-arch-fitness-tooling.md) — R1 and the domain-platform allowlist this ADR extends
- [ADR-0003: Composition Root and E2E Proof Slice](0003-composition-root-and-e2e-proof-slice.md) — the transactional-outbox precedent this design mirrors
- [ADR-0005: Concurrency & Isolation Strategy for Operations Module](0005-operations-concurrency-and-isolation.md) — the write-ordering discipline this ADR follows for the audit entry itself
- `BACKLOG.md` item 7 — original gap, now partially closed
