package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresStudentAttendance(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO role_permissions(role_id,permission_id) VALUES ('admin','attendance:students_read'),('admin','attendance:students_manage'),('admin','attendance:alerts_read'),('teacher','attendance:students_read'),('teacher','attendance:students_manage'),('student','attendance:students_read_self'),('guardian','attendance:guardians_read_children') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var n int64
	if err = db.Table("schema_migrations").Where("version IN ?", []int{1, 2, 3, 4, 101, 102, 103, 201, 401, 501, 502}).Count(&n).Error; err != nil || n != 11 {
		t.Fatalf("migration ledger %d %v", n, err)
	}
	var random [7]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := "M5Secret!" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		u, e := authService.BootstrapUser(ctx, label+"-m5-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, teacher, unassigned, studentUser, guardianUser, otherGuardian := makeUser("admin", "admin"), makeUser("teacher", "teacher"), makeUser("teacher", "other-teacher"), makeUser("student", "student"), makeUser("guardian", "guardian"), makeUser("guardian", "other-guardian")
	ids := []string{admin.ID, teacher.ID, unassigned.ID, studentUser.ID, guardianUser.ID, otherGuardian.ID}
	var oldEnabled bool
	if err = db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
	})
	if err = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var sessionID, classID, sectionID, subjectID, employeeID, studentID, otherStudentID, guardianID string
	t.Cleanup(func() {
		if sessionID != "" {
			_ = db.Exec("DELETE FROM student_attendance WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
			_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM teacher_assignments WHERE session_id=?", sessionID).Error
		}
		if guardianID != "" {
			_ = db.Exec("DELETE FROM guardians WHERE id=?", guardianID).Error
		}
		if employeeID != "" {
			_ = db.Exec("DELETE FROM employees WHERE id=?", employeeID).Error
		}
		if sectionID != "" {
			_ = db.Exec("DELETE FROM academic_sections WHERE id=?", sectionID).Error
		}
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
		}
		if subjectID != "" {
			_ = db.Exec("DELETE FROM academic_subjects WHERE id=?", subjectID).Error
		}
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		_ = db.Exec("DELETE FROM users WHERE id IN ?", ids).Error
	})
	sessionName := fmt.Sprintf("%04d", 4500+int(random[0]))
	if err = db.Raw("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,'2026-01-01','2026-12-31') RETURNING id", sessionName).Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "M5 Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "A "+suffix, "A"+strings.ToUpper(suffix)).Scan(&sectionID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Subject',?) RETURNING id", "M5 Subject "+suffix, "M5"+strings.ToUpper(suffix)).Scan(&subjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','M5 Teacher','Teacher','2026-01-01') RETURNING id", "M5E"+strings.ToUpper(suffix), teacher.ID).Scan(&employeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", employeeID, sessionID, classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO students(admission_no,name_en,status,session_id,class_id,section_id,user_id) VALUES (?,?,'active',?,?,?,?) RETURNING id", "M5S"+suffix, "M5 Student "+suffix, sessionID, classID, sectionID, studentUser.ID).Scan(&studentID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO students(admission_no,name_en,status,session_id,class_id,section_id) VALUES (?,?,'active',?,?,?) RETURNING id", "M5O"+suffix, "M5 Other "+suffix, sessionID, classID, sectionID).Scan(&otherStudentID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Raw("INSERT INTO guardians(name_en,user_id) VALUES (?,?) RETURNING id", "M5 Guardian "+suffix, guardianUser.ID).Scan(&guardianID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'mother',true)", studentID, guardianID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id) VALUES (?,?,?,?,?)", "bad"+suffix, "Bad", sessionID, classID, admin.ID).Error; err == nil {
		t.Fatal("invalid section FK accepted")
	}
	router := NewRouterWithStudentAttendance(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), nil, nil, nil, attendance.NewStudentService(attendance.NewStudentRepository(db)))
	token := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, teacherH, otherH, studentH, guardianH, otherGuardianH := token(admin), token(teacher), token(unassigned), token(studentUser), token(guardianUser), token(otherGuardian)
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, headers)
		if r.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	rosterPath := "/attendance/students/roster?session_id=" + sessionID + "&section_id=" + sectionID + "&date=2026-04-10&page=1&page_size=1&search=" + suffix
	check("GET", rosterPath, "", nil, 401)
	check("GET", rosterPath, "", teacherH, 403)
	if err = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", rosterPath, "", otherH, 403)
	rosterBody := check("GET", rosterPath, "", teacherH, 200)
	var roster struct {
		Data attendance.Page[attendance.StudentRosterItem] `json:"data"`
	}
	if err = json.Unmarshal(rosterBody, &roster); err != nil || roster.Data.Total != 2 || len(roster.Data.Items) != 1 {
		t.Fatalf("roster %+v %v", roster, err)
	}
	check("GET", "/attendance/students/sections?session_id="+sessionID, "", teacherH, 200)
	input := func(id, date, status string) string {
		return fmt.Sprintf(`{"student_id":%q,"attendance_date":%q,"status":%q}`, id, date, status)
	}
	check("POST", "/attendance/students", input(studentID, "2027-01-01", "absent"), teacherH, 409)
	check("POST", "/attendance/students", input(studentID, "2026-04-10", "absent"), studentH, 403)
	check("POST", "/attendance/students", input(studentID, "2026-04-10", "absent"), otherH, 403)
	body := check("POST", "/attendance/students", input(studentID, "2026-04-10", "absent"), teacherH, 201)
	var recorded struct {
		Data attendance.StudentRecord `json:"data"`
	}
	if err = json.Unmarshal(body, &recorded); err != nil || recorded.Data.ID == "" {
		t.Fatalf("record %+v %v", recorded, err)
	}
	check("POST", "/attendance/students", input(studentID, "2026-04-10", "absent"), adminH, 409)
	if err = db.Table("attendance_absence_events").Where("attendance_id=? AND state='pending'", recorded.Data.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("absence trigger %d %v", n, err)
	}
	check("GET", "/attendance/alerts?page=1&page_size=10", "", adminH, 200)
	check("GET", "/attendance/alerts?page=1&page_size=10", "", teacherH, 403)
	check("PATCH", "/attendance/students/"+recorded.Data.ID, `{"status":"present"}`, teacherH, 200)
	if err = db.Table("attendance_absence_events").Where("attendance_id=? AND state='cancelled'", recorded.Data.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("absence cancellation %d %v", n, err)
	}
	check("PATCH", "/attendance/students/"+recorded.Data.ID, `{"status":"absent"}`, teacherH, 200)
	if err = db.Table("attendance_absence_events").Where("attendance_id=? AND state='pending'", recorded.Data.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("absence replay %d %v", n, err)
	}
	qr := fmt.Sprintf(`{"payload":"FB-STUDENT:%s","attendance_date":"2026-04-10"}`, otherStudentID)
	check("POST", "/attendance/students/qr", qr, otherH, 403)
	check("POST", "/attendance/students/qr", `{"payload":"`+otherStudentID+`","attendance_date":"2026-04-10"}`, teacherH, 400)
	qrBody := check("POST", "/attendance/students/qr", qr, teacherH, 201)
	var qrResult struct {
		Data attendance.StudentRecord `json:"data"`
	}
	if err = json.Unmarshal(qrBody, &qrResult); err != nil || qrResult.Data.Source != "qr" || qrResult.Data.Status != "present" {
		t.Fatalf("QR record %+v %v", qrResult, err)
	}
	check("POST", "/attendance/students/qr", qr, teacherH, 409)
	monthlyBody := check("GET", "/attendance/students/monthly?session_id="+sessionID+"&section_id="+sectionID+"&month=2026-04&page=1&page_size=1&search="+suffix, "", teacherH, 200)
	var monthly struct {
		Data attendance.Page[attendance.StudentSummary] `json:"data"`
	}
	if err = json.Unmarshal(monthlyBody, &monthly); err != nil || monthly.Data.Total != 2 || len(monthly.Data.Items) != 1 {
		t.Fatalf("monthly %+v %v", monthly, err)
	}
	selfBody := check("GET", "/attendance/students/me/report?month=2026-04", "", studentH, 200)
	var self struct {
		Data attendance.StudentReport `json:"data"`
	}
	if err = json.Unmarshal(selfBody, &self); err != nil || self.Data.Student.Absent != 1 || self.Data.Student.Total != 1 {
		t.Fatalf("self report %+v %v", self, err)
	}
	check("GET", "/attendance/guardians/children/"+studentID+"/report?month=2026-04", "", guardianH, 200)
	check("GET", "/attendance/guardians/children/"+studentID+"/report?month=2026-04", "", otherGuardianH, 404)
	if err = db.Exec("DELETE FROM student_guardians WHERE student_id=? AND guardian_id=?", studentID, guardianID).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/attendance/guardians/children/"+studentID+"/report?month=2026-04", "", guardianH, 404)
	if err = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/attendance/students/me/report?month=2026-04", "", studentH, 403)
}
