package refresh

import (
	"errors"
	"testing"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// fakeRow is a hand-rolled rowScanner used to unit test scanRecord without
// a real database connection.
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		panic("fakeRow: dest/values length mismatch")
	}
	for i, d := range dest {
		switch dst := d.(type) {
		case *string:
			*dst = r.values[i].(string)
		case *[]byte:
			*dst = r.values[i].([]byte)
		case *time.Time:
			*dst = r.values[i].(time.Time)
		case **time.Time:
			*dst, _ = r.values[i].(*time.Time)
		default:
			panic("fakeRow: unsupported dest type")
		}
	}
	return nil
}

func TestScanRecord_PropagatesScanError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := scanRecord(fakeRow{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("scanRecord error = %v, want %v", err, wantErr)
	}
}

func TestScanRecord_MapsColumnsIntoRecord(t *testing.T) {
	issuedAt := time.Now().UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(30 * 24 * time.Hour)
	hashBytes := make([]byte, 32)
	for i := range hashBytes {
		hashBytes[i] = byte(i)
	}

	row := fakeRow{values: []any{
		"11111111-1111-1111-1111-111111111111", // tenant_id
		hashBytes,                              // hash
		"family-1",                             // family_id
		"subject-1",                            // subject_id
		issuedAt,                               // issued_at
		expiresAt,                              // expires_at
		(*time.Time)(nil),                      // rotated_at
		(*time.Time)(nil),                      // revoked_at
	}}

	record, err := scanRecord(row)
	if err != nil {
		t.Fatalf("scanRecord: unexpected error: %v", err)
	}

	if record.TenantID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("record.TenantID = %q, want the tenant uuid string", record.TenantID)
	}
	if record.FamilyID != "family-1" {
		t.Errorf("record.FamilyID = %q, want %q", record.FamilyID, "family-1")
	}
	if record.SubjectID != "subject-1" {
		t.Errorf("record.SubjectID = %q, want %q", record.SubjectID, "subject-1")
	}
	if !record.IssuedAt.Equal(issuedAt) {
		t.Errorf("record.IssuedAt = %v, want %v", record.IssuedAt, issuedAt)
	}
	if !record.ExpiresAt.Equal(expiresAt) {
		t.Errorf("record.ExpiresAt = %v, want %v", record.ExpiresAt, expiresAt)
	}
	if record.RotatedAt != nil {
		t.Errorf("record.RotatedAt = %v, want nil", record.RotatedAt)
	}
	if record.RevokedAt != nil {
		t.Errorf("record.RevokedAt = %v, want nil", record.RevokedAt)
	}
	for i, b := range hashBytes {
		if record.Hash[i] != b {
			t.Fatalf("record.Hash[%d] = %d, want %d", i, record.Hash[i], b)
		}
	}
}

func TestNotFoundErr_HasNotFoundKind(t *testing.T) {
	err := notFoundErr(errors.New("no rows"))
	if err == nil {
		t.Fatal("notFoundErr: want a non-nil error")
	}
	if !errs.IsKind(err, errs.NotFound) {
		t.Errorf("errs.KindOf(notFoundErr(...)) = %v, want errs.NotFound", errs.KindOf(err))
	}
}

func TestAlreadyRotatedOrRevokedErr_HasConflictKind(t *testing.T) {
	err := alreadyRotatedOrRevokedErr()
	if !errs.IsKind(err, errs.Conflict) {
		t.Errorf("errs.KindOf(alreadyRotatedOrRevokedErr()) = %v, want errs.Conflict", errs.KindOf(err))
	}
}

func TestNewPostgresStore_SatisfiesRefreshStore(t *testing.T) {
	var _ RefreshStore = NewPostgresStore(nil)
}
