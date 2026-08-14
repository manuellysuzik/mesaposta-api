package http_test

import (
	"context"
	"sort"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// This file defines a small, hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories for this package's httptest-based
// handler tests -- the same fake-implements-the-port pattern
// ../../application's own fakes_test.go establishes for that package,
// duplicated (not imported) here because it is a test-only, unexported
// type and each package's _test.go files are their own, separate Go
// package.

type fakeState struct {
	products     map[domain.ID]*domain.Product
	recipes      map[domain.ID]*domain.Recipe
	events       []outbox.DomainEvent
	auditEntries []audit.Entry
}

func newFakeState() *fakeState {
	return &fakeState{
		products: map[domain.ID]*domain.Product{},
		recipes:  map[domain.ID]*domain.Recipe{},
	}
}

func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		products:     make(map[domain.ID]*domain.Product, len(s.products)),
		recipes:      make(map[domain.ID]*domain.Recipe, len(s.recipes)),
		events:       append([]outbox.DomainEvent{}, s.events...),
		auditEntries: append([]audit.Entry{}, s.auditEntries...),
	}
	for id, p := range s.products {
		pc := *p
		clone.products[id] = &pc
	}
	for id, rc := range s.recipes {
		cc := *rc
		clone.recipes[id] = &cc
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

func (r *fakeRepositories) Products() domain.ProductRepository {
	return &fakeProductRepository{state: r.state}
}

func (r *fakeRepositories) Recipes() domain.RecipeRepository {
	return &fakeRecipeRepository{state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{state: r.state}
}

func (r *fakeRepositories) Audit() audit.Recorder {
	return &fakeAuditRecorder{state: r.state}
}

type fakeProductRepository struct {
	state *fakeState
}

var _ domain.ProductRepository = (*fakeProductRepository)(nil)

func (r *fakeProductRepository) FindByID(_ context.Context, id domain.ID) (*domain.Product, error) {
	product, ok := r.state.products[id]
	if !ok {
		return nil, errs.New(errs.NotFound, "product_not_found", "product not found", nil)
	}
	return product, nil
}

func (r *fakeProductRepository) FindActive(_ context.Context) ([]*domain.Product, error) {
	var out []*domain.Product
	for _, p := range r.state.products {
		if p.IsActive() {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (r *fakeProductRepository) Insert(_ context.Context, product *domain.Product) error {
	r.state.products[product.ID()] = product
	return nil
}

func (r *fakeProductRepository) UpdateWithVersion(_ context.Context, product *domain.Product, _ int64) error {
	r.state.products[product.ID()] = product
	return nil
}

type fakeRecipeRepository struct {
	state *fakeState
}

var _ domain.RecipeRepository = (*fakeRecipeRepository)(nil)

func (r *fakeRecipeRepository) FindByID(_ context.Context, id domain.ID) (*domain.Recipe, error) {
	recipe, ok := r.state.recipes[id]
	if !ok {
		return nil, errs.New(errs.NotFound, "recipe_not_found", "recipe not found", nil)
	}
	return recipe, nil
}

func (r *fakeRecipeRepository) Insert(_ context.Context, recipe *domain.Recipe) error {
	r.state.recipes[recipe.ID()] = recipe
	return nil
}

type fakeEventPublisher struct {
	state *fakeState
}

var _ outbox.DomainEventPublisher = (*fakeEventPublisher)(nil)

func (p *fakeEventPublisher) Publish(_ context.Context, events ...outbox.DomainEvent) error {
	p.state.events = append(p.state.events, events...)
	return nil
}

type fakeAuditRecorder struct {
	state *fakeState
}

var _ audit.Recorder = (*fakeAuditRecorder)(nil)

func (a *fakeAuditRecorder) Record(_ context.Context, entries ...audit.Entry) error {
	a.state.auditEntries = append(a.state.auditEntries, entries...)
	return nil
}
