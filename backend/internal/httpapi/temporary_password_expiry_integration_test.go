package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
)

// TestPostgresTemporaryPasswordExpiry: a temporary password stops working 14
// days after it was issued if the account never set its own. Signing in with
// it then says it expired (a wrong password still gets the usual answer), a
// session opened with it ends and cannot be refreshed, and the administrator
// can issue a new one. Choosing a password clears the expiry.
func TestPostgresTemporaryPasswordExpiry(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('teacher','auth:read_self'),('teacher','auth:password_change_self'),('admin','users:update'),('admin','users:read') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	domain := "-expiry-" + hex.EncodeToString(random[:]) + "@example.test"
	const password = "Expiry-test-password-2026!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "admin"+domain, password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "teacher"+domain, password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%"+domain).Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%"+domain).Error
	})
	router := NewRouterWithAll(authService, true, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), users.NewService(users.NewRepository(db)), audit.NewService(audit.NewRepository(db)), dashboard.NewService(dashboard.NewRepository(db)))
	signIn := func(email, pw string) *http.Response {
		return perform(router, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`, nil, nil).Result()
	}
	login := func(email, pw string) (int, string) {
		r := perform(router, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`, nil, nil)
		return r.Code, r.Body.String()
	}
	adminLogin := perform(router, http.MethodPost, "/auth/login", `{"email":"`+admin.Email+`","password":"`+password+`"}`, nil, nil)
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin sign-in %d", adminLogin.Code)
	}
	adminH := map[string]string{"Authorization": "Bearer " + credentialsFromResponse(t, adminLogin)}
	reset := func() users.PasswordReset {
		t.Helper()
		r := perform(router, http.MethodPost, "/users/"+teacher.ID+"/password-reset", "", nil, adminH)
		if r.Code != http.StatusOK {
			t.Fatalf("reset %d %s", r.Code, r.Body.String())
		}
		var body struct {
			Data users.PasswordReset `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data
	}
	storedExpiry := func() *time.Time {
		var row struct{ TemporaryPasswordExpiresAt *time.Time }
		if err := db.Table("users").Select("temporary_password_expires_at").Where("id=?", teacher.ID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row.TemporaryPasswordExpiresAt
	}
	expire := func() {
		if err := db.Exec("UPDATE users SET temporary_password_expires_at = now() - interval '1 second' WHERE id=?", teacher.ID).Error; err != nil {
			t.Fatal(err)
		}
	}

	// The reset slip carries the expiry: 14 days from now, as stored.
	first := reset()
	want := time.Now().Add(14 * 24 * time.Hour)
	if first.PasswordExpiresAt.Before(want.Add(-2*time.Minute)) || first.PasswordExpiresAt.After(want.Add(2*time.Minute)) {
		t.Fatalf("reset expiry %v, want about %v", first.PasswordExpiresAt, want)
	}
	if got := storedExpiry(); got == nil || got.Sub(first.PasswordExpiresAt).Abs() > time.Millisecond {
		t.Fatalf("stored expiry %v, slip %v", got, first.PasswordExpiresAt)
	}

	// Before the date it works: a session opens (limited to changing the password).
	open := signIn(teacher.Email, first.TemporaryPassword)
	if open.StatusCode != http.StatusOK {
		t.Fatalf("temporary password before expiry: %d", open.StatusCode)
	}
	var openBody struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(open.Body).Decode(&openBody)
	var refreshCookies []*http.Cookie
	var csrf string
	for _, c := range open.Cookies() {
		if c.Name == refreshCookie || c.Name == csrfCookie {
			refreshCookies = append(refreshCookies, c)
		}
		if c.Name == csrfCookie {
			csrf = c.Value
		}
	}
	teacherH := map[string]string{"Authorization": "Bearer " + openBody.Data.AccessToken}
	if r := perform(router, http.MethodGet, "/auth/me", "", nil, teacherH); r.Code != http.StatusOK {
		t.Fatalf("me before expiry %d", r.Code)
	}

	// After the date: the right password says it expired, a wrong one gets the usual answer.
	expire()
	if code, body := login(teacher.Email, first.TemporaryPassword); code != http.StatusUnauthorized || !strings.Contains(body, `"code":"temporary_password_expired"`) {
		t.Fatalf("expired temporary password: %d %s", code, body)
	}
	if code, body := login(teacher.Email, "Wrong-password-entirely-1"); code != http.StatusUnauthorized || !strings.Contains(body, `"code":"invalid_credentials"`) {
		t.Fatalf("wrong password after expiry: %d %s", code, body)
	}
	// An expired sign-in is not a failed attempt: it does not count toward lockout.
	var failures int
	db.Table("users").Select("failed_login_attempts").Where("id=?", teacher.ID).Scan(&failures)
	if failures != 1 {
		t.Fatalf("failed attempts %d, want 1 (only the wrong password)", failures)
	}
	// The session opened with it ends, cannot be refreshed and cannot change the password.
	if r := perform(router, http.MethodGet, "/auth/me", "", nil, teacherH); r.Code != http.StatusUnauthorized {
		t.Fatalf("me after expiry %d", r.Code)
	}
	if r := perform(router, http.MethodPost, "/auth/refresh", "", refreshCookies, map[string]string{"X-CSRF-Token": csrf}); r.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after expiry %d %s", r.Code, r.Body.String())
	}
	if _, err := authService.ChangePassword(ctx, teacher.ID, "00000000-0000-4000-8000-000000000000", first.TemporaryPassword, "Chosen-by-the-teacher-1"); err != auth.ErrTemporaryPasswordExpired {
		t.Fatalf("change with an expired temporary password: %v", err)
	}

	// The administrator issues a new one, which works and starts a fresh 14 days.
	second := reset()
	if second.TemporaryPassword == first.TemporaryPassword || !second.PasswordExpiresAt.After(time.Now().Add(13*24*time.Hour)) {
		t.Fatalf("second reset %+v", second)
	}
	if code, body := login(teacher.Email, first.TemporaryPassword); code != http.StatusUnauthorized || !strings.Contains(body, "invalid_credentials") {
		t.Fatalf("old temporary password after a new reset: %d %s", code, body)
	}
	fresh := perform(router, http.MethodPost, "/auth/login", `{"email":"`+teacher.Email+`","password":"`+second.TemporaryPassword+`"}`, nil, nil)
	if fresh.Code != http.StatusOK {
		t.Fatalf("new temporary password: %d", fresh.Code)
	}
	freshH := map[string]string{"Authorization": "Bearer " + credentialsFromResponse(t, fresh)}

	// Setting their own password clears the expiry; it keeps working past the old date.
	if r := perform(router, http.MethodPost, "/auth/password", `{"current_password":"`+second.TemporaryPassword+`","new_password":"Chosen-by-the-teacher-1"}`, nil, freshH); r.Code != http.StatusOK {
		t.Fatalf("change password %d %s", r.Code, r.Body.String())
	}
	if got := storedExpiry(); got != nil {
		t.Fatalf("expiry kept after the change: %v", got)
	}
	if err := db.Exec("UPDATE users SET temporary_password_expires_at = now() - interval '1 day' WHERE id=?", teacher.ID).Error; err != nil {
		t.Fatal(err)
	}
	if code, body := login(teacher.Email, "Chosen-by-the-teacher-1"); code != http.StatusOK {
		t.Fatalf("own password with a stale date: %d %s", code, body)
	}
}
