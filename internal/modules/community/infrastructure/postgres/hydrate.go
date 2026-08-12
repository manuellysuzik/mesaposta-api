package postgres

import (
	"reflect"
	"unsafe"
)

// setUnexportedField sets structPtr's field named fieldName -- which must
// be an UNEXPORTED field of the struct structPtr points at -- to value,
// bypassing Go's usual encapsulation via reflect+unsafe.
//
// Why this exists: CommunityPost (../../domain/community_post.go) exposes
// only unexported fields and a "fresh" constructor (NewCommunityPost) that
// always stamps CreatedAt as time.Now() -- by design, so that no code
// outside domain/ can ever construct the aggregate with an arbitrary,
// backdated CreatedAt. That is exactly the right shape for every OTHER
// caller of this module, but it leaves this package -- whose entire job is
// reconstructing a CommunityPost's EXACT previously-persisted CreatedAt
// from a database row -- with no public constructor to call.
//
// This is byte-for-byte the same technique
// internal/modules/operations/infrastructure/postgres/hydrate.go and
// internal/modules/crm/infrastructure/postgres/hydrate.go already
// establish for those modules' own aggregates -- duplicated (not imported)
// here because architecture rule R3 forbids one module's infrastructure/
// from importing another module's internals outside <module>/contracts,
// and this helper is not a contracts-worthy cross-module concern, just a
// small, self-contained implementation detail each module's own postgres
// adapter needs.
func setUnexportedField(structPtr any, fieldName string, value any) {
	field := reflect.ValueOf(structPtr).Elem().FieldByName(fieldName)
	writable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	writable.Set(reflect.ValueOf(value))
}
