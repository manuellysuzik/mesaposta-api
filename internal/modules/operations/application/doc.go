// Package application holds the operations module's use cases: one
// application service per use case (e.g. registering a stock movement,
// receiving a purchase order, recording a transaction), each orchestrating
// ../domain objects and ports (repositories, internal/platform/outbox's
// DomainEventPublisher, ...) to fulfil exactly one specific use case. See
// internal/modules/README.md for how application/ fits between domain/ and
// infrastructure/.
package application
