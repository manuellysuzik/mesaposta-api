package domain_test

import (
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

func TestNewQuantity(t *testing.T) {
	tests := []struct {
		name    string
		count   int64
		wantErr bool
	}{
		{"positive", 5, false},
		{"one", 1, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := domain.NewQuantity(tt.count)
			if tt.wantErr {
				if !errs.IsKind(err, errs.Invalid) {
					t.Fatalf("errs.KindOf(err) = %v, want errs.Invalid", errs.KindOf(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("NewQuantity(%d): unexpected error: %v", tt.count, err)
			}
			if q.Count() != tt.count {
				t.Errorf("Count() = %d, want %d", q.Count(), tt.count)
			}
		})
	}
}
