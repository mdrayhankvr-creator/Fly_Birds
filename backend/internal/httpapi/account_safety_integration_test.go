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

// TestPostgresAdminPasswordResetAndForcedChange: an administrator resets
// another account's password to a temporary one shown once; the account's
// sessions end, the audit log never holds the password, and until the account
// sets its own password every request but reading itself and changing the
// password is refused.
func TestPostgresAdminPasswordResetAndForcedChange(t *testing.T) {
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
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('teacher','auth:read_self'),('teacher','auth:password_change_self'),('admin','users:update'),('admin','users:read'),('admin','permissions:manage') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	const password = "Reset-test-password-2026!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x72}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(label, role string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-reset-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, otherAdmin, head, teacher := makeUser("admin", "admin"), makeUser("admin2", "admin"), makeUser("head", "head_teacher"), makeUser("teacher", "teacher")
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-reset-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-reset-"+suffix+"@example.test").Error
	})
	router := NewRouterWithAll(authService, true, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), users.NewService(users.NewRepository(db)), audit.NewService(audit.NewRepository(db)), dashboard.NewService(dashboard.NewRepository(db)))
	bearer := func(token string) map[string]string { return map[string]string{"Authorization": "Bearer " + token} }
	signIn := func(u auth.User, pw string) (auth.Credentials, int) {
		r := perform(router, http.MethodPost, "/auth/login", `{"email":"`+u.Email+`","password":"`+pw+`"}`, nil, nil)
		var body struct {
			Data auth.Credentials `json:"data"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &body)
		return body.Data, r.Code
	}
	mustSignIn := func(u auth.User, pw string) auth.Credentials {
		t.Helper()
		c, code := signIn(u, pw)
		if code != http.StatusOK {
			t.Fatalf("sign in %s: %d", u.Email, code)
		}
		return c
	}
	expect := func(name, method, path, body, token string, status int, code string) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, bearer(token))
		if r.Code != status || (code != "" && !strings.Contains(r.Body.String(), `"code":"`+code+`"`)) {
			t.Fatalf("%s: got %d %s, want %d %s", name, r.Code, r.Body.String(), status, code)
		}
		return r.Body.Bytes()
	}
	adminToken := mustSignIn(admin, password).AccessToken
	headToken := mustSignIn(head, password).AccessToken
	teacherFirst, teacherSecond := mustSignIn(teacher, password), mustSignIn(teacher, password)
	resetPath := "/users/" + teacher.ID + "/password-reset"

	// Who may reset: an administrator, never for their own account.
	if r := perform(router, http.MethodPost, resetPath, "", nil, nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous reset: %d", r.Code)
	}
	expect("head teacher reset", http.MethodPost, resetPath, "", headToken, http.StatusForbidden, "")
	expect("teacher resets someone", http.MethodPost, "/users/"+admin.ID+"/password-reset", "", teacherFirst.AccessToken, http.StatusForbidden, "")
	expect("own reset", http.MethodPost, "/users/"+admin.ID+"/password-reset", "", adminToken, http.StatusConflict, "own_password")
	expect("unknown account", http.MethodPost, "/users/00000000-0000-4000-8000-000000000000/password-reset", "", adminToken, http.StatusNotFound, "not_found")

	// A locked account is unlocked by the reset.
	for range auth.MaxLoginFailure {
		signIn(teacher, "wrong-password-wrong")
	}
	if _, code := signIn(teacher, password); code == http.StatusOK {
		t.Fatal("lockout did not engage")
	}

	var reset struct {
		Data users.PasswordReset `json:"data"`
	}
	if err := json.Unmarshal(expect("admin reset", http.MethodPost, resetPath, "", adminToken, http.StatusOK, ""), &reset); err != nil {
		t.Fatal(err)
	}
	temporary := reset.Data.TemporaryPassword
	if len(temporary) != 14 || reset.Data.User.ID != teacher.ID || reset.Data.SessionsRevoked != 2 {
		t.Fatalf("reset result %+v", reset.Data)
	}
	// The teacher's sessions ended.
	expect("old access token after reset", http.MethodGet, "/auth/me", "", teacherFirst.AccessToken, http.StatusUnauthorized, "")
	expect("second session after reset", http.MethodGet, "/auth/me", "", teacherSecond.AccessToken, http.StatusUnauthorized, "")
	// Audited, without the password.
	var events []struct{ Metadata string }
	db.Table("audit_events").Select("metadata::text AS metadata").Where("action='user.password_reset' AND target_id=? AND actor_user_id=?", teacher.ID, admin.ID).Scan(&events)
	if len(events) != 1 || !strings.Contains(events[0].Metadata, `"sessions_revoked": 2`) {
		t.Fatalf("reset audit %+v", events)
	}
	var leaked int64
	db.Table("audit_events").Where("metadata::text LIKE ?", "%"+temporary+"%").Count(&leaked)
	var hash string
	db.Table("users").Where("id=?", teacher.ID).Pluck("password_hash", &hash)
	if leaked != 0 || strings.Contains(hash, temporary) {
		t.Fatal("the temporary password was stored or audited in plain text")
	}

	// The old password is gone; the temporary one signs in, marked.
	if _, code := signIn(teacher, password); code != http.StatusUnauthorized {
		t.Fatalf("old password after reset: %d", code)
	}
	forced := mustSignIn(teacher, temporary)
	if !forced.User.MustChangePassword {
		t.Fatal("a reset account is not asked to change its password")
	}
	// Until the password changes, only reading itself and changing it work.
	if !bytes.Contains(expect("me while forced", http.MethodGet, "/auth/me", "", forced.AccessToken, http.StatusOK, ""), []byte(`"must_change_password":true`)) {
		t.Fatal("/auth/me does not report the forced change")
	}
	expect("permissions while forced", http.MethodGet, "/auth/permissions", "", forced.AccessToken, http.StatusOK, "")
	for _, path := range []string{"/users", "/school/settings", "/dashboard", "/audit/events"} {
		expect("blocked "+path, http.MethodGet, path, "", forced.AccessToken, http.StatusForbidden, "password_change_required")
	}
	expect("reset while forced", http.MethodPost, "/users/"+admin.ID+"/password-reset", "", forced.AccessToken, http.StatusForbidden, "password_change_required")
	expect("same password refused", http.MethodPost, "/auth/password", `{"current_password":"`+temporary+`","new_password":"`+temporary+`"}`, forced.AccessToken, http.StatusBadRequest, "")
	const chosen = "Teacher-chosen-password-2026"
	expect("change while forced", http.MethodPost, "/auth/password", `{"current_password":"`+temporary+`","new_password":"`+chosen+`"}`, forced.AccessToken, http.StatusOK, "")
	if bytes.Contains(expect("me after change", http.MethodGet, "/auth/me", "", forced.AccessToken, http.StatusOK, ""), []byte(`"must_change_password":true`)) {
		t.Fatal("the forced change was not cleared")
	}
	expect("dashboard after the change", http.MethodGet, "/dashboard", "", forced.AccessToken, http.StatusOK, "")
	if _, code := signIn(teacher, temporary); code != http.StatusUnauthorized {
		t.Fatalf("temporary password after the change: %d", code)
	}
	mustSignIn(teacher, chosen)

	// An administrator may reset another administrator; the last-admin
	// protection on deactivation is unchanged.
	expect("reset another admin", http.MethodPost, "/users/"+otherAdmin.ID+"/password-reset", "", adminToken, http.StatusOK, "")
}
