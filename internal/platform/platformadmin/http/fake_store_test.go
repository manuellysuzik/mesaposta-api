package http_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// fakeTenantAdminStore is a small, hand-written, in-memory implementation
// of this package's tenantAdminStore port for httptest-based handler
// tests -- mirrors the fake-implements-the-port pattern
// ../../../modules/identity/infrastructure/http's own
// fakeCredentialsLookup establishes.
type fakeTenantAdminStore struct {
	mu      sync.Mutex
	tenants map[string]db.PlatformTenant
}

func newFakeTenantAdminStore() *fakeTenantAdminStore {
	return &fakeTenantAdminStore{tenants: map[string]db.PlatformTenant{}}
}

func (f *fakeTenantAdminStore) CreateTenant(_ context.Context, input db.CreateTenantInput) (db.PlatformTenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, t := range f.tenants {
		if t.Slug == input.Slug {
			return db.PlatformTenant{}, errs.New(errs.Conflict, "tenant_slug_taken", "a tenant with this slug already exists", nil)
		}
	}

	t := db.PlatformTenant{
		ID:         input.ID,
		Slug:       input.Slug,
		Name:       input.Name,
		OwnerName:  input.OwnerName,
		OwnerEmail: input.OwnerEmail,
		Phone:      input.Phone,
		Plan:       input.Plan,
		Status:     "trial",
		IsActive:   true,
		CreatedAt:  time.Now().UTC(),
	}
	f.tenants[t.ID] = t
	return t, nil
}

func (f *fakeTenantAdminStore) ListTenants(_ context.Context) ([]db.PlatformTenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]db.PlatformTenant, 0, len(f.tenants))
	for _, t := range f.tenants {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func tenantNotFoundErr() error {
	return errs.New(errs.NotFound, "tenant_not_found", "tenant not found", nil)
}

func (f *fakeTenantAdminStore) UpdateTenantDetails(_ context.Context, id string, input db.UpdateTenantDetailsInput) (db.PlatformTenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, ok := f.tenants[id]
	if !ok {
		return db.PlatformTenant{}, tenantNotFoundErr()
	}
	t.Name = input.Name
	t.OwnerName = input.OwnerName
	t.OwnerEmail = input.OwnerEmail
	t.Phone = input.Phone
	t.Plan = input.Plan
	f.tenants[id] = t
	return t, nil
}

func (f *fakeTenantAdminStore) SetTenantStatus(_ context.Context, id, status string) (db.PlatformTenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, ok := f.tenants[id]
	if !ok {
		return db.PlatformTenant{}, tenantNotFoundErr()
	}
	t.Status = status
	t.IsActive = status == "trial" || status == "active"
	f.tenants[id] = t
	return t, nil
}
