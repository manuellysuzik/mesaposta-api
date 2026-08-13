package http_test

import (
	"context"
	"sort"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// This file defines a small, hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories for this package's httptest-based
// handler tests -- the same fake-implements-the-port pattern
// ../../application's own fakes_test.go establishes for that package,
// duplicated (not imported) here because it is a test-only, unexported
// type and each package's _test.go files are their own, separate Go
// package.

type fakeState struct {
	tables        map[domain.ID]*domain.Table
	orders        map[domain.ID]*domain.Order
	tableSessions map[[32]byte]domain.TableSession
}

func newFakeState() *fakeState {
	return &fakeState{
		tables:        map[domain.ID]*domain.Table{},
		orders:        map[domain.ID]*domain.Order{},
		tableSessions: map[[32]byte]domain.TableSession{},
	}
}

func (s *fakeState) clone() *fakeState {
	clone := &fakeState{
		tables:        make(map[domain.ID]*domain.Table, len(s.tables)),
		orders:        make(map[domain.ID]*domain.Order, len(s.orders)),
		tableSessions: make(map[[32]byte]domain.TableSession, len(s.tableSessions)),
	}
	for id, table := range s.tables {
		tc := *table
		clone.tables[id] = &tc
	}
	for id, order := range s.orders {
		oc := *order
		clone.orders[id] = &oc
	}
	for hash, session := range s.tableSessions {
		clone.tableSessions[hash] = session
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

func (r *fakeRepositories) Tables() domain.TableRepository {
	return &fakeTableRepository{state: r.state}
}

func (r *fakeRepositories) Orders() domain.OrderRepository {
	return &fakeOrderRepository{state: r.state}
}

func (r *fakeRepositories) TableSessions() domain.TableSessionRepository {
	return &fakeTableSessionRepository{state: r.state}
}

func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}

type fakeTableRepository struct{ state *fakeState }

var _ domain.TableRepository = (*fakeTableRepository)(nil)

func (r *fakeTableRepository) FindByID(_ context.Context, id domain.ID) (*domain.Table, error) {
	table, ok := r.state.tables[id]
	if !ok {
		return nil, notFoundErr("table_not_found", "table not found")
	}
	return table, nil
}

func (r *fakeTableRepository) FindByCode(_ context.Context, code string) (*domain.Table, error) {
	for _, table := range r.state.tables {
		if table.Code() == code {
			return table, nil
		}
	}
	return nil, notFoundErr("table_not_found", "table not found")
}

func (r *fakeTableRepository) FindAll(_ context.Context) ([]*domain.Table, error) {
	var out []*domain.Table
	for _, table := range r.state.tables {
		out = append(out, table)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code() < out[j].Code() })
	return out, nil
}

func (r *fakeTableRepository) Insert(_ context.Context, table *domain.Table) error {
	r.state.tables[table.ID()] = table
	return nil
}

func (r *fakeTableRepository) UpdateWithVersion(_ context.Context, table *domain.Table, _ int64) error {
	r.state.tables[table.ID()] = table
	return nil
}

type fakeOrderRepository struct{ state *fakeState }

var _ domain.OrderRepository = (*fakeOrderRepository)(nil)

func (r *fakeOrderRepository) FindByID(_ context.Context, id domain.ID) (*domain.Order, error) {
	order, ok := r.state.orders[id]
	if !ok {
		return nil, notFoundErr("order_not_found", "order not found")
	}
	return order, nil
}

func (r *fakeOrderRepository) FindAll(_ context.Context) ([]*domain.Order, error) {
	var out []*domain.Order
	for _, order := range r.state.orders {
		out = append(out, order)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt().After(out[j].CreatedAt()) })
	return out, nil
}

func (r *fakeOrderRepository) Insert(_ context.Context, order *domain.Order) error {
	r.state.orders[order.ID()] = order
	return nil
}

func (r *fakeOrderRepository) UpdateWithVersion(_ context.Context, order *domain.Order, _ int64) error {
	r.state.orders[order.ID()] = order
	return nil
}

type fakeTableSessionRepository struct{ state *fakeState }

var _ domain.TableSessionRepository = (*fakeTableSessionRepository)(nil)

func (r *fakeTableSessionRepository) Insert(_ context.Context, session domain.TableSession) error {
	r.state.tableSessions[session.TokenHash()] = session
	return nil
}

func (r *fakeTableSessionRepository) FindByTokenHash(_ context.Context, hash [32]byte) (*domain.TableSession, error) {
	session, ok := r.state.tableSessions[hash]
	if !ok {
		return nil, notFoundErr("table_session_not_found", "table session not found")
	}
	return &session, nil
}
