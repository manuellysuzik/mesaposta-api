# ADR-0008: Platform Tenant Provisioning API

**Status**: Accepted
**Date**: 2026-09-10
**Author**: Manuelly Suzik
**Context**: `mesaposta-frontend`'s `backoffice` module ships a full tenant-management UI (create/edit/activate/suspend) that has always been mock-only, backed by an in-memory array. Wiring it to a real backend requires a way to create a tenant over HTTP -- something `0008_tenants.sql` deliberately did not provide.

## Problem

`0008_tenants.sql`'s own header comment states the original decision plainly: "there is NO INSERT/UPDATE/DELETE grant [on `tenants`] to `mesaposta_app` at all -- provisioning a tenant is an administrative operation that happens outside the application's runtime request path ... never via a request `mesaposta_app` services." Provisioning has, until now, only ever happened via a migration, `psql`, or `cmd/seedtenant` (a local CLI tool with no auth of its own, meant to be run by a developer with direct database access).

The frontend's backoffice needs the opposite: a real, authenticated HTTP endpoint an operator can call from a browser session. This ADR documents the deliberate reversal of `0008_tenants.sql`'s original stance, and the new authentication model that makes reversing it safe.

**Why not just reuse the existing tenant-scoped JWT (`gestor`/`funcionario`) for this?** Every route under `/v1` today is scoped to exactly one tenant, via `auth.Authenticate`'s `tenant_id` claim and Postgres RLS. Managing tenants is inherently cross-tenant -- there is no `tenant_id` to put in that claim, and a `gestor` of one restaurant must never be able to create or suspend a *different* restaurant's tenant. Reusing the per-tenant role model here would either be meaningless (no tenant to scope to) or a serious isolation bug (one tenant's staff managing another's existence). This was explicitly considered and rejected during planning.

## Decision

### New package: `internal/platform/platformadmin`

A platform operator is not a `gestor`/`funcionario` of any tenant -- they are staff of the platform itself, a concept this codebase has never needed before. Authentication is deliberately the simplest thing that could work for a Phase 1 rollout: a single static shared secret (`Config.PlatformAdminToken`, required, ≥32 bytes, guarded against the `.env.example` placeholder in production exactly like `JWTSecret` already is), compared in constant time (`crypto/subtle.ConstantTimeCompare`) against the `X-Platform-Admin-Token` header. `internal/platform/platformadmin/http` holds the actual handlers (`Mount`), mounted at `/v1/platform/tenants` -- a sibling of `/v1/auth`, outside the `/v1` group that carries `auth.Authenticate`, since this API is not scoped to any tenant at all.

This is explicitly a Phase 1 decision, not a permanent one: a real "platform operator" identity (its own table, its own login flow, individual accountability instead of one shared secret) is a natural evolution once this API has more than a handful of trusted operators. Tracked as follow-up, not solved here.

### Migration `0012_tenants_platform_fields.sql`

Adds `owner_name`, `owner_email`, `phone` (nullable -- required-ness is enforced at the HTTP DTO layer, not the schema), `plan` and `status` (both `NOT NULL` with a `CHECK` constraint against a closed set, defaulting to `basic`/`trial`) to `tenants`. Grants `mesaposta_app` `INSERT`/`UPDATE`/`SELECT` on exactly the columns the new API needs -- with two columns deliberately excluded from `UPDATE`: `id` and `slug`. `id` is a primary key; `slug` is embedded in every `/public/v1/{tenantSlug}/...` URL a diner may have bookmarked or shared, so it must never change after creation. Two new RLS policies (`platform_admin_insert`, `platform_admin_update`) are permissive (`USING (true)`/`WITH CHECK (true)`), mirroring `0008_tenants.sql`'s own `public_slug_lookup` policy for the identical reason: `tenants` has no `tenant_id` column to filter by, so real authorization lives entirely in `platformadmin.RequireToken` at the HTTP layer, never in RLS.

This migration was kept **separate** from `0008_tenants.sql` rather than edited in place, even though both were still pre-production at the time: `0008`'s header comment is itself a valuable historical record of a real security decision being made, and later reversed, for a documented reason. Folding the new grants into `0008` directly would have made that header comment contradict the SQL immediately below it, erasing the "why" this ADR exists to preserve.

### API shape: narrow named actions, not a generic PATCH

Mirroring `internal/modules/ordering`'s existing `ativar`/`desativar` convention (not a REST-generic `PUT`/`PATCH`):

- `POST /v1/platform/tenants` -- create (always starts `status=trial`, `is_active=true`; caller cannot set status directly)
- `GET /v1/platform/tenants` -- list, newest first
- `POST /v1/platform/tenants/{id}/atualizar` -- update `name`/`owner_name`/`owner_email`/`phone`/`plan`. **Full replace, not a partial patch** -- omitting a field clears it, mirroring `mesaposta-frontend`'s own form, which always submits every field on save.
- `POST /v1/platform/tenants/{id}/ativar` / `.../suspender` -- status transitions. `is_active` is derived from `status` in the same statement (`trial`/`active` → `true`, anything else → `false`), the only place this codebase ever writes `tenants.is_active` for an existing row.

**No delete endpoint.** Deleting a tenant that already has rows across every tenant-scoped module (orders, products, customers, ...) is a materially harder problem -- retention, cascading cleanup, whether "delete" even means the same thing as GDPR-style erasure -- that deserves its own design, not a rushed addition here. The frontend's existing delete button is removed from the UI as an explicit scope cut, not silently left dangling against an endpoint that doesn't exist.

## Known limitation: frontend login stays mock

`mesaposta-frontend`'s NextAuth session (including its already-existing `admin` role, which gates the `/backoffice` route in `middleware.ts`) is not wired to any real backend login -- it authenticates against `mockUsers` today, and continues to. Only the Server Actions that call this new API attach the real `PLATFORM_ADMIN_TOKEN` (read server-side, never sent to the browser). This means "being logged in as `admin`" in the frontend UI and "being authorized to call the platform API" are two independent things for now, bridged by a single shared secret both sides happen to know. This is a known, accepted gap for this phase, not an oversight -- real frontend authentication against the real backend (starting with `/v1/auth/login`) is a separate, larger piece of work this ADR does not attempt.

## Related Files

- `internal/platform/config/config.go` -- `PlatformAdminToken`, `ExamplePlatformAdminToken`
- `internal/platform/platformadmin/middleware.go` -- `RequireToken`
- `internal/platform/db/platform_tenant_admin.go` -- `CreateTenant`/`ListTenants`/`UpdateTenantDetails`/`SetTenantStatus`
- `internal/platform/platformadmin/http/{router,handlers,dto}.go` -- the HTTP surface
- `internal/platform/app/app.go` -- `mountPlatformAdminRoutes`
- `internal/platform/db/migrations/0012_tenants_platform_fields.sql` -- schema

## See Also

- `internal/platform/db/migrations/0008_tenants.sql` -- the decision this ADR partially reverses, and why its header is left intact rather than rewritten
- [ADR-0006: Audit Log](0006-audit-log.md) -- another example of this codebase documenting a deliberate scope cut (there: five modules' rollout staged to one; here: no delete endpoint) rather than leaving it unexplained
