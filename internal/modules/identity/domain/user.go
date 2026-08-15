package domain

import (
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// User is a tenant's staff account: the aggregate root this whole module
// exists to introduce. It carries just enough to authenticate a login
// (Email, PasswordHash) and authorize what the resulting session may do
// (Role, IsActive) -- nothing about a person's name, contact details, or
// any other profile data, which is deliberately out of scope for this
// phase (see this module's own doc comment and the task notes this module
// was built from).
//
// User has no exported mutation method in this phase: there is no "change
// password", "change role", or "deactivate" use case yet (see
// ../application's own doc comment for the deliberately narrow set of use
// cases Phase 1 ships). Adding one later is a small, additive change to
// this file -- it does not require touching NewUser's validation or any of
// this type's existing accessors.
type User struct {
	id           ID
	tenantID     tenant.TenantID
	email        string
	passwordHash string
	role         Role
	isActive     bool
	createdAt    time.Time
	updatedAt    time.Time
}

// NewUser builds a brand-new User, validating email (non-blank, no
// whitespace, contains exactly one "@" with a non-empty local part and
// domain part -- a deliberately lightweight shape check, not a full RFC
// 5322 validator: the database's own UNIQUE constraint on users.email, see
// internal/platform/db/migrations/0011_users.sql, is this system's real
// defense against a malformed/duplicate email, exactly as
// internal/modules/crm/domain.NewCustomer's own doc comment explains for
// its email field), passwordHash (non-blank -- this constructor takes an
// ALREADY-HASHED value; hashing the plaintext password is
// ../application/create_user.go's job, via
// internal/platform/auth/password.Hash, never this package's, per
// architecture rule R1) and role (must be one of the closed Role set).
//
// A freshly constructed User always starts out active (IsActive() == true)
// at the current time -- there is no "invite a deactivated user" use case.
func NewUser(id ID, tenantID tenant.TenantID, email, passwordHash string, role Role) (User, error) {
	if err := validateEmail(email); err != nil {
		return User{}, err
	}
	if strings.TrimSpace(passwordHash) == "" {
		return User{}, errs.New(errs.Invalid, "invalid_user_password_hash",
			"user password hash must not be empty", nil)
	}
	if !role.Valid() {
		return User{}, errs.New(errs.Invalid, "invalid_user_role", "user role must be a known, non-empty value", nil)
	}

	now := time.Now().UTC()
	return User{
		id:           id,
		tenantID:     tenantID,
		email:        email,
		passwordHash: passwordHash,
		role:         role,
		isActive:     true,
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

// validateEmail applies NewUser's deliberately lightweight email shape
// check -- see NewUser's own doc comment for why this is not a full RFC
// 5322 validator.
func validateEmail(email string) error {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return errs.New(errs.Invalid, "invalid_user_email", "user email must not be empty", nil)
	}
	if trimmed != email {
		return errs.New(errs.Invalid, "invalid_user_email", "user email must not have leading/trailing whitespace", nil)
	}
	if strings.ContainsAny(email, " \t\n\r") {
		return errs.New(errs.Invalid, "invalid_user_email", "user email must not contain whitespace", nil)
	}
	at := strings.Index(email, "@")
	if at <= 0 || at == len(email)-1 || strings.Contains(email[at+1:], "@") {
		return errs.New(errs.Invalid, "invalid_user_email",
			"user email must contain exactly one '@' with a non-empty local part and domain part", nil)
	}
	return nil
}

// ID returns the user's identity.
func (u User) ID() ID { return u.id }

// TenantID returns the tenant the user belongs to.
func (u User) TenantID() tenant.TenantID { return u.tenantID }

// Email returns the user's login email.
func (u User) Email() string { return u.email }

// PasswordHash returns the user's Argon2id PHC-encoded password hash. This
// is exported purely so ../infrastructure/postgres can persist it -- no
// application-layer code should ever compare a plaintext password against
// this value directly; use internal/platform/auth/password.Verify.
func (u User) PasswordHash() string { return u.passwordHash }

// Role returns the user's role within TenantID.
func (u User) Role() Role { return u.role }

// IsActive reports whether this user is currently allowed to authenticate.
func (u User) IsActive() bool { return u.isActive }

// CreatedAt returns the timestamp the user was created at.
func (u User) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns the timestamp of the user's last update (or its
// construction time, if never updated).
func (u User) UpdatedAt() time.Time { return u.updatedAt }
