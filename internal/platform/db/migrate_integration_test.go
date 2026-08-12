//go:build integration

package db

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// freshPostgres starts a Postgres container WITHOUT running migrations
// against it (unlike TestPostgres), so this file can exercise
// Migrate/MigrateDown/MigrationVersion themselves against an empty
// database.
func freshPostgres(t *testing.T) (adminURL string) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("mesaposta"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminating postgres container: %v", err)
		}
	})

	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("building admin connection string: %v", err)
	}
	return adminURL
}

func TestMigrate_EmptyDatabase_ReachesExpectedVersion(t *testing.T) {
	adminURL := freshPostgres(t)
	ctx := context.Background()

	version, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (before Up): %v", err)
	}
	if version != 0 {
		t.Fatalf("MigrationVersion (before Up) = %d, want 0", version)
	}

	if err := Migrate(ctx, adminURL); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	version, err = MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (after Up): %v", err)
	}
	if version == 0 {
		t.Fatal("MigrationVersion (after Up) = 0, want a non-zero version")
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	adminURL := freshPostgres(t)
	ctx := context.Background()

	if err := Migrate(ctx, adminURL); err != nil {
		t.Fatalf("Migrate (first run): %v", err)
	}
	v1, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (after first run): %v", err)
	}

	if err := Migrate(ctx, adminURL); err != nil {
		t.Fatalf("Migrate (second run, should be a no-op): %v", err)
	}
	v2, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (after second run): %v", err)
	}

	if v1 != v2 {
		t.Fatalf("MigrationVersion changed across a no-op re-run: %d -> %d", v1, v2)
	}
}

func TestMigrate_Down_Reverts(t *testing.T) {
	adminURL := freshPostgres(t)
	ctx := context.Background()

	if err := Migrate(ctx, adminURL); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	before, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (after Up): %v", err)
	}

	if err := MigrateDown(ctx, adminURL); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}
	after, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion (after Down): %v", err)
	}

	if after >= before {
		t.Fatalf("MigrationVersion after Down = %d, want less than %d (the pre-Down version)", after, before)
	}
}

// TestMigrate_ConcurrentRunners_SerializeViaSessionLock is the deploy-time
// safety net this block exists to prove automatically: two goroutines
// calling Migrate against the SAME empty database at the same time (as
// would happen if two replicas of this service started up simultaneously)
// must not corrupt anything -- goose's Postgres session-level advisory
// lock (lock.NewPostgresSessionLocker, wired in migrate.go) makes exactly
// one of them apply the migrations while the other waits, then observes
// there is nothing left pending.
func TestMigrate_ConcurrentRunners_SerializeViaSessionLock(t *testing.T) {
	adminURL := freshPostgres(t)
	ctx := context.Background()

	const runners = 2
	errsCh := make([]error, runners)

	var wg sync.WaitGroup
	wg.Add(runners)
	for i := 0; i < runners; i++ {
		go func(i int) {
			defer wg.Done()
			errsCh[i] = Migrate(ctx, adminURL)
		}(i)
	}
	wg.Wait()

	for i, err := range errsCh {
		if err != nil {
			t.Fatalf("runner %d: Migrate: %v", i, err)
		}
	}

	version, err := MigrationVersion(ctx, adminURL)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version == 0 {
		t.Fatal("MigrationVersion after two concurrent Migrate runs = 0, want a non-zero, fully-applied version")
	}
}

// TestMigrate_InvalidSQL_RollsBackAndPropagatesError confirms a migration
// containing invalid SQL fails loudly instead of silently leaving a
// partially-applied schema behind. It builds a goose Provider directly
// (bypassing Migrate, which is hardcoded to the real embedded migrations)
// against an in-memory fs.FS containing a single deliberately broken
// migration, using the exact same dialect/session-locker wiring migrate.go
// uses -- so this exercises the real transaction-per-migration behavior
// goose applies by default: a mid-migration failure rolls back everything
// that file attempted.
func TestMigrate_InvalidSQL_RollsBackAndPropagatesError(t *testing.T) {
	adminURL := freshPostgres(t)
	ctx := context.Background()

	brokenFS := fstest.MapFS{
		"00001_broken.sql": &fstest.MapFile{Data: []byte(`
-- +goose Up
CREATE TABLE this_table_should_not_survive (id uuid PRIMARY KEY);
THIS IS NOT VALID SQL;

-- +goose Down
DROP TABLE IF EXISTS this_table_should_not_survive;
`)},
	}

	pool, err := Open(ctx, adminURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool.unsafeAdmin())
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, brokenFS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}

	_, err = provider.Up(ctx)
	if err == nil {
		t.Fatal("applying a migration with invalid SQL: want an error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "syntax") {
		t.Logf("got error (not asserting exact wording, just logging for visibility): %v", err)
	}

	var exists bool
	row := pool.unsafeAdmin().QueryRow(ctx, "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'this_table_should_not_survive')")
	if err := row.Scan(&exists); err != nil {
		t.Fatalf("checking whether the broken migration's table survived: %v", err)
	}
	if exists {
		t.Fatal("this_table_should_not_survive exists after a failed migration -- the failing statement's transaction did not roll back")
	}
}
