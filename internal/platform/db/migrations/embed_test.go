package migrations_test

import (
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/db/migrations"
)

// TestEveryMigrationHasAGooseDownDirective requires no database at all: it
// walks the embedded migration files and confirms every single one follows
// goose's reversible-migration convention (a "-- +goose Down" section), so
// a migration that can never be rolled back can't silently sneak into the
// codebase.
func TestEveryMigrationHasAGooseDownDirective(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("reading embedded migrations dir: %v", err)
	}

	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		sqlFiles = append(sqlFiles, entry.Name())
	}

	if len(sqlFiles) == 0 {
		t.Fatal("expected at least one embedded .sql migration file, found none")
	}

	for _, name := range sqlFiles {
		name := name
		t.Run(name, func(t *testing.T) {
			content, err := migrations.FS.ReadFile(name)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			if !strings.Contains(string(content), "-- +goose Down") {
				t.Errorf("%s is missing a \"-- +goose Down\" section: every migration must be reversible", name)
			}
			if !strings.Contains(string(content), "-- +goose Up") {
				t.Errorf("%s is missing a \"-- +goose Up\" section", name)
			}
		})
	}
}
