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
)

const (
	AccessLifetime  = 15 * time.Minute
	RefreshLifetime = 7 * 24 * time.Hour
	LockoutDuration = 15 * time.Minute
	MaxLoginFailure = 5
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
	CreatedAt           time.Time  `json:"-"`
	UpdatedAt           time.Time  `json:"-"`
}

func (User) TableName() string { return "users" }

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
