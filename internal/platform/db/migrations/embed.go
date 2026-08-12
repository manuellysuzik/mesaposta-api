// Package migrations embeds every goose SQL migration file mesaposta-api
// ships, so the compiled binary carries its own schema history and never
// depends on migration files being present on disk at deploy time. See
// internal/platform/db/migrate.go for the runner that applies these against
// a real database.
package migrations

import "embed"

// FS embeds every "*.sql" file in this directory (goose migration files,
// named "NNNN_description.sql" by convention). It is passed directly to
// goose.NewProvider as the migration source.
//
//go:embed *.sql
var FS embed.FS
