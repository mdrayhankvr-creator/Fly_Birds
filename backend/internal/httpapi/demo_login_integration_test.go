package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresDemoQuickLogin(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL quick-login test not run")
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
	var suffix [6]byte
	_, _ = rand.Read(suffix[:])
	unique := hex.EncodeToString(suffix[:])
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x44}, 32))
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithServices(service, true, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)))

	// Use this test's own accounts instead of the real seed-demo references.
	saved := auth.DemoQuickLoginAccounts
	t.Cleanup(func() { auth.DemoQuickLoginAccounts = saved })
	demoRef, lookalikeRef, mismatchRef := "demo.qlt-"+unique, "demo.qlh-"+unique, "demo.qla-"+unique
	auth.DemoQuickLoginAccounts = map[string]string{"teacher": demoRef, "head_teacher": lookalikeRef, "accountant": mismatchRef}
	const password = "QuickLoginTest123!"
	// A seed-demo teacher: demo address and a demo_records entry.
	demoTeacher, err := service.BootstrapUser(ctx, demoRef+"@"+auth.DemoDomain, password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	// Demo-looking address, but seed-demo never recorded it.
	lookalike, err := service.BootstrapUser(ctx, lookalikeRef+"@"+auth.DemoDomain, password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	// Recorded under a demo reference, but not at the demo address.
	mismatch, err := service.BootstrapUser(ctx, "accounts-"+unique+"@example.test", password, "accountant")
	if err != nil {
		t.Fatal(err)
	}
	for ref, id := range map[string]string{demoRef: demoTeacher.ID, mismatchRef: mismatch.ID} {
		if err := db.Exec("INSERT INTO demo_records (kind, ref, record_id) VALUES ('user', ?, ?)", ref, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM demo_records WHERE kind = 'user' AND ref IN (?, ?)", demoRef, mismatchRef).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (?, ?, ?)", demoTeacher.ID, lookalike.ID, mismatch.ID).Error
		_ = db.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", demoTeacher.ID, lookalike.ID, mismatch.ID).Error
	})

	send := func(method, host, body string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/auth/demo-login", strings.NewReader(body))
		request.Host = host
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			request.Header.Set(k, v)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	local := map[string]string{"Origin": "http://localhost:8092", "Sec-Fetch-Site": "same-origin"}
	expect := func(name string, got *httptest.ResponseRecorder, want int, code string) {
		t.Helper()
		if got.Code != want || (code != "" && !strings.Contains(got.Body.String(), `"`+code+`"`)) {
			t.Fatalf("%s: got %d %s, want %d %s", name, got.Code, got.Body.String(), want, code)
		}
	}

	// Off (the default): the routes look absent.
	expect("info while off", send("GET", "localhost:8092", "", nil), 404, "not_found")
	expect("login while off", send("POST", "localhost:8092", `{"role":"teacher"}`, local), 404, "not_found")

	service.EnableDemoQuickLogin(true)
	t.Cleanup(func() { service.EnableDemoQuickLogin(false) })

	// Only from this machine.
	expect("info from a school domain", send("GET", "school.example.edu.bd", "", nil), 404, "not_found")
	if got := send("GET", "127.0.0.1:8092", "", nil); got.Code != 200 || !strings.Contains(got.Body.String(), `"teacher"`) || strings.Contains(strings.ToLower(got.Body.String()), "password") {
		t.Fatalf("info from localhost: %d %s", got.Code, got.Body.String())
	}
	expect("login from a school domain", send("POST", "school.example.edu.bd", `{"role":"teacher"}`, map[string]string{"Origin": "https://school.example.edu.bd"}), 403, "local_only")
	expect("login with a forged Host but foreign Origin", send("POST", "localhost:8092", `{"role":"teacher"}`, map[string]string{"Origin": "https://evil.example"}), 403, "local_only")
	expect("login without Origin", send("POST", "localhost:8092", `{"role":"teacher"}`, nil), 403, "local_only")
	expect("cross-site browser request", send("POST", "localhost:8092", `{"role":"teacher"}`, map[string]string{"Origin": "http://localhost:8092", "Sec-Fetch-Site": "cross-site"}), 403, "local_only")

	// Only seed-demo accounts.
	expect("unknown role", send("POST", "localhost:8092", `{"role":"guardian"}`, local), 403, "not_demo_account")
	expect("role with no account mapped", send("POST", "localhost:8092", `{"role":"admin"}`, local), 403, "not_demo_account")
	expect("demo address not recorded by seed-demo", send("POST", "localhost:8092", `{"role":"head_teacher"}`, local), 403, "not_demo_account")
	expect("recorded account outside the demo address", send("POST", "localhost:8092", `{"role":"accountant"}`, local), 403, "not_demo_account")
	expect("extra fields", send("POST", "localhost:8092", `{"role":"teacher","password":"x"}`, local), 400, "invalid_request")

	// The seed-demo account works, without any password.
	ok := send("POST", "localhost:8092", `{"role":"teacher"}`, local)
	expect("demo teacher", ok, 200, "")
	if !strings.Contains(ok.Body.String(), demoTeacher.ID) || strings.Contains(ok.Body.String(), password) {
		t.Fatalf("demo login body: %s", ok.Body.String())
	}
	if cookies := ok.Result().Cookies(); len(cookies) == 0 {
		t.Fatal("demo login set no session cookies")
	}
	me := perform(router, "GET", "/auth/me", "", nil, map[string]string{"Authorization": "Bearer " + credentialsFromResponse(t, ok)})
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"teacher"`) {
		t.Fatalf("session from quick login: %d %s", me.Code, me.Body.String())
	}
	var audits int64
	if err := db.Table("audit_events").Where("actor_user_id = ? AND action = 'auth.demo_quick_login'", demoTeacher.ID).Count(&audits).Error; err != nil || audits != 1 {
		t.Fatalf("quick-login audit entries %d %v", audits, err)
	}

	// A deactivated demo account is refused.
	if err := db.Exec("UPDATE users SET is_active = false WHERE id = ?", demoTeacher.ID).Error; err != nil {
		t.Fatal(err)
	}
	expect("deactivated demo account", send("POST", "localhost:8092", `{"role":"teacher"}`, local), 403, "not_demo_account")
}
