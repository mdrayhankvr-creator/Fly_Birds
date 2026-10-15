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

func TestPostgresAuthenticationLifecycle(t *testing.T) {
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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	random, err := randomOpaque()
	if err != nil {
		t.Fatal(err)
	}
	email := random[:20] + "@example.test"
	service, err := NewService(NewRepository(db), bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.BootstrapUser(ctx, email, "StrongPasswordForTest123!", "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Where("id = ?", user.ID).Delete(&User{}) })
	for i := 0; i < MaxLoginFailure; i++ {
		if _, err := service.Login(ctx, email, "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failed login %d: %v", i, err)
		}
	}
	if _, err := service.Login(ctx, email, "StrongPasswordForTest123!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("lockout failed: %v", err)
	}
	if err := db.Model(&User{}).Where("id = ?", user.ID).Update("locked_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	first, err := service.Login(ctx, email, "StrongPasswordForTest123!")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, first.RefreshToken, first.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old refresh token accepted: %v", err)
	}
	if err := service.Logout(ctx, second.RefreshToken, second.CSRFToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, second.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked access accepted: %v", err)
	}
}
