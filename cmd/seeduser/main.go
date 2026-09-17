// Command seeduser inserts a single, real user row directly into Postgres,
// with a correctly-computed Argon2id password hash, so a developer can
// exercise POST /v1/auth/login end-to-end without the (not-yet-built)
// POST /v1/users administrative endpoint.
//
// This is a one-off administrative tool, not a production code path: it
// connects to Postgres directly via pgx (bypassing internal/platform/db.Pool
// entirely, since that package deliberately refuses to run any query
// without an already-authenticated tenant.TenantID in hand -- see its own
// package doc comment), and sets the "app.tenant_id" row-level-security GUC
// for its own INSERT transaction by hand.
//
// Usage:
//
//	go run ./cmd/seeduser \
//	  -database-url "postgres://mesaposta:mesaposta@localhost:5432/mesaposta?sslmode=disable" \
//	  -email manager@example.com \
//	  -password "correct-horse-battery-staple" \
//	  -role manager
//
// -tenant-id is optional: when omitted, a brand-new random UUID is
// generated and printed, since this tool has no way to provision a real row
// in the tenants table (mesaposta_app has no INSERT grant there by design --
// see internal/platform/db/migrations/0008_tenants.sql's own doc comment).
// A synthetic tenant id is sufficient for every authenticated /v1/** route
// (which scopes purely by the tenant_id row-level-security GUC), but NOT
// for the public, slug-addressed digital-menu routes under
// /public/v1/{tenantSlug}, which resolve a real tenants row.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	identitydomain "github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/password"
)

const insertUserSQL = `
	INSERT INTO users (id, tenant_id, email, password_hash, role, is_active, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, true, $6, $6)
`

const setTenantGUCSQL = `SELECT set_config('app.tenant_id', $1, true)`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("seeduser", flag.ContinueOnError)
	databaseURL := fs.String("database-url", "", "Postgres connection string (required)")
	email := fs.String("email", "", "login email for the new user (required)")
	plainPassword := fs.String("password", "", "plaintext password for the new user (required)")
	role := fs.String("role", "", "manager or employee (required)")
	tenantIDFlag := fs.String("tenant-id", "", "existing tenant UUID to attach the user to (optional -- a random one is generated if omitted)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *databaseURL == "" || *email == "" || *plainPassword == "" || *role == "" {
		fmt.Fprintln(stderr, "seeduser: -database-url, -email, -password and -role are all required")
		return 2
	}

	userRole, err := identitydomain.ParseRole(*role)
	if err != nil {
		fmt.Fprintf(stderr, "seeduser: invalid -role %q: %v\n", *role, err)
		return 2
	}

	tenantID := *tenantIDFlag
	if tenantID == "" {
		tenantID = identitydomain.NewID().String()
		fmt.Fprintf(stdout, "seeduser: no -tenant-id given, generated a new one: %s\n", tenantID)
	} else if _, err := identitydomain.ParseID(tenantID); err != nil {
		fmt.Fprintf(stderr, "seeduser: invalid -tenant-id %q: %v\n", tenantID, err)
		return 2
	}

	passwordHash, err := password.Hash(*plainPassword)
	if err != nil {
		fmt.Fprintln(stderr, "seeduser: hashing password:", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, *databaseURL)
	if err != nil {
		fmt.Fprintln(stderr, "seeduser: connecting to database:", err)
		return 1
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "seeduser: beginning transaction:", err)
		return 1
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once tx.Commit succeeds below

	// Sets app.tenant_id for THIS transaction only (set_config's third
	// argument, is_local=true), matching internal/platform/db.Scope's own
	// convention -- required for the users table's "tenant_isolation" RLS
	// policy's WITH CHECK to accept this INSERT (see 0011_users.sql's own
	// doc comment).
	if _, err := tx.Exec(ctx, setTenantGUCSQL, tenantID); err != nil {
		fmt.Fprintln(stderr, "seeduser: setting app.tenant_id:", err)
		return 1
	}

	userID := identitydomain.NewID().String()
	now := time.Now().UTC()

	if _, err := tx.Exec(ctx, insertUserSQL, userID, tenantID, *email, passwordHash, string(userRole), now); err != nil {
		fmt.Fprintln(stderr, "seeduser: inserting user:", err)
		return 1
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintln(stderr, "seeduser: committing transaction:", err)
		return 1
	}

	fmt.Fprintf(stdout, "seeduser: created user %s (id=%s, tenant_id=%s, role=%s)\n", *email, userID, tenantID, userRole)
	return 0
}
