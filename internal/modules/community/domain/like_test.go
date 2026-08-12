package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewLike_Valid(t *testing.T) {
	tenantID := mustTestTenantID(t)
	postID := domain.NewID()
	userID := domain.NewID()

	like, err := domain.NewLike(tenantID, postID, userID)
	if err != nil {
		t.Fatalf("NewLike: unexpected error: %v", err)
	}
	if like.TenantID() != tenantID {
		t.Errorf("TenantID() = %v, want %v", like.TenantID(), tenantID)
	}
	if like.PostID() != postID {
		t.Errorf("PostID() = %v, want %v", like.PostID(), postID)
	}
	if like.UserID() != userID {
		t.Errorf("UserID() = %v, want %v", like.UserID(), userID)
	}
	if like.CreatedAt().IsZero() {
		t.Error("CreatedAt() is zero, want a real timestamp")
	}
}

func TestNewLike_ZeroPostID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewLike(mustTestTenantID(t), domain.ID{}, domain.NewID())
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewLike_ZeroUserID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewLike(mustTestTenantID(t), domain.NewID(), domain.ID{})
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
