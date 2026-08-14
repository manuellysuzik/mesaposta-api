package application_test

import (
	"context"
	"sort"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// This file defines a hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories, following the same
// fake-implements-the-port pattern
// internal/modules/operations/application's and internal/modules/crm/application's
// own fakes_test.go already establish for this project. It is
// intentionally unexported and lives only in this (application_test) test
// package -- nothing in application/'s production code, nor any other
// package, can see or depend on it.
//
// The single most important property this fake must have -- the one the
// rest of this package's tests lean on to prove atomicity without a real
// Postgres transaction -- is that fakeUnitOfWork.Do simulates a genuine
// rollback: it operates on a CLONE of its state, and only replaces the
// real state with that clone if fn returns nil. See
// TestFakeUnitOfWork_Do_RollsBackOnError in fake_unit_of_work_test.go,
// which pins that behavior down directly, with nothing to do with any
// specific use case.

// fakeState is the mutable data a fakeUnitOfWork operates over.
type fakeState struct {
	products     map[domain.ID]*domain.Product
	recipes      map[domain.ID]*domain.Recipe
	events       []outbox.DomainEvent
	auditEntries []audit.Entry
}

// newFakeState returns an empty, ready-to-use fakeState.
func newFakeState() *fakeState {
	return &fakeState{
		products: map[domain.ID]*domain.Product{},
		recipes:  map[domain.ID]*domain.Recipe{},
	}
}

// clone returns an independent copy of s: new maps and slices, and new
// copies of every pointed-to aggregate, so that mutating a *Product or
// *Recipe obtained from the clone during a Do call -- including one that is
// ultimately rolled back -- can never reach back into s itself.
func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		products:     make(map[domain.ID]*domain.Product, len(s.products)),
		recipes:      make(map[domain.ID]*domain.Recipe, len(s.recipes)),
		events:       append([]outbox.DomainEvent{}, s.events...),
		auditEntries: append([]audit.Entry{}, s.auditEntries...),
	}
	for id, product := range s.products {
		productCopy := *product
		clone.products[id] = &productCopy
	}
	for id, recipe := range s.recipes {
		recipeCopy := *recipe
		clone.recipes[id] = &recipeCopy
	}
	return clone
}

// fakeRepoErrors lets a test inject a canned error to be returned by a
// specific repository method, on every call, for the duration of a single
// fakeUnitOfWork.Do invocation.
type fakeRepoErrors struct {
	productFindByID          error
	productFindActive        error
	productInsert            error
	productUpdateWithVersion error
	recipeFindByID           error
	recipeInsert             error
	eventsPublish            error
	auditRecord              error
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

func (r *fakeRepositories) Products() domain.ProductRepository {
	return &fakeProductRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Recipes() domain.RecipeRepository {
	return &fakeRecipeRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Audit() audit.Recorder {
	return &fakeAuditRecorder{uow: r.uow, state: r.state}
}

// --- ProductRepository -------------------------------------------------

type fakeProductRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.ProductRepository = (*fakeProductRepository)(nil)

func (r *fakeProductRepository) FindByID(_ context.Context, id domain.ID) (*domain.Product, error) {
	if err := r.uow.errors.productFindByID; err != nil {
		return nil, err
	}
	product, ok := r.state.products[id]
	if !ok {
		return nil, notFoundErr("product_not_found", "product not found")
	}
	return product, nil
}

func (r *fakeProductRepository) FindActive(_ context.Context) ([]*domain.Product, error) {
	if err := r.uow.errors.productFindActive; err != nil {
		return nil, err
	}
	var out []*domain.Product
	for _, product := range r.state.products {
		if product.IsActive() {
			out = append(out, product)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (r *fakeProductRepository) Insert(_ context.Context, product *domain.Product) error {
	if err := r.uow.errors.productInsert; err != nil {
		return err
	}
	r.state.products[product.ID()] = product
	return nil
}

func (r *fakeProductRepository) UpdateWithVersion(_ context.Context, product *domain.Product, _ int64) error {
	if err := r.uow.errors.productUpdateWithVersion; err != nil {
		return err
	}
	r.state.products[product.ID()] = product
	return nil
}

// --- RecipeRepository -------------------------------------------------

type fakeRecipeRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.RecipeRepository = (*fakeRecipeRepository)(nil)

func (r *fakeRecipeRepository) FindByID(_ context.Context, id domain.ID) (*domain.Recipe, error) {
	if err := r.uow.errors.recipeFindByID; err != nil {
		return nil, err
	}
	recipe, ok := r.state.recipes[id]
	if !ok {
		return nil, notFoundErr("recipe_not_found", "recipe not found")
	}
	return recipe, nil
}

func (r *fakeRecipeRepository) Insert(_ context.Context, recipe *domain.Recipe) error {
	if err := r.uow.errors.recipeInsert; err != nil {
		return err
	}
	r.state.recipes[recipe.ID()] = recipe
	return nil
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

// --- audit.Recorder -----------------------------------------------------

// fakeAuditRecorder accumulates recorded entries into the same fakeState
// clone every other repository writes through, so an entry recorded inside
// a Do call that ultimately rolls back is discarded along with every other
// write in that call -- the in-memory equivalent of the atomicity the real
// Postgres adapter gets from sharing a db.Scope.
type fakeAuditRecorder struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ audit.Recorder = (*fakeAuditRecorder)(nil)

func (a *fakeAuditRecorder) Record(_ context.Context, entries ...audit.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := a.uow.errors.auditRecord; err != nil {
		return err
	}
	a.state.auditEntries = append(a.state.auditEntries, entries...)
	return nil
}

// notFoundErr builds the *errs.Error the fake repositories return for a
// missing lookup.
func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}
