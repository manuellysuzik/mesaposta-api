package http_test

import (
	"context"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/crm/domain"
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
	customers      map[domain.ID]*domain.Customer
	loyaltyEntries []domain.LoyaltyEntry
	events         []outbox.DomainEvent
	auditEntries   []audit.Entry
}

func newFakeState() *fakeState {
	return &fakeState{customers: map[domain.ID]*domain.Customer{}}
}

func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		customers:      make(map[domain.ID]*domain.Customer, len(s.customers)),
		loyaltyEntries: append([]domain.LoyaltyEntry{}, s.loyaltyEntries...),
		events:         append([]outbox.DomainEvent{}, s.events...),
		auditEntries:   append([]audit.Entry{}, s.auditEntries...),
	}
	for id, c := range s.customers {
		cc := *c
		clone.customers[id] = &cc
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

func (r *fakeRepositories) Customers() domain.CustomerRepository {
	return &fakeCustomerRepository{state: r.state}
}

func (r *fakeRepositories) LoyaltyEntries() domain.LoyaltyEntryRepository {
	return &fakeLoyaltyEntryRepository{state: r.state}
}

func (r *fakeRepositories) Events() outbox.DomainEventPublisher {
	return &fakeEventPublisher{state: r.state}
}

func (r *fakeRepositories) Audit() audit.Recorder {
	return &fakeAuditRecorder{state: r.state}
}

type fakeCustomerRepository struct {
	state *fakeState
}

var _ domain.CustomerRepository = (*fakeCustomerRepository)(nil)

func (r *fakeCustomerRepository) FindByID(_ context.Context, id domain.ID) (*domain.Customer, error) {
	customer, ok := r.state.customers[id]
	if !ok {
		return nil, errs.New(errs.NotFound, "customer_not_found", "customer not found", nil)
	}
	return customer, nil
}

func (r *fakeCustomerRepository) Insert(_ context.Context, customer *domain.Customer) error {
	r.state.customers[customer.ID()] = customer
	return nil
}

func (r *fakeCustomerRepository) UpdateWithVersion(_ context.Context, customer *domain.Customer, _ int64) error {
	r.state.customers[customer.ID()] = customer
	return nil
}

type fakeLoyaltyEntryRepository struct {
	state *fakeState
}

var _ domain.LoyaltyEntryRepository = (*fakeLoyaltyEntryRepository)(nil)

func (r *fakeLoyaltyEntryRepository) Insert(_ context.Context, entry domain.LoyaltyEntry) error {
	r.state.loyaltyEntries = append(r.state.loyaltyEntries, entry)
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
