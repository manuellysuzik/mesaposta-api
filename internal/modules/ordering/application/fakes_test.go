package application_test

import (
	"context"
	"sort"
	"sync"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/audit"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// This file defines a hand-written, in-memory implementation of
// domain.UnitOfWork/domain.Repositories, following the same
// fake-implements-the-port pattern every other module's own fakes_test.go
// already establishes in this codebase (see e.g.
// internal/modules/crm/application/fakes_test.go). It is intentionally
// unexported and lives only in this (application_test) test package.
//
// The single most important property this fake must have -- the one the
// rest of this package's tests lean on to prove atomicity without a real
// Postgres transaction -- is that fakeUnitOfWork.Do simulates a genuine
// rollback: it operates on a CLONE of its state, and only replaces the
// real state with that clone if fn returns nil. See
// fake_unit_of_work_test.go, which pins that behavior down directly.

type fakeState struct {
	tables        map[domain.ID]*domain.Table
	orders        map[domain.ID]*domain.Order
	tableSessions map[[32]byte]domain.TableSession
	auditEntries  []audit.Entry
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
		auditEntries:  append([]audit.Entry{}, s.auditEntries...),
	}
	for id, table := range s.tables {
		tableCopy := *table
		clone.tables[id] = &tableCopy
	}
	for id, order := range s.orders {
		orderCopy := *order
		clone.orders[id] = &orderCopy
	}
	for hash, session := range s.tableSessions {
		clone.tableSessions[hash] = session
	}
	return clone
}

// fakeRepoErrors lets a test inject a canned error to be returned by a
// specific repository method, on every call, for the duration of a single
// fakeUnitOfWork.Do invocation.
type fakeRepoErrors struct {
	tableFindByID          error
	tableFindByCode        error
	tableInsert            error
	tableUpdateWithVersion error
	orderFindByID          error
	orderInsert            error
	orderUpdateWithVersion error
	sessionFindByTokenHash error
	sessionInsert          error
	auditRecord            error
}

// fakeUnitOfWork is an in-memory domain.UnitOfWork. Its zero value is not
// usable; construct one via newFakeUnitOfWork.
type fakeUnitOfWork struct {
	mu    sync.Mutex
	state *fakeState

	errors fakeRepoErrors
}

func newFakeUnitOfWork(state *fakeState) *fakeUnitOfWork {
	return &fakeUnitOfWork{state: state}
}

var _ domain.UnitOfWork = (*fakeUnitOfWork)(nil)

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

func (u *fakeUnitOfWork) snapshot() *fakeState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.clone()
}

type fakeRepositories struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.Repositories = (*fakeRepositories)(nil)

func (r *fakeRepositories) Tables() domain.TableRepository {
	return &fakeTableRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Orders() domain.OrderRepository {
	return &fakeOrderRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) TableSessions() domain.TableSessionRepository {
	return &fakeTableSessionRepository{uow: r.uow, state: r.state}
}

func (r *fakeRepositories) Audit() audit.Recorder {
	return &fakeAuditRecorder{uow: r.uow, state: r.state}
}

// --- TableRepository -----------------------------------------------------

type fakeTableRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.TableRepository = (*fakeTableRepository)(nil)

func notFoundErr(code, msg string) error {
	return errs.New(errs.NotFound, code, msg, nil)
}

func (r *fakeTableRepository) FindByID(_ context.Context, id domain.ID) (*domain.Table, error) {
	if err := r.uow.errors.tableFindByID; err != nil {
		return nil, err
	}
	table, ok := r.state.tables[id]
	if !ok {
		return nil, notFoundErr("table_not_found", "table not found")
	}
	return table, nil
}

func (r *fakeTableRepository) FindByCode(_ context.Context, code string) (*domain.Table, error) {
	if err := r.uow.errors.tableFindByCode; err != nil {
		return nil, err
	}
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
	if err := r.uow.errors.tableInsert; err != nil {
		return err
	}
	r.state.tables[table.ID()] = table
	return nil
}

func (r *fakeTableRepository) UpdateWithVersion(_ context.Context, table *domain.Table, _ int64) error {
	if err := r.uow.errors.tableUpdateWithVersion; err != nil {
		return err
	}
	r.state.tables[table.ID()] = table
	return nil
}

// --- OrderRepository -------------------------------------------------------

type fakeOrderRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.OrderRepository = (*fakeOrderRepository)(nil)

func (r *fakeOrderRepository) FindByID(_ context.Context, id domain.ID) (*domain.Order, error) {
	if err := r.uow.errors.orderFindByID; err != nil {
		return nil, err
	}
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
	if err := r.uow.errors.orderInsert; err != nil {
		return err
	}
	r.state.orders[order.ID()] = order
	return nil
}

func (r *fakeOrderRepository) UpdateWithVersion(_ context.Context, order *domain.Order, _ int64) error {
	if err := r.uow.errors.orderUpdateWithVersion; err != nil {
		return err
	}
	r.state.orders[order.ID()] = order
	return nil
}

// --- TableSessionRepository ------------------------------------------------

type fakeTableSessionRepository struct {
	uow   *fakeUnitOfWork
	state *fakeState
}

var _ domain.TableSessionRepository = (*fakeTableSessionRepository)(nil)

func (r *fakeTableSessionRepository) Insert(_ context.Context, session domain.TableSession) error {
	if err := r.uow.errors.sessionInsert; err != nil {
		return err
	}
	r.state.tableSessions[session.TokenHash()] = session
	return nil
}

func (r *fakeTableSessionRepository) FindByTokenHash(_ context.Context, hash [32]byte) (*domain.TableSession, error) {
	if err := r.uow.errors.sessionFindByTokenHash; err != nil {
		return nil, err
	}
	session, ok := r.state.tableSessions[hash]
	if !ok {
		return nil, notFoundErr("table_session_not_found", "table session not found")
	}
	return &session, nil
}

// --- audit.Recorder -----------------------------------------------------

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
