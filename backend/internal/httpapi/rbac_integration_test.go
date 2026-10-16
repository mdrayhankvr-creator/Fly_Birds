package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
)

func TestPostgresRBACLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL RBAC test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	for i := 0; i < 2; i++ {
		if err := database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var versions int64
	if err := db.Table("schema_migrations").Where("version IN ?", []int{1, 2, 3, 4, 101}).Count(&versions).Error; err != nil || versions != 5 {
		t.Fatalf("required migration versions=%d, error=%v", versions, err)
	}
	var roleCount int64
	if err := db.Table("roles").Count(&roleCount).Error; err != nil || roleCount != 6 {
		t.Fatalf("role count=%d, error=%v", roleCount, err)
	}
	for _, role := range []string{"admin", "head_teacher", "teacher", "student", "guardian", "accountant"} {
		var count int64
		if err := db.Table("role_permissions").Where("role_id = ? AND permission_id = ?", role, "auth:read_self").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("self permission for %s count=%d, error=%v", role, count, err)
		}
	}
	var adminGrants int64
	if err := db.Table("role_permissions").Where("role_id = ?", "admin").Count(&adminGrants).Error; err != nil || adminGrants < 19 {
		t.Fatalf("admin grants=%d, error=%v", adminGrants, err)
	}

	var suffix [10]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	unique := hex.EncodeToString(suffix[:])
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	const password = "SecureIsolatedTest123!"
	admin, err := service.BootstrapUser(ctx, "admin-"+unique+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := service.BootstrapUser(ctx, "teacher-"+unique+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM users WHERE id IN (?, ?)", admin.ID, teacher.ID).Error
		_ = db.Exec("DELETE FROM role_permissions WHERE role_id = 'teacher' AND permission_id <> 'auth:read_self'").Error
		_ = db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('teacher','auth:read_self') ON CONFLICT DO NOTHING").Error
	})
	router := NewRouterWithAuth(service, true, rbac.NewService(rbac.NewRepository(db)))
	login := func(email string) string {
		body, _ := json.Marshal(map[string]string{"email": email, "password": password})
		response := perform(router, http.MethodPost, "/auth/login", string(body), nil, nil)
		if response.Code != 200 {
			t.Fatalf("login status=%d", response.Code)
		}
		return credentialsFromResponse(t, response)
	}
	adminAccess := login(admin.Email)
	teacherAccess := login(teacher.Email)
	adminHeader := map[string]string{"Authorization": "Bearer " + adminAccess}
	teacherHeader := map[string]string{"Authorization": "Bearer " + teacherAccess}
	check := func(method, path, body string, headers map[string]string, want int) {
		t.Helper()
		got := perform(router, method, path, body, nil, headers)
		if got.Code != want {
			t.Fatalf("%s %s: got %d, want %d; body=%s", method, path, got.Code, want, got.Body.String())
		}
	}
	check("GET", "/roles", "", nil, 401)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self"]}`, nil, 401)
	check("GET", "/roles", "", teacherHeader, 403)
	check("PUT", "/roles/teacher/permissions", `{"ids":["users:read"]}`, teacherHeader, 403)
	rolePage := perform(router, "GET", "/roles?page=1&page_size=2&search=teach", "", nil, adminHeader)
	if rolePage.Code != 200 {
		t.Fatalf("filtered roles status=%d", rolePage.Code)
	}
	var filtered struct {
		Data struct {
			Items []rbac.Role `json:"items"`
			Total int64       `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rolePage.Body.Bytes(), &filtered); err != nil || filtered.Data.Total != 2 || len(filtered.Data.Items) != 2 {
		t.Fatalf("filtered role page mismatch: %v", err)
	}
	check("GET", "/permissions", "", adminHeader, 200)
	check("GET", "/roles/teacher/permissions", "", adminHeader, 200)
	check("GET", "/roles/nonexistent/permissions", "", adminHeader, 404)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self","unknown:permission"]}`, adminHeader, 400)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self","auth:read_self"]}`, adminHeader, 400)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self","roles:read"]}`, adminHeader, 200)
	check("GET", "/roles", "", teacherHeader, 200) // existing session sees new grant
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self","roles:read","permissions:manage"]}`, adminHeader, 200)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self"]}`, teacherHeader, 403) // management also requires the admin role
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self"]}`, adminHeader, 200)
	check("GET", "/roles", "", teacherHeader, 403) // revocation is immediate
	check("PUT", "/roles/teacher/permissions", `{"ids":[]}`, adminHeader, 200)
	check("GET", "/auth/me", "", teacherHeader, 403)
	check("PUT", "/roles/teacher/permissions", `{"ids":["auth:read_self"]}`, adminHeader, 200)
	check("GET", "/auth/me", "", teacherHeader, 200)
	check("PUT", "/roles/admin/permissions", `{"ids":["auth:read_self"]}`, adminHeader, 409)
	check("PUT", "/roles/admin/permissions", `{"ids":["permissions:manage"]}`, adminHeader, 409)
	check("GET", "/users/"+teacher.ID+"/roles", "", adminHeader, 200)
	check("PUT", "/users/"+teacher.ID+"/roles", `{"ids":["nonexistent"]}`, adminHeader, 400)
	check("PUT", "/users/"+teacher.ID+"/roles", `{"ids":["teacher","student"]}`, adminHeader, 400)
	check("PUT", "/users/"+teacher.ID+"/roles", `{"ids":["student"]}`, teacherHeader, 403)
	check("PUT", "/users/"+teacher.ID+"/roles", `{"ids":["student"]}`, adminHeader, 200)
	check("GET", "/auth/me", "", teacherHeader, 200)
	check("PUT", "/users/"+teacher.ID+"/roles", `{"ids":["teacher"]}`, adminHeader, 200)
	var activeAdmins int64
	if err := db.Table("users").Where("role = 'admin' AND is_active = true").Count(&activeAdmins).Error; err != nil {
		t.Fatal(err)
	}
	if activeAdmins == 1 {
		check("PUT", "/users/"+admin.ID+"/roles", `{"ids":["teacher"]}`, adminHeader, 409)
	}
	check("GET", "/auth/permissions", "", adminHeader, 200)
	check("GET", "/auth/permissions", "", teacherHeader, 200)
	var stored auth.User
	if err := db.First(&stored, "id = ?", teacher.ID).Error; err != nil || stored.Role != "teacher" {
		t.Fatalf("role persisted incorrectly: %v", err)
	}
	// Concurrent replacements serialize through the same transaction lock.
	// The stored set must be one complete request, never an interleaving.
	sets := []string{`{"ids":["auth:read_self","users:read"]}`, `{"ids":["auth:read_self","roles:read"]}`}
	statuses := make([]int, 2)
	var group sync.WaitGroup
	for i := range sets {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			statuses[index] = perform(router, "PUT", "/roles/teacher/permissions", sets[index], nil, adminHeader).Code
		}(i)
	}
	group.Wait()
	if statuses[0] != 200 || statuses[1] != 200 {
		t.Fatalf("concurrent update statuses: %v", statuses)
	}
	ids, err := rbac.NewRepository(db).RolePermissions(ctx, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "auth:read_self" || (ids[1] != "users:read" && ids[1] != "roles:read") {
		t.Fatalf("concurrent update produced partial set: %v", ids)
	}
}
