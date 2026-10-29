package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

type Credentials struct {
	User         User   `json:"user"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"-"`
	CSRFToken    string `json:"-"`
}

type Service struct {
	repo      Repository
	tokens    *TokenManager
	dummyHash string
	now       func() time.Time
	// demoQuickLogin is set only from DEMO_QUICK_LOGIN (see demo.go).
	demoQuickLogin bool
}

func NewService(repo Repository, signingKey []byte) (*Service, error) {
	if repo == nil {
		return nil, errors.New("auth repository is required")
	}
	tokens, err := NewTokenManager(signingKey)
	if err != nil {
		return nil, err
	}
	dummy, err := randomOpaque()
	if err != nil {
		return nil, err
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte(dummy), passwordCost)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, tokens: tokens, dummyHash: string(dummyHash), now: func() time.Time { return time.Now().UTC() }}, nil
}

func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 254 {
		return "", ErrInvalidCredentials
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return "", ErrInvalidCredentials
	}
	return email, nil
}

func (s *Service) Login(ctx context.Context, rawEmail, password string) (Credentials, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil || password == "" || len(password) > 72 {
		return Credentials{}, ErrInvalidCredentials
	}
	user, err := s.repo.FindUserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		VerifyPassword(s.dummyHash, password)
		return Credentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	now := s.now()
	valid := VerifyPassword(user.PasswordHash, password)
	if !user.IsActive || (user.LockedUntil != nil && user.LockedUntil.After(now)) {
		return Credentials{}, ErrInvalidCredentials
	}
	if !valid {
		if err := s.repo.RecordLoginFailure(ctx, user.ID, now); err != nil {
			return Credentials{}, err
		}
		return Credentials{}, ErrInvalidCredentials
	}
	if err := s.repo.ClearLoginFailures(ctx, user.ID, now); err != nil {
		return Credentials{}, err
	}
	return s.newSession(ctx, user, now)
}

func (s *Service) newSession(ctx context.Context, user User, now time.Time) (Credentials, error) {
	refresh, err := randomOpaque()
	if err != nil {
		return Credentials{}, err
	}
	csrf, err := randomOpaque()
	if err != nil {
		return Credentials{}, err
	}
	refreshHash, _ := hashOpaque(refresh)
	csrfHash, _ := hashOpaque(csrf)
	id, err := randomUUID()
	if err != nil {
		return Credentials{}, err
	}
	access, err := s.tokens.Issue(user.ID, id, now)
	if err != nil {
		return Credentials{}, err
	}
	session := &Session{ID: id, UserID: user.ID, RefreshHash: refreshHash, CSRFHash: csrfHash, ExpiresAt: now.Add(RefreshLifetime)}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return Credentials{}, err
	}
	return Credentials{User: user, AccessToken: access, RefreshToken: refresh, CSRFToken: csrf}, nil
}

func (s *Service) Refresh(ctx context.Context, refresh, csrf string) (Credentials, error) {
	oldHash, err := hashOpaque(refresh)
	if err != nil {
		return Credentials{}, ErrInvalidSession
	}
	oldCSRFHash, err := hashOpaque(csrf)
	if err != nil {
		return Credentials{}, ErrInvalidSession
	}
	newRefresh, err := randomOpaque()
	if err != nil {
		return Credentials{}, err
	}
	newCSRF, err := randomOpaque()
	if err != nil {
		return Credentials{}, err
	}
	newHash, _ := hashOpaque(newRefresh)
	newCSRFHash, _ := hashOpaque(newCSRF)
	now := s.now()
	session, user, err := s.repo.RotateSession(ctx, oldHash, oldCSRFHash, newHash, newCSRFHash, now.Add(RefreshLifetime), now)
	if err != nil {
		return Credentials{}, err
	}
	access, err := s.tokens.Issue(user.ID, session.ID, now)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{User: user, AccessToken: access, RefreshToken: newRefresh, CSRFToken: newCSRF}, nil
}

func (s *Service) Logout(ctx context.Context, refresh, csrf string) error {
	refreshHash, err := hashOpaque(refresh)
	if err != nil {
		return ErrInvalidSession
	}
	csrfHash, err := hashOpaque(csrf)
	if err != nil {
		return ErrInvalidSession
	}
	return s.repo.RevokeSession(ctx, refreshHash, csrfHash, s.now())
}

func (s *Service) Authenticate(ctx context.Context, rawAccess string) (User, error) {
	user, _, err := s.AuthenticateSession(ctx, rawAccess)
	return user, err
}

// AuthenticateSession also returns the session the access token belongs to.
func (s *Service) AuthenticateSession(ctx context.Context, rawAccess string) (User, string, error) {
	userID, sessionID, err := s.tokens.Verify(rawAccess)
	if err != nil {
		return User{}, "", ErrInvalidToken
	}
	user, err := s.repo.ActiveUserBySession(ctx, sessionID, s.now())
	if errors.Is(err, ErrInvalidSession) {
		return User{}, "", ErrInvalidToken
	}
	if err != nil {
		return User{}, "", err
	}
	if user.ID != userID {
		return User{}, "", ErrInvalidToken
	}
	return user, sessionID, nil
}

// ChangePassword lets a signed-in user replace their own password. A wrong
// current password counts toward the login lockout. On success every other
// session is revoked and the current one stays signed in.
func (s *Service) ChangePassword(ctx context.Context, userID, sessionID, current, next string) (int64, error) {
	if !utf8.ValidString(next) || len(next) < 12 || len(next) > 72 {
		return 0, ErrWeakPassword
	}
	if current == "" || len(current) > 72 {
		return 0, ErrInvalidCredentials
	}
	user, err := s.repo.FindActiveUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return 0, ErrInvalidSession
	}
	if err != nil {
		return 0, err
	}
	now := s.now()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return 0, ErrAccountLocked
	}
	if !VerifyPassword(user.PasswordHash, current) {
		if err := s.repo.RecordLoginFailure(ctx, user.ID, now); err != nil {
			return 0, err
		}
		return 0, ErrInvalidCredentials
	}
	if current == next {
		return 0, ErrSamePassword
	}
	hash, err := HashPassword(next)
	if err != nil {
		return 0, err
	}
	return s.repo.ChangePassword(ctx, user.ID, sessionID, user.PasswordHash, hash, now)
}

func (s *Service) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	return s.repo.HasPermission(ctx, userID, permission)
}
func (s *Service) PermissionsForUser(ctx context.Context, userID string) ([]string, error) {
	return s.repo.PermissionsForUser(ctx, userID)
}

func (s *Service) BootstrapUser(ctx context.Context, rawEmail, password, role string) (User, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, errors.New("invalid email address")
	}
	if !ValidRole(role) {
		return User{}, errors.New("invalid role")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	user := User{Email: email, PasswordHash: hash, Role: role, IsActive: true}
	if err := s.repo.CreateUser(ctx, &user); err != nil {
		return User{}, err
	}
	return user, nil
}
