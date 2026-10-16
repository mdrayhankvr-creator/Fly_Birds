package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

func TestPostgresStaffLifecycle(t *testing.T) {
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
	var count int64
	if err := db.Table("schema_migrations").Where("version=?", 401).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("staff migration missing: %d %v", count, err)
	}
	// Earlier authorization tests may replace role grants in this shared disposable database.
	// Restore only the grants this lifecycle needs, as the fresh 0401 migration seeds them.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','staff:manage'),('admin','staff_salary:read'),('admin','staff_salary:manage'),('admin','staff_leaves:manage'),('admin','staff_leaves:read'),('admin','staff:read'),('teacher','staff:read_self'),('teacher','staff_leaves:request_self'),('head_teacher','staff:read'),('head_teacher','staff:read_self') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		role, permission string
		want             int64
	}{{"admin", "staff:manage", 1}, {"admin", "staff_salary:read", 1}, {"teacher", "staff:read_self", 1}, {"teacher", "staff_salary:read", 0}, {"head_teacher", "staff:read", 1}} {
		if err := db.Table("role_permissions").Where("role_id=? AND permission_id=?", entry.role, entry.permission).Count(&count).Error; err != nil || count != entry.want {
			t.Fatalf("grant %s %s=%d err=%v", entry.role, entry.permission, count, err)
		}
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-StaffSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "staff-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "staff-teacher-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	head, err := authService.BootstrapUser(ctx, "staff-head-"+suffix+"@example.test", password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN (?,?,?)", admin.ID, teacher.ID, head.ID).Error })
	var oldEnabled bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
	})
	var sessionID, classID, subjectID string
	t.Cleanup(func() {
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		}
		if subjectID != "" {
			_ = db.Exec("DELETE FROM academic_subjects WHERE id=?", subjectID).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
	})
	router := NewRouterWithAcademicServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, staff.NewService(staff.NewRepository(db)))
	adminLogin, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	teacherLogin, err := authService.Login(ctx, teacher.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	headLogin, err := authService.Login(ctx, head.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	adminHeader := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken}
	teacherHeader := map[string]string{"Authorization": "Bearer " + teacherLogin.AccessToken}
	headHeader := map[string]string{"Authorization": "Bearer " + headLogin.AccessToken}
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	check("GET", "/staff/employees", "", nil, 401)
	check("GET", "/staff/employees", "", adminHeader, 403)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/staff/employees", "", teacherHeader, 403)
	check("GET", "/staff/employees", "", headHeader, 200)
	check("POST", "/staff/employees", `{"employee_code":"DENIED","kind":"teacher","name_en":"Denied","designation":"Teacher","joined_on":"2026-01-01"}`, headHeader, 403)
	check("POST", "/staff/employees", `{"employee_code":"INVALID","kind":"teacher","name_en":"Invalid","designation":"Teacher","joined_on":"2026-02-30"}`, adminHeader, 400)
	input := `{"employee_code":"T` + strings.ToUpper(suffix) + `","user_id":"` + teacher.ID + `","kind":"teacher","name_en":"Teacher ` + suffix + `","name_bn":"শিক্ষক","designation":"Assistant Teacher","joined_on":"2026-01-01"}`
	body := check("POST", "/staff/employees", input, adminHeader, 201)
	var created struct {
		Data staff.Employee `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Data.ID == "" || created.Data.UserID == nil || created.Data.JoinedOn != "2026-01-01" {
		t.Fatalf("created staff %+v err %v", created, err)
	}
	employeeID := created.Data.ID
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM teacher_assignments WHERE employee_id=?", employeeID).Error
		_ = db.Exec("DELETE FROM employee_leaves WHERE employee_id=?", employeeID).Error
		_ = db.Exec("DELETE FROM employee_salary_records WHERE employee_id=?", employeeID).Error
		_ = db.Exec("DELETE FROM employees WHERE id=?", employeeID).Error
	})
	check("POST", "/staff/employees", input, adminHeader, 409)
	check("GET", "/staff/me", "", teacherHeader, 200)
	check("GET", "/staff/me", "", headHeader, 404)
	check("GET", "/staff/employees?search="+suffix+"&page=1&page_size=1", "", adminHeader, 200)
	check("GET", "/staff/employees/"+employeeID+"/salaries", "", teacherHeader, 403)
	check("GET", "/staff/employees/"+employeeID+"/salaries", "", headHeader, 403)
	check("POST", "/staff/employees/"+employeeID+"/salaries", `{"effective_on":"2026-01-01","basic_bdt":"25000.00","allowance_bdt":"1000","deduction_bdt":"200"}`, adminHeader, 201)
	check("POST", "/staff/employees/"+employeeID+"/salaries", `{"effective_on":"2026-01-01","basic_bdt":"25000.00"}`, adminHeader, 409)
	check("POST", "/staff/employees/"+employeeID+"/salaries", `{"effective_on":"2026-02-01","basic_bdt":"-1"}`, adminHeader, 400)
	check("GET", "/staff/employees/"+employeeID+"/salaries", "", adminHeader, 200)
	leaveBody := check("POST", "/staff/me/leaves", `{"starts_on":"2026-02-01","ends_on":"2026-02-03","leave_type":"sick","reason":"Illness"}`, teacherHeader, 201)
	var leave struct {
		Data staff.Leave `json:"data"`
	}
	if err := json.Unmarshal(leaveBody, &leave); err != nil || leave.Data.ID == "" {
		t.Fatalf("leave %+v %v", leave, err)
	}
	check("POST", "/staff/me/leaves", `{"starts_on":"2026-02-03","ends_on":"2026-02-01","leave_type":"sick"}`, teacherHeader, 400)
	check("PATCH", "/staff/leaves/"+leave.Data.ID+"/decision", `{"status":"approved"}`, teacherHeader, 403)
	check("PATCH", "/staff/leaves/"+leave.Data.ID+"/decision", `{"status":"approved"}`, adminHeader, 200)
	check("PATCH", "/staff/leaves/"+leave.Data.ID+"/decision", `{"status":"rejected"}`, adminHeader, 409)
	check("GET", "/staff/me/leaves", "", teacherHeader, 200)
	// A valid class-subject pair and session are required before assignment.
	check("POST", "/staff/employees/"+employeeID+"/assignments", `{"session_id":"00000000-0000-0000-0000-000000000001","class_id":"00000000-0000-0000-0000-000000000002","subject_id":"00000000-0000-0000-0000-000000000003"}`, adminHeader, 409)
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprintf("%04d", 3000+int(random[0])), "2026-01-01", "2026-12-31").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,?) RETURNING id", "Staff class "+suffix, 10).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_subjects (name_en,name_bn,code) VALUES (?,?,?) RETURNING id", "Staff subject "+suffix, "বিষয়", "S"+strings.ToUpper(suffix)).Scan(&subjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	assignmentInput := `{"session_id":"` + sessionID + `","class_id":"` + classID + `","subject_id":"` + subjectID + `"}`
	assignmentBody := check("POST", "/staff/employees/"+employeeID+"/assignments", assignmentInput, adminHeader, 201)
	var assignment struct {
		Data staff.Assignment `json:"data"`
	}
	if err := json.Unmarshal(assignmentBody, &assignment); err != nil || assignment.Data.ID == "" {
		t.Fatalf("assignment %+v %v", assignment, err)
	}
	check("POST", "/staff/employees/"+employeeID+"/assignments", assignmentInput, adminHeader, 409)
	check("GET", "/staff/me/assignments", "", teacherHeader, 200)
	check("DELETE", "/staff/employees/"+employeeID+"/assignments/"+assignment.Data.ID, "", teacherHeader, 403)
	check("DELETE", "/staff/employees/"+employeeID+"/assignments/"+assignment.Data.ID, "", adminHeader, 200)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/staff/me", "", teacherHeader, 403)
}
