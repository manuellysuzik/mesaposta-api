package audit

import "context"

// Recorder is the shared-kernel port for writing audit-trail entries. It
// is deliberately a single-method interface (per the project's "small
// interfaces" convention) so that both application code recording actions
// and infrastructure code implementing a concrete store only depend on
// exactly what they need.
//
// Record must be atomic with respect to the caller's unit of work in
// concrete (non-memory) implementations -- i.e. it is expected to be
// called as part of the same transaction that persisted the state change
// the entries describe. That is the whole point of the audit trail: an
// entry that survives a rolled-back mutation (or a mutation that commits
// with no entry) turns the trail into a source of false statements about
// what happened. This package does not enforce that; it is a concern for
// the concrete implementation (see internal/platform/audit/postgres, whose
// WithScope operates on an already-open transaction owned by the caller).
type Recorder interface {
	// Record persists the given audit entries. Implementations should
	// treat an empty entries list as a no-op. ctx cancellation must be
	// honored: implementations should not perform the write if ctx is
	// already done.
	Record(ctx context.Context, entries ...Entry) error
}
