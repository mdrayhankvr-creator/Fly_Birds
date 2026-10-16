package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// The migration runner must install both Joy's sessions schema and the
// independent classes/subjects schema before this lifecycle can run.
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
		t.Fatal("academic migration 0102 was not installed")
	}
	var requiredVersions int64
	if err := db.Table("schema_migrations").Where("version IN ?", []int{1, 2, 3, 4, 101, 102}).Count(&requiredVersions).Error; err != nil || requiredVersions != 6 {
		t.Fatalf("required migrations: %d, error %v", requiredVersions, err)
	}
	for _, check := range []struct {
		role, permission string
		want             int64
	}{{"admin", "academics:read", 1}, {"admin", "academics:manage", 1}, {"head_teacher", "academics:read", 1}, {"head_teacher", "academics:manage", 0}} {
		var count int64
		if err := db.Table("role_permissions").Where("role_id=? AND permission_id=?", check.role, check.permission).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("%s %s grant: count=%d error=%v", check.role, check.permission, count, err)
		}
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
	headTeacher, err := authService.BootstrapUser(ctx, "academic-head-"+suffix+"@example.test", password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM users WHERE id IN (?,?,?)", admin.ID, teacher.ID, headTeacher.ID).Error
	})
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
	headLogin, err := authService.Login(ctx, headTeacher.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken}
	teacherHeaders := map[string]string{"Authorization": "Bearer " + teacherLogin.AccessToken}
	headHeaders := map[string]string{"Authorization": "Bearer " + headLogin.AccessToken}
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
	check("GET", "/academic/classes", "", headHeaders, 200)
	check("POST", "/academic/classes", `{"name":"Denied","display_order":1,"is_active":true}`, headHeaders, 403)
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
	classUpdated := check("PATCH", "/academic/classes/"+classID, `{"name":"Class `+suffix+`","display_order":12,"is_active":true}`, adminHeaders, 200)
	if err := json.Unmarshal(classUpdated, &classResponse); err != nil || classResponse.Data.DisplayOrder != 12 {
		t.Fatalf("class update failed: %+v %v", classResponse.Data, err)
	}
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
	if err := db.Exec("INSERT INTO academic_subjects (name_en,name_bn,code,is_active) VALUES (?,?,?,true)", "Direct", "সরাসরি", code).Error; err == nil {
		t.Fatal("database accepted duplicate subject code")
	}
	check("GET", "/academic/subjects", "", headHeaders, 200)
	check("PATCH", "/academic/subjects/"+subjectID, `{"name_en":"Mathematics `+suffix+`","name_bn":"উচ্চতর গণিত","code":"`+code+`","is_active":true}`, adminHeaders, 200)
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
	check("GET", path, "", headHeaders, 200)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, headHeaders, 403)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":true}`, adminHeaders, 400)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, adminHeaders, 201)
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, adminHeaders, 409)
	if err := db.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, subjectID).Error; err == nil {
		t.Fatal("database accepted duplicate class-subject pair")
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
	mappingList = check("GET", path+"?search="+code+"&page=2&page_size=1", "", adminHeaders, 200)
	if err := json.Unmarshal(mappingList, &mappingPage); err != nil || mappingPage.Data.Total != 1 || len(mappingPage.Data.Items) != 0 {
		t.Fatalf("mapping pagination failed: %+v %v", mappingPage.Data, err)
	}
	if err := db.Exec("UPDATE academic_class_subjects SET is_required=true,is_fourth_subject=true WHERE class_id=? AND subject_id=?", classID, subjectID).Error; err == nil {
		t.Fatal("database accepted both mapping flags")
	}
	if err := db.Exec("UPDATE academic_class_subjects SET is_required=false,is_fourth_subject=false WHERE class_id=? AND subject_id=?", classID, subjectID).Error; err == nil {
		t.Fatal("database accepted neither mapping flag")
	}
	check("DELETE", path+"/"+subjectID, "", adminHeaders, 200)
	check("DELETE", path+"/"+subjectID, "", adminHeaders, 404)
	check("PATCH", "/academic/classes/"+classID, `{"name":"Class `+suffix+`","display_order":12,"is_active":false}`, adminHeaders, 200)
	classList = check("GET", "/academic/classes?status=active&search="+suffix, "", adminHeaders, 200)
	if err := json.Unmarshal(classList, &classPage); err != nil || classPage.Data.Total != 0 {
		t.Fatalf("active class filter failed: %+v %v", classPage.Data, err)
	}
	classList = check("GET", "/academic/classes?status=inactive&search="+suffix, "", adminHeaders, 200)
	if err := json.Unmarshal(classList, &classPage); err != nil || classPage.Data.Total != 1 {
		t.Fatalf("inactive class filter failed: %+v %v", classPage.Data, err)
	}
	check("POST", path, `{"subject_id":"`+subjectID+`","is_required":true,"is_fourth_subject":false}`, adminHeaders, 409)
	if err := db.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,gen_random_uuid(),true,false)", classID).Error; err == nil {
		t.Fatal("database accepted missing subject foreign key")
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='academics'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/academic/classes", "", adminHeaders, 403)
}
