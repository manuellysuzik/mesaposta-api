package outbox

import "context"

// DomainEventPublisher is the shared-kernel port for publishing domain
// events. It is deliberately a single-method interface (per the project's
// "small interfaces" convention) so that both domain code recording events
// and infrastructure code implementing a concrete broker only depend on
// exactly what they need.
//
// Publish must be atomic with respect to the caller's unit of work in
// concrete (non-memory) implementations -- i.e. it is expected to be
// called as part of the same transaction that persisted the aggregate
// state the events describe (the classic "transactional outbox" pattern).
// This package does not enforce that; it is a concern for the concrete
// implementation introduced in a later, infrastructure-layer block.
type DomainEventPublisher interface {
	// Publish records/publishes the given events. Implementations should
	// treat an empty events list as a no-op. ctx cancellation must be
	// honored: implementations should not perform the publish if ctx is
	// already done.
	Publish(ctx context.Context, events ...DomainEvent) error
}
