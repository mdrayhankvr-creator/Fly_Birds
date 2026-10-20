package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestChangePasswordRules(t *testing.T) {
	const current = "Current-password-2026!"
	const next = "Brand-new-password-2026!"
	hash, err := HashPassword(current)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{user: User{ID: "4f1c2f0e-1d1d-4c1e-9d6e-2b9e8a7c6d5e", Email: "guardian@example.test", PasswordHash: hash, Role: "guardian", IsActive: true}, sessions: make(map[string]Session)}
	service, err := NewService(repo, bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	kept, err := service.Login(ctx, repo.user.Email, current)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Login(ctx, repo.user.Email, current)
	if err != nil {
		t.Fatal(err)
	}
	_, keptSession, err := service.AuthenticateSession(ctx, kept.AccessToken)
	if err != nil {
		t.Fatal(err)
	}

	for name, value := range map[string]string{"too short": "short-pass1", "too long": string(bytes.Repeat([]byte("a"), 73)), "invalid UTF-8": "\xff\xfe-not-a-valid-password"} {
		if _, err := service.ChangePassword(ctx, repo.user.ID, keptSession, current, value); !errors.Is(err, ErrWeakPassword) {
			t.Fatalf("%s: got %v, want weak password", name, err)
		}
	}
	if _, err := service.ChangePassword(ctx, repo.user.ID, keptSession, current, current); !errors.Is(err, ErrSamePassword) {
		t.Fatalf("same password: %v", err)
	}
	if _, err := service.ChangePassword(ctx, repo.user.ID, keptSession, "Wrong-password-2026!", next); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current password: %v", err)
	}
	if repo.user.PasswordHash != hash || repo.user.FailedLoginAttempts != 1 {
		t.Fatalf("wrong current password changed credentials or skipped the lockout counter: %+v", repo.user)
	}

	revoked, err := service.ChangePassword(ctx, repo.user.ID, keptSession, current, next)
	if err != nil || revoked != 1 {
		t.Fatalf("change: revoked=%d err=%v", revoked, err)
	}
	if !VerifyPassword(repo.user.PasswordHash, next) || VerifyPassword(repo.user.PasswordHash, current) || repo.user.FailedLoginAttempts != 0 {
		t.Fatal("new password not stored or failures not cleared")
	}
	if _, err := service.Authenticate(ctx, kept.AccessToken); err != nil {
		t.Fatalf("current session was revoked: %v", err)
	}
	if _, err := service.Authenticate(ctx, other.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("other session still valid: %v", err)
	}
	if _, err := service.Refresh(ctx, other.RefreshToken, other.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("other session refreshed: %v", err)
	}
}

func TestChangePasswordLockout(t *testing.T) {
	const current = "Current-password-2026!"
	hash, err := HashPassword(current)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{user: User{ID: "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d", Email: "student@example.test", PasswordHash: hash, Role: "student", IsActive: true}, sessions: make(map[string]Session)}
	service, err := NewService(repo, bytes.Repeat([]byte{0x62}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for attempt := 0; attempt < MaxLoginFailure; attempt++ {
		if _, err := service.ChangePassword(ctx, repo.user.ID, "", "Wrong-password-2026!", "Another-password-2026!"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	// Once locked, even the correct current password is refused.
	if _, err := service.ChangePassword(ctx, repo.user.ID, "", current, "Another-password-2026!"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("locked account: %v", err)
	}
	if _, err := service.Login(ctx, repo.user.Email, current); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login while locked: %v", err)
	}
	if repo.user.PasswordHash != hash {
		t.Fatal("locked attempts changed the password")
	}
	repo.user.LockedUntil = nil
	repo.user.FailedLoginAttempts = 0
	service.now = func() time.Time { return time.Now().UTC() }
	if _, err := service.ChangePassword(ctx, "00000000-0000-4000-8000-000000000000", "", current, "Another-password-2026!"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("unknown user: %v", err)
	}
}
