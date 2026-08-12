// Package platform contains cross-cutting infrastructure concerns shared
// across the mesaposta-api modules (e.g. tenant context, error taxonomy,
// outbox pattern, authentication). It is the only place outside the domain
// layer that domain code is allowed to depend on, and even then only through
// an explicit allowlist enforced by the architecture fitness function (see
// architecture/rules.go).
//
// No implementation lives here yet; this file exists to establish the
// package during the Phase 0 skeleton (Bloco A). Concrete sub-packages
// (errs, tenant, outbox, auth, ...) are introduced in later blocks.
package platform
