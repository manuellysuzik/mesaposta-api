package contracts

import "context"

// MenuItem is the primitive-only snapshot of a catalog Product exposed for
// listing an active menu to a module outside catalog -- today, exclusively
// internal/modules/ordering's public digital-menu route
// (GET /public/v1/{tenantSlug}/cardapio/mesa/{code}). See this package's
// own doc comment (doc.go) on why contracts/ is stdlib-only, and
// pricing.go's own doc comment for the identical rationale applied to
// pricing instead of listing.
type MenuItem struct {
	ProductID   string
	Name        string
	Description string
	PriceCents  int64
}

// MenuReader is the port a module outside catalog implements against to
// list every active product (in the catalog's current, real-time state)
// without depending on catalog/domain's types. The concrete implementation
// lives in catalog/infrastructure/pricing (see menu.go there), built when
// a consuming module (ordering) needs it.
type MenuReader interface {
	// ListActiveMenu returns every active Product visible to the caller's
	// tenant, as MenuItem, ordered ascending by name.
	ListActiveMenu(ctx context.Context) ([]MenuItem, error)
}
