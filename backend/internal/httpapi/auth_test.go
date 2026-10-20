package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type handlerRepository struct {
	user    auth.User
	session auth.Session
}

func (r *handlerRepository) FindUserByEmail(_ context.Context, email string) (auth.User, error) {
	if email != r.user.Email {
		return auth.User{}, auth.ErrNotFound
	}
	return r.user, nil
}
func (r *handlerRepository) RecordLoginFailure(context.Context, string, time.Time) error { return nil }
func (r *handlerRepository) ClearLoginFailures(context.Context, string, time.Time) error { return nil }
func (r *handlerRepository) CreateSession(_ context.Context, session *auth.Session) error {
	r.session = *session
	return nil
}
func (r *handlerRepository) RotateSession(_ context.Context, oldHash, csrfHash, newHash, newCSRFHash []byte, expiry, now time.Time) (auth.Session, auth.User, error) {
	if !bytes.Equal(r.session.RefreshHash, oldHash) || !bytes.Equal(r.session.CSRFHash, csrfHash) || r.session.RevokedAt != nil || !r.session.ExpiresAt.After(now) {
		return auth.Session{}, auth.User{}, auth.ErrInvalidSession
	}
	r.session.RefreshHash, r.session.CSRFHash, r.session.ExpiresAt = newHash, newCSRFHash, expiry
	return r.session, r.user, nil
}
func (r *handlerRepository) RevokeSession(_ context.Context, hash, csrfHash []byte, now time.Time) error {
	if !bytes.Equal(r.session.RefreshHash, hash) || !bytes.Equal(r.session.CSRFHash, csrfHash) {
		return auth.ErrInvalidSession
	}
	r.session.RevokedAt = &now
	return nil
}
func (r *handlerRepository) ActiveUserBySession(_ context.Context, id string, now time.Time) (auth.User, error) {
	if id != r.session.ID || r.session.RevokedAt != nil || !r.session.ExpiresAt.After(now) {
		return auth.User{}, auth.ErrInvalidSession
	}
	return r.user, nil
}
func (r *handlerRepository) FindActiveUser(context.Context, string) (auth.User, error) {
	return auth.User{}, auth.ErrNotFound
}
func (r *handlerRepository) ChangePassword(context.Context, string, string, string, string, time.Time) (int64, error) {
	return 0, auth.ErrInvalidCredentials
}
func (r *handlerRepository) CreateUser(context.Context, *auth.User) error {
	return errors.New("unexpected create")
}
func (r *handlerRepository) HasPermission(_ context.Context, _ string, permission string) (bool, error) {
	return permission == "auth:read_self", nil
}
func (r *handlerRepository) PermissionsForUser(context.Context, string) ([]string, error) {
	return []string{"auth:read_self"}, nil
}

