package application_test

import (
	"context"
	"sync"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// testTenantID is a fixed, valid tenant UUID reused across this package's
// tests via mustTestTenantID.
const testTenantID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// mustTestTenantID parses testTenantID into a tenant.TenantID, failing the
// test on any error.
func mustTestTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(testTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", testTenantID, err)
	}
	return id
}

// This file defines a hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories, following the same
// fake-implements-the-port pattern
// internal/modules/operations/application's own fakes_test.go already
// establishes for this codebase. It is intentionally unexported and lives
// only in this (application_test) test package -- nothing in
// application/'s production code, nor any other package, can see or depend
// on it.
//
// The single most important property this fake must have -- the one the
// rest of this package's tests lean on to prove atomicity without a real
// Postgres transaction -- is that fakeUnitOfWork.Do simulates a genuine
// rollback: it operates on a CLONE of its state, and only replaces the
// real state with that clone if fn returns nil. See
// TestFakeUnitOfWork_Do_RollsBackOnError in fake_unit_of_work_test.go,
// which pins that behavior down directly.

// likeKey is the composite (postID, userID) key fakeState.likes is indexed
// by, mirroring the UNIQUE (tenant_id, post_id, user_id) constraint the
// real Postgres-backed LikeRepository enforces (see
// internal/platform/db/migrations/0007_community.sql).
type likeKey struct {
	postID domain.ID
	userID domain.ID
}

// fakeState is the mutable data a fakeUnitOfWork operates over.
type fakeState struct {
	posts    map[domain.ID]*domain.CommunityPost
	comments []domain.Comment
	likes    map[likeKey]domain.Like
	events   []outbox.DomainEvent
}

// newFakeState returns an empty, ready-to-use fakeState.
func newFakeState() *fakeState {
	return &fakeState{
		posts: map[domain.ID]*domain.CommunityPost{},
		likes: map[likeKey]domain.Like{},
	}
}

// clone returns an independent copy of s: new maps and slices, and new
// copies of every pointed-to aggregate, so that mutating anything obtained
// from the clone during a Do call -- including one that is ultimately
// rolled back -- can never reach back into s itself.
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

// fakeRepoErrors lets a test inject a canned error to be returned by a
// specific repository method, on every call, for the duration of a single
// fakeUnitOfWork.Do invocation.
type fakeRepoErrors struct {
	postFindByID  error
	postInsert    error
	commentInsert error
	likeInsert    error
	likeDelete    error
	eventsPublish error
}

// fakeUnitOfWork is an in-memory domain.UnitOfWork. Its zero value is not
// usable; construct one via newFakeUnitOfWork.
type fakeUnitOfWork struct {
	mu    sync.Mutex
	state *fakeState

	// errors is consulted by every repository method built on top of this
	// fakeUnitOfWork, for every Do call, letting a test set it up once
	// before calling the use case under test.
	errors fakeRepoErrors
}

// newFakeUnitOfWork returns a fakeUnitOfWork seeded with state.
func newFakeUnitOfWork(state *fakeState) *fakeUnitOfWork {
	return &fakeUnitOfWork{state: state}
}

// compile-time check: *fakeUnitOfWork must satisfy domain.UnitOfWork.
var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

// Do clones u's current state, runs fn against Repositories backed by that
// clone, and -- only if fn returns nil -- replaces u's real state with the
// clone. If fn returns a non-nil error, the clone is discarded and u's real
// state is left exactly as it was before Do was called: a rollback.
func (u *fakeUnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, r domain.Repositories) error) error {
	u.mu.Lock()
	clone := u.state.clone()
	u.mu.Unlock()

	repos := &fakeRepositories{uow: u, state: clone}

	if err := fn(ctx, repos); err != nil {
		return err
	}

	u.mu.Lock()
	u.state = clone
	u.mu.Unlock()
	return nil
}

// snapshot returns a clone of u's current, real (committed) state -- for
// test assertions only.
func (u *fakeUnitOfWork) snapshot() *fakeState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.clone()
}

// fakeRepositories is the domain.Repositories implementation handed to fn
// by fakeUnitOfWork.Do, scoped to a single clone of state.
type fakeRepositories struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (r *fakeRepositories) Posts() domain.CommunityPostRepository {
	return &fakeCommunityPostRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Comments() domain.CommentRepository {
	return &fakeCommentRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Likes() domain.LikeRepository {
	return &fakeLikeRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{uow: r.uow, state: r.state}
}

// --- CommunityPostRepository ---------------------------------------------

type fakeCommunityPostRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.CommunityPostRepository = (*fakeCommunityPostRepository)(nil)

func (r *fakeCommunityPostRepository) FindByID(_ context.Context, id domain.ID) (*domain.CommunityPost, error) {
	if err := r.uow.errors.postFindByID; err != nil {
		return nil, err
	}
	post, ok := r.state.posts[id]
	if !ok {
		return nil, notFoundErr("post_not_found", "post not found")
	}
	return post, nil
}

func (r *fakeCommunityPostRepository) Insert(_ context.Context, post *domain.CommunityPost) error {
	if err := r.uow.errors.postInsert; err != nil {
		return err
	}
	r.state.posts[post.ID()] = post
	return nil
}

// --- CommentRepository -----------------------------------------------------

type fakeCommentRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.CommentRepository = (*fakeCommentRepository)(nil)

func (r *fakeCommentRepository) Insert(_ context.Context, comment domain.Comment) error {
	if err := r.uow.errors.commentInsert; err != nil {
		return err
	}
	r.state.comments = append(r.state.comments, comment)
	return nil
}

// --- LikeRepository ----------------------------------------------------------

type fakeLikeRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.LikeRepository = (*fakeLikeRepository)(nil)

func (r *fakeLikeRepository) Insert(_ context.Context, like domain.Like) (bool, error) {
	if err := r.uow.errors.likeInsert; err != nil {
		return false, err
	}
	key := likeKey{postID: like.PostID(), userID: like.UserID()}
	if _, exists := r.state.likes[key]; exists {
		return false, nil
	}
	r.state.likes[key] = like
	return true, nil
}

func (r *fakeLikeRepository) Delete(_ context.Context, postID domain.ID, userID domain.ID) (bool, error) {
	if err := r.uow.errors.likeDelete; err != nil {
		return false, err
	}
	key := likeKey{postID: postID, userID: userID}
	if _, exists := r.state.likes[key]; !exists {
		return false, nil
	}
	delete(r.state.likes, key)
	return true, nil
}

// --- DomainEventPublisher ---------------------------------------------------

type fakeEventPublisher struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ outbox.DomainEventPublisher = (*fakeEventPublisher)(nil)

func (p *fakeEventPublisher) Publish(_ context.Context, events ...outbox.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := p.uow.errors.eventsPublish; err != nil {
		return err
	}
	p.state.events = append(p.state.events, events...)
	return nil
}

// notFoundErr builds the *errs.Error the fake repositories return for a
// missing lookup.
func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}
