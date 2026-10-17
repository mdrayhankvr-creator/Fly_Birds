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
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresRoutineRouteProtection(t *testing.T) {
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
	if err := db.Exec("INSERT INTO role_permissions(role_id,permission_id) VALUES ('admin','routine:manage'),('admin','routine:read'),('teacher','routine:read_self'),('student','routine:read_self'),('guardian','routine:read_children') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var oldEnabled bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
	})
	if err := db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x66}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "RoutinePass!" + suffix
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
	router := NewRouterWithRoutine(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, routine.NewService(routine.NewRepository(db)))
	check := func(method, path, body string, headers map[string]string, want int) {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
	}
	check("GET", "/routines/classes", "", nil, 401)
	check("GET", "/routines/classes", "", admin, 403)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/routines/classes?page=1&page_size=5", "", admin, 200)
	check("GET", "/routines/exams?page=1&page_size=5", "", admin, 200)
	check("GET", "/routines/classes", "", teacher, 403)
	check("POST", "/routines/classes", `{"day_of_week":0}`, teacher, 403)
	check("GET", "/routines/classes/me", "", teacher, 200)
	check("GET", "/routines/classes/me", "", student, 200)
	check("GET", "/routines/exams/me", "", student, 200)
	check("GET", "/routines/exams/me", "", teacher, 403)
	check("GET", "/routines/classes/children/00000000-0000-0000-0000-000000000001", "", guardian, 404)
	check("POST", "/routines/exams", `{"name":"Invalid"}`, admin, 400)
}
