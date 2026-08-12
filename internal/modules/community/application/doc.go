// Package application holds the community module's use cases: one
// application service per use case (creating a post, reading a post,
// commenting on a post, liking/unliking a post), each orchestrating
// ../domain objects and ports (repositories, internal/platform/outbox's
// DomainEventPublisher, ...) to fulfil exactly one specific use case. See
// internal/modules/README.md for how application/ fits between domain/ and
// infrastructure/.
package application
