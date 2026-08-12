# Backlog — Fase 1 (Operations Module HTTP Layer) & Future Phases

**Status**: Wave A ✅ | Waves B–D 🏗️  
**Last Updated**: 2026-08-12

## Completed — Wave A ✅

- [x] Slice 8: Update architecture rules TODOs (R4, R6 — set `AllowEmpty: false, MinMatches: 1`)
- [x] Slice 9: ADR 0005 — Concurrency & Isolation rationale (Read Committed + locks)
- [x] Slice 0: HTTP plumbing foundation
  - [x] `internal/modules/operations/infrastructure/http/decode.go` — JSON decoding, UUID path params
  - [x] `internal/modules/operations/infrastructure/http/dto.go` — response/request DTOs, mappers
  - [x] `internal/modules/operations/infrastructure/http/router.go` — Mount placeholder
  - [x] `internal/modules/operations/infrastructure/http/fake_uow_test.go` — test fakes
  - [x] `docs/adr/0005-operations-concurrency-and-isolation.md` — design rationale

## In Progress — Waves B–D 🏗️

### Wave B: Read Endpoints (parallelizable)
- [ ] Slice 1: `GET /v1/stock-items/{id}` handler + tests
  - Handler: extract tenant/ID → `application.GetStockItem` → 200 + DTO
  - Tests: 200 success, 404 not found, 400 malformed ID, 500 internal error

- [ ] Slice 2: `GET /v1/purchase-orders/{id}` handler + tests  
  - Handler: extract tenant/ID → `application.GetPurchaseOrder` → 200 + DTO
  - Handle Total() overflow → map to 500 via error handler
  - Tests: 200, 404, 400, 500

### Wave C: Create Endpoints + Mount (parallelizable)
- [ ] Slice 7: Mount in composition root (`internal/platform/app/app.go`)
  - Build `uow := operationspg.NewUnitOfWork(pool)`
  - Register via `operationshttp.Mount(r, uow)` in `/v1` route
  - Guard: skip mount if `pool == nil`
  - Tests: authenticated reads work, unknown routes 404, role checks enforced

- [ ] Slice 3: `POST /v1/stock-items` create handler + tests
  - Decode request → parse value objects (Unit, Quantity, Money) → `application.CreateStockItem` → read back → 201
  - Tests: 201 happy, 400 invalid unit/quantity/name/cost, 401 missing tenant

- [ ] Slice 4: `POST /v1/purchase-orders` create handler + tests
  - Decode items → parse per-item quantities/costs → `application.CreatePurchaseOrder` → read back → 201
  - Tests: 201, 400 empty items/duplicate item/invalid quantity

