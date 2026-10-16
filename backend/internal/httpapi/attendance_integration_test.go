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

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresTeacherAttendance(t *testing.T) {
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
	if err := db.Table("schema_migrations").Where("version=?", 501).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("attendance migration missing: %d %v", count, err)
	}
	// Other integration tests can edit the shared role matrix. Restore the grants under test.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','attendance:teachers_read'),('admin','attendance:teachers_manage'),('admin','attendance:teachers_reports'),('head_teacher','attendance:teachers_read'),('head_teacher','attendance:teachers_manage'),('head_teacher','attendance:teachers_reports'),('head_teacher','attendance:teachers_read_self'),('teacher','attendance:teachers_read_self') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var oldEnabled bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
	})
	if err := db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-AttendanceSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "attendance-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "attendance-teacher-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	head, err := authService.BootstrapUser(ctx, "attendance-head-"+suffix+"@example.test", password, "head_teacher")
	if err != nil {
		t.Fatal(err)
	}
	var sessionID, teacherEmployeeID, headEmployeeID string
	t.Cleanup(func() {
		if sessionID != "" {
			_ = db.Exec("DELETE FROM teacher_attendance WHERE session_id=?", sessionID).Error
		}
		for _, id := range []string{teacherEmployeeID, headEmployeeID} {
			if id != "" {
				_ = db.Exec("DELETE FROM employee_leaves WHERE employee_id=?", id).Error
				_ = db.Exec("DELETE FROM employees WHERE id=?", id).Error
			}
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		_ = db.Exec("DELETE FROM users WHERE id IN (?,?,?)", admin.ID, teacher.ID, head.ID).Error
	})
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprintf("%04d", 3500+int(random[0])), "2026-01-01", "2026-12-31").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,joined_on) VALUES (?,?,?,'Attendance Teacher','শিক্ষক','Teacher','2026-01-01') RETURNING id", "AT"+strings.ToUpper(suffix), teacher.ID, "teacher").Scan(&teacherEmployeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,joined_on) VALUES (?,?,?,'Attendance Head','প্রধান শিক্ষক','Head Teacher','2026-01-01') RETURNING id", "AH"+strings.ToUpper(suffix), head.ID, "teacher").Scan(&headEmployeeID).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithBatchServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, attendance.NewService(attendance.NewRepository(db)))
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
	rosterPath := "/attendance/teachers/roster?session_id=" + sessionID + "&date=2026-01-01&page=1&page_size=1&search=" + suffix
	check("GET", rosterPath, "", nil, 401)
	check("GET", rosterPath, "", adminHeader, 403)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", rosterPath, "", teacherHeader, 403)
	rosterBody := check("GET", rosterPath, "", headHeader, 200)
	var roster struct {
		Data attendance.Page[attendance.RosterItem] `json:"data"`
	}
	if err := json.Unmarshal(rosterBody, &roster); err != nil || roster.Data.Total != 2 || len(roster.Data.Items) != 1 {
		t.Fatalf("roster %+v %v", roster, err)
	}
	input := func(employee, date, status string) string {
		return `{"session_id":"` + sessionID + `","employee_id":"` + employee + `","attendance_date":"` + date + `","status":"` + status + `"}`
	}
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-02-30", "present"), adminHeader, 400)
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2027-01-01", "present"), adminHeader, 409)
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-01", "late"), adminHeader, 400)
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-03", "leave"), adminHeader, 409)
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-01", "present"), teacherHeader, 403)
	check("POST", "/attendance/teachers", input(headEmployeeID, "2026-01-01", "present"), headHeader, 403)
	recordBody := check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-01", "present"), headHeader, 201)
	var recorded struct {
		Data attendance.Record `json:"data"`
	}
	if err := json.Unmarshal(recordBody, &recorded); err != nil || recorded.Data.ID == "" {
		t.Fatalf("record %+v %v", recorded, err)
	}
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-01", "present"), adminHeader, 409)
	check("PATCH", "/attendance/teachers/"+recorded.Data.ID, `{"status":"leave"}`, adminHeader, 409)
	check("PATCH", "/attendance/teachers/"+recorded.Data.ID, `{"status":"absent","note":"Corrected"}`, headHeader, 200)
	check("PATCH", "/attendance/teachers/"+recorded.Data.ID, `{"status":"present","note":"Verified"}`, adminHeader, 200)
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-02", "absent"), adminHeader, 201)
	if err := db.Exec("INSERT INTO employee_leaves (employee_id,starts_on,ends_on,leave_type,status,reviewed_by,reviewed_at) VALUES (?,?,?,'casual','approved',?,now())", teacherEmployeeID, "2026-01-03", "2026-01-03", admin.ID).Error; err != nil {
		t.Fatal(err)
	}
	check("POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-03", "leave"), headHeader, 201)
	historyBody := check("GET", "/attendance/teachers/me?session_id="+sessionID+"&page=1&page_size=2", "", teacherHeader, 200)
	var history struct {
		Data attendance.Page[attendance.Record] `json:"data"`
	}
	if err := json.Unmarshal(historyBody, &history); err != nil || history.Data.Total != 3 || len(history.Data.Items) != 2 {
		t.Fatalf("history %+v %v", history, err)
	}
	reportPath := "/attendance/teachers/monthly?session_id=" + sessionID + "&month=2026-01&search=" + suffix + "&page=1&page_size=10"
	reportBody := check("GET", reportPath, "", adminHeader, 200)
	var report struct {
		Data attendance.Page[attendance.Summary] `json:"data"`
	}
	if err := json.Unmarshal(reportBody, &report); err != nil || report.Data.Total != 2 {
		t.Fatalf("report %+v %v", report, err)
	}
	ownBody := check("GET", "/attendance/teachers/me/monthly?session_id="+sessionID+"&month=2026-01", "", teacherHeader, 200)
	var own struct {
		Data attendance.Summary `json:"data"`
	}
	if err := json.Unmarshal(ownBody, &own); err != nil || own.Data.Present != 1 || own.Data.Absent != 1 || own.Data.Leave != 1 {
		t.Fatalf("own report %+v %v", own, err)
	}
	check("GET", "/attendance/teachers/me/monthly?session_id="+sessionID+"&month=2027-01", "", teacherHeader, 409)
	concurrent := make(chan int, 2)
	for range 2 {
		go func() {
			response := perform(router, "POST", "/attendance/teachers", input(teacherEmployeeID, "2026-01-04", "absent"), nil, adminHeader)
			concurrent <- response.Code
		}()
	}
	first, second := <-concurrent, <-concurrent
	if !((first == 201 && second == 409) || (first == 409 && second == 201)) {
		t.Fatalf("concurrent duplicate entry results: %d, %d", first, second)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/attendance/teachers/me", "", teacherHeader, 403)
}
