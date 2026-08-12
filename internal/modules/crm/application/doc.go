// Package application holds the crm module's use cases: one application
// service per use case (creating a customer, reading a customer, crediting
// loyalty points, redeeming loyalty points), each orchestrating ../domain
// objects and ports (repositories, internal/platform/outbox's
// DomainEventPublisher, ...) to fulfil exactly one specific use case. See
// internal/modules/README.md for how application/ fits between domain/ and
// infrastructure/.
package application
