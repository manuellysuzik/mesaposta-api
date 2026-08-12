# ADR-0005: Concurrency & Isolation Strategy for Operations Module

**Status**: Accepted  
**Date**: 2026-08-12  
**Author**: Manuelly Suzik  
**Context**: Fase 1 (operations module) — StockItem, PurchaseOrder, Transaction, StockMovement aggregates.

## Problem

The `ReceivePurchaseOrder` use case is a critical operation that must atomically:
1. Mark a PurchaseOrder as received (state transition, version bump).
2. Create N StockMovements (one per line item).
3. Create a Transaction (financial record).
4. Update N StockItems (increase their quantities).

All four changes must succeed together or none at all. Concurrent calls to receive the same order must result in exactly one winner; losers must receive a 409 Conflict.

**The question**: Which Postgres isolation level, and what concurrency controls, guarantee this atomicity and prevent the "lost update" anomaly?

## Decision

**Use Postgres Read Committed isolation + explicit row-level locks + optimistic version control.**

- `internal\platform\db\scope.go` calls `pool.Begin(ctx)` without `pgx.TxOptions`, which defaults to Postgres **Read Committed** (not Serializable).
- `internal\modules\operations\infrastructure\postgres\purchase_order_repository.go` calls `FindByIDForUpdate`, which wraps `SELECT ... FOR UPDATE`, acquiring a row-level lock on the target PurchaseOrder.
- `internal\modules\operations\application\receive_purchase_order.go` calls `UpdateWithVersion(expectedVersion)`, which uses `UPDATE ... WHERE id=$1 AND version=$2`, returning `errs.Conflict` (→ HTTP 409) if zero rows were affected.
- `internal\modules\operations\domain\receiving.go` (the domain service) enforces deterministic order: stock items are processed in ascending ID order, eliminating deadlock risk.

## Rationale

**Why not Serializable?**

Postgres Serializable isolation would prevent *any* concurrent anomaly, but it comes at a cost:
- Every transaction that conflicts must be retried by the application (receive `40001` error, backoff, retry).
- For this specific access pattern (single row update with a version guard), Serializable provides no additional safety over Read Committed + row lock + optimistic version.
- The anomaly actually at risk is "lost update" on concurrent receives — exactly what `SELECT FOR UPDATE` closes: only one transaction can hold the lock.
- Retrying on `40001` is noise for a use case where conflicts are *expected to be rare* and the application should simply surface 409 to the user ("already received by someone else").

**Evidence: the concurrency integration test**

`internal\modules\operations\infrastructure\postgres\receive_purchase_order_concurrency_integration_test.go` proves the mechanism:
- Two goroutines, released by a barrier at the same time, each attempt to receive the same PurchaseOrder.
- One acquires the lock (rows affected: 1), the other blocks, then wakes to find `version` no longer matches (rows affected: 0 → `errs.Conflict`).
- Exactly one HTTP 200, exactly one HTTP 409. No data corruption, no double-receive.

**Why the version check matters**

The version check protects against a narrower (but real) race: if the lock were held too briefly — e.g., only for the PurchaseOrder itself, not for the entire transaction — a concurrent receipt could slip in after lock release but before commit of the stock movements. The version on both PurchaseOrder and StockItem makes that impossible (if version drifts, the second transaction sees a stale snapshot and conflicts).

**When to revisit**

Introduce Serializable (or move to a saga / distributed lock) if:
1. A read-modify-write operation involves **two or more aggregates without explicit cross-row locking**, e.g., checking inventory on one stock item, then updating another without a lock on the first.
2. Deadlock detection at the database layer (Postgres logs them) becomes routine, suggesting the lock-ordering discipline (by aggregate ID) is insufficient.
3. Latency profiles show 40001 retries are not rare (currently expected to be <0.1% of receives).

## Related Files

- `internal\platform\db\scope.go` — `Begin(ctx)` entry point
- `internal\modules\operations\domain\receiving.go` — load-bearing detail: ascending-ID sort order on stock items
- `internal\modules\operations\infrastructure\postgres\receive_purchase_order_concurrency_integration_test.go` — proof
- `internal\modules\operations\infrastructure\postgres\unit_of_work.go` — `Do(...)` orchestrator, calls `UpdateWithVersion`

## See Also

- [ADR-0002: Architecture Fitness Tooling](0002-arch-fitness-tooling.md) — how R1–R6 enforce this discipline
- [ADR-0004: CI Pipeline and Coverage Gate](0004-ci-pipeline-and-coverage-gate.md) — integration test scoping
