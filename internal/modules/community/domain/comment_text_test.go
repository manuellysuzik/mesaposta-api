package domain_test

import (
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewCommentText_Valid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "Muito bom!", "Muito bom!"},
		{"trims surrounding whitespace", "  ola mundo  ", "ola mundo"},
		{"single character", "a", "a"},
		{"exactly 2000 runes", strings.Repeat("a", 2000), strings.Repeat("a", 2000)},
		{"unicode and emoji", "café ☕ 🎉", "café ☕ 🎉"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewCommentText(tt.in)
			if err != nil {
				t.Fatalf("NewCommentText(%q): unexpected error: %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("NewCommentText(%q).String() = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

func TestNewCommentText_Invalid(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"only spaces", "   "},
		{"only tabs and newlines", "\t\n\t"},
		{"2001 runes", strings.Repeat("a", 2001)},
		{"2001 emoji runes", strings.Repeat("🎉", 2001)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewCommentText(tt.in)
			if err == nil {
				t.Fatalf("NewCommentText(%q): want error, got nil", tt.in)
			}
			if !errs.IsKind(err, errs.Invalid) {
				t.Errorf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
			}
		})
	}
}
