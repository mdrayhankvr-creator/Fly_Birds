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
	// dummyTemporary pads sign-in work; see Login.
	dummyTemporary string
	now            func() time.Time
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
	dummyTemporary, err := bcrypt.GenerateFromPassword([]byte(dummy), temporaryCost)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, tokens: tokens, dummyHash: string(dummyHash), dummyTemporary: string(dummyTemporary), now: func() time.Time { return time.Now().UTC() }}, nil
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

// Login signs in with an email address or, for students, a sign-in ID. An
// unknown email, an unknown ID and a wrong password all take the same path
// (a password check against a dummy hash) and give the same error, so the
// ID form reveals no more about which accounts exist than the email form.
func (s *Service) Login(ctx context.Context, identifier, password string) (Credentials, error) {
	if !ValidIdentifier(identifier) || password == "" || len(password) > 72 {
		return Credentials{}, ErrInvalidCredentials
	}
	// A number may belong to several children of one family, so it takes its
	// own path; everything else resolves to one account.
	if phone, ok := NormalizePhone(identifier); ok {
		return s.loginByPhone(ctx, phone, password)
	}
	user, err := s.findForLogin(ctx, identifier)
	if errors.Is(err, ErrNotFound) {
		s.verifyPadded(s.dummyHash, password)
		return Credentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	now := s.now()
	valid := s.verifyPadded(user.PasswordHash, password)
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
	// Only someone who knows the password learns that it has expired.
	if user.TemporaryPasswordExpired(now) {
		return Credentials{}, ErrTemporaryPasswordExpired
	}
	return s.newSession(ctx, user, now)
}

// verifyPadded checks a password and then does one more check at the other
// bcrypt cost, so every sign-in costs one full-cost and one temporary-cost
// comparison: an unknown account, a temporary password and a chosen one all
// take the same time.
func (s *Service) verifyPadded(hash, password string) bool {
	valid := VerifyPassword(hash, password)
	if cost, err := bcrypt.Cost([]byte(hash)); err == nil && cost < passwordCost {
		VerifyPassword(s.dummyHash, password)
	} else {
		VerifyPassword(s.dummyTemporary, password)
	}
	return valid
}

// loginByPhone signs in with a mobile number. Staff numbers match one
// account; a guardian's number may match several children, so the password
// is checked against every account with the number and the one it matches
// signs in. Every candidate is checked, so the time taken says nothing about
// which child it was, and an unknown number costs the same dummy check as a
// wrong password does.
func (s *Service) loginByPhone(ctx context.Context, phone, password string) (Credentials, error) {
	users, err := s.repo.FindUsersByLoginPhone(ctx, phone)
	if err != nil {
		return Credentials{}, err
	}
	if len(users) == 0 {
		s.verifyPadded(s.dummyHash, password)
		return Credentials{}, ErrInvalidCredentials
	}
	now := s.now()
	matched := -1
	for i := range users {
		if s.verifyPadded(users[i].PasswordHash, password) && matched < 0 {
			matched = i
		}
	}
	if matched < 0 {
		// The attempt counts against every account the number could have
		// opened, so guessing is limited the same way as by email.
		for i := range users {
			if err := s.repo.RecordLoginFailure(ctx, users[i].ID, now); err != nil {
				return Credentials{}, err
			}
		}
		return Credentials{}, ErrInvalidCredentials
	}
	user := users[matched]
	if !user.IsActive || (user.LockedUntil != nil && user.LockedUntil.After(now)) {
		return Credentials{}, ErrInvalidCredentials
	}
	if err := s.repo.ClearLoginFailures(ctx, user.ID, now); err != nil {
		return Credentials{}, err
	}
	if user.TemporaryPasswordExpired(now) {
		return Credentials{}, ErrTemporaryPasswordExpired
	}
	return s.newSession(ctx, user, now)
}

// Sibling is another child of the same family, as the header's child switch
// lists them.
type Sibling struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	LoginID     string `json:"login_id,omitempty"`
	Current     bool   `json:"current"`
}

// Siblings lists the student accounts that share this account's login phone,
// itself included. It is empty unless the account is a student with a number
// shared by at least one other child.
func (s *Service) Siblings(ctx context.Context, userID string) ([]Sibling, error) {
	user, err := s.repo.FindActiveUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Role != "student" || user.LoginPhone == nil {
		return []Sibling{}, nil
	}
	others, err := s.repo.FindUsersByLoginPhone(ctx, *user.LoginPhone)
	if err != nil {
		return nil, err
	}
	out := []Sibling{}
	for i := range others {
		if others[i].Role != "student" || !others[i].IsActive {
			continue
		}
		sib := Sibling{UserID: others[i].ID, DisplayName: others[i].DisplayName, Current: others[i].ID == user.ID}
		if others[i].LoginID != nil {
			sib.LoginID = *others[i].LoginID
		}
		out = append(out, sib)
	}
	if len(out) < 2 {
		return []Sibling{}, nil
	}
	return out, nil
}

// SwitchTo opens a session on a sibling's account without asking for the
// password again: the guardian already gave it for an account that shares
// the same login phone. Only a student account may switch, and only to
// another active student account with the very same number.
func (s *Service) SwitchTo(ctx context.Context, userID, targetID string) (Credentials, error) {
	user, err := s.repo.FindActiveUser(ctx, userID)
	if err != nil {
		return Credentials{}, ErrInvalidCredentials
	}
	target, err := s.repo.FindActiveUser(ctx, targetID)
	if err != nil {
		return Credentials{}, ErrInvalidCredentials
	}
	if user.Role != "student" || target.Role != "student" || user.LoginPhone == nil || target.LoginPhone == nil ||
		*user.LoginPhone != *target.LoginPhone || !target.IsActive {
		return Credentials{}, ErrInvalidCredentials
	}
	now := s.now()
	if target.LockedUntil != nil && target.LockedUntil.After(now) {
		return Credentials{}, ErrInvalidCredentials
	}
	return s.newSession(ctx, target, now)
}

func (s *Service) findForLogin(ctx context.Context, identifier string) (User, error) {
	identifier = strings.TrimSpace(identifier)
	if strings.Contains(identifier, "@") {
		email, err := NormalizeEmail(identifier)
		if err != nil || IsPlaceholderEmail(email) {
			return User{}, ErrNotFound
		}
		return s.repo.FindUserByEmail(ctx, email)
	}
	return s.repo.FindUserByLoginID(ctx, identifier)
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
	// A session opened with a temporary password ends when the password expires.
	if user.TemporaryPasswordExpired(now) {
		return Credentials{}, ErrInvalidSession
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
	if user.ID != userID || user.TemporaryPasswordExpired(s.now()) {
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
	if user.TemporaryPasswordExpired(now) {
		return 0, ErrTemporaryPasswordExpired
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
