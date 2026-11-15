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
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/mailer"
)

// sinkMailer keeps the messages the endpoint caused to be sent.
type sinkMailer struct{ sent chan mailer.Message }

func (m sinkMailer) Send(_ context.Context, message mailer.Message) error {
	m.sent <- message
	return nil
}

// TestPostgresForgotPasswordEndpoints checks what the sign-in page can see:
// the feature is announced only when a relay is configured, a known and an
// unknown address get byte-for-byte the same answer, the limit answers 429,
// and the link sets the password once.
func TestPostgresForgotPasswordEndpoints(t *testing.T) {
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
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}

	post := func(router http.Handler, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		// What nginx forwards: one address, the client's own.
		request.Header.Set("X-Forwarded-For", "2001:db8:f0:"+suffix+"::1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	get := func(router http.Handler, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}

	// 1. With no relay configured the page is told the feature is off, and
	//    the endpoints refuse with 503 rather than pretending.
	off := NewRouterWithAuth(service, true)
	available := get(off, "/auth/password/forgot")
	if available.Code != http.StatusOK || !strings.Contains(available.Body.String(), `"enabled":false`) {
		t.Fatalf("availability with no relay: %d %s", available.Code, available.Body.String())
	}
	if refused := post(off, "/auth/password/forgot", `{"email":"someone@example.test"}`); refused.Code != http.StatusServiceUnavailable {
		t.Fatalf("a request with no relay: %d %s", refused.Code, refused.Body.String())
	}
	if refused := get(off, "/auth/password/reset?token=whatever"); refused.Code != http.StatusServiceUnavailable {
		t.Fatalf("a link check with no relay: %d", refused.Code)
	}

	// 2. Switch a relay on.
	sink := sinkMailer{sent: make(chan mailer.Message, 8)}
	if err := service.EnablePasswordResetEmail(auth.NewRepository(db), sink, "https://school.example.test", nil, nil); err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithAuth(service, true)
	email := "forgot-" + suffix + "@example.test"
	user, err := service.BootstrapUser(ctx, email, "Old-account-password-2026!", "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM password_reset_requests WHERE user_id = ? OR requester_ip LIKE ?", user.ID, "2001:db8:f0:"+suffix+"%").Error
		_ = db.Exec("DELETE FROM audit_events WHERE target_id = ?", user.ID).Error
		_ = db.Exec("DELETE FROM users WHERE id = ?", user.ID).Error
	})
	if available := get(router, "/auth/password/forgot"); !strings.Contains(available.Body.String(), `"enabled":true`) {
		t.Fatalf("availability with a relay: %s", available.Body.String())
	}

	// 3. A known and an unknown address answer identically, so the endpoint
	//    cannot be used to find out who has an account.
	known := post(router, "/auth/password/forgot", `{"email":"`+email+`","language":"bn"}`)
	message := <-sink.sent
	unknown := post(router, "/auth/password/forgot", `{"email":"nobody-`+suffix+`@example.test"}`)
	if known.Code != http.StatusAccepted || unknown.Code != known.Code || unknown.Body.String() != known.Body.String() {
		t.Fatalf("the answers differ: %d %s vs %d %s", known.Code, known.Body.String(), unknown.Code, unknown.Body.String())
	}
	if !strings.Contains(known.Body.String(), `"sent":true`) {
		t.Fatalf("unexpected body: %s", known.Body.String())
	}
	select {
	case stray := <-sink.sent:
		t.Fatalf("an email was sent for an address with no account: %q", stray.Subject)
	case <-time.After(300 * time.Millisecond):
	}

	// 4. The limit on one address answers 429. It counts attempts for
	//    addresses with no account too, so this leaks nothing.
	for i := 1; i < auth.ResetMaxPerEmail; i++ {
		if again := post(router, "/auth/password/forgot", `{"email":"`+email+`"}`); again.Code != http.StatusAccepted {
			t.Fatalf("request %d: %d %s", i+1, again.Code, again.Body.String())
		}
		<-sink.sent
	}
	if limited := post(router, "/auth/password/forgot", `{"email":"`+email+`"}`); limited.Code != http.StatusTooManyRequests ||
		!strings.Contains(limited.Body.String(), "reset_rate_limited") {
		t.Fatalf("over the limit: %d %s", limited.Code, limited.Body.String())
	}

	// 5. The link from the email checks out, sets the password once, and is
	//    dead afterwards.
	marker := "/reset-password?token="
	index := strings.Index(message.Text, marker)
	if index < 0 {
		t.Fatalf("no link in the message: %q", message.Text)
	}
	token := message.Text[index+len(marker):]
	if cut := strings.IndexAny(token, " \r\n"); cut >= 0 {
		token = token[:cut]
	}
	if check := get(router, "/auth/password/reset?token="+token); check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"valid":true`) {
		t.Fatalf("check the link: %d %s", check.Code, check.Body.String())
	}
	if weak := post(router, "/auth/password/reset", `{"token":"`+token+`","new_password":"short"}`); weak.Code != http.StatusBadRequest ||
		!strings.Contains(weak.Body.String(), "weak_password") {
		t.Fatalf("a short password: %d %s", weak.Code, weak.Body.String())
	}
	done := post(router, "/auth/password/reset", `{"token":"`+token+`","new_password":"New-account-password-2026!"}`)
	if done.Code != http.StatusOK {
		t.Fatalf("set the password: %d %s", done.Code, done.Body.String())
	}
	var body struct {
		Data struct {
			SessionsRevoked int64 `json:"sessions_revoked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(done.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if again := post(router, "/auth/password/reset", `{"token":"`+token+`","new_password":"Third-account-password-26!"}`); again.Code != http.StatusBadRequest ||
		!strings.Contains(again.Body.String(), "invalid_reset_token") {
		t.Fatalf("the link worked twice: %d %s", again.Code, again.Body.String())
	}
	if _, err := service.Login(ctx, email, "New-account-password-2026!"); err != nil {
		t.Fatalf("the new password does not sign in: %v", err)
	}

	// 6. A link check with no token, and an unknown token, are one answer.
	for _, path := range []string{"/auth/password/reset", "/auth/password/reset?token=not-a-token"} {
		if bad := get(router, path); bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_reset_token") {
			t.Fatalf("%s: %d %s", path, bad.Code, bad.Body.String())
		}
	}
}
