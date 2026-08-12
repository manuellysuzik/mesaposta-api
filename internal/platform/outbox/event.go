// Package outbox defines the shared-kernel "outbox" port: the DomainEvent
// value type and the DomainEventPublisher interface any layer (including
// internal/domain/**, per the R1 platform allowlist in
// architecture/rules.go) may depend on to record domain events, without
// coupling to a concrete broker or persistence mechanism. Concrete
// infrastructure (a SQL-backed outbox table, a message-broker publisher,
// ...) is intentionally out of scope here -- that lives in later,
// infrastructure-layer blocks. This package only ships the port
// (DomainEventPublisher) and a MemoryPublisher good enough for tests.
package outbox

import (
	"fmt"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// DomainEvent represents a single fact that happened to an aggregate,
// destined to be recorded in the outbox and eventually published. Every
// DomainEvent is scoped to exactly one tenant.
//
// DomainEvent has no exported constructor other than NewDomainEvent: all
// fields are exported (so callers can read them freely once constructed),
// but the zero value is never valid on its own -- always go through
// NewDomainEvent, which validates eagerly at construction time.
type DomainEvent struct {
	// TenantID is the tenant this event belongs to. Must not be the zero
	// value.
	TenantID tenant.TenantID
	// Name is the event's stable, dot-namespaced identifier (e.g.
	// "bet.placed"). Must not be empty.
	Name string
	// AggregateID identifies the aggregate instance the event happened to.
	// Must not be empty.
	AggregateID string
	// OccurredAt is when the event happened (domain time, not necessarily
	// when it was published). Must not be the zero time.Time.
	OccurredAt time.Time
	// Payload carries the event's domain-specific data. It is intentionally
	// typed as `any`: this package has no opinion on serialization, which
	// is an infrastructure-layer concern (Bloco E).
	Payload any
}

// NewDomainEvent constructs a DomainEvent, validating every required field
// at construction time -- not deferring validation to whenever the event
// is later used or published. On any validation failure it returns the
// zero DomainEvent and an *errs.Error with Kind errs.Invalid.
func NewDomainEvent(tenantID tenant.TenantID, name, aggregateID string, occurredAt time.Time, payload any) (DomainEvent, error) {
	if tenantID.IsZero() {
		return DomainEvent{}, errs.New(errs.Invalid, "invalid_domain_event",
			"domain event requires a non-zero tenant id", fmt.Errorf("tenant id is zero-value"))
	}
	if name == "" {
		return DomainEvent{}, errs.New(errs.Invalid, "invalid_domain_event",
			"domain event requires a non-empty name", fmt.Errorf("name is empty"))
	}
	if aggregateID == "" {
		return DomainEvent{}, errs.New(errs.Invalid, "invalid_domain_event",
			"domain event requires a non-empty aggregate id", fmt.Errorf("aggregate id is empty"))
	}
	if occurredAt.IsZero() {
		return DomainEvent{}, errs.New(errs.Invalid, "invalid_domain_event",
			"domain event requires a non-zero occurred-at timestamp", fmt.Errorf("occurred at is zero-value"))
	}

	return DomainEvent{
		TenantID:    tenantID,
		Name:        name,
		AggregateID: aggregateID,
		OccurredAt:  occurredAt,
		Payload:     payload,
	}, nil
}
