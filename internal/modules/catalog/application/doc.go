// Package application holds the catalog module's use cases: one
// application service per use case (creating/reading a Product, applying a
// discount, creating/reading a Recipe, promoting a Recipe into a Product),
// each orchestrating ../domain objects and ports (repositories,
// internal/platform/outbox's DomainEventPublisher, ...) to fulfil exactly
// one specific use case. See internal/modules/README.md for how
// application/ fits between domain/ and infrastructure/.
package application
