package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresExamRouteProtection(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Other integration tests can edit the shared role matrix. Restore the grants under test.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','exams:manage'),('admin','exams:read'),('admin','exams:enter_marks'),('head_teacher','exams:read'),('teacher','exams:enter_marks'),('student','exams:read_self'),('guardian','exams:read_children') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var old bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key='examinations'").Scan(&old).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key='examinations'", old).Error
	})
	if err := db.Exec("UPDATE school_modules SET enabled=false WHERE school_id=1 AND module_key='examinations'").Error; err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x77}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "M7TestPassword!" + suffix
	users := []auth.User{}
	login := func(role string) map[string]string {
		u, err := authService.BootstrapUser(ctx, role+suffix+"@example.test", password, role)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		credential, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + credential.AccessToken}
	}
	admin, teacher, student, guardian := login("admin"), login("teacher"), login("student"), login("guardian")
	t.Cleanup(func() {
		for _, u := range users {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id=?", u.ID).Error
			_ = db.Exec("DELETE FROM users WHERE id=?", u.ID).Error
		}
	})
	router := NewRouterWithExams(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, exams.NewService(exams.NewRepository(db)))
	check := func(method, path, body string, headers map[string]string, want int) {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
	}
	check("GET", "/exams", "", nil, 401)
	check("GET", "/exams", "", admin, 403)
	if err := db.Exec("UPDATE school_modules SET enabled=true WHERE school_id=1 AND module_key='examinations'").Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/exams?page=1&page_size=5", "", admin, 200)
	check("GET", "/exams", "", teacher, 403)
	check("POST", "/exams", `{"name":"Invalid"}`, teacher, 403)
	check("GET", "/exams/assigned?page=1&page_size=5", "", teacher, 200)
	check("GET", "/exams/me/results", "", student, 404)
	check("GET", "/exams/children/00000000-0000-0000-0000-000000000001/results", "", guardian, 404)
	check("PATCH", "/exams/00000000-0000-0000-0000-000000000001/state", `{"status":"published"}`, teacher, 403)
	check("POST", "/exams", `{"name":"Invalid"}`, admin, 400)
}
