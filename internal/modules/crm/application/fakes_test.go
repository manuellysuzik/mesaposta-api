package application_test

import (
	"context"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
)

// This file defines a hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories, following the same
// fake-implements-the-port pattern
// internal/modules/operations/application's own fakes_test.go already
// establishes for this project (in turn following
// internal/platform/auth/refresh's memStore). It is intentionally
// unexported and lives only in this (application_test) test package --
// nothing in application/'s production code, nor any other package, can
// see or depend on it.
//
// The single most important property this fake must have -- the one the
// rest of this package's tests lean on to prove atomicity without a real
// Postgres transaction -- is that fakeUnitOfWork.Do simulates a genuine
// rollback: it operates on a CLONE of its state, and only replaces the
// real state with that clone if fn returns nil. See
// TestFakeUnitOfWork_Do_RollsBackOnError below, which pins that behavior
// down directly, with nothing to do with any specific use case.

// fakeState is the mutable data a fakeUnitOfWork operates over.
type fakeState struct {
	customers      map[domain.ID]*domain.Customer
	loyaltyEntries []domain.LoyaltyEntry
	events         []outbox.DomainEvent
}

// newFakeState returns an empty, ready-to-use fakeState.
func newFakeState() *fakeState {
	return &fakeState{
		customers: map[domain.ID]*domain.Customer{},
	}
}

// clone returns an independent copy of s: new maps and slices, and new
// copies of every pointed-to aggregate, so that mutating a *Customer
// obtained from the clone during a Do call -- including one that is
// ultimately rolled back -- can never reach back into s itself.
func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		customers:      make(map[domain.ID]*domain.Customer, len(s.customers)),
		loyaltyEntries: append([]domain.LoyaltyEntry{}, s.loyaltyEntries...),
		events:         append([]outbox.DomainEvent{}, s.events...),
	}
	for id, customer := range s.customers {
		customerCopy := *customer
		clone.customers[id] = &customerCopy
	}
	return clone
}

// fakeRepoErrors lets a test inject a canned error to be returned by a
// specific repository method, on every call, for the duration of a single
// fakeUnitOfWork.Do invocation -- used to simulate failure modes (e.g. an
// optimistic-concurrency conflict on Customers().UpdateWithVersion) that
// would otherwise require a real second, concurrent transaction to
// reproduce.
type fakeRepoErrors struct {
	customerFindByID          error
	customerInsert            error
	customerUpdateWithVersion error
	loyaltyEntryInsert        error
	eventsPublish             error
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

func (r *fakeRepositories) Customers() domain.CustomerRepository {
	return &fakeCustomerRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) LoyaltyEntries() domain.LoyaltyEntryRepository {
	return &fakeLoyaltyEntryRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{uow: r.uow, state: r.state}
}

// --- CustomerRepository ---------------------------------------------------

type fakeCustomerRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.CustomerRepository = (*fakeCustomerRepository)(nil)

func (r *fakeCustomerRepository) FindByID(_ context.Context, id domain.ID) (*domain.Customer, error) {
	if err := r.uow.errors.customerFindByID; err != nil {
		return nil, err
	}
	customer, ok := r.state.customers[id]
	if !ok {
		return nil, notFoundErr("customer_not_found", "customer not found")
	}
	return customer, nil
}

func (r *fakeCustomerRepository) Insert(_ context.Context, customer *domain.Customer) error {
	if err := r.uow.errors.customerInsert; err != nil {
		return err
	}
	r.state.customers[customer.ID()] = customer
	return nil
}

func (r *fakeCustomerRepository) UpdateWithVersion(_ context.Context, customer *domain.Customer, _ int64) error {
	if err := r.uow.errors.customerUpdateWithVersion; err != nil {
		return err
	}
	r.state.customers[customer.ID()] = customer
	return nil
}

// --- LoyaltyEntryRepository -------------------------------------------------

type fakeLoyaltyEntryRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.LoyaltyEntryRepository = (*fakeLoyaltyEntryRepository)(nil)

func (r *fakeLoyaltyEntryRepository) Insert(_ context.Context, entry domain.LoyaltyEntry) error {
	if err := r.uow.errors.loyaltyEntryInsert; err != nil {
		return err
	}
	r.state.loyaltyEntries = append(r.state.loyaltyEntries, entry)
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

// notFoundErr builds the *errs.Error the fake repositories return for a
// missing lookup.
func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}
