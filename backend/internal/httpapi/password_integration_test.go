package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func TestPostgresPasswordChangeAndRefreshReuse(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	// Other integration tests can edit the shared role matrix. Restore the grants under test.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('teacher','auth:read_self'),('teacher','auth:password_change_self'),('guardian','auth:read_self'),('guardian','auth:password_change_self'),('accountant','auth:read_self'),('accountant','auth:password_change_self') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	const oldPassword = "Old-account-password-2026!"
	const newPassword = "New-account-password-2026!"
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x64}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(label, role string) auth.User {
		u, e := service.BootstrapUser(ctx, label+"-pw-"+suffix+"@example.test", oldPassword, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	owner, bystander, locked, restricted := makeUser("owner", "teacher"), makeUser("bystander", "guardian"), makeUser("locked", "teacher"), makeUser("restricted", "accountant")
	ids := []string{owner.ID, bystander.ID, locked.ID, restricted.ID}
	t.Cleanup(func() {
		_ = db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('accountant','auth:password_change_self') ON CONFLICT DO NOTHING").Error
		_ = db.Exec("DELETE FROM users WHERE id IN ?", ids).Error
	})
	router := NewRouterWithAuth(service, true)

	type session struct {
		access  string
		cookies []*http.Cookie
		csrf    string
	}
	login := func(u auth.User, password string) (session, int) {
		r := perform(router, http.MethodPost, "/auth/login", `{"email":"`+u.Email+`","password":"`+password+`"}`, nil, nil)
		if r.Code != http.StatusOK {
			return session{}, r.Code
		}
		var body struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		cookies, csrf := sessionCookies(t, r)
		return session{access: body.Data.AccessToken, cookies: cookies, csrf: csrf}, r.Code
	}
	mustLogin := func(u auth.User, password string) session {
		t.Helper()
		s, code := login(u, password)
		if code != http.StatusOK {
			t.Fatalf("login %s: %d", u.Email, code)
		}
		return s
	}
	me := func(s session) int {
		return perform(router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + s.access}).Code
	}
	refresh := func(s session) (session, *httptest.ResponseRecorder) {
		r := perform(router, http.MethodPost, "/auth/refresh", "", s.cookies, map[string]string{"X-CSRF-Token": s.csrf})
		if r.Code != http.StatusOK {
			return session{}, r
		}
		var body struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &body)
		cookies, csrf := sessionCookies(t, r)
		return session{access: body.Data.AccessToken, cookies: cookies, csrf: csrf}, r
	}
	change := func(s session, body string) *httptest.ResponseRecorder {
		return perform(router, http.MethodPost, "/auth/password", body, nil, map[string]string{"Authorization": "Bearer " + s.access})
	}
	expectCode := func(name string, r *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if r.Code != status || (code != "" && !strings.Contains(r.Body.String(), `"code":"`+code+`"`)) {
			t.Fatalf("%s: got %d %s, want %d %s", name, r.Code, r.Body.String(), status, code)
		}
	}
	payload := func(current, next string) string {
		return `{"current_password":"` + current + `","new_password":"` + next + `"}`
	}

	current := mustLogin(owner, oldPassword)
	other := mustLogin(owner, oldPassword)
	unrelated := mustLogin(bystander, oldPassword)

	// Access control and input handling.
	if got := perform(router, http.MethodPost, "/auth/password", payload(oldPassword, newPassword), nil, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous change: %d", got)
	}
	expectCode("unknown field", change(current, `{"current_password":"`+oldPassword+`","new_password":"`+newPassword+`","role":"admin"}`), http.StatusBadRequest, "")
	expectCode("weak password", change(current, payload(oldPassword, "short")), http.StatusBadRequest, "weak_password")
	expectCode("same password", change(current, payload(oldPassword, oldPassword)), http.StatusBadRequest, "same_password")
	expectCode("wrong current password", change(current, payload("Wrong-password-2026!", newPassword)), http.StatusBadRequest, "invalid_current_password")
	if _, code := login(owner, oldPassword); code != http.StatusOK {
		t.Fatalf("wrong current password changed credentials: login %d", code)
	}
	if me(other) != http.StatusOK {
		t.Fatal("failed attempts revoked other sessions")
	}

	// Success keeps this session and revokes the owner's others only.
	ok := change(current, payload(oldPassword, newPassword))
	expectCode("change", ok, http.StatusOK, "")
	var result struct {
		Data struct {
			SessionsRevoked int64 `json:"sessions_revoked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &result); err != nil || result.Data.SessionsRevoked < 2 {
		t.Fatalf("sessions revoked: %s %v", ok.Body.String(), err)
	}
	if me(current) != http.StatusOK {
		t.Fatal("current session was signed out")
	}
	if me(other) != http.StatusUnauthorized {
		t.Fatal("other session still has access")
	}
	if _, r := refresh(other); r.Code != http.StatusUnauthorized {
		t.Fatalf("other session refreshed: %d", r.Code)
	}
	if _, code := login(owner, oldPassword); code != http.StatusUnauthorized {
		t.Fatalf("old password still works: %d", code)
	}
	if _, code := login(owner, newPassword); code != http.StatusOK {
		t.Fatalf("new password rejected: %d", code)
	}
	if me(unrelated) != http.StatusOK {
		t.Fatal("unrelated user's session was revoked")
	}
	if _, r := refresh(unrelated); r.Code != http.StatusOK {
		t.Fatalf("unrelated user's refresh failed: %d", r.Code)
	}

	// Refresh reuse within the grace period is refused without revocation.
	reuser := mustLogin(owner, newPassword)
	rotated, r := refresh(reuser)
	expectCode("rotation", r, http.StatusOK, "")
	if _, r := refresh(reuser); r.Code != http.StatusUnauthorized {
		t.Fatalf("immediate replay: %d", r.Code)
	}
	if me(rotated) != http.StatusOK {
		t.Fatal("immediate replay revoked the session")
	}
	// Later, replaying a rotated token revokes the whole session family.
	if err := db.Exec("UPDATE auth_refresh_history SET rotated_at = rotated_at - interval '1 minute' WHERE session_id IN (SELECT id FROM auth_sessions WHERE user_id = ?)", owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	replay := perform(router, http.MethodPost, "/auth/refresh", "", reuser.cookies, map[string]string{"X-CSRF-Token": reuser.csrf})
	expectCode("late replay", replay, http.StatusUnauthorized, "invalid_session")
	cleared := false
	for _, c := range replay.Result().Cookies() {
		if c.Name == refreshCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("late replay did not clear the refresh cookie")
	}
	if me(rotated) != http.StatusUnauthorized {
		t.Fatal("access token survived refresh reuse")
	}
	if _, r := refresh(rotated); r.Code != http.StatusUnauthorized {
		t.Fatalf("newest refresh token survived reuse: %d", r.Code)
	}
	if me(current) != http.StatusOK {
		t.Fatal("reuse revoked an unrelated session of the same user")
	}
	if me(unrelated) != http.StatusOK {
		t.Fatal("reuse revoked another user's session")
	}

	// Wrong current passwords count toward the lockout.
	victim := mustLogin(locked, oldPassword)
	for attempt := 0; attempt < auth.MaxLoginFailure; attempt++ {
		expectCode("lockout attempt", change(victim, payload("Wrong-password-2026!", newPassword)), http.StatusBadRequest, "invalid_current_password")
	}
	expectCode("locked change", change(victim, payload(oldPassword, newPassword)), http.StatusTooManyRequests, "account_locked")
	if me(victim) != http.StatusOK {
		t.Fatal("lockout revoked the current session")
	}

	// The permission is enforced per role.
	limited := mustLogin(restricted, oldPassword)
	if err := db.Exec("DELETE FROM role_permissions WHERE role_id='accountant' AND permission_id='auth:password_change_self'").Error; err != nil {
		t.Fatal(err)
	}
	expectCode("without permission", change(limited, payload(oldPassword, newPassword)), http.StatusForbidden, "forbidden")
	if _, code := login(restricted, oldPassword); code != http.StatusOK {
		t.Fatal("denied change altered the password")
	}

	// Audit events record the change and the reuse, never secrets.
	var events []struct {
		Action   string
		Metadata string
	}
	if err := db.Raw("SELECT action, metadata::text AS metadata FROM audit_events WHERE target_id IN ? OR actor_user_id IN ?", ids, ids).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	secrets := []string{oldPassword, newPassword, "Wrong-password-2026!", current.access, current.csrf}
	for _, c := range append(append([]*http.Cookie{}, reuser.cookies...), rotated.cookies...) {
		secrets = append(secrets, c.Value)
	}
	for _, e := range events {
		actions[e.Action]++
		for _, secret := range secrets {
			if secret != "" && strings.Contains(e.Metadata, secret) {
				t.Fatalf("audit event %s contains a secret: %s", e.Action, e.Metadata)
			}
		}
		if strings.Contains(strings.ToLower(e.Metadata), "password\"") || strings.Contains(strings.ToLower(e.Metadata), "token") {
			t.Fatalf("audit event %s has a secret-like field: %s", e.Action, e.Metadata)
		}
	}
	if actions["user.password_changed"] != 1 || actions["auth.refresh_reuse_detected"] != 1 {
		t.Fatalf("audit actions: %+v", actions)
	}
}
