package tenant

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// contextKey is an unexported type used exclusively as the context.Context
// key under which the request's TenantID is stored. Because it is
// unexported, no package outside internal/platform/tenant can construct a
// value of this exact type -- context.WithValue keys are compared by
// (dynamic type, value), so nothing outside this package can collide with
// or forge the key From reads.
type contextKey struct{}

// tenantContextKey is the single key instance used by With and From.
var tenantContextKey = contextKey{}

// With returns a copy of ctx carrying id, retrievable via From. It does not
// mutate ctx (context.Context values are immutable).
func With(ctx context.Context, id TenantID) context.Context {
	return context.WithValue(ctx, tenantContextKey, id)
}

// From extracts the TenantID previously attached to ctx via With. If ctx
// carries no tenant (e.g. it was never passed through With, or the request
// never went through the authentication middleware that calls With), From
// returns the zero TenantID and an *errs.Error with Kind
// errs.Unauthenticated.
func From(ctx context.Context) (TenantID, error) {
	id, ok := ctx.Value(tenantContextKey).(TenantID)
	if !ok {
		return TenantID{}, errs.New(errs.Unauthenticated, "tenant_missing",
			"request has no authenticated tenant", nil)
	}
	return id, nil
}
