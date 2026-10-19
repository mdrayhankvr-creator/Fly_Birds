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
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresStudentAttendanceRosterSave(t *testing.T) {
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
	var random [7]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := "RollSecret!" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x74}, 32))
	if err != nil {
		t.Fatal(err)
	}
	users := []string{}
	makeUser := func(role string) auth.User {
		u, e := authService.BootstrapUser(ctx, fmt.Sprintf("%s-roll-%d-%s@example.test", role, len(users), suffix), password, role)
		if e != nil {
			t.Fatal(e)
		}
		users = append(users, u.ID)
		return u
	}
	admin, teacher, unassigned, studentUser, guardianUser := makeUser("admin"), makeUser("teacher"), makeUser("teacher"), makeUser("student"), makeUser("guardian")
	var oldEnabled bool
	if err = db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key='attendance'").Scan(&oldEnabled).Error; err != nil {
		t.Fatal(err)
	}
	var sessionID, classID, sectionID, otherSectionID, subjectID, employeeID, guardianID string
	t.Cleanup(func() {
		_ = db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key='attendance'", oldEnabled).Error
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
		_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
		_ = db.Exec("DELETE FROM academic_subjects WHERE id=?", subjectID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN ?", users).Error
		_ = db.Exec("DELETE FROM users WHERE id IN ?", users).Error
	})
	if err = db.Exec("UPDATE school_modules SET enabled=false WHERE school_id=1 AND module_key='attendance'").Error; err != nil {
		t.Fatal(err)
	}
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if e := db.Raw(query, args...).Row().Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	sessionID = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,'2026-01-01','2199-12-31') RETURNING id", fmt.Sprintf("%04d", 6000+int(random[0])))
	classID = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Roll Class "+suffix)
	sectionID = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", sessionID, classID)
	otherSectionID = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", sessionID, classID)
	subjectID = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'বিষয়',?) RETURNING id", "Roll Subject "+suffix, "RL"+strings.ToUpper(suffix))
	if err = db.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	employeeID = insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Roll Teacher','Teacher','2026-01-01') RETURNING id", "RL"+strings.ToUpper(suffix), teacher.ID)
	if err = db.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", employeeID, sessionID, classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	student := func(roll int, status, section string, account any) string {
		return insert("INSERT INTO students(admission_no,name_en,status,session_id,class_id,section_id,roll_no,user_id) VALUES (?,?,?,?,?,?,?,?) RETURNING id", fmt.Sprintf("RL%d-%s", roll, suffix), fmt.Sprintf("Roll %d", roll), status, sessionID, classID, section, roll, account)
	}
	second := student(2, "active", sectionID, studentUser.ID)
	first := student(1, "active", sectionID, nil)
	inactive := student(3, "inactive", sectionID, nil)
	elsewhere := student(4, "active", otherSectionID, nil)
	guardianID = insert("INSERT INTO guardians(name_en,user_id) VALUES ('Roll Guardian',?) RETURNING id", guardianUser.ID)
	if err = db.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'father',true)", first, guardianID).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouterWithStudentAttendance(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), nil, nil, nil, attendance.NewStudentService(attendance.NewStudentRepository(db)))
	login := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, teacherH, otherH, studentH, guardianH := login(admin), login(teacher), login(unassigned), login(studentUser), login(guardianUser)
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, headers)
		if r.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	type entry struct {
		StudentID string  `json:"student_id"`
		Status    string  `json:"status"`
		Note      string  `json:"note,omitempty"`
		RecordID  *string `json:"record_id,omitempty"`
		Version   string  `json:"version,omitempty"`
	}
	payload := func(date string, entries ...entry) string {
		b, _ := json.Marshal(map[string]any{"session_id": sessionID, "section_id": sectionID, "attendance_date": date, "entries": entries})
		return string(b)
	}
	result := func(body []byte) attendance.RosterResult {
		var v struct{ Data attendance.RosterResult }
		if e := json.Unmarshal(body, &v); e != nil {
			t.Fatal(e)
		}
		return v.Data
	}
	roster := func(date string) []attendance.StudentRosterItem {
		var v struct {
			Data attendance.Page[attendance.StudentRosterItem]
		}
		body := call("GET", "/attendance/students/roster?session_id="+sessionID+"&section_id="+sectionID+"&date="+date+"&page=1&page_size=100", "", teacherH, 200)
		if e := json.Unmarshal(body, &v); e != nil {
			t.Fatal(e)
		}
		return v.Data.Items
	}
	const day = "2026-04-10"
	both := payload(day, entry{StudentID: first, Status: "absent", Note: "Fever"}, entry{StudentID: second, Status: "present"})

	call("PUT", "/attendance/students/roster", both, nil, 401)
	call("PUT", "/attendance/students/roster", both, teacherH, 403)
	if err = db.Exec("UPDATE school_modules SET enabled=true WHERE school_id=1 AND module_key='attendance'").Error; err != nil {
		t.Fatal(err)
	}
	call("PUT", "/attendance/students/roster", both, otherH, 403)
	call("PUT", "/attendance/students/roster", both, studentH, 403)
	call("PUT", "/attendance/students/roster", both, guardianH, 403)
	call("PUT", "/attendance/students/roster", payload(day), teacherH, 400)
	call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: first, Status: "present"}, entry{StudentID: first, Status: "late"}), teacherH, 400)
	call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: first, Status: "excused"}), teacherH, 400)
	tomorrow := time.Now().In(attendance.Dhaka).AddDate(0, 0, 1).Format("2006-01-02")
	call("PUT", "/attendance/students/roster", payload(tomorrow, entry{StudentID: first, Status: "present"}), teacherH, 400)
	call("PUT", "/attendance/students/roster", payload("2025-12-31", entry{StudentID: first, Status: "present"}), teacherH, 409)
	call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: first, Status: "present"}, entry{StudentID: elsewhere, Status: "present"}), teacherH, 409)
	call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: inactive, Status: "present"}), teacherH, 409)
	var count int64
	if db.Table("student_attendance").Where("session_id=?", sessionID).Count(&count); count != 0 {
		t.Fatalf("rejected saves wrote %d rows", count)
	}

	// Roll order (inactive unmarked students are hidden), then a full save that
	// leaves the unmarked student untouched.
	items := roster(day)
	if len(items) != 2 || items[0].StudentID != first || items[1].StudentID != second {
		t.Fatalf("roster order %+v", items)
	}
	if r := result(call("PUT", "/attendance/students/roster", both, teacherH, 200)); r.Created != 2 || r.Updated != 0 {
		t.Fatalf("first save %+v", r)
	}
	if r := result(call("PUT", "/attendance/students/roster", both, adminH, 200)); r.Unchanged != 2 || r.Created+r.Updated != 0 {
		t.Fatalf("replay %+v", r)
	}
	if db.Table("student_attendance").Where("session_id=? AND student_id=?", sessionID, inactive).Count(&count); count != 0 {
		t.Fatal("unmarked student was recorded")
	}
	if db.Table("attendance_absence_events").Where("student_id=? AND state='pending'", first).Count(&count); count != 1 {
		t.Fatalf("absence event %d", count)
	}
	var guardianReport struct{ Data attendance.StudentReport }
	if err = json.Unmarshal(call("GET", "/attendance/guardians/children/"+first+"/report?month=2026-04", "", guardianH, 200), &guardianReport); err != nil || guardianReport.Data.Student.Absent != 1 || guardianReport.Data.Items[0].Note != "Fever" {
		t.Fatalf("guardian sees absence %+v %v", guardianReport.Data, err)
	}
	if !strings.Contains(string(call("GET", "/attendance/alerts?page=1&page_size=100", "", adminH, 200)), first) {
		t.Fatal("admin alert list misses the absence")
	}

	// Correction with the loaded version, then a stale correction.
	items = roster(day)
	loaded := items[0]
	if loaded.RecordID == nil || loaded.Version == nil || *loaded.Status != "absent" {
		t.Fatalf("loaded record %+v", loaded)
	}
	fix := payload(day, entry{StudentID: first, Status: "present", RecordID: loaded.RecordID, Version: *loaded.Version})
	if r := result(call("PUT", "/attendance/students/roster", fix, teacherH, 200)); r.Updated != 1 {
		t.Fatalf("correction %+v", r)
	}
	if db.Table("attendance_absence_events").Where("student_id=? AND state='cancelled'", first).Count(&count); count != 1 {
		t.Fatalf("absence not cancelled %d", count)
	}
	stale := call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: first, Status: "late", RecordID: loaded.RecordID, Version: *loaded.Version}), adminH, 409)
	if !strings.Contains(string(stale), "attendance_changed") || !strings.Contains(string(stale), first) {
		t.Fatalf("stale body %s", stale)
	}
	call("PUT", "/attendance/students/roster", payload(day, entry{StudentID: second, Status: "late"}), teacherH, 409)

	// QR keeps working, and a roster that did not know about the scan cannot overwrite it.
	const qrDay = "2026-04-11"
	call("POST", "/attendance/students/qr", fmt.Sprintf(`{"payload":"FB-STUDENT:%s","attendance_date":%q}`, second, qrDay), teacherH, 201)
	call("PUT", "/attendance/students/roster", payload(qrDay, entry{StudentID: second, Status: "absent"}, entry{StudentID: first, Status: "present"}), teacherH, 409)
	if r := result(call("PUT", "/attendance/students/roster", payload(qrDay, entry{StudentID: second, Status: "present"}, entry{StudentID: first, Status: "late"}), teacherH, 200)); r.Unchanged != 1 || r.Created != 1 {
		t.Fatalf("roster after QR %+v", r)
	}

	// Five simultaneous submissions of the same marks create each record once.
	const raceDay = "2026-04-13"
	race := payload(raceDay, entry{StudentID: first, Status: "present"}, entry{StudentID: second, Status: "absent"})
	var wg sync.WaitGroup
	codes := make(chan int, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- perform(router, "PUT", "/attendance/students/roster", race, nil, teacherH).Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent save returned %d", code)
		}
	}
	if db.Table("student_attendance").Where("session_id=? AND attendance_date=?", sessionID, raceDay).Count(&count); count != 2 {
		t.Fatalf("concurrent saves wrote %d rows", count)
	}
	if db.Table("attendance_absence_events").Where("student_id=? AND attendance_date=? AND state='pending'", second, raceDay).Count(&count); count != 1 {
		t.Fatalf("concurrent absence events %d", count)
	}

	// Monthly totals reflect saves and corrections.
	var monthly struct {
		Data attendance.Page[attendance.StudentSummary]
	}
	if err = json.Unmarshal(call("GET", "/attendance/students/monthly?session_id="+sessionID+"&section_id="+sectionID+"&month=2026-04&page=1&page_size=100", "", teacherH, 200), &monthly); err != nil {
		t.Fatal(err)
	}
	totals := map[string]attendance.StudentSummary{}
	for _, s := range monthly.Data.Items {
		totals[s.StudentID] = s
	}
	if f, s := totals[first], totals[second]; f.Present != 2 || f.Late != 1 || f.Absent != 0 || s.Present != 2 || s.Absent != 1 || s.Total != 3 {
		t.Fatalf("monthly totals first=%+v second=%+v", f, s)
	}
	var audits int64
	db.Table("audit_events").Where("actor_user_id=? AND action='attendance.student_roster_saved'", teacher.ID).Count(&audits)
	if audits < 4 {
		t.Fatalf("roster audit events %d", audits)
	}
}
