package domain

import (
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// CommunityPost is a single post in a tenant's internal community feed. It
// is an aggregate root, deliberately SEPARATE from Like (see like.go's own
// doc comment for why): CommunityPost owns a post's own content and
// authorship, nothing about who has liked it.
//
// CommunityPost is immutable once created: this phase exposes no setter,
// no version, and no edit/delete use case (out of scope -- see
// internal/modules/README.md and this module's own task notes: content
// moderation, editing, and deletion are backlog items, not part of this
// phase's contract). Comments and likes are tracked by their own,
// independently-persisted types (Comment, Like) that reference a
// CommunityPost by ID -- mirroring exactly how
// internal/modules/operations/domain.StockMovement/Transaction reference
// their own parent aggregate by ID rather than living inside it.
type CommunityPost struct {
	id        ID
	tenantID  tenant.TenantID
	authorID  ID
	content   string
	createdAt time.Time
}

// NewCommunityPost builds a CommunityPost, validating that authorID is not
// the zero ID and that content is not empty or made up entirely of
// whitespace. content is stored TRIMMED, mirroring CommentText's own
// normalization (see comment_text.go) -- surrounding whitespace is not
// meaningful content.
func NewCommunityPost(id ID, tenantID tenant.TenantID, authorID ID, content string) (CommunityPost, error) {
	if authorID.IsZero() {
		return CommunityPost{}, errs.New(errs.Invalid, "invalid_post_author_id",
			"post author id must not be zero", nil)
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return CommunityPost{}, errs.New(errs.Invalid, "invalid_post_content",
			"post content must not be empty or only whitespace", nil)
	}

	return CommunityPost{
		id:        id,
		tenantID:  tenantID,
		authorID:  authorID,
		content:   trimmed,
		createdAt: time.Now().UTC(),
	}, nil
}

// ID returns the post's identity.
func (p CommunityPost) ID() ID { return p.id }

// TenantID returns the tenant the post belongs to.
func (p CommunityPost) TenantID() tenant.TenantID { return p.tenantID }

// AuthorID returns the ID of the user who authored the post.
func (p CommunityPost) AuthorID() ID { return p.authorID }

// Content returns the post's (trimmed, non-empty) body text.
func (p CommunityPost) Content() string { return p.content }

// CreatedAt returns the timestamp the post was created at.
func (p CommunityPost) CreatedAt() time.Time { return p.createdAt }
