package contracts

import "time"

// Event name constants for the community module. Every name is
// dot-namespaced as "<aggregate_type>.<event>", with exactly one "." --
// internal/platform/outbox/postgres/postgres_publisher.go's aggregateTypeOf
// derives the domain_events.aggregate_type column from the prefix up to
// the first ".", so the prefix below must always match the aggregate the
// event describes (see events_test.go for the regression test pinning
// this down). Both events below describe something that happened to a
// CommunityPost -- a comment was added to it, or someone liked it -- so
// both share the "post" aggregate-type prefix and carry the post's own ID
// as AggregateID (never the comment's or the like's own identity), mirroring
// how internal/modules/operations' EventStockItemMovementRegistered uses
// the STOCK ITEM's id, not the movement's, as its aggregate id.
const (
	// EventPostCommented is published whenever a comment is added to a
	// CommunityPost. Payload: PostCommentedPayload.
	EventPostCommented = "post.commented"

	// EventPostLiked is published the first time a given user likes a
	// CommunityPost -- never on a subsequent, idempotent repeat of an
	// already-existing like (see
	// internal/modules/community/domain.LikeRepository's own doc comment).
	// Payload: PostLikedPayload.
	EventPostLiked = "post.liked"
)

// PostCommentedPayload is the payload for EventPostCommented.
type PostCommentedPayload struct {
	PostID     string    `json:"post_id"`
	CommentID  string    `json:"comment_id"`
	AuthorID   string    `json:"author_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// PostLikedPayload is the payload for EventPostLiked.
type PostLikedPayload struct {
	PostID     string    `json:"post_id"`
	UserID     string    `json:"user_id"`
	OccurredAt time.Time `json:"occurred_at"`
}
