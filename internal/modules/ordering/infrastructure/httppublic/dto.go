package httppublic

import (
	"time"

	catalogcontracts "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/contracts"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/application"
)

// tableSessionHeader is the HTTP header POST .../pedidos reads the table
// session token from. A header (rather than a body field) was chosen
// deliberately: it keeps the request body focused purely on cart contents
// (items/nome/telefone), and mirrors how every other credential in this
// codebase (the Authorization Bearer token, see internal/platform/auth)
// already travels as a header rather than a body field.
const tableSessionHeader = "X-Table-Session-Token"

// menuItemResponse is the public, minimal JSON representation of a single
// catalog product on the digital menu -- deliberately NOT reusing
// catalogcontracts.MenuItem's field names 1:1 as a promise: this is its
// own, independently-defined response shape (see this package's own doc
// comment).
type menuItemResponse struct {
	ProductID   string `json:"produto_id"`
	Name        string `json:"nome"`
	Description string `json:"descricao"`
	PriceCents  int64  `json:"preco_centavos"`
}

func newMenuItemResponse(item catalogcontracts.MenuItem) menuItemResponse {
	return menuItemResponse{
		ProductID:   item.ProductID,
		Name:        item.Name,
		Description: item.Description,
		PriceCents:  item.PriceCents,
	}
}

// cardapioTableResponse is the minimal, PUBLIC-safe view of a Table: no
// version, no tenant_id, no created_at/updated_at -- a diner has no
// legitimate use for any of those.
type cardapioTableResponse struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// cardapioResponse is the response body for
// GET /public/v1/{tenantSlug}/cardapio/mesa/{code}.
type cardapioResponse struct {
	Table        cardapioTableResponse `json:"mesa"`
	SessionToken string                `json:"session_token"`
	ExpiresAt    time.Time             `json:"expira_em"`
	Items        []menuItemResponse    `json:"itens"`
}

func newCardapioResponse(result application.ResolveTableForMenuResult, menu []catalogcontracts.MenuItem) cardapioResponse {
	items := make([]menuItemResponse, len(menu))
	for i, item := range menu {
		items[i] = newMenuItemResponse(item)
	}
	return cardapioResponse{
		Table: cardapioTableResponse{
			Code:  result.Table.Code(),
			Label: result.Table.Label(),
		},
		SessionToken: result.SessionToken,
		ExpiresAt:    result.Session.ExpiresAt(),
		Items:        items,
	}
}

// createPublicOrderItemRequest is one line of a diner's cart. Deliberately
// has NO price field at all -- see
// internal/modules/ordering/application.CreateOrderItemInput's own doc
// comment: price is never accepted from the client, so it is not even
// representable in this request shape.
type createPublicOrderItemRequest struct {
	ProductID string `json:"produto_id"`
	Quantity  int64  `json:"quantidade"`
}

// createPublicOrderRequest is the request payload for
// POST /public/v1/{tenantSlug}/pedidos.
type createPublicOrderRequest struct {
	Items []createPublicOrderItemRequest `json:"items"`
	Name  *string                        `json:"nome"`
	Phone *string                        `json:"telefone"`
}

// createPublicOrderResponse is the MINIMAL response for a successfully
// created public order: just enough for the client to track its own
// order, never any management-only detail (customer contact info echoed
// back, internal version, ...).
type createPublicOrderResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
