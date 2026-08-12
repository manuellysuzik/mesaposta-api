package outbox

import (
	"context"
	"sync"
)

// MemoryPublisher is an in-memory DomainEventPublisher, intended for use in
// tests across Phases 1-6 (and anywhere else a real broker is undesirable),
// never in production. It records every published event in order, guarded
// by a mutex so it is safe under concurrent Publish calls.
type MemoryPublisher struct {
	mu     sync.Mutex
	events []DomainEvent
}

// compile-time check: *MemoryPublisher must satisfy DomainEventPublisher.
var _ DomainEventPublisher = (*MemoryPublisher)(nil)

// NewMemoryPublisher returns a ready-to-use, empty MemoryPublisher.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{}
}

// Publish appends events (in order) to the publisher's in-memory log. It
// honors ctx cancellation: if ctx is already done, Publish returns ctx.Err()
// without recording anything. Calling Publish with zero events is a no-op.
func (p *MemoryPublisher) Publish(ctx context.Context, events ...DomainEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, events...)
	return nil
}

// Events returns a defensive copy of every event published so far, in
// publish order. Mutating the returned slice never affects the
// publisher's internal state.
func (p *MemoryPublisher) Events() []DomainEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]DomainEvent, len(p.events))
	copy(out, p.events)
	return out
}
