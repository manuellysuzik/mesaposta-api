package tenant_test

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

func TestWithFrom_RoundTrip(t *testing.T) {
	// Arrange
	id, err := tenant.Parse(validUUID)
	if err != nil {
		t.Fatalf("Parse(%q): %v", validUUID, err)
	}
	ctx := tenant.With(context.Background(), id)

	// Act
	got, err := tenant.From(ctx)

	// Assert
	if err != nil {
		t.Fatalf("From(ctx): unexpected error: %v", err)
	}
	if got != id {
		t.Errorf("From(ctx) = %v, want %v", got, id)
	}
}

func TestFrom_NoTenantInContext_ReturnsUnauthenticatedError(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	got, err := tenant.From(ctx)

	// Assert
	if err == nil {
		t.Fatal("From(context.Background()) returned nil error, want an error")
	}
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("From(context.Background()) error kind = %v, want %v", errs.KindOf(err), errs.Unauthenticated)
	}
	if !got.IsZero() {
		t.Errorf("From(context.Background()) returned non-zero TenantID %v, want zero value", got)
	}
}

// foreignContextKey stands in for "any key type an external caller could
// possibly define" when trying to forge a tenant context entry. It is
// deliberately declared here, in this external tenant_test package, and is
// a completely different type from tenant's own unexported context-key
// type -- which is exactly the point of the test below.
type foreignContextKey struct{}

// TestContextKey_CannotBeForgedFromOutside is the load-bearing test proving
// the context key used by With/From is not accessible outside this package:
// this file lives in package tenant_test (an external test package), so it
// has no access to any unexported identifier of package tenant, including
// its context-key type. The only way an external caller can attempt to
// smuggle a value under "the same key" is to define and use its own key
// type (foreignContextKey, above) -- which, because Go context keys compare
// by (dynamic type, value), can never collide with tenant's own unexported
// key type. From must therefore still report "no tenant" even though a
// value was stored under a same-shaped but distinct key type.
func TestContextKey_CannotBeForgedFromOutside(t *testing.T) {
	// Arrange
	id, err := tenant.Parse(validUUID)
	if err != nil {
		t.Fatalf("Parse(%q): %v", validUUID, err)
	}
	// Attempt to forge the tenant context entry using a key type declared
	// outside package tenant. This can never be the same type as tenant's
	// own unexported context-key type, so it must not satisfy From.
	forgedCtx := context.WithValue(context.Background(), foreignContextKey{}, id)

	// Act
	got, err := tenant.From(forgedCtx)

	// Assert
	if err == nil {
		t.Fatal("From(forgedCtx) returned nil error, want an error -- the context key must not be forgeable from outside the package")
	}
	if !errs.IsKind(err, errs.Unauthenticated) {
		t.Errorf("From(forgedCtx) error kind = %v, want %v", errs.KindOf(err), errs.Unauthenticated)
	}
	if !got.IsZero() {
		t.Errorf("From(forgedCtx) returned non-zero TenantID %v, want zero value", got)
	}
}

func TestWith_ReturnsChildContext_ParentUnaffected(t *testing.T) {
	// Arrange
	id, err := tenant.Parse(validUUID)
	if err != nil {
		t.Fatalf("Parse(%q): %v", validUUID, err)
	}
	parent := context.Background()

	// Act
	child := tenant.With(parent, id)

	// Assert -- the parent context must remain untouched (context.Context
	// values are immutable by design; this guards against a future
	// implementation regressing into some mutable global/shared state).
	if _, err := tenant.From(parent); err == nil {
		t.Error("From(parent) succeeded after With(parent, id) -- parent context must not be mutated")
	}
	if _, err := tenant.From(child); err != nil {
		t.Errorf("From(child) failed: %v", err)
	}
}
