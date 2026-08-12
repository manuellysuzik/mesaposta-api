package http

import (
	"net/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/application"
	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// createPostHandler returns a handler for POST /v1/posts.
func createPostHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		authorID, err := currentUserID(r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createPostRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		post, err := application.CreatePost(r.Context(), uow, application.CreatePostInput{
			TenantID: tenantID,
			AuthorID: authorID,
			Content:  req.Content,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newPostResponse(&post), nil)
	}
}

// getPostHandler returns a handler for GET /v1/posts/{id}.
func getPostHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := tenant.From(r.Context()); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		id, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		post, err := application.GetPost(r.Context(), uow, id)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, newPostResponse(post), nil)
	}
}

// commentOnPostHandler returns a handler for POST /v1/posts/{id}/comments.
func commentOnPostHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		authorID, err := currentUserID(r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		postID, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		req, err := decodeJSON[createCommentRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		comment, err := application.CommentOnPost(r.Context(), uow, application.CommentOnPostInput{
			TenantID: tenantID,
			PostID:   postID,
			AuthorID: authorID,
			Text:     req.Text,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusCreated, newCommentResponse(&comment), nil)
	}
}

// likePostHandler returns a handler for the IDEMPOTENT
// PUT /v1/posts/{id}/curtidas/me: liking a post you already like is a
// no-op that still responds 200, never an error (see
// application.LikePost's own doc comment).
func likePostHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		userID, err := currentUserID(r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		postID, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := application.LikePost(r.Context(), uow, application.LikePostInput{
			TenantID: tenantID,
			PostID:   postID,
			UserID:   userID,
		}); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, likeResponse{PostID: postID.String(), Liked: true}, nil)
	}
}

// unlikePostHandler returns a handler for the IDEMPOTENT
// DELETE /v1/posts/{id}/curtidas/me: unliking a post you never liked is a
// no-op that still responds 200, never an error (see
// application.UnlikePost's own doc comment).
func unlikePostHandler(uow domain.UnitOfWork) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tenant.From(r.Context())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		userID, err := currentUserID(r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		postID, err := pathID(r, "id")
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		if err := application.UnlikePost(r.Context(), uow, application.UnlikePostInput{
			TenantID: tenantID,
			PostID:   postID,
			UserID:   userID,
		}); err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, likeResponse{PostID: postID.String(), Liked: false}, nil)
	}
}
