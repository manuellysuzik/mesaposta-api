package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func mustCommentText(t *testing.T, raw string) domain.CommentText {
	t.Helper()
	text, err := domain.NewCommentText(raw)
	if err != nil {
		t.Fatalf("NewCommentText(%q): %v", raw, err)
	}
	return text
}

func TestNewComment_Valid(t *testing.T) {
	id := domain.NewID()
	tenantID := mustTestTenantID(t)
	postID := domain.NewID()
	authorID := domain.NewID()
	text := mustCommentText(t, "ótimo trabalho!")

	comment, err := domain.NewComment(id, tenantID, postID, authorID, text)
	if err != nil {
		t.Fatalf("NewComment: unexpected error: %v", err)
	}
	if comment.ID() != id {
		t.Errorf("ID() = %v, want %v", comment.ID(), id)
	}
	if comment.PostID() != postID {
		t.Errorf("PostID() = %v, want %v", comment.PostID(), postID)
	}
	if comment.AuthorID() != authorID {
		t.Errorf("AuthorID() = %v, want %v", comment.AuthorID(), authorID)
	}
	if comment.Text() != text {
		t.Errorf("Text() = %v, want %v", comment.Text(), text)
	}
	if comment.CreatedAt().IsZero() {
		t.Error("CreatedAt() is zero, want a real timestamp")
	}
}

func TestNewComment_ZeroPostID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewComment(domain.NewID(), mustTestTenantID(t), domain.ID{}, domain.NewID(), mustCommentText(t, "texto"))
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewComment_ZeroAuthorID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewComment(domain.NewID(), mustTestTenantID(t), domain.NewID(), domain.ID{}, mustCommentText(t, "texto"))
	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}
