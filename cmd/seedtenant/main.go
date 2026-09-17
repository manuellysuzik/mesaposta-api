// Command seedtenant inserts a single row into the tenants table directly
// via Postgres, so a developer can provision a new tenant to exercise the
// public, slug-addressed digital-menu routes under /public/v1/{tenantSlug}
// (which resolve a real tenants row via db.Pool.LookupTenantBySlug), or to
// obtain a real tenant id to hand to cmd/seeduser.
//
// This is a one-off administrative tool, not a production code path: as
// documented on internal/platform/db/migrations/0008_tenants.sql, the
// mesaposta_app runtime role has no INSERT grant on tenants by design --
// provisioning a tenant is deliberately kept outside the application's
// request path. Because of that, -database-url here must carry a role with
// INSERT privilege on tenants (e.g. the same administrative connection
// string used for db.Migrate), not the mesaposta_app runtime credential.
//
// Usage:
//
//	go run ./cmd/seedtenant \
//	  -database-url "postgres://mesaposta:mesaposta@localhost:5432/mesaposta?sslmode=disable" \
//	  -slug acme-burger \
//	  -name "Acme Burger"
//
// -tenant-id is optional: when omitted, a brand-new random UUID is
// generated and printed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	identitydomain "github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
)

const insertTenantSQL = `
	INSERT INTO tenants (id, slug, name, is_active, created_at)
	VALUES ($1, $2, $3, $4, $5)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("seedtenant", flag.ContinueOnError)
	databaseURL := fs.String("database-url", "", "Postgres connection string with INSERT privilege on tenants (required)")
	slug := fs.String("slug", "", "public, URL-carried slug for the new tenant (required)")
	name := fs.String("name", "", "display name for the new tenant (required)")
	active := fs.Bool("active", true, "whether the new tenant is active")
	tenantIDFlag := fs.String("tenant-id", "", "UUID to use for the new tenant (optional -- a random one is generated if omitted)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *databaseURL == "" || *slug == "" || *name == "" {
		fmt.Fprintln(stderr, "seedtenant: -database-url, -slug and -name are all required")
		return 2
	}

	tenantID := *tenantIDFlag
	if tenantID == "" {
		tenantID = identitydomain.NewID().String()
	} else if _, err := identitydomain.ParseID(tenantID); err != nil {
		fmt.Fprintf(stderr, "seedtenant: invalid -tenant-id %q: %v\n", tenantID, err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, *databaseURL)
	if err != nil {
		fmt.Fprintln(stderr, "seedtenant: connecting to database:", err)
		return 1
	}
	defer conn.Close(ctx)

	now := time.Now().UTC()

	if _, err := conn.Exec(ctx, insertTenantSQL, tenantID, *slug, *name, *active, now); err != nil {
		fmt.Fprintln(stderr, "seedtenant: inserting tenant:", err)
		return 1
	}

	fmt.Fprintf(stdout, "seedtenant: created tenant %q (id=%s, slug=%s, active=%t)\n", *name, tenantID, *slug, *active)
	return 0
}
