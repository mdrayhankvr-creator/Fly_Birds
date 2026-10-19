package auth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func TestPostgresOperatorPasswordReset(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	random, _ := randomOpaque()
	email := "reset-" + random[:16] + "@example.test"
	repo := NewRepository(db)
	service, err := NewService(repo, bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.BootstrapUser(ctx, email, "OriginalPassword!123", "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Where("id = ?", user.ID).Delete(&User{}).Error })
	before, err := service.Login(ctx, email, "OriginalPassword!123")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxLoginFailure; i++ {
		_, _ = service.Login(ctx, email, "wrong-password")
	}
	if _, err := service.Login(ctx, email, "OriginalPassword!123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("account should be locked: %v", err)
	}
	if _, err := repo.ResetPassword(ctx, "nobody-"+random[:8]+"@example.test", "AnotherPassword!123", time.Now()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	if _, err := repo.ResetPassword(ctx, email, "short", time.Now()); err == nil {
		t.Fatal("short password accepted")
	}
	if _, err := repo.ResetPassword(ctx, "  "+email+"  ", "ReplacementPassword!456", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, before.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("old access token still valid: %v", err)
	}
	if _, err := service.Refresh(ctx, before.RefreshToken, before.CSRFToken); err == nil {
		t.Fatal("old refresh token still valid")
	}
	if _, err := service.Login(ctx, email, "OriginalPassword!123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	if _, err := service.Login(ctx, email, "ReplacementPassword!456"); err != nil {
		t.Fatalf("new password refused (lockout not cleared?): %v", err)
	}
	var audited int64
	db.Table("audit_events").Where("action = 'user.password_reset' AND target_id = ?", user.ID).Count(&audited)
	if audited != 1 {
		t.Fatalf("reset audited %d times", audited)
	}
}
