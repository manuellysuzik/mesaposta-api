package postgres

import (
	"reflect"
	"unsafe"
)

// setUnexportedField sets structPtr's field named fieldName -- which must
// be an UNEXPORTED field of the struct structPtr points at -- to value,
// bypassing Go's usual encapsulation via reflect+unsafe.
//
// Why this exists: this module's aggregates (Table, Order) expose only
// unexported fields, a "fresh, Version 1" constructor, and a small set of
// incremental mutation methods -- by design, so that no code outside
// domain/ can ever construct an aggregate in an invalid, half-initialized,
// or backdated state. That is exactly the right shape for every OTHER
// caller of this module, but it leaves this package -- whose entire job is
// reconstructing an aggregate's EXACT previously-persisted state (its real
// Version, its real Status/IsActive, its real CreatedAt/UpdatedAt) from a
// database row -- with no public constructor to call.
//
// This is byte-for-byte the same technique every other module's own
// infrastructure/postgres/hydrate.go already establishes (e.g.
// internal/modules/crm/infrastructure/postgres/hydrate.go) -- duplicated
// (not imported) here because architecture rule R3 forbids one module's
// infrastructure/ from importing another module's internals outside
// <module>/contracts, and this helper is not a contracts-worthy
// cross-module concern, just a small, self-contained implementation
// detail each module's own postgres adapter needs.
func setUnexportedField(structPtr any, fieldName string, value any) {
	field := reflect.ValueOf(structPtr).Elem().FieldByName(fieldName)
	writable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	writable.Set(reflect.ValueOf(value))
}
