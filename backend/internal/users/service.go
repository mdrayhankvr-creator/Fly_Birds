package users

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

var (
	ErrInvalid   = errors.New("invalid user input")
	ErrNotFound  = errors.New("user not found")
	ErrForbidden = errors.New("user operation denied")
	ErrDuplicate = errors.New("email already registered")
	ErrLastAdmin = errors.New("last active administrator")
	// ErrOwnReset: administrators change their own password themselves.
	ErrOwnReset = errors.New("use change password for your own account")
)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	// LoginID is a student login's sign-in ID. A student login without an
	// email has a placeholder address (auth.PlaceholderDomain) instead.
	LoginID string `json:"login_id,omitempty"`
	// LoginPhone is the number this account signs in with, +8801XXXXXXXXX.
	LoginPhone string `json:"login_phone,omitempty"`
}
type CreateInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}
type UpdateInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// PhoneInput sets or clears an account's login phone. An empty number
// removes it.
type PhoneInput struct {
	LoginPhone string `json:"login_phone"`
}
type Query struct {
	Search   string
	Role     string
	Status   string
	Page     int
	PageSize int
}
type Page struct {
	Items    []User `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type Repository interface {
	Create(context.Context, string, User, string) (User, error)
	List(context.Context, Query) (Page, error)
	Get(context.Context, string) (User, error)
	Update(context.Context, string, string, UpdateInput) (User, error)
	SetStatus(context.Context, string, string, bool) (User, error)
	SetLoginPhone(context.Context, string, string, string) (User, error)
	ResetPassword(context.Context, string, string, string) (User, int64, time.Time, error)
}

// PasswordReset is an administrator's reset of another account's password.
// TemporaryPassword is returned once, in this response only; the account
// must replace it at its next sign-in.
type PasswordReset struct {
	User              User   `json:"user"`
	TemporaryPassword string `json:"temporary_password"`
	// PasswordExpiresAt: the temporary password stops working after this
	// unless the account has set its own.
	PasswordExpiresAt time.Time `json:"password_expires_at"`
	SessionsRevoked   int64     `json:"sessions_revoked"`
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }
func validName(name string) bool {
	return utf8.ValidString(name) && len(name) <= 160 && strings.TrimSpace(name) != ""
}

func (s *Service) Create(ctx context.Context, actor string, input CreateInput) (User, error) {
	mail, err := auth.NormalizeEmail(input.Email)
	if !uuid.MatchString(actor) || err != nil || !auth.ValidRole(input.Role) || !validName(input.DisplayName) {
		return User{}, ErrInvalid
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return User{}, ErrInvalid
	}
	return s.repo.Create(ctx, actor, User{Email: mail, DisplayName: strings.TrimSpace(input.DisplayName), Role: input.Role, IsActive: true}, hash)
}

// ResetPassword gives another account a generated temporary password, ends
// all its sessions and makes it choose a new password at the next sign-in.
func (s *Service) ResetPassword(ctx context.Context, actor, id string) (PasswordReset, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return PasswordReset{}, ErrInvalid
	}
	if strings.EqualFold(actor, id) {
		return PasswordReset{}, ErrOwnReset
	}
	password, err := auth.TemporaryPassword()
	if err != nil {
		return PasswordReset{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return PasswordReset{}, err
	}
	user, revoked, expires, err := s.repo.ResetPassword(ctx, actor, id, hash)
	if err != nil {
		return PasswordReset{}, err
	}
	return PasswordReset{User: user, TemporaryPassword: password, PasswordExpiresAt: expires, SessionsRevoked: revoked}, nil
}
func (s *Service) List(ctx context.Context, query Query) (Page, error) {
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 || len(query.Search) > 100 || (query.Role != "" && !auth.ValidRole(query.Role)) || (query.Status != "" && query.Status != "active" && query.Status != "inactive") {
		return Page{}, ErrInvalid
	}
	query.Search = strings.TrimSpace(query.Search)
	return s.repo.List(ctx, query)
}
func (s *Service) Get(ctx context.Context, id string) (User, error) {
	if !uuid.MatchString(id) {
		return User{}, ErrInvalid
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Update(ctx context.Context, actor, id string, input UpdateInput) (User, error) {
	mail, err := auth.NormalizeEmail(input.Email)
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || err != nil || !validName(input.DisplayName) {
		return User{}, ErrInvalid
	}
	input.Email = mail
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	return s.repo.Update(ctx, actor, id, input)
}

// SetLoginPhone sets the number an account signs in with. Staff numbers
// must be unique (the database enforces it); student accounts of one family
// may share a guardian's number.
func (s *Service) SetLoginPhone(ctx context.Context, actor, id string, input PhoneInput) (User, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return User{}, ErrInvalid
	}
	phone := strings.TrimSpace(input.LoginPhone)
	if phone == "" {
		return s.repo.SetLoginPhone(ctx, actor, id, "")
	}
	stored, ok := auth.NormalizePhone(phone)
	if !ok {
		return User{}, ErrInvalid
	}
	return s.repo.SetLoginPhone(ctx, actor, id, stored)
}

func (s *Service) SetStatus(ctx context.Context, actor, id string, active bool) (User, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return User{}, ErrInvalid
	}
	return s.repo.SetStatus(ctx, actor, id, active)
}
