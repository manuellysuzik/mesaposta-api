package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// minCommentTextRunes and maxCommentTextRunes bound a comment's length, in
// Unicode code points (not bytes -- a comment full of multi-byte
// characters, e.g. emoji, must be judged by how many characters it holds,
// not by how many bytes UTF-8 happens to need to encode them).
const (
	minCommentTextRunes = 1
	maxCommentTextRunes = 2000
)

// CommentText is a validated, non-empty comment body, at most
// maxCommentTextRunes characters long. It is the value object every
// Comment carries (see comment.go) -- the community module's one
// dedicated "text" value object, since a comment's length is bounded in a
// way a post's own content is not (see community_post.go).
//
// The zero value is invalid; obtain a real CommentText via NewCommentText.
type CommentText struct {
	v string
}

// NewCommentText validates raw and, on success, returns a CommentText
// wrapping its TRIMMED form (leading/trailing whitespace is not considered
// meaningful content, so it is normalized away here rather than persisted
// verbatim).
//
// NewCommentText rejects: raw that is empty or made up entirely of
// whitespace (Kind errs.Invalid, code "invalid_comment_text_empty"), and
// raw whose trimmed rune count exceeds maxCommentTextRunes (Kind
// errs.Invalid, code "invalid_comment_text_too_long"). A comment exactly
// at the 1 or 2000 rune boundary is accepted.
func NewCommentText(raw string) (CommentText, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CommentText{}, errs.New(errs.Invalid, "invalid_comment_text_empty",
			"comment text must not be empty or only whitespace", nil)
	}

	length := utf8.RuneCountInString(trimmed)
	if length < minCommentTextRunes {
		// Unreachable given the trimmed == "" check above (any non-empty
		// trimmed string has at least 1 rune), kept as an explicit guard so
		// the min/max bounds both have a corresponding, visible check.
		return CommentText{}, errs.New(errs.Invalid, "invalid_comment_text_empty",
			"comment text must not be empty", nil)
	}
	if length > maxCommentTextRunes {
		return CommentText{}, errs.New(errs.Invalid, "invalid_comment_text_too_long",
			fmt.Sprintf("comment text must be at most %d characters", maxCommentTextRunes),
			fmt.Errorf("comment text has %d characters, want at most %d", length, maxCommentTextRunes))
	}

	return CommentText{v: trimmed}, nil
}

// String returns the comment's trimmed text.
func (c CommentText) String() string {
	return c.v
}
