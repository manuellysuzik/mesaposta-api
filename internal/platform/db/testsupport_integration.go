//go:build integration

package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// AppRoleDSN is the DSN internal/platform/db's own integration tests (and
// the Postgres adapters built on top of Scope in
// internal/platform/outbox and internal/platform/auth/refresh) use to
// connect as the restricted mesaposta_app role provisioned by
// migrations/0001_baseline.sql -- the same role a real deployment's
// DATABASE_URL is expected to authenticate as.
const appRolePassword = "mesaposta_app_dev_only_change_me" // matches migrations/0001_baseline.sql; test/dev only.

// TestPostgres starts an ephemeral Postgres container, applies every
// embedded migration against it via Migrate (using the container's default
// superuser as the administrative role), and returns both that admin DSN
// and the restricted mesaposta_app application DSN the migrations
// provision.
//
// It is exported -- despite living in a package otherwise reserved for
// production code -- specifically so the outbox and auth/refresh Postgres
// adapters' own integration test suites (different packages) can reuse
// this exact bootstrap instead of duplicating testcontainers wiring three
// times over. This file is only compiled under -tags=integration, so it
// never reaches a production build, and testcontainers-go never becomes a
// production dependency.
func TestPostgres(t *testing.T) (adminURL, appURL string) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("mesaposta"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("db.TestPostgres: starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("db.TestPostgres: terminating postgres container: %v", err)
		}
	})

	adminURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("db.TestPostgres: building admin connection string: %v", err)
	}

	if err := Migrate(ctx, adminURL); err != nil {
		t.Fatalf("db.TestPostgres: running migrations: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("db.TestPostgres: resolving container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("db.TestPostgres: resolving mapped port: %v", err)
	}

	appURL = fmt.Sprintf("postgres://mesaposta_app:%s@%s:%s/mesaposta?sslmode=disable",
		appRolePassword, host, port.Port())

	return adminURL, appURL
}
