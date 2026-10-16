package auth

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type memoryRepository struct {
	mu       sync.Mutex
	user     User
	sessions map[string]Session
}

func (r *memoryRepository) FindUserByEmail(_ context.Context, email string) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user.Email != email {
		return User{}, ErrNotFound
	}
	return r.user, nil
}
func (r *memoryRepository) RecordLoginFailure(_ context.Context, _ string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.user.FailedLoginAttempts++
	if r.user.FailedLoginAttempts >= MaxLoginFailure {
		until := now.Add(LockoutDuration)
		r.user.LockedUntil = &until
	}
	return nil
}
func (r *memoryRepository) ClearLoginFailures(_ context.Context, _ string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user.LockedUntil != nil && r.user.LockedUntil.After(now) {
		return ErrInvalidCredentials
	}
	r.user.FailedLoginAttempts = 0
	r.user.LockedUntil = nil
	return nil
}
func (r *memoryRepository) CreateSession(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[session.ID] = *session
	return nil
}
func (r *memoryRepository) RotateSession(_ context.Context, oldHash, csrfHash, newHash, newCSRFHash []byte, expiresAt, now time.Time) (Session, User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, session := range r.sessions {
		if bytes.Equal(session.RefreshHash, oldHash) {
			if !bytes.Equal(session.CSRFHash, csrfHash) || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
				return Session{}, User{}, ErrInvalidSession
			}
			session.RefreshHash, session.CSRFHash, session.ExpiresAt = newHash, newCSRFHash, expiresAt
			r.sessions[id] = session
			return session, r.user, nil
		}
	}
	return Session{}, User{}, ErrInvalidSession
}
func (r *memoryRepository) RevokeSession(_ context.Context, refreshHash, csrfHash []byte, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, session := range r.sessions {
		if bytes.Equal(session.RefreshHash, refreshHash) && bytes.Equal(session.CSRFHash, csrfHash) && session.RevokedAt == nil {
			session.RevokedAt = &now
			r.sessions[id] = session
			return nil
		}
	}
	return ErrInvalidSession
}
func (r *memoryRepository) ActiveUserBySession(_ context.Context, id string, now time.Time) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok || session.RevokedAt != nil || !session.ExpiresAt.After(now) || !r.user.IsActive {
		return User{}, ErrInvalidSession
	}
	return r.user, nil
}
func (r *memoryRepository) CreateUser(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.user = *user
	return nil
}
func (r *memoryRepository) HasPermission(context.Context, string, string) (bool, error) {
	return true, nil
}
func (r *memoryRepository) PermissionsForUser(context.Context, string) ([]string, error) {
	return []string{"auth:read_self"}, nil
}

func TestPasswordHashAndLoginLifecycle(t *testing.T) {
	const password = "A-long-example-password-123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password || !VerifyPassword(hash, password) || VerifyPassword(hash, "wrong") {
		t.Fatal("password hash verification failed")
	}
	repo := &memoryRepository{user: User{ID: "27c3ae08-b2ae-4941-bb12-3569e264383f", Email: "teacher@example.test", PasswordHash: hash, Role: "teacher", IsActive: true}, sessions: make(map[string]Session)}
	service, err := NewService(repo, bytes.Repeat([]byte{0x53}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := service.Login(ctx, "missing@example.test", password); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown account: %v", err)
	}
	for attempt := 0; attempt < MaxLoginFailure; attempt++ {
		if _, err := service.Login(ctx, repo.user.Email, "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if _, err := service.Login(ctx, repo.user.Email, password); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked account: %v", err)
	}
	repo.mu.Lock()
	past := time.Now().Add(-time.Second)
	repo.user.LockedUntil = &past
	repo.mu.Unlock()
	first, err := service.Login(ctx, "TEACHER@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccessToken == "" || first.RefreshToken == "" || first.CSRFToken == "" {
		t.Fatal("missing issued credentials")
	}
	claims := new(accessClaims)
	if _, _, err := jwt.NewParser().ParseUnverified(first.AccessToken, claims); err != nil {
		t.Fatal(err)
	}
	if claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) != AccessLifetime {
		t.Fatal("incorrect access lifetime")
	}
	for _, session := range repo.sessions {
		if remaining := time.Until(session.ExpiresAt); remaining < RefreshLifetime-time.Minute || remaining > RefreshLifetime {
			t.Fatalf("incorrect refresh lifetime: %s", remaining)
		}
	}
	if _, err := service.Authenticate(ctx, first.AccessToken); err != nil {
		t.Fatalf("valid access token: %v", err)
	}
	if _, err := service.Refresh(ctx, first.RefreshToken, "wrong"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("wrong CSRF accepted: %v", err)
	}
	second, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refresh did not rotate credentials")
	}
	if _, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("reused refresh token accepted: %v", err)
	}
	for _, session := range repo.sessions {
		if bytes.Equal(session.RefreshHash, []byte(second.RefreshToken)) || bytes.Equal(session.CSRFHash, []byte(second.CSRFToken)) {
			t.Fatal("raw session secret stored")
		}
	}
	if err := service.Logout(ctx, second.RefreshToken, second.CSRFToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, second.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked access token accepted: %v", err)
	}
	if _, err := service.Refresh(ctx, second.RefreshToken, second.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked refresh token accepted: %v", err)
	}
}

func TestPasswordValidationAndTokenTampering(t *testing.T) {
	for _, password := range []string{"short", string(bytes.Repeat([]byte{'x'}, 73))} {
		if _, err := HashPassword(password); err == nil {
			t.Fatalf("accepted invalid password length %d", len(password))
		}
	}
	manager, err := NewTokenManager(bytes.Repeat([]byte{0x41}, 32))
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Issue("user-id", "session-id", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Verify(token + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered token accepted: %v", err)
	}
	expired, err := manager.Issue("user-id", "session-id", time.Now().Add(-AccessLifetime-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Verify(expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}
