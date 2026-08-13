package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
)

// MountTables registers /mesas routes (table management: list, get,
// create, activate, deactivate) under r. Papel gestor only -- see this
// package's own doc comment for why that role restriction is applied by
// the CALLER (internal/platform/app.NewApp), not here: pass
// r.With(auth.RequireRole(token.RoleGestor)) as r.
func MountTables(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/mesas", func(r chi.Router) {
		r.Get("/", listTablesHandler(uow))
		r.Post("/", createTableHandler(uow))
		r.Get("/{id}", getTableHandler(uow))
		r.Post("/{id}/ativar", activateTableHandler(uow))
		r.Post("/{id}/desativar", deactivateTableHandler(uow))
	})
}

// MountOrders registers /pedidos routes (list, get, confirm, and advance
// through the kitchen-status lifecycle) under r. Papel gestor + funcionario
// -- see MountTables' own doc comment for how the caller applies the
// right role.
func MountOrders(r chi.Router, uow domain.UnitOfWork) {
	r.Route("/pedidos", func(r chi.Router) {
		r.Get("/", listOrdersHandler(uow))
		r.Get("/{id}", getOrderHandler(uow))
		r.Post("/{id}/confirmar", confirmOrderHandler(uow))
		r.Post("/{id}/preparar", startPreparingOrderHandler(uow))
		r.Post("/{id}/pronto", markOrderReadyHandler(uow))
		r.Post("/{id}/entregar", markOrderDeliveredHandler(uow))
		r.Post("/{id}/cancelar", cancelOrderHandler(uow))
	})
}
