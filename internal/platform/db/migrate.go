package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db/migrations"
)

// Migrate applies every pending "up" migration embedded in
// internal/platform/db/migrations against the database identified by
// adminDatabaseURL, returning once the schema is fully up to date.
//
// adminDatabaseURL is expected to carry elevated privileges (able to CREATE
// ROLE / CREATE EXTENSION / CREATE TABLE, and to own every table it
// creates) -- it must NOT be the same, RLS-restricted DSN passed to Open
// for the application's runtime Pool. See migrations/0001_baseline.sql for
// why: mesaposta_app is deliberately created without BYPASSRLS, and table
// ownership by an unrelated administrative role is what makes that
// attribute actually enforce anything.
//
// Migrate takes out a Postgres session-level advisory lock for the
// duration of the run (goose's session locker, backed by
// pg_advisory_lock/pg_advisory_unlock), so multiple replicas invoking
// Migrate concurrently at deploy time serialize instead of racing: exactly
// one applies the pending migrations while the others block, then observe
// -- once they acquire the lock in turn -- that there is nothing left to
// do.
func Migrate(ctx context.Context, adminDatabaseURL string) error {
	return withMigrationProvider(ctx, adminDatabaseURL, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// MigrateDown reverts exactly one previously applied migration (goose's
// single-step "down" semantics), under the same session-lock protection as
// Migrate.
func MigrateDown(ctx context.Context, adminDatabaseURL string) error {
	return withMigrationProvider(ctx, adminDatabaseURL, func(p *goose.Provider) error {
		_, err := p.Down(ctx)
		return err
	})
}

// MigrationVersion reports the database's current applied migration
// version (0 if none have been applied yet).
func MigrationVersion(ctx context.Context, adminDatabaseURL string) (int64, error) {
	var version int64
	err := withMigrationProvider(ctx, adminDatabaseURL, func(p *goose.Provider) error {
		v, err := p.GetDBVersion(ctx)
		version = v
		return err
	})
	return version, err
}

// withMigrationProvider opens an administrative Pool against
// adminDatabaseURL, builds a goose.Provider over it (wired to this
// package's embedded migrations.FS and a Postgres session locker), invokes
// run, and tears everything back down -- regardless of whether run
// succeeded.
func withMigrationProvider(ctx context.Context, adminDatabaseURL string, run func(*goose.Provider) error) error {
	pool, err := Open(ctx, adminDatabaseURL)
	if err != nil {
		return fmt.Errorf("db: opening administrative connection for migrations: %w", err)
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool.unsafeAdmin())
	defer sqlDB.Close()

	sessionLocker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("db: building migration session locker: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS, goose.WithSessionLocker(sessionLocker))
	if err != nil {
		return fmt.Errorf("db: building migration provider: %w", err)
	}

	if err := run(provider); err != nil {
		return fmt.Errorf("db: running migrations: %w", err)
	}
	return nil
}
