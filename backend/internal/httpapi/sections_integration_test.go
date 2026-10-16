package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresSectionsLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL Sections test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migration not idempotent: %v", err)
	}
	var versions int64
	if err := db.Table("schema_migrations").Where("version IN ?", []int{1, 2, 3, 4, 101, 102, 103}).Count(&versions).Error; err != nil || versions != 7 {
		t.Fatalf("migration ledger: %d %v", versions, err)
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-SectionSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "section-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	head, err := authService.BootstrapUser(ctx, "section-head-"+suffix+"@example.test", password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "section-teacher-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN (?,?,?)", admin.ID, head.ID, teacher.ID).Error })
	var oldEnabled bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
	})
	var sessionID, classID, otherClassID string
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('2188','2188-01-01','2188-12-31') RETURNING id").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", "Section Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,2) RETURNING id", "Other Class "+suffix).Scan(&otherClassID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id IN (?,?)", classID, otherClassID).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
	})
	router := NewRouterWithAcademicServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)))
	login := func(email string) map[string]string {
		t.Helper()
		result, err := authService.Login(ctx, email, password)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + result.AccessToken}
	}
	adminHeaders, headHeaders, teacherHeaders := login(admin.Email), login(head.Email), login(teacher.Email)
	check := func(method, path, body string, headers map[string]string, status int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response.Body.Bytes()
	}
	path := "/academic/sections"
	check("GET", path, "", nil, 401)
	check("GET", path, "", teacherHeaders, 403)
	check("GET", path, "", headHeaders, 200)
	input := func(class, name, code string, active bool) string {
		return fmt.Sprintf(`{"session_id":%q,"class_id":%q,"name":%q,"code":%q,"display_order":3,"is_active":%t}`, sessionID, class, name, code, active)
	}
	check("POST", path, input(classID, "Alpha", "A", true), headHeaders, 403)
	check("POST", path, input(classID, "Alpha", "bad code", true), adminHeaders, 400)
	check("POST", path, `{"session_id":"bad","class_id":"bad","name":"A","code":"A","display_order":0,"is_active":true}`, adminHeaders, 400)
	created := check("POST", path, input(classID, "Alpha", "a", true), adminHeaders, 201)
	var one struct {
		Data academic.Section `json:"data"`
	}
	if err := json.Unmarshal(created, &one); err != nil || one.Data.ID == "" || one.Data.Code != "A" || one.Data.ClassName == "" || one.Data.SessionName != "2188" {
		t.Fatalf("created section: %+v %v", one.Data, err)
	}
	id := one.Data.ID
	check("POST", path, input(classID, "Beta", "A", true), adminHeaders, 409)
	check("POST", path, input(classID, "alpha", "B", true), adminHeaders, 409)
	check("POST", path, input(otherClassID, "Alpha", "A", true), adminHeaders, 201)
	check("POST", path, input(classID, "Beta", "B", true), adminHeaders, 201)
	if err := db.Exec("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?)", sessionID, classID, "Direct", "A").Error; err == nil {
		t.Fatal("database accepted duplicate section code")
	}
	if err := db.Exec("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?)", sessionID, classID, "alpha", "C").Error; err == nil {
		t.Fatal("database accepted duplicate section name")
	}
	if err := db.Exec("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (gen_random_uuid(),?,?,?)", classID, "Missing", "X").Error; err == nil {
		t.Fatal("database accepted missing session foreign key")
	}
	if err := db.Exec("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,gen_random_uuid(),?,?)", sessionID, "Missing", "X").Error; err == nil {
		t.Fatal("database accepted missing class foreign key")
	}
	check("PATCH", path+"/"+id, input(otherClassID, "Alpha", "A", false), adminHeaders, 400)
	check("PATCH", path+"/"+id, input(classID, "Alpha Prime", "A", false), adminHeaders, 200)
	result := check("GET", path+"/"+id, "", headHeaders, 200)
	if err := json.Unmarshal(result, &one); err != nil || one.Data.IsActive || one.Data.Name != "Alpha Prime" {
		t.Fatalf("updated section: %+v %v", one.Data, err)
	}
	list := check("GET", path+"?session_id="+sessionID+"&class_id="+classID+"&status=inactive&search=Prime&page=1&page_size=1", "", headHeaders, 200)
	var page struct {
		Data academic.Page[academic.Section] `json:"data"`
	}
	if err := json.Unmarshal(list, &page); err != nil || page.Data.Total != 1 || len(page.Data.Items) != 1 {
		t.Fatalf("filtering: %+v %v", page.Data, err)
	}
	list = check("GET", path+"?session_id="+sessionID+"&class_id="+classID+"&page=2&page_size=1", "", adminHeaders, 200)
	if err := json.Unmarshal(list, &page); err != nil || page.Data.Total != 2 || len(page.Data.Items) != 1 {
		t.Fatalf("pagination: %+v %v", page.Data, err)
	}
	check("GET", path+"?session_id=bad", "", adminHeaders, 400)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", path, "", adminHeaders, 403)
}