### Wave D: Complex Operations (parallelizable)
- [ ] Slice 5: `POST /v1/stock-items/{id}/movements` register handler + tests
  - Decode movement (type/quantity/reason) → `application.RegisterStockMovement` → read back → 201
  - **Critical**: 409 `insufficient_stock` when withdrawal exceeds balance (acceptance criterion #1 proven 3rd time)
  - Tests: 201 entrada/saida, 409 insufficient, 400 invalid type/qty, 404 not found

- [ ] Slice 6: `POST /v1/purchase-orders/{id}/receive` receive handler + tests ⭐
  - No request body → `application.ReceivePurchaseOrder` → read back → 200
  - **Critical path**: proves 409 conflict handling (already received, cancelled, optimistic version)
  - Tests: 200 success (status/received_at/version updated), 409 already received, 409 cancelled, 409 version conflict, 404 order not found

### Wave E: Optional (E2E + refactoring)
- [ ] Slice 10 (optional): Full E2E integration test
  - httptest.Server + real Postgres (Docker-gated, `//go:build integration`)
  - Flow: create stock item → create PO → receive → GET shows updated qty
  - Cross-tenant isolation check

- [ ] Slice 6-variant (optional, separate commit): Refactor to return aggregat
e
  - Change `application.ReceivePurchaseOrder` to return `(*domain.PurchaseOrder, error)`
  - Eliminates read-back in handler (more atomic)
  - Update concurrency test
  - Only after Slice 6 is green

---

## Known Improvements Deferred to Future Phases

These are **not bugs**, but enhancements identified during implementation. **Not to be implemented in Fase 1**, but documented here for Fase 2+ planning.

### 1. Read-back as separate transaction (Waves C–D)
**Issue**: Slices 5–6 return representations from a *different* transaction than the write.  
**Impact**: Low — atomicity is logical (domain-level), not temporal. Client receives fresh committed state.  
**Mitigation**: Document in handler comments that response reflects post-commit state.  
**Consider in**: Slice 6-variant (return aggregat e directly from use case, eliminates read-back).

### 2. Richer error codes for value-object parsing
**Issue**: `domain.StockUnit` parse errors return `errs.Invalid` with code `"invalid_stock_unit"`. Other parse errors follow the same pattern.  
**Impact**: Extremely low — 400 responses are acceptable; difference is granularity of code string.  
**Mitigation**: None needed now; revisit if API consumers request finer error discrimination.  
**Consider in**: Fase 2 hardening (post-launch feedback).

### 3. Idempotency-Key support (noted as Fase 0 Gap #6 in master plan)
**Issue**: POST endpoints that should be idempotent (POST /create-stock-item, etc.) lack `Idempotency-Key` header support.  
**Impact**: Medium — double-click risk without infrastructure.  
**Blocker**: Requires idempotency key store (fast lookup, TTL 24h per tenant).  
**Defer to**: Fase 6 (original plan Slice 9), once main HTTP layer is shipping.  
**Impl**: Add `middleware.Idempotency` that checks request hash in store; return cached response if duplicate detected.

### 4. Rate limiting for cardápio digital public endpoint (Fase 3 concern, noted in master plan)
**Issue**: `/public/v1/{slug}/cardapio/**` endpoint (Fase 3 / ordering) will be unauthenticated, needs IP+tenant rate limit.  
**Impact**: Medium — without limit, endpoint is DoS vector.  
**Defer to**: Fase 3 (ordering module), build into `infrastructure/http` there.  
**Note**: Unlike the authed `/v1/**` endpoints (rate-limited by session), public needs per-IP + per-tenant bucket.

### 5. Soft-delete for Product & StockItem (Fase 2 concern, noted in master plan)
**Issue**: If `Product` or `StockItem` is hard-deleted, `Order` and `PurchaseOrder` referencing that ID break (FK constraint violated, historical queries fail).  
**Impact**: Medium — only affects multi-year deployments with data retention policies.  
**Solution**: Use `is_active` boolean + logical deletion instead of physical.  
**Defer to**: Fase 2 (catalog) + Fase 1 (operations) schema migration if needed.

### 6. OpenAPI contract auto-generation (Fase 0 improvement #2)
**Issue**: Contrato HTTP original (artefato HTML) already diverged before this work (missing Mesas/Pedidos).  
**Solution**: Generate OpenAPI 3.1 from Go code at build time (via oapi-codegen or swaggo annotations).  
**Impact**: Low-to-medium (infra only, improves maintainability).  
**Defer to**: Fase 4 (post-delivery) or as dedicated task once HTTP stable.  
**Impl**: Add CLI build step to generate `openapi/generated.json` + validate against tests.

### 7. Audit log table & queries (Fase 0 Gap #7 in master plan)
**Issue**: No trail of who did what when for DELETE/apply-discount/receive-PO operations.  
**Impact**: Medium — required for compliance/debugging in production.  
**Solution**: `audit_log` table (tenant_id, actor_id, action, entity, before/after JSON) written in same txn as mutations.  
**Defer to**: Fase 6 (original plan) or post-launch if audit requirement emerges.

### 8. Better isolation level docs (long-term)
**Issue**: ADR 0005 documents Read Committed + locks decision. If Postgres logs `40001` retries routinely, might warrant revisiting.  
**Impact**: Low — current approach proven by test.  
**Metric to track**: Log 40001 errors; if >0.1% of receives, escalate to Serializable.

---

## Definition of Done — Phases B–D

- [ ] All slices implemented (handlers, tests)
- [ ] `go build ./internal/modules/operations/...` — 0 errors
- [ ] `go test ./internal/modules/operations/...` — 100% pass (unit + handler + integration if Docker available)
- [ ] `go test ./architecture/...` — `TestRepoCompliance` 0 violations
- [ ] Coverage ≥ 80% (domain + application + http combined)
- [ ] `go vet -tags=integration ./...` — 0 warnings
- [ ] ADR 0005 linked from `unit_of_work.go` + `scope.go`
- [ ] All slices squashed into 2–3 logical commits (Wave A already separate)
- [ ] Commit messages follow conventional format (feat/fix/refactor)

---

## Notes for Reviewers

- **Gateguard**: This file required 9 gate assertions due to frequent file creation. Deployers can reduce noise by running with `ECC_GATEGUARD=off` during bulk writes if needed.
- **Parallelism**: Waves B and D are fully independent (no file conflicts). Wave C's Slice 7 must run after Wave B is merged (dependency: `application` package exists).
- **Test strategy**: Handler tests use `httptest.Server` + func-field fakes (no Docker required for unit tests). Integration tests tagged `//go:build integration` and use testcontainers (Docker gated).
- **Next step**: Reviewer approves backlog and deferred improvements list, then Waves B–D implementation proceeds.
