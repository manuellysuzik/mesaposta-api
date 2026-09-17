//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func strPtr(s string) *string { return &s }

func TestPool_CreateTenant_Succeeds_StartsTrialAndActive(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	got, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID:         "44444444-4444-4444-4444-444444444444",
		Slug:       "acme-burger",
		Name:       "Acme Burger",
		OwnerName:  strPtr("Jane Doe"),
		OwnerEmail: strPtr("jane@example.com"),
		Phone:      strPtr("+55 11 90000-0000"),
		Plan:       "basic",
	})
	if err != nil {
		t.Fatalf("CreateTenant: unexpected error: %v", err)
	}

	if got.Status != "trial" {
		t.Errorf("got.Status = %q, want %q", got.Status, "trial")
	}
	if !got.IsActive {
		t.Error("got.IsActive = false, want true for a newly created tenant")
	}
	if got.Slug != "acme-burger" {
		t.Errorf("got.Slug = %q, want %q", got.Slug, "acme-burger")
	}
}

func TestPool_CreateTenant_DuplicateSlug_ReturnsConflict(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	input := CreateTenantInput{
		ID:   "55555555-5555-5555-5555-555555555555",
		Slug: "duplicate-slug",
		Name: "First",
		Plan: "basic",
	}
	if _, err := pool.CreateTenant(ctx, input); err != nil {
		t.Fatalf("CreateTenant (first): unexpected error: %v", err)
	}

	input.ID = "66666666-6666-6666-6666-666666666666"
	input.Name = "Second"
	_, err = pool.CreateTenant(ctx, input)
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(err) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestPool_ListTenants_ReturnsNewestFirst(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if _, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID: "77777777-7777-7777-7777-777777777777", Slug: "first-created", Name: "First", Plan: "basic",
	}); err != nil {
		t.Fatalf("CreateTenant (first): unexpected error: %v", err)
	}
	if _, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID: "88888888-8888-8888-8888-888888888888", Slug: "second-created", Name: "Second", Plan: "pro",
	}); err != nil {
		t.Fatalf("CreateTenant (second): unexpected error: %v", err)
	}

	got, err := pool.ListTenants(ctx)
	if err != nil {
		t.Fatalf("ListTenants: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Slug != "second-created" || got[1].Slug != "first-created" {
		t.Errorf("got = [%q, %q], want newest-first order [second-created, first-created]", got[0].Slug, got[1].Slug)
	}
}

func TestPool_UpdateTenantDetails_UpdatesFieldsAndLeavesSlugUnchanged(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	created, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID: "99999999-9999-9999-9999-999999999999", Slug: "original-slug", Name: "Original Name", Plan: "basic",
	})
	if err != nil {
		t.Fatalf("CreateTenant: unexpected error: %v", err)
	}

	updated, err := pool.UpdateTenantDetails(ctx, created.ID, UpdateTenantDetailsInput{
		Name:       "Updated Name",
		OwnerName:  strPtr("New Owner"),
		OwnerEmail: strPtr("new-owner@example.com"),
		Phone:      strPtr("+55 11 91111-1111"),
		Plan:       "enterprise",
	})
	if err != nil {
		t.Fatalf("UpdateTenantDetails: unexpected error: %v", err)
	}

	if updated.Slug != "original-slug" {
		t.Errorf("updated.Slug = %q, want unchanged %q", updated.Slug, "original-slug")
	}
	if updated.Name != "Updated Name" {
		t.Errorf("updated.Name = %q, want %q", updated.Name, "Updated Name")
	}
	if updated.Plan != "enterprise" {
		t.Errorf("updated.Plan = %q, want %q", updated.Plan, "enterprise")
	}
}

func TestPool_UpdateTenantDetails_UnknownID_ReturnsNotFound(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	_, err = pool.UpdateTenantDetails(ctx, "00000000-0000-0000-0000-000000000001", UpdateTenantDetailsInput{
		Name: "Doesn't Matter", Plan: "basic",
	})
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestPool_SetTenantStatus_Suspended_SetsIsActiveFalse(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	created, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID: "00000000-0000-0000-0000-000000000002", Slug: "to-suspend", Name: "To Suspend", Plan: "basic",
	})
	if err != nil {
		t.Fatalf("CreateTenant: unexpected error: %v", err)
	}

	got, err := pool.SetTenantStatus(ctx, created.ID, "suspended")
	if err != nil {
		t.Fatalf("SetTenantStatus: unexpected error: %v", err)
	}
	if got.Status != "suspended" {
		t.Errorf("got.Status = %q, want %q", got.Status, "suspended")
	}
	if got.IsActive {
		t.Error("got.IsActive = true, want false for a suspended tenant")
	}
}

func TestPool_SetTenantStatus_Active_SetsIsActiveTrue(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	created, err := pool.CreateTenant(ctx, CreateTenantInput{
		ID: "00000000-0000-0000-0000-000000000003", Slug: "to-reactivate", Name: "To Reactivate", Plan: "basic",
	})
	if err != nil {
		t.Fatalf("CreateTenant: unexpected error: %v", err)
	}
	if _, err := pool.SetTenantStatus(ctx, created.ID, "suspended"); err != nil {
		t.Fatalf("SetTenantStatus (suspend): unexpected error: %v", err)
	}

	got, err := pool.SetTenantStatus(ctx, created.ID, "active")
	if err != nil {
		t.Fatalf("SetTenantStatus (reactivate): unexpected error: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("got.Status = %q, want %q", got.Status, "active")
	}
	if !got.IsActive {
		t.Error("got.IsActive = false, want true after reactivating")
	}
}

func TestPool_SetTenantStatus_UnknownID_ReturnsNotFound(t *testing.T) {
	_, appURL := TestPostgres(t)

	ctx := context.Background()
	pool, err := Open(ctx, appURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	_, err = pool.SetTenantStatus(ctx, "00000000-0000-0000-0000-000000000004", "active")
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(err) = %v, want errs.NotFound", errs.KindOf(err))
	}
}
