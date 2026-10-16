package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func TestPostgresAuthHTTPLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL HTTP integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	email := hex.EncodeToString(suffix[:]) + "@example.test"
	const password = "IsolatedTestPassword123!"
	key := bytes.Repeat([]byte{0x62}, 32)
	service, err := auth.NewService(auth.NewRepository(db), key)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.BootstrapUser(ctx, email, password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Where("id = ?", user.ID).Delete(&auth.User{}).Error; err != nil {
			t.Errorf("remove isolated user: %v", err)
		}
	})
	router := NewRouterWithAuth(service, true)

	if got := perform(router, http.MethodGet, "/auth/me", "", nil, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous /auth/me status %d", got)
	}
	if got := perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer invalid"}).Code; got != http.StatusUnauthorized {
		t.Fatalf("invalid token /auth/me status %d", got)
	}

	badLogin := `{"email":"` + email + `","password":"incorrect"}`
	var invalidBody string
	for attempt := 0; attempt < auth.MaxLoginFailure; attempt++ {
		response := perform(router, http.MethodPost, "/auth/login", badLogin, nil, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("invalid password attempt %d status %d", attempt+1, response.Code)
		}
		invalidBody = response.Body.String()
	}
	unknown := perform(router, http.MethodPost, "/auth/login", `{"email":"missing-`+email+`","password":"incorrect"}`, nil, nil)
	if unknown.Code != http.StatusUnauthorized || unknown.Body.String() != invalidBody {
		t.Fatal("unknown account disclosed its status")
	}
	var locked auth.User
	if err := db.First(&locked, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if locked.FailedLoginAttempts != auth.MaxLoginFailure || locked.LockedUntil == nil || !locked.LockedUntil.After(time.Now()) {
		t.Fatal("login lockout was not persisted")
	}
	if locked.PasswordHash == password || !auth.VerifyPassword(locked.PasswordHash, password) {
		t.Fatal("password was not stored as a valid hash")
	}
	goodLogin := `{"email":"` + email + `","password":"` + password + `"}`
	whileLocked := perform(router, http.MethodPost, "/auth/login", goodLogin, nil, nil)
	if whileLocked.Code != http.StatusUnauthorized || whileLocked.Body.String() != invalidBody {
		t.Fatal("locked account disclosed its status or accepted login")
	}
	if err := db.Model(&auth.User{}).Where("id = ?", user.ID).Update("locked_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	login := perform(router, http.MethodPost, "/auth/login", goodLogin, nil, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("valid login status %d", login.Code)
	}
	access := credentialsFromResponse(t, login)
	cookies, csrf := sessionCookies(t, login)
	if bytes.Contains(login.Body.Bytes(), []byte(password)) || bytes.Contains(login.Body.Bytes(), []byte(locked.PasswordHash)) {
		t.Fatal("login response disclosed password material")
	}
	var claims jwt.MapClaims
	if _, _, err := jwt.NewParser().ParseUnverified(access, &claims); err != nil {
		t.Fatal(err)
	}
	sessionID, ok := claims["sid"].(string)
	if !ok || sessionID == "" {
		t.Fatal("access token has no session identifier")
	}
	var session auth.Session
	if err := db.First(&session, "id = ?", sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if len(session.RefreshHash) != 32 || bytes.Equal(session.RefreshHash, []byte(cookies[0].Value)) {
		t.Fatal("refresh token was not stored as a hash")
	}
	firstHash := append([]byte(nil), session.RefreshHash...)
	me := perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + access})
	if me.Code != http.StatusOK {
		t.Fatalf("authorized /auth/me status %d", me.Code)
	}
	var meBody struct {
		Data auth.User `json:"data"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &meBody); err != nil || meBody.Data.Email != email {
		t.Fatal("/auth/me returned the wrong user")
	}
	manager, err := auth.NewTokenManager(key)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := manager.Issue(user.ID, sessionID, time.Now().Add(-auth.AccessLifetime-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got := perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + expired}).Code; got != http.StatusUnauthorized {
		t.Fatalf("expired token /auth/me status %d", got)
	}
	if got := perform(router, http.MethodPost, "/auth/refresh", "", cookies, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("refresh without CSRF status %d", got)
	}
	refresh := perform(router, http.MethodPost, "/auth/refresh", "", cookies, map[string]string{"X-CSRF-Token": csrf})
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status %d", refresh.Code)
	}
	newAccess := credentialsFromResponse(t, refresh)
	newCookies, newCSRF := sessionCookies(t, refresh)
	if newCookies[0].Value == cookies[0].Value {
		t.Fatal("refresh token did not rotate")
	}
	if err := db.First(&session, "id = ?", sessionID).Error; err != nil || bytes.Equal(session.RefreshHash, firstHash) {
		t.Fatal("rotated refresh hash was not persisted")
	}
	if got := perform(router, http.MethodPost, "/auth/refresh", "", cookies, map[string]string{"X-CSRF-Token": csrf}).Code; got != http.StatusUnauthorized {
		t.Fatalf("reused refresh token status %d", got)
	}
	if got := perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + newAccess}).Code; got != http.StatusOK {
		t.Fatalf("refreshed access /auth/me status %d", got)
	}
	if got := perform(router, http.MethodPost, "/auth/logout", "", newCookies, map[string]string{"X-CSRF-Token": newCSRF}).Code; got != http.StatusNoContent {
		t.Fatalf("logout status %d", got)
	}
	if err := db.First(&session, "id = ?", sessionID).Error; err != nil || session.RevokedAt == nil {
		t.Fatal("logout revocation was not persisted")
	}
	if got := perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + newAccess}).Code; got != http.StatusUnauthorized {
		t.Fatalf("revoked access /auth/me status %d", got)
	}
	if got := perform(router, http.MethodPost, "/auth/refresh", "", newCookies, map[string]string{"X-CSRF-Token": newCSRF}).Code; got != http.StatusUnauthorized {
		t.Fatalf("revoked refresh token status %d", got)
	}
}
