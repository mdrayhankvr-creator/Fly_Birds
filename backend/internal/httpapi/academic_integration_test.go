package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// This test intentionally waits for the coordinated migration. Its table check
// prevents this branch from creating or numbering Joy's session migration.
func TestPostgresAcademicLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL academic test not run")
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
	var hasSchema bool
	if err := db.Raw("SELECT to_regclass('public.academic_class_subjects') IS NOT NULL").Scan(&hasSchema).Error; err != nil {
		t.Fatal(err)
	}
	if !hasSchema {
		t.Skip("academic migration is awaiting Joy's migration number and registration")
	}
	var suffixBytes [6]byte
	if _, err := rand.Read(suffixBytes[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(suffixBytes[:])
	password := suffix + "-AcademicSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "academic-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "academic-teacher-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN (?,?)", admin.ID, teacher.ID).Error })
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
	router := NewRouterWithAcademicServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, academic.NewService(academic.NewRepository(db)))
	adminLogin, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	teacherLogin, err := authService.Login(ctx, teacher.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken}
	teacherHeaders := map[string]string{"Authorization": "Bearer " + teacherLogin.AccessToken}
	check := func(method, path, body string, headers map[string]string, status int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response.Body.Bytes()
	}
	check("GET", "/academic/classes", "", nil, 401)
	check("GET", "/academic/classes", "", teacherHeaders, 403)
	check("POST", "/academic/classes", `{"name":"Invalid","display_order":-1,"is_active":true}`, adminHeaders, 400)
	classBody := check("POST", "/academic/classes", `{"name":"Class `+suffix+`","display_order":10,"is_active":true}`, adminHeaders, 201)
	var classResponse struct {
		Data academic.Class `json:"data"`
	}
	if err := json.Unmarshal(classBody, &classResponse); err != nil {
		t.Fatal(err)
	}
	classID := classResponse.Data.ID
	if classID == "" {
		t.Fatal("class ID missing")
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
	})
	check("POST", "/academic/classes", `{"name":"class `+suffix+`","display_order":11,"is_active":true}`, adminHeaders, 409)
	code := "MAT" + suffix
	subjectBody := check("POST", "/academic/subjects", `{"name_en":"Mathematics `+suffix+`","name_bn":"গণিত","code":"`+code+`","is_active":true}`, adminHeaders, 201)
	var subjectResponse struct {
		Data academic.Subject `json:"data"`
	}
	if err := json.Unmarshal(subjectBody, &subjectResponse); err != nil {
		t.Fatal(err)
	}
	subjectID := subjectResponse.Data.ID
	if subjectID == "" {
		t.Fatal("subject ID missing")
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM academic_class_subjects WHERE subject_id=?", subjectID).Error
		_ = db.Exec("DELETE FROM academic_subjects WHERE id=?", subjectID).Error
	})
	check("POST", "/academic/subjects", `{"name_en":"Another","name_bn":"আরেকটি","code":"`+code+`","is_active":true}`, adminHeaders, 409)
	check("POST", "/academic/subjects", `{"name_en":"Another","name_bn":"আরেকটি","code":"bad code","is_active":true}`, adminHeaders, 400)
	if err := db.Exec("INSERT INTO academic_subjects (name_en,name_bn,code,is_active) VALUES (?,?,?,true)", "Direct", "সরাসরি", strings.ToLower(code)).Error; err == nil {
		t.Fatal("database accepted case-insensitive duplicate subject code")
	}
	classList := check("GET", "/academic/classes?search="+suffix+"&page=1&page_size=1", "", adminHeaders, 200)
	var classPage struct {
		Data academic.Page[academic.Class] `json:"data"`
	}
	if err := json.Unmarshal(classList, &classPage); err != nil {
		t.Fatal(err)
	}
	if classPage.Data.Total != 1 || len(classPage.Data.Items) != 1 {
		t.Fatalf("class filtering failed: %+v", classPage.Data)
	}
	classList = check("GET", "/academic/classes?search="+suffix+"&page=2&page_size=1", "", adminHeaders, 200)
	if err := json.Unmarshal(classList, &classPage); err != nil {
		t.Fatal(err)
	}
	if classPage.Data.Total != 1 || len(classPage.Data.Items) != 0 {
		t.Fatalf("class pagination failed: %+v", classPage.Data)
	}
	subjectList := check("GET", "/academic/subjects?search="+code+"&page=1&page_size=1", "", adminHeaders, 200)
	var subjectPage struct {
		Data academic.Page[academic.Subject] `json:"data"`
	}
	if err := json.Unmarshal(subjectList, &subjectPage); err != nil {
		t.Fatal(err)
	}
	if subjectPage.Data.Total != 1 || len(subjectPage.Data.Items) != 1 {
		t.Fatalf("subject filtering failed: %+v", subjectPage.Data)
	}
	path := "/academic/classes/" + classID + "/subjects"
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":true}`, adminHeaders, 400)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, adminHeaders, 201)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, adminHeaders, 409)
	if err := db.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,true)", classID, subjectID).Error; err == nil {
		t.Fatal("database accepted conflicting mapping flags or duplicate pair")
	}
	check("PATCH", path+"/"+subjectID, `{"is_required":false,"is_fourth_subject":true}`, adminHeaders, 200)
	mappingList := check("GET", path+"?search="+code+"&page=1&page_size=1", "", adminHeaders, 200)
	var mappingPage struct {
		Data academic.Page[academic.Mapping] `json:"data"`
	}
	if err := json.Unmarshal(mappingList, &mappingPage); err != nil {
		t.Fatal(err)
	}
	if mappingPage.Data.Total != 1 || len(mappingPage.Data.Items) != 1 || !mappingPage.Data.Items[0].IsFourthSubject || mappingPage.Data.Items[0].IsRequired {
		t.Fatalf("mapping semantics or search failed: %+v", mappingPage.Data)
	}
	if err := db.Exec("UPDATE academic_class_subjects SET is_required=true,is_fourth_subject=true WHERE class_id=? AND subject_id=?", classID, subjectID).Error; err == nil {
		t.Fatal("database accepted both mapping flags")
	}
	check("DELETE", path+"/"+subjectID, "", adminHeaders, 200)
	check("DELETE", path+"/"+subjectID, "", adminHeaders, 404)
	if err := db.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,gen_random_uuid(),true,false)", classID).Error; err == nil {
		t.Fatal("database accepted missing subject foreign key")
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/academic/classes", "", adminHeaders, 403)
}
