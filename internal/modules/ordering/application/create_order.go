package application

import (
	"context"
	"time"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CreateOrderItemInput is one line of a customer's cart, exactly as
// decoded off the public POST .../pedidos request body -- see
// domain.RequestedLine's own doc comment for why price is deliberately
// absent here: it is NEVER accepted from the client.
type CreateOrderItemInput struct {
	ProductID     string
	QuantityCount int64
}

// CreateOrderInput carries everything CreateOrder needs to create a
// brand-new, PENDING-confirmation Order from the public cardápio flow.
type CreateOrderInput struct {
	TenantID      tenant.TenantID
	SessionToken  string
	Items         []CreateOrderItemInput
	CustomerName  *string
	CustomerPhone *string
}

// sessionInvalidErr is the single error every failure to validate
// input.SessionToken is reported as -- an unknown, expired, or missing
// token are all indistinguishable to the caller.
func sessionInvalidErr(cause error) error {
	return errs.New(errs.Unauthenticated, "invalid_table_session",
		"table session is missing, invalid, or expired", cause)
}

// tableUnavailableForOrderErr is returned when a session's own table has
// since been deactivated (a narrow race: the table was active when the
// session was minted, but a staff member deactivated it before this order
// was submitted).
func tableUnavailableForOrderErr(cause error) error {
	return errs.New(errs.Conflict, "table_unavailable",
		"table is no longer available to receive orders", cause)
}

// CreateOrder is the application-layer use case backing the public
// POST .../pedidos route. It:
//  1. resolves every DISTINCT product referenced by input.Items to its
//     CURRENT price via priceReader -- BEFORE opening the UnitOfWork.Do
//     transaction below, and NEVER from input itself (see
//     resolvePriceSnapshot's own doc comment);
//  2. within a single UnitOfWork.Do call: validates input.SessionToken
//     against TableSessions().FindByTokenHash, confirms the table it
//     points at still exists and is active, prices every line via
//     domain.PriceOrderLines, builds the Order via domain.NewOrder (always
//     born OrderStatusPendenteConfirmacao -- see that constructor's own
//     doc comment for why), and persists it.
//
// Every failure here -- an unknown/expired session token, a deactivated
// table, an unpriceable item -- rolls back the whole Do call: nothing is
// ever partially persisted.
func CreateOrder(ctx context.Context, uow domain.UnitOfWork, priceReader catalogcontracts.ProductPriceReader, input CreateOrderInput) (domain.Order, error) {
	if input.SessionToken == "" {
		return domain.Order{}, sessionInvalidErr(nil)
	}

	snapshot, err := resolvePriceSnapshot(ctx, priceReader, input.Items)
	if err != nil {
		return domain.Order{}, err
	}

	tokenHash := domain.HashSessionToken(input.SessionToken)

	var order domain.Order
	err = uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		session, err := r.TableSessions().FindByTokenHash(ctx, tokenHash)
		if err != nil {
			return sessionInvalidErr(err)
		}
		if !session.IsValid(time.Now().UTC()) {
			return sessionInvalidErr(nil)
		}

		table, err := r.Tables().FindByID(ctx, session.TableID())
		if err != nil {
			return tableUnavailableForOrderErr(err)
		}
		if !table.IsActive() {
			return tableUnavailableForOrderErr(nil)
		}

		items, err := domain.PriceOrderLines(toRequestedLines(input.Items), snapshot)
		if err != nil {
			return err
		}

		newOrder, err := domain.NewOrder(domain.NewID(), input.TenantID, table.ID(), input.CustomerName, input.CustomerPhone, items)
		if err != nil {
			return err
		}
		if err := r.Orders().Insert(ctx, &newOrder); err != nil {
			return err
		}
		order = newOrder
		return nil
	})
	if err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

// toRequestedLines maps input-level cart lines onto domain.RequestedLine.
func toRequestedLines(items []CreateOrderItemInput) []domain.RequestedLine {
	lines := make([]domain.RequestedLine, len(items))
	for i, item := range items {
		lines[i] = domain.RequestedLine{ProductID: item.ProductID, QuantityCount: item.QuantityCount}
	}
	return lines
}

// resolvePriceSnapshot resolves the CURRENT price of every DISTINCT
// product referenced by items via priceReader, building the
// map[string]domain.PriceSnapshot domain.PriceOrderLines needs. A product
// priceReader reports as errs.NotFound is deliberately left OUT of the
// returned snapshot (rather than propagated as an error here) so that
// domain.PriceOrderLines reports it uniformly, alongside every other
// unpriceable-item reason, as errs.Invalid -- see that function's own doc
// comment. Any OTHER error from priceReader (a malformed product id, an
// internal failure) propagates immediately, unchanged.
func resolvePriceSnapshot(ctx context.Context, priceReader catalogcontracts.ProductPriceReader, items []CreateOrderItemInput) (map[string]domain.PriceSnapshot, error) {
	snapshot := make(map[string]domain.PriceSnapshot, len(items))
	for _, item := range items {
		if _, already := snapshot[item.ProductID]; already {
			continue
		}
		price, err := priceReader.GetProductPrice(ctx, item.ProductID)
		if err != nil {
			if errs.IsKind(err, errs.NotFound) {
				continue
			}
			return nil, err
		}
		snapshot[item.ProductID] = domain.PriceSnapshot{PriceCents: price.PriceCents, Active: price.IsActive}
	}
	return snapshot, nil
}
