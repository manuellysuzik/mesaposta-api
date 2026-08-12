package domain

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// Comment is a single comment posted against a CommunityPost. It is NOT an
// aggregate root of its own: it has an identity (ID) purely so it can be
// addressed as a row/resource, but every Comment always belongs to --  and
// is only ever created through -- exactly one CommunityPost, referenced by
// PostID. This mirrors exactly how
// internal/modules/operations/domain.StockMovement references its parent
// StockItem by ID: an independently-persisted, APPEND-ONLY record (see
// domain.CommentRepository -- Insert only, no Update/Delete) rather than an
// in-memory child collection CommunityPost itself carries.
type Comment struct {
	id        ID
	tenantID  tenant.TenantID
	postID    ID
	authorID  ID
	text      CommentText
	createdAt time.Time
}

// NewComment builds a Comment, validating that postID and authorID are
// both non-zero. text is already guaranteed non-empty and within its
// length bounds by CommentText's own constructor (NewCommentText), so no
// further validation happens here.
func NewComment(id ID, tenantID tenant.TenantID, postID ID, authorID ID, text CommentText) (Comment, error) {
	if postID.IsZero() {
		return Comment{}, errs.New(errs.Invalid, "invalid_comment_post_id",
			"comment post id must not be zero", nil)
	}
	if authorID.IsZero() {
		return Comment{}, errs.New(errs.Invalid, "invalid_comment_author_id",
			"comment author id must not be zero", nil)
	}

	return Comment{
		id:        id,
		tenantID:  tenantID,
		postID:    postID,
		authorID:  authorID,
		text:      text,
		createdAt: time.Now().UTC(),
	}, nil
}

// ID returns the comment's identity.
func (c Comment) ID() ID { return c.id }

// TenantID returns the tenant the comment belongs to.
func (c Comment) TenantID() tenant.TenantID { return c.tenantID }

// PostID returns the ID of the CommunityPost this comment was made on.
func (c Comment) PostID() ID { return c.postID }

// AuthorID returns the ID of the user who authored the comment.
func (c Comment) AuthorID() ID { return c.authorID }

// Text returns the comment's (validated, non-empty) body.
func (c Comment) Text() CommentText { return c.text }

// CreatedAt returns the timestamp the comment was created at.
func (c Comment) CreatedAt() time.Time { return c.createdAt }
