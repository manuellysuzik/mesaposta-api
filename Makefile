.PHONY: check fmt vet lint test seed-tenant migrate-status migrate-up migrate-down migrate-create

check: fmt vet lint test

fmt:
	@echo "==> gofmt"
	@bash -c 'files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "$$files"; echo "gofmt: files need formatting"; exit 1; fi'

vet:
	@echo "==> go vet"
	go vet ./...

lint:
	@echo "==> golangci-lint"
	golangci-lint run

test:
	@echo "==> go test"
	# -count=1 disables the test result cache. This matters specifically for
	# ./architecture/...: its tests load packages from disk at run time via
	# golang.org/x/tools/go/packages (a dynamic filesystem read Go's build
	# cache does not track as a test input), so a cached "ok" could mask a
	# real, newly introduced architecture violation elsewhere in the repo.
	go test -race -cover -count=1 ./...

# seed-tenant provisions a new row in the tenants table (see
# cmd/seedtenant's own doc comment for why this can't happen through the
# application's runtime request path). DATABASE_URL must carry a role with
# INSERT privilege on tenants -- the same administrative connection string
# used for migrations, not the mesaposta_app runtime credential. TENANT_ID
# is optional; a random UUID is generated when it's omitted.
#
# Usage:
#   make seed-tenant DATABASE_URL="postgres://..." SLUG=acme-burger NAME="Acme Burger"
seed-tenant:
	@if [ -z "$(DATABASE_URL)" ] || [ -z "$(SLUG)" ] || [ -z "$(NAME)" ]; then \
		echo "usage: make seed-tenant DATABASE_URL=... SLUG=... NAME=... [TENANT_ID=...]"; \
		exit 2; \
	fi
	go run ./cmd/seedtenant \
		-database-url "$(DATABASE_URL)" \
		-slug "$(SLUG)" \
		-name "$(NAME)" \
		$(if $(TENANT_ID),-tenant-id "$(TENANT_ID)",)

# migrate-status / migrate-up / migrate-down drive the `goose` CLI (brew
# install goose) directly against internal/platform/db/migrations, for local
# inspection/troubleshooting outside the application's own boot path -- see
# internal/platform/db/migrate.go's doc comment for the Go-side equivalent
# (db.Migrate / db.MigrateDown), which is what actually runs in production
# via MIGRATE_ON_BOOT (see README.md, "Migrations"). MIGRATE_DATABASE_URL
# must carry the same elevated, administrative privileges db.Migrate's own
# doc comment requires (CREATE ROLE/EXTENSION/TABLE, ownership of every
# table it creates) -- never the RLS-restricted runtime DATABASE_URL.
#
# Usage:
#   make migrate-status MIGRATE_DATABASE_URL="postgres://..."
#   make migrate-up     MIGRATE_DATABASE_URL="postgres://..."
#   make migrate-down   MIGRATE_DATABASE_URL="postgres://..."   # reverts exactly one migration
migrate-status:
	@if [ -z "$(MIGRATE_DATABASE_URL)" ]; then \
		echo "usage: make migrate-status MIGRATE_DATABASE_URL=..."; \
		exit 2; \
	fi
	goose -dir internal/platform/db/migrations postgres "$(MIGRATE_DATABASE_URL)" status

migrate-up:
	@if [ -z "$(MIGRATE_DATABASE_URL)" ]; then \
		echo "usage: make migrate-up MIGRATE_DATABASE_URL=..."; \
		exit 2; \
	fi
	goose -dir internal/platform/db/migrations postgres "$(MIGRATE_DATABASE_URL)" up

migrate-down:
	@if [ -z "$(MIGRATE_DATABASE_URL)" ]; then \
		echo "usage: make migrate-down MIGRATE_DATABASE_URL=..."; \
		exit 2; \
	fi
	goose -dir internal/platform/db/migrations postgres "$(MIGRATE_DATABASE_URL)" down

# migrate-create scaffolds a new, empty migration file under
# internal/platform/db/migrations, continuing this repo's existing
# sequential (0001, 0002, ...) numbering. goose's sequential mode zero-pads
# to 5 digits (e.g. 00012_name.sql) -- rename the generated file to match
# this repo's existing 4-digit convention (0012_name.sql) before committing.
#
# Usage:
#   make migrate-create NAME=add_foo_table
migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "usage: make migrate-create NAME=..."; \
		exit 2; \
	fi
	goose -dir internal/platform/db/migrations -s create "$(NAME)" sql
