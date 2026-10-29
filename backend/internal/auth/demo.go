package auth

import (
	"context"
	"errors"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Demo quick login lets the owner open each demo role with one click while
// testing. It is off unless the API starts with DEMO_QUICK_LOGIN=true, works
// only for the accounts seed-demo created (recorded in demo_records, at the
// demo address) and never handles a password.

// DemoDomain is the address seed-demo uses for its accounts (demo.Domain;
// repeated here because the demo package imports auth).
const DemoDomain = "demo.flybirds.test"

// DemoQuickLoginAccounts maps each role offered on the sign-in form to the
// seed-demo account reference. Tests may replace it.
var DemoQuickLoginAccounts = map[string]string{
	"admin":        "demo.admin",
	"head_teacher": "demo.head",
	"teacher":      "demo.teacher",
	"accountant":   "demo.accounts",
	"student":      "demo.student",
}

// DemoQuickLoginRoles is the order the sign-in form shows the roles in.
var DemoQuickLoginRoles = []string{"admin", "head_teacher", "teacher", "accountant", "student"}

var ErrDemoUnavailable = errors.New("demo quick login is not available")

// DemoRepository finds seed-demo accounts and audits quick logins.
// GormRepository implements it.
type DemoRepository interface {
	DemoUser(context.Context, string) (User, error)
	RecordDemoLogin(context.Context, string, map[string]any) error
}

// EnableDemoQuickLogin is called once at start-up from configuration.
func (s *Service) EnableDemoQuickLogin(enabled bool) { s.demoQuickLogin = enabled }

func (s *Service) DemoQuickLoginEnabled() bool { return s != nil && s.demoQuickLogin }

// DemoLogin signs in as the seed-demo account for role. Anything other than
// an active seed-demo account with that role at the demo address is refused.
func (s *Service) DemoLogin(ctx context.Context, role string, details map[string]any) (Credentials, error) {
	if !s.DemoQuickLoginEnabled() {
		return Credentials{}, ErrDemoUnavailable
	}
	repo, ok := s.repo.(DemoRepository)
	if !ok {
		return Credentials{}, ErrDemoUnavailable
	}
	ref, ok := DemoQuickLoginAccounts[role]
	if !ok {
		return Credentials{}, ErrInvalidCredentials
	}
	user, err := repo.DemoUser(ctx, ref)
	if errors.Is(err, ErrNotFound) {
		return Credentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	now := s.now()
	if user.Email != ref+"@"+DemoDomain || user.Role != role || !user.IsActive || (user.LockedUntil != nil && user.LockedUntil.After(now)) {
		return Credentials{}, ErrInvalidCredentials
	}
	// Audit first, so no quick-login session exists without its record.
	if details == nil {
		details = map[string]any{}
	}
	details["role"] = role
	if err := repo.RecordDemoLogin(ctx, user.ID, details); err != nil {
		return Credentials{}, err
	}
	return s.newSession(ctx, user, now)
}

// DemoUser returns the user that seed-demo recorded under ref.
func (r *GormRepository) DemoUser(ctx context.Context, ref string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Table("users").
		Joins("JOIN demo_records AS d ON d.record_id = users.id AND d.kind = 'user' AND d.ref = ?", ref).
		Select("users.*").Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (r *GormRepository) RecordDemoLogin(ctx context.Context, userID string, details map[string]any) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return audit.Record(tx, &userID, "auth.demo_quick_login", "user", userID, details)
	})
}
