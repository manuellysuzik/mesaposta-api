package postgres

import (
	"reflect"
	"unsafe"
)

// setUnexportedField sets structPtr's field named fieldName -- which must
// be an UNEXPORTED field of the struct structPtr points at -- to value,
// bypassing Go's usual encapsulation via reflect+unsafe.
//
// Why this exists: Customer (../../domain/customer.go) exposes only
// unexported fields, a "fresh, Version 1" constructor (NewCustomer), and a
// small set of incremental mutation methods (CreditPoints, RedeemPoints) --
// by design, so that no code outside domain/ can ever construct the
// aggregate in an invalid, half-initialized, or backdated state. That is
// exactly the right shape for every OTHER caller of this module, but it
// leaves this package -- whose entire job is reconstructing a Customer's
// EXACT previously-persisted state (its real Version, its real
// LoyaltyPoints, its real CreatedAt/UpdatedAt) from a database row -- with
// no public constructor to call: NewCustomer always starts a fresh
// aggregate at Version 1 with a zero loyalty point balance, and there is no
// "rehydrate with an arbitrary, already-persisted balance/version"
// counterpart, nor should ../../domain grow one just to serve this single
// infrastructure-layer concern.
//
// This is byte-for-byte the same technique
// internal/modules/operations/infrastructure/postgres/hydrate.go already
// establishes for that module's own aggregates (StockItem, PurchaseOrder)
// -- duplicated (not imported) here because architecture rule R3 forbids
// one module's infrastructure/ from importing another module's internals
// outside <module>/contracts, and this helper is not a contracts-worthy
// cross-module concern, just a small, self-contained implementation detail
// each module's own postgres adapter needs.
func setUnexportedField(structPtr any, fieldName string, value any) {
	field := reflect.ValueOf(structPtr).Elem().FieldByName(fieldName)
	writable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	writable.Set(reflect.ValueOf(value))
}
