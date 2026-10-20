package auth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func TestPostgresRefreshReuseRevokesSession(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	random, err := randomOpaque()
	if err != nil {
		t.Fatal(err)
	}
	const password = "Refresh-reuse-password-2026!"
	service, err := NewService(NewRepository(db), bytes.Repeat([]byte{0x63}, 32))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := service.BootstrapUser(ctx, "reuse-"+strings.ToLower(random[:12])+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	bystander, err := service.BootstrapUser(ctx, "bystander-"+strings.ToLower(random[:12])+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN ?", []string{owner.ID, bystander.ID}).Error })

	clock := time.Now().UTC()
	service.now = func() time.Time { return clock }
	first, err := service.Login(ctx, owner.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := service.Login(ctx, bystander.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken)
	if err != nil {
		t.Fatal(err)
	}

	// Two tabs refreshing together: the loser is refused but nothing is revoked.
	clock = clock.Add(RefreshReuseGrace / 2)
	if _, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("replay within grace: %v", err)
	}
	if _, err := service.Authenticate(ctx, second.AccessToken); err != nil {
		t.Fatalf("grace replay revoked the session: %v", err)
	}
	third, err := service.Refresh(ctx, second.RefreshToken, second.CSRFToken)
	if err != nil {
		t.Fatalf("legitimate refresh after grace replay: %v", err)
	}

	// A later replay of any rotated token revokes the whole session.
	clock = clock.Add(RefreshReuseGrace + time.Second)
	if _, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("replay after grace: %v", err)
	}
	if _, err := service.Authenticate(ctx, third.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token survived reuse: %v", err)
	}
	if _, err := service.Refresh(ctx, third.RefreshToken, third.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("current refresh token survived reuse: %v", err)
	}
	var detections int64
	if err := db.Table("audit_events").Where("action = 'auth.refresh_reuse_detected' AND target_id = ?", owner.ID).Count(&detections).Error; err != nil || detections != 1 {
		t.Fatalf("reuse audit events: %d %v", detections, err)
	}
	// A second replay of an already revoked session is refused without new events.
	if _, err := service.Refresh(ctx, second.RefreshToken, second.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("second replay: %v", err)
	}
	if err := db.Table("audit_events").Where("action = 'auth.refresh_reuse_detected' AND target_id = ?", owner.ID).Count(&detections).Error; err != nil || detections != 1 {
		t.Fatalf("duplicate reuse audit events: %d %v", detections, err)
	}

	// Other users are unaffected, and the owner can sign in again.
	if _, err := service.Authenticate(ctx, unrelated.AccessToken); err != nil {
		t.Fatalf("unrelated session revoked: %v", err)
	}
	if _, err := service.Refresh(ctx, unrelated.RefreshToken, unrelated.CSRFToken); err != nil {
		t.Fatalf("unrelated refresh failed: %v", err)
	}
	if _, err := service.Login(ctx, owner.Email, password); err != nil {
		t.Fatalf("owner cannot sign in again: %v", err)
	}
	var stored int64
	if err := db.Table("auth_refresh_history h").Joins("JOIN auth_sessions s ON s.id = h.session_id").Where("s.user_id = ?", owner.ID).Count(&stored).Error; err != nil || stored != 2 {
		t.Fatalf("rotated token history: %d %v", stored, err)
	}
}
