package http

import (
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
)

// postResponse is the JSON representation of a CommunityPost aggregate.
type postResponse struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// newPostResponse maps a domain CommunityPost to its HTTP response.
func newPostResponse(p *domain.CommunityPost) postResponse {
	return postResponse{
		ID:        p.ID().String(),
		TenantID:  p.TenantID().String(),
		AuthorID:  p.AuthorID().String(),
		Content:   p.Content(),
		CreatedAt: p.CreatedAt(),
	}
}

// commentResponse is the JSON representation of a Comment.
type commentResponse struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	PostID    string    `json:"post_id"`
	AuthorID  string    `json:"author_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// newCommentResponse maps a domain Comment to its HTTP response.
func newCommentResponse(c *domain.Comment) commentResponse {
	return commentResponse{
		ID:        c.ID().String(),
		TenantID:  c.TenantID().String(),
		PostID:    c.PostID().String(),
		AuthorID:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: c.CreatedAt(),
	}
}

// likeResponse is the JSON response for both PUT and DELETE
// /v1/posts/{id}/curtidas/me -- both are idempotent, so the response shape
// simply confirms the caller's own current liked/not-liked state after the
// call, regardless of whether this specific call changed anything.
type likeResponse struct {
	PostID string `json:"post_id"`
	Liked  bool   `json:"liked"`
}

// createPostRequest is the request payload for POST /v1/posts.
type createPostRequest struct {
	Content string `json:"content"`
}

// createCommentRequest is the request payload for
// POST /v1/posts/{id}/comments.
type createCommentRequest struct {
	Text string `json:"text"`
}