func authTestRouter(t *testing.T) *httpRouterFixture {
	t.Helper()
	hash, err := auth.HashPassword("StrongTestPassword123!")
	if err != nil {
		t.Fatal(err)
	}
	repo := &handlerRepository{user: auth.User{ID: "db763ef1-73ed-4f07-a4fb-6ed46397b675", Email: "teacher@example.test", PasswordHash: hash, Role: "teacher", IsActive: true}}
	service, err := auth.NewService(repo, bytes.Repeat([]byte{0x37}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return &httpRouterFixture{router: NewRouterWithAuth(service, true), repo: repo}
}

type httpRouterFixture struct {
	router http.Handler
	repo   *handlerRepository
}

func perform(router http.Handler, method, path, body string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func credentialsFromResponse(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AccessToken == "" {
		t.Fatal("missing access token")
	}
	return body.Data.AccessToken
}

func sessionCookies(t *testing.T, response *httptest.ResponseRecorder) ([]*http.Cookie, string) {
	t.Helper()
	var refresh, csrf *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == refreshCookie {
			refresh = cookie
		}
		if cookie.Name == csrfCookie {
			csrf = cookie
		}
	}
	if refresh == nil || csrf == nil {
		t.Fatal("missing session cookies")
	}
	if !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteStrictMode || csrf.HttpOnly || !csrf.Secure {
		t.Fatal("incorrect session cookie security attributes")
	}
	if strings.Contains(response.Body.String(), refresh.Value) || strings.Contains(response.Body.String(), csrf.Value) {
		t.Fatal("refresh or CSRF secret leaked into JSON")
	}
	return []*http.Cookie{refresh, csrf}, csrf.Value
}

func TestAuthHTTPLifecycle(t *testing.T) {
	fixture := authTestRouter(t)
	login := perform(fixture.router, http.MethodPost, "/auth/login", `{"email":"teacher@example.test","password":"StrongTestPassword123!"}`, nil, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", login.Code, login.Body.String())
	}
	if login.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("auth response may be cached")
	}
	access := credentialsFromResponse(t, login)
	cookies, csrf := sessionCookies(t, login)
	me := perform(fixture.router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + access})
	if me.Code != http.StatusOK {
		t.Fatalf("me status %d", me.Code)
	}
	missingCSRF := perform(fixture.router, http.MethodPost, "/auth/refresh", "", cookies, nil)
	if missingCSRF.Code != http.StatusUnauthorized {
		t.Fatalf("missing CSRF status %d", missingCSRF.Code)
	}
	refreshed := perform(fixture.router, http.MethodPost, "/auth/refresh", "", cookies, map[string]string{"X-CSRF-Token": csrf})
	if refreshed.Code != http.StatusOK {
		t.Fatalf("refresh status %d: %s", refreshed.Code, refreshed.Body.String())
	}
	newAccess := credentialsFromResponse(t, refreshed)
	newCookies, newCSRF := sessionCookies(t, refreshed)
	if newCookies[0].Value == cookies[0].Value {
		t.Fatal("refresh cookie did not rotate")
	}
	oldRefresh := perform(fixture.router, http.MethodPost, "/auth/refresh", "", cookies, map[string]string{"X-CSRF-Token": csrf})
	if oldRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh accepted: %d", oldRefresh.Code)
	}
	logout := perform(fixture.router, http.MethodPost, "/auth/logout", "", newCookies, map[string]string{"X-CSRF-Token": newCSRF})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status %d", logout.Code)
	}
	revoked := perform(fixture.router, http.MethodGet, "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + newAccess})
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked access accepted: %d", revoked.Code)
	}
}

func TestLoginErrorsDoNotRevealAccountStatus(t *testing.T) {
	fixture := authTestRouter(t)
	wrong := perform(fixture.router, http.MethodPost, "/auth/login", `{"email":"teacher@example.test","password":"wrong"}`, nil, nil)
	unknown := perform(fixture.router, http.MethodPost, "/auth/login", `{"email":"missing@example.test","password":"wrong"}`, nil, nil)
	if wrong.Code != http.StatusUnauthorized || unknown.Code != wrong.Code || wrong.Body.String() != unknown.Body.String() {
		t.Fatalf("login errors differ: %d %q vs %d %q", wrong.Code, wrong.Body.String(), unknown.Code, unknown.Body.String())
	}
	bad := perform(fixture.router, http.MethodPost, "/auth/login", `{"email":"bad","password":"x"}`, nil, nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid input status %d", bad.Code)
	}
	form := perform(fixture.router, http.MethodPost, "/auth/login", `{"email":"teacher@example.test","password":"x"}`, nil, map[string]string{"Content-Type": "text/plain"})
	if form.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("form-compatible request accepted: %d", form.Code)
	}
	unavailable := perform(NewRouter(), http.MethodPost, "/auth/login", `{"email":"teacher@example.test","password":"x"}`, nil, nil)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status %d", unavailable.Code)
	}
}

func TestRequirePermissionDeniesUnknownPermission(t *testing.T) {
	router := gin.New()
	router.GET("/denied", RequirePermission(nil, "unknown:action"), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	response := perform(router, http.MethodGet, "/denied", "", nil, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown permission status %d", response.Code)
	}
}
