package http_test

import (
	"context"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// This file defines a small, hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories for this package's httptest-based
// handler tests -- the same fake-implements-the-port pattern
// ../../application's own fakes_test.go establishes for that package,
// duplicated (not imported) here because it is a test-only, unexported
// type and each package's _test.go files are their own, separate Go
// package (mirroring internal/modules/crm/infrastructure/http's own
// fake_uow_test.go).

type likeKey struct {
	postID domain.ID
	userID domain.ID
}

type fakeState struct {
	posts    map[domain.ID]*domain.CommunityPost
	comments []domain.Comment
	likes    map[likeKey]domain.Like
	events   []outbox.DomainEvent
}

func newFakeState() *fakeState {
	return &fakeState{
		posts: map[domain.ID]*domain.CommunityPost{},
		likes: map[likeKey]domain.Like{},
	}
}

func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		posts:    make(map[domain.ID]*domain.CommunityPost, len(s.posts)),
		comments: append([]domain.Comment{}, s.comments...),
		likes:    make(map[likeKey]domain.Like, len(s.likes)),
		events:   append([]outbox.DomainEvent{}, s.events...),
	}
	for id, post := range s.posts {
		postCopy := *post
		clone.posts[id] = &postCopy
	}
	for key, like := range s.likes {
		clone.likes[key] = like
	}
	return clone
}

type fakeUnitOfWork struct {
	mu    sync.Mutex
	state *fakeState
}

func newFakeUnitOfWork(state *fakeState) *fakeUnitOfWork {
	return &fakeUnitOfWork{state: state}
}

var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

func (u *fakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	u.mu.Lock()
	clone := u.state.clone()
	u.mu.Unlock()

	repos := &fakeRepositories{state: clone}

	if err := fn(ctx, repos); err != nil {
		return err
	}

	u.mu.Lock()
	u.state = clone
	u.mu.Unlock()
	return nil
}

func (u *fakeUnitOfWork) snapshot() *fakeState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.clone()
}

type fakeRepositories struct {
	state *fakeState
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (r *fakeRepositories) Posts() domain.CommunityPostRepository {
	return &fakeCommunityPostRepository{state: r.state}
}

func (r *fakeRepositories) Comments() domain.CommentRepository {
	return &fakeCommentRepository{state: r.state}
}

func (r *fakeRepositories) Likes() domain.LikeRepository {
	return &fakeLikeRepository{state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{state: r.state}
}

type fakeCommunityPostRepository struct {
	state *fakeState
}

var _ domain.CommunityPostRepository = (*fakeCommunityPostRepository)(nil)

func (r *fakeCommunityPostRepository) FindByID(_ context.Context, id domain.ID) (*domain.CommunityPost, error) {
	post, ok := r.state.posts[id]
	if !ok {
		return nil, errs.New(errs.NotFound, "post_not_found", "post not found", nil)
	}
	return post, nil
}

func (r *fakeCommunityPostRepository) Insert(_ context.Context, post *domain.CommunityPost) error {
	r.state.posts[post.ID()] = post
	return nil
}

type fakeCommentRepository struct {
	state *fakeState
}

var _ domain.CommentRepository = (*fakeCommentRepository)(nil)

func (r *fakeCommentRepository) Insert(_ context.Context, comment domain.Comment) error {
	r.state.comments = append(r.state.comments, comment)
	return nil
}

type fakeLikeRepository struct {
	state *fakeState
}

var _ domain.LikeRepository = (*fakeLikeRepository)(nil)

func (r *fakeLikeRepository) Insert(_ context.Context, like domain.Like) (bool, error) {
	key := likeKey{postID: like.PostID(), userID: like.UserID()}
	if _, exists := r.state.likes[key]; exists {
		return false, nil
	}
	r.state.likes[key] = like
	return true, nil
}

func (r *fakeLikeRepository) Delete(_ context.Context, postID domain.ID, userID domain.ID) (bool, error) {
	key := likeKey{postID: postID, userID: userID}
	if _, exists := r.state.likes[key]; !exists {
		return false, nil
	}
	delete(r.state.likes, key)
	return true, nil
}

type fakeEventPublisher struct {
	state *fakeState
}

var _ outbox.DomainEventPublisher = (*fakeEventPublisher)(nil)

func (p *fakeEventPublisher) Publish(_ context.Context, events ...outbox.DomainEvent) error {
	p.state.events = append(p.state.events, events...)
	return nil
}
