package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Like records that one user likes one CommunityPost. It is a SEPARATE
// aggregate root from CommunityPost -- a deliberate design decision, not an
// oversight: Like's identity is the composite (PostID, UserID), it is
// small, and it exists specifically to make PUT/DELETE
// /v1/posts/{id}/curtidas/me IDEMPOTENT (see domain.LikeRepository's own
// doc comment) without forcing every concurrent like/unlike on a post to
// serialize against that post's own aggregate lock. If a like lived inside
// CommunityPost itself, liking a popular post would contend on the SAME
// row every other write to that post (a new comment, a future edit)
// contends on -- liking a post has nothing to do with the post's own
// content and should never need to know about it.
//
// Like carries no version: idempotency is enforced by the database's own
// UNIQUE (tenant_id, post_id, user_id) constraint (see
// internal/platform/db/migrations/0007_community.sql) together with
// LikeRepository.Insert's ON CONFLICT DO NOTHING semantics, not by
// optimistic concurrency.
type Like struct {
	tenantID  tenant.TenantID
	postID    ID
	userID    ID
	createdAt time.Time
}

// NewLike builds a Like, validating that postID and userID are both
// non-zero.
func NewLike(tenantID tenant.TenantID, postID ID, userID ID) (Like, error) {
	if postID.IsZero() {
		return Like{}, errs.New(errs.Invalid, "invalid_like_post_id",
			"like post id must not be zero", nil)
	}
	if userID.IsZero() {
		return Like{}, errs.New(errs.Invalid, "invalid_like_user_id",
			"like user id must not be zero", nil)
	}

	return Like{
		tenantID:  tenantID,
		postID:    postID,
		userID:    userID,
		createdAt: time.Now().UTC(),
	}, nil
}

// TenantID returns the tenant the like belongs to.
func (l Like) TenantID() tenant.TenantID { return l.tenantID }

// PostID returns the ID of the CommunityPost this like applies to.
func (l Like) PostID() ID { return l.postID }

// UserID returns the ID of the user who liked the post.
func (l Like) UserID() ID { return l.userID }

// CreatedAt returns the timestamp the like was recorded at.
func (l Like) CreatedAt() time.Time { return l.createdAt }
