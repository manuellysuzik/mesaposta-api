package postgres

import (
	"reflect"
	"unsafe"
)

// setUnexportedField sets structPtr's field named fieldName -- which must
// be an UNEXPORTED field of the struct structPtr points at -- to value,
// bypassing Go's usual encapsulation via reflect+unsafe.
//
// Why this exists: every aggregate in ../../domain (StockItem,
// PurchaseOrder, ...) exposes only unexported fields, a "fresh, Version 1"
// constructor (NewStockItem, NewPurchaseOrder), and a small set of
// incremental mutation methods (Receive, Withdraw, MarkAsReceived, Cancel)
// -- by design, so that no code outside domain/ can ever construct an
// aggregate in an invalid, half-initialized, or backdated state. That is
// exactly the right shape for every OTHER caller of this module, but it
// leaves this package -- whose entire job is reconstructing an aggregate's
// EXACT previously-persisted state (its real Version, its real
// UpdatedAt/CreatedAt, its real Status/ReceivedAt) from a database row --
// with no public constructor to call: NewStockItem always starts a fresh
// aggregate at Version 1 with UpdatedAt set to time.Now(), and there is no
// "rehydrate with an arbitrary, already-persisted version/timestamp"
// counterpart, nor should ../../domain grow one just to serve this single
// infrastructure-layer concern (see ../../domain/ports.go and
// stock_item.go's own doc comments on why that surface is deliberately
// narrow).
//
// setUnexportedField is the pragmatic way to bridge that gap without
// touching a single line of domain/: build the aggregate through its real
// public constructor first (so every constructor-time validation still
// runs against the row's data), then overwrite the handful of fields the
// constructor could not have known about (Version, UpdatedAt, Status,
// CreatedAt, ReceivedAt) with the values actually read back from Postgres.
// See stock_item_repository.go's hydrateStockItem and
// purchase_order_repository.go's hydratePurchaseOrder for the two call
// sites.
//
// This relies on the standard (if officially "unsupported") reflect+unsafe
// technique for writing to an unexported struct field: reflect.Value's
// normal CanSet() is false for a field obtained via FieldByName on an
// unexported field, but re-wrapping its address via reflect.NewAt drops
// that read-only flag. It requires structPtr to be a pointer to an
// addressable struct value (never a copy sitting in an interface, and
// never nil) and fieldName to name a field that actually exists on that
// struct -- both are met unconditionally by every call site in this
// package, so a mismatch here is a programmer error, not a data error.
func setUnexportedField(structPtr any, fieldName string, value any) {
	field := reflect.ValueOf(structPtr).Elem().FieldByName(fieldName)
	writable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	writable.Set(reflect.ValueOf(value))
}
