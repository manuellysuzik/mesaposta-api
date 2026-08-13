package domain

import (
	"fmt"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// OrderStatus identifies the lifecycle state of an Order, following the
// same closed-set pattern every other enum in this codebase already
// establishes (e.g. internal/modules/operations/domain.PurchaseOrderStatus):
// a string-based type whose zero value ("") is deliberately invalid, a
// Valid method reporting membership in the known set, and a
// ParseOrderStatus constructor that rejects any unknown (including empty)
// value.
//
// See Order's own doc comment (order.go) for the exact state machine these
// values form.
type OrderStatus string

const (
	// OrderStatusPendenteConfirmacao is an order's INITIAL state, for every
	// order born from the public, unauthenticated cardápio flow: it has
	// been submitted by a diner but has not yet been reviewed/accepted by
	// staff, and must NOT be treated as "in the kitchen queue" yet -- see
	// this module's product decision, documented on NewOrder.
	OrderStatusPendenteConfirmacao OrderStatus = "pendente_confirmacao"
	// OrderStatusConfirmado is an order a staff member has reviewed and
	// accepted (see application.ConfirmOrder), but preparation has not yet
	// started.
	OrderStatusConfirmado OrderStatus = "confirmado"
	// OrderStatusEmPreparo is an order currently being prepared in the
	// kitchen.
	OrderStatusEmPreparo OrderStatus = "em_preparo"
	// OrderStatusPronto is an order that is ready, awaiting delivery to the
	// table.
	OrderStatusPronto OrderStatus = "pronto"
	// OrderStatusEntregue is an order that has been delivered -- a
	// TERMINAL state: no further transition (including Cancel) is ever
	// valid from here.
	OrderStatusEntregue OrderStatus = "entregue"
	// OrderStatusCancelado is an order called off before delivery -- the
	// OTHER terminal state, reachable from every non-terminal status.
	OrderStatusCancelado OrderStatus = "cancelado"
)

// Valid reports whether s is one of the closed set of known order statuses.
func (s OrderStatus) Valid() bool {
	switch s {
	case OrderStatusPendenteConfirmacao, OrderStatusConfirmado, OrderStatusEmPreparo,
		OrderStatusPronto, OrderStatusEntregue, OrderStatusCancelado:
		return true
	default:
		return false
	}
}

// ParseOrderStatus validates s against the closed set of known order
// statuses.
func ParseOrderStatus(s string) (OrderStatus, error) {
	status := OrderStatus(s)
	if !status.Valid() {
		return "", errs.New(errs.Invalid, "invalid_order_status", "unknown order status",
			fmt.Errorf("order status %q is not one of the known values", s))
	}
	return status, nil
}
