//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresadapter "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/postgres"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/outbox"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// openTestPool starts an ephemeral, fully-migrated Postgres container (via
// db.TestPostgres, the same helper every other module's own integration
// suite already reuses) and returns a Pool connected as the restricted
// mesaposta_app role.
func openTestPool(t *testing.T) *db.Pool {
	t.Helper()
	_, appURL := db.TestPostgres(t)
	pool, err := db.Open(context.Background(), appURL)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mustTenantID parses s (a canonical UUID literal) into a tenant.TenantID,
// failing the test on any error.
func mustTenantID(t *testing.T, s string) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(s)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", s, err)
	}
	return id
}

func mustCommentText(t *testing.T, raw string) domain.CommentText {
	t.Helper()
	text, err := domain.NewCommentText(raw)
	if err != nil {
		t.Fatalf("NewCommentText(%q): %v", raw, err)
	}
	return text
}

// mustPost builds and inserts a valid, ready-to-use CommunityPost for
// tenantID via the given repository, failing the test on any error.
func mustPost(t *testing.T, ctx context.Context, repo *postgresadapter.CommunityPostRepository, tenantID tenant.TenantID) *domain.CommunityPost {
	t.Helper()
	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, domain.NewID(), "conteudo de teste")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}
	if err := repo.Insert(ctx, &post); err != nil {
		t.Fatalf("Insert(post): %v", err)
	}
	return &post
}

// buildTestDomainEvent builds a minimal, valid outbox.DomainEvent for
// tenantID/aggregateID, for tests that only care about a domain event's
// presence/tenant scoping, not its specific payload.
func buildTestDomainEvent(t *testing.T, tenantID tenant.TenantID, aggregateID string) (outbox.DomainEvent, error) {
	t.Helper()
	return outbox.NewDomainEvent(tenantID, "post.commented", aggregateID, time.Now().UTC(), map[string]any{"test": true})
}
