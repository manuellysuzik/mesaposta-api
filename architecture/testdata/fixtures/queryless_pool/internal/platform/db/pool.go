// Package db is a fixture standing in for the real
// internal/platform/db package (see internal/platform/db/pool.go in the
// real module). It reproduces that package's real, deliberate design
// constraint: Pool exposes no Query/Exec/QueryRow method and never hands
// out a raw database handle -- the ONLY way to reach the database is
// Scope(ctx), which this fixture also stubs out faithfully (its Scope type
// DOES expose Query, mirroring the real db.Scope), so the compile failure
// this fixture exists to prove is specifically about Pool, not merely
// "this package has no query methods anywhere at all".
package db

import "context"

// Pool mirrors the real db.Pool's public surface: Open/Scope/Ping/Close/
// Stat, and deliberately nothing else.
type Pool struct{}

// Open mirrors db.Open.
func Open(ctx context.Context, databaseURL string) (*Pool, error) {
	return &Pool{}, nil
}

// Scope mirrors db.Pool.Scope: the only way to obtain something with
// Query/Exec/QueryRow methods.
func (p *Pool) Scope(ctx context.Context) (*Scope, error) {
	return &Scope{}, nil
}

// Ping mirrors db.Pool.Ping.
func (p *Pool) Ping(ctx context.Context) error {
	return nil
}

// Close mirrors db.Pool.Close.
func (p *Pool) Close() {}

// Stat mirrors db.Pool.Stat.
func (p *Pool) Stat() Stat {
	return Stat{}
}

// Stat is a minimal stand-in for pgxpool.Stat.
type Stat struct{}

// Scope mirrors db.Scope: it DOES expose Query/Exec/QueryRow, proving the
// fixture's failure is about calling those methods on a *Pool specifically,
// not about the fixture module lacking any query capability whatsoever.
type Scope struct{}

// Query mirrors db.Scope.Query.
func (s *Scope) Query(ctx context.Context, sql string, args ...any) error {
	return nil
}
