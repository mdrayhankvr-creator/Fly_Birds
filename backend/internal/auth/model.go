package auth

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("record not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidToken       = errors.New("invalid access token")
	ErrWeakPassword       = errors.New("password must be 12 to 72 UTF-8 bytes")
	ErrSamePassword       = errors.New("new password must differ from the current one")
	ErrAccountLocked      = errors.New("account is temporarily locked")
	// ErrTemporaryPasswordExpired: the password someone else set for the
	// account was never replaced and is past its expiry date.
	ErrTemporaryPasswordExpired = errors.New("temporary password has expired")
)

const (
	AccessLifetime  = 15 * time.Minute
	RefreshLifetime = 7 * 24 * time.Hour
	LockoutDuration = 15 * time.Minute
	MaxLoginFailure = 5
	// RefreshReuseGrace tolerates a rotated refresh token for a few seconds so
	// two tabs refreshing at once do not revoke their own session.
	RefreshReuseGrace = 10 * time.Second
	// TemporaryPasswordLifetime: a temporary password (admission, reset,
	// one-click, section or bulk logins) stops working this long after it
	// was issued if the account never set its own.
	TemporaryPasswordLifetime = 14 * 24 * time.Hour
)

var roles = map[string]bool{
	"admin": true, "head_teacher": true, "teacher": true,
	"student": true, "guardian": true, "accountant": true,
}

func ValidRole(role string) bool { return roles[role] }

type User struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email               string     `gorm:"not null;uniqueIndex" json:"email"`
	DisplayName         string     `gorm:"not null" json:"display_name"`
	PasswordHash        string     `gorm:"not null" json:"-"`
	Role                string     `gorm:"not null" json:"role"`
	IsActive            bool       `gorm:"not null" json:"-"`
	FailedLoginAttempts int        `gorm:"not null" json:"-"`
	LockedUntil         *time.Time `json:"-"`
	// MustChangePassword: the password was set by someone else (admission or
	// an administrator's reset); the account may only change it until it does.
	MustChangePassword bool `gorm:"not null;default:false" json:"must_change_password"`
	// TemporaryPasswordExpiresAt is when a temporary password stops working;
	// it only counts while MustChangePassword is set.
	TemporaryPasswordExpiresAt *time.Time `gorm:"column:temporary_password_expires_at" json:"-"`
	// LoginID is a student login's sign-in ID (Student ID or admission
	// number); staff and guardians have none and sign in with their email.
	LoginID *string `gorm:"column:login_id" json:"login_id,omitempty"`
	// LoginPhone is the number this account signs in with, stored as
	// +8801XXXXXXXXX. Staff numbers are unique; student accounts of one
	// family may share a guardian's number (migration 1811).
	LoginPhone *string   `gorm:"column:login_phone" json:"login_phone,omitempty"`
	CreatedAt  time.Time `json:"-"`
	UpdatedAt  time.Time `json:"-"`
}

func (User) TableName() string { return "users" }

// TemporaryPasswordExpired reports whether the account still has a temporary
// password and it is past its expiry date.
func (u User) TemporaryPasswordExpired(now time.Time) bool {
	return u.MustChangePassword && u.TemporaryPasswordExpiresAt != nil && !u.TemporaryPasswordExpiresAt.After(now)
}

type Session struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	UserID      string    `gorm:"type:uuid;not null"`
	RefreshHash []byte    `gorm:"type:bytea;not null"`
	CSRFHash    []byte    `gorm:"type:bytea;not null"`
	ExpiresAt   time.Time `gorm:"not null"`
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Session) TableName() string { return "auth_sessions" }

// RefreshHistory records a refresh token hash a session has rotated away from.
type RefreshHistory struct {
	RefreshHash []byte    `gorm:"type:bytea;primaryKey"`
	SessionID   string    `gorm:"type:uuid;not null"`
	RotatedAt   time.Time `gorm:"not null"`
}

func (RefreshHistory) TableName() string { return "auth_refresh_history" }
