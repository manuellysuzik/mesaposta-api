package domain_test

import (
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

const testTenantID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func mustTestTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(testTenantID)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", testTenantID, err)
	}
	return id
}

func TestNewCommunityPost_Valid_TrimsContentAndStampsCreatedAt(t *testing.T) {
	before := time.Now().UTC()

	post, err := domain.NewCommunityPost(domain.NewID(), mustTestTenantID(t), domain.NewID(), "  Bom dia, equipe!  ")

	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewCommunityPost: unexpected error: %v", err)
	}
	if post.Content() != "Bom dia, equipe!" {
		t.Errorf("Content() = %q, want %q", post.Content(), "Bom dia, equipe!")
	}
	if post.CreatedAt().Before(before) || post.CreatedAt().After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", post.CreatedAt(), before, after)
	}
}

func TestNewCommunityPost_ZeroAuthorID_ReturnsInvalid(t *testing.T) {
	_, err := domain.NewCommunityPost(domain.NewID(), mustTestTenantID(t), domain.ID{}, "conteudo valido")

	if !errs.IsKind(err, errs.Invalid) {
		t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
	}
}

func TestNewCommunityPost_EmptyOrWhitespaceContent_ReturnsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty string", ""},
		{"only spaces", "    "},
		{"only tabs and newlines", "\t\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewCommunityPost(domain.NewID(), mustTestTenantID(t), domain.NewID(), tt.content)
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
			}
		})
	}
}

func TestCommunityPost_Getters(t *testing.T) {
	id := domain.NewID()
	tenantID := mustTestTenantID(t)
	authorID := domain.NewID()

	post, err := domain.NewCommunityPost(id, tenantID, authorID, "conteudo")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}

	if post.ID() != id {
		t.Errorf("ID() = %v, want %v", post.ID(), id)
	}
	if post.TenantID() != tenantID {
		t.Errorf("TenantID() = %v, want %v", post.TenantID(), tenantID)
	}
	if post.AuthorID() != authorID {
		t.Errorf("AuthorID() = %v, want %v", post.AuthorID(), authorID)
	}
}
