package contracts

import "time"

// Event name constants for the crm module. Every name is dot-namespaced as
// "<aggregate_type>.<event>", with exactly one "." --
// internal/platform/outbox/postgres/postgres_publisher.go's aggregateTypeOf
// derives the domain_events.aggregate_type column from the prefix up to
// the first ".", so the prefix below must always match the aggregate the
// event describes (see events_test.go for the regression test pinning this
// down). Both events below describe a change to the Customer aggregate
// (specifically, its nested loyalty ledger -- see
// ../domain/customer.go's own doc comment on that aggregate boundary), so
// both are prefixed "customer.".
const (
	// EventLoyaltyPointsCredited is published whenever loyalty points are
	// credited to a customer's balance. Payload:
	// LoyaltyPointsCreditedPayload.
	EventLoyaltyPointsCredited = "customer.loyalty_points_credited"

	// EventLoyaltyPointsRedeemed is published whenever loyalty points are
	// redeemed from a customer's balance. Payload:
	// LoyaltyPointsRedeemedPayload.
	EventLoyaltyPointsRedeemed = "customer.loyalty_points_redeemed"
)

// LoyaltyPointsCreditedPayload is the payload for EventLoyaltyPointsCredited.
// Points is always positive (the amount credited); BalanceAfter is the
// customer's total loyalty point balance immediately after this credit was
// applied.
type LoyaltyPointsCreditedPayload struct {
	CustomerID   string    `json:"customer_id"`
	EntryID      string    `json:"entry_id"`
	Points       int64     `json:"points"`
	BalanceAfter int64     `json:"balance_after"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// LoyaltyPointsRedeemedPayload is the payload for EventLoyaltyPointsRedeemed.
// Points is always positive (the amount redeemed); BalanceAfter is the
// customer's total loyalty point balance immediately after this redemption
// was applied.
type LoyaltyPointsRedeemedPayload struct {
	CustomerID   string    `json:"customer_id"`
	EntryID      string    `json:"entry_id"`
	Points       int64     `json:"points"`
	BalanceAfter int64     `json:"balance_after"`
	OccurredAt   time.Time `json:"occurred_at"`
}
