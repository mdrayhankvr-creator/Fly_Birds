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

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestRejectNULQuery(t *testing.T) {
	router := NewRouter()
	for _, path := range []string{"/healthz?search=a%00b", "/healthz?a%00=1"} {
		if r := perform(router, "GET", path, "", nil, nil); r.Code != http.StatusBadRequest {
			t.Fatalf("%s got %d: %s", path, r.Code, r.Body.String())
		}
	}
	if r := perform(router, "GET", "/healthz?search=%E0%A6%86", "", nil, nil); r.Code != http.StatusOK {
		t.Fatalf("Bangla query rejected: %d", r.Code)
	}
}

func TestContainsJSONNUL(t *testing.T) {
	cases := map[string]bool{
		`{"name":"a\u0000b"}`:   true,
		`{"name":"\\\u0000"}`:   true,
		"{\"name\":\"a\x00b\"}": true,
		`{"name":"a\\u0000b"}`:  false,
		`{"name":"à আরিফ"}`:     false,
		`{"name":"plain"}`:      false,
	}
	for body, want := range cases {
		if got := containsJSONNUL([]byte(body)); got != want {
			t.Errorf("%q: got %v want %v", body, got, want)
		}
	}
}

func TestDecodeSchoolJSONRejectsNUL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	decode := func(body string) (int, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var v struct {
			Name string `json:"name"`
		}
		ok := decodeSchoolJSON(c, 1024, &v)
		return w.Code, ok
	}
	if code, ok := decode(`{"name":"a\u0000"}`); ok || code != http.StatusBadRequest {
		t.Fatalf("NUL escape accepted: %d %v", code, ok)
	}
	if _, ok := decode(`{"name":"a\\u0000"}`); !ok {
		t.Fatal("escaped backslash text rejected")
	}
}

// Before the boundary checks, NUL reached PostgreSQL and every module returned 500.
func TestPostgresNULInputReturnsBadRequest(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','academics:read'),('admin','academics:manage') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [6]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-NulSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x4e}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "nul-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	var oldEnabled bool
	if err = db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
		_ = db.Exec("DELETE FROM users WHERE id=?", admin.ID).Error
	})
	if err = db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithAcademicServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, academic.NewService(academic.NewRepository(db)))
	login, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": "Bearer " + login.AccessToken}
	if r := perform(router, "GET", "/academic/classes?search=a%00b&page=1&page_size=5", "", nil, headers); r.Code != http.StatusBadRequest {
		t.Fatalf("search with NUL got %d: %s", r.Code, r.Body.String())
	}
	if r := perform(router, "POST", "/academic/classes", `{"name":"Class `+suffix+`\u0000","display_order":1,"is_active":true}`, nil, headers); r.Code != http.StatusBadRequest {
		t.Fatalf("body with NUL got %d: %s", r.Code, r.Body.String())
	}
	var n int64
	if err = db.Table("academic_classes").Where("name LIKE ?", "Class "+suffix+"%").Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("NUL class stored: %d %v", n, err)
	}
	if r := perform(router, "GET", "/academic/classes?search="+strings.Repeat("a", 3)+"&page=1&page_size=5", "", nil, headers); r.Code != http.StatusOK {
		t.Fatalf("ordinary search got %d", r.Code)
	}
}
