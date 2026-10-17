package routine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

func TestPostgresRoutineLifecycle(t *testing.T) {
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
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.Table("schema_migrations").Where("version=601").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("migration 601: %d %v", n, err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	userIndex := 0
	user := func(role string) string {
		userIndex++
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("%s%d%s@example.test",role,userIndex,unique), "unused", role, role)
	}
	admin, guardian, otherGuardian, studentUser, teacherUser, teacher2User := user("admin"), user("guardian"), user("guardian"), user("student"), user("teacher"), user("teacher")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2188','2188-01-01','2188-12-31',false) RETURNING id")
	class1 := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Six "+unique)
	class2 := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Seven "+unique)
	section1 := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, class1)
	section2 := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, class2)
	subject1 := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'বাংলা',?) RETURNING id", "Bangla "+unique, "BAN"+unique)
	subject2 := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'গণিত',?) RETURNING id", "Math "+unique, "MAT"+unique)
	for _, pair := range [][2]string{{class1, subject1}, {class1, subject2}, {class2, subject1}} {
		if err := tx.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", pair[0], pair[1]).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher1 := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Teacher One','Teacher','2187-01-01') RETURNING id", "T"+unique, teacherUser)
	teacher2 := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Teacher Two','Teacher','2187-01-01') RETURNING id", "U"+unique, teacher2User)
	for _, a := range [][3]string{{teacher1, class1, subject1}, {teacher1, class2, subject1}, {teacher2, class1, subject2}, {teacher2, class2, subject1}} {
		if err := tx.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", a[0], session, a[1], a[2]).Error; err != nil {
			t.Fatal(err)
		}
	}
	student1 := insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,user_id,status) VALUES (?,'Student One',?,?,?,?,'active') RETURNING id", "S"+unique, session, class1, section1, studentUser)
	guardianID := insert("INSERT INTO guardians(name_en,user_id) VALUES ('Guardian One',?) RETURNING id", guardian)
	if err := tx.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'parent',true)", student1, guardianID).Error; err != nil {
		t.Fatal(err)
	}
	s := NewService(NewRepository(tx))
	query := Query{SessionID: session, Page: 1, PageSize: 100}
	in := ClassInput{SessionID: session, ClassID: class1, SectionID: section1, SubjectID: subject1, TeacherID: teacher1, DayOfWeek: 0, StartTime: "09:00", EndTime: "10:00", Room: "101"}
	first, err := s.SaveClass(ctx, admin, "", in)
	if err != nil {
		t.Fatal(err)
	}
	sectionConflict := in
	sectionConflict.SubjectID = subject2
	sectionConflict.TeacherID = teacher2
	sectionConflict.Room = "102"
	if _, err := s.SaveClass(ctx, admin, "", sectionConflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("section conflict: %v", err)
	}
	teacherConflict := in
	teacherConflict.ClassID = class2
	teacherConflict.SectionID = section2
	teacherConflict.Room = "102"
	if _, err := s.SaveClass(ctx, admin, "", teacherConflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("teacher conflict: %v", err)
	}
	roomConflict := in
	roomConflict.TeacherID = teacher2
	roomConflict.SectionID = section2
	roomConflict.ClassID = class2
	if _, err := s.SaveClass(ctx, admin, "", roomConflict); !errors.Is(err, ErrConflict) {
		t.Fatalf("room conflict: %v", err)
	}
	badTeacher := roomConflict
	badTeacher.SubjectID = subject2
	badTeacher.Room = "102"
	if _, err := s.SaveClass(ctx, admin, "", badTeacher); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid teacher mapping: %v", err)
	}
	adjacent := in
	adjacent.StartTime = "10:00"
	adjacent.EndTime = "11:00"
	if _, err := s.SaveClass(ctx, admin, "", adjacent); err != nil {
		t.Fatalf("adjacent period: %v", err)
	}
	invalid := in
	invalid.StartTime = "12:00"
	invalid.EndTime = "11:00"
	if _, err := s.SaveClass(ctx, admin, "", invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid time: %v", err)
	}
	teacherPage, err := s.ListClasses(ctx, query, "teacher", teacherUser, "")
	if err != nil || teacherPage.Total != 2 || teacherPage.Items[0].StartTime != "09:00" {
		t.Fatalf("teacher timetable: %+v %v", teacherPage, err)
	}
	studentPage, err := s.ListClasses(ctx, query, "student", studentUser, "")
	if err != nil || studentPage.Total != 2 {
		t.Fatalf("student timetable: %+v %v", studentPage, err)
	}
	guardianPage, err := s.ListClasses(ctx, query, "guardian", guardian, student1)
	if err != nil || guardianPage.Total != 2 {
		t.Fatalf("guardian timetable: %+v %v", guardianPage, err)
	}
	if _, err := s.ListClasses(ctx, query, "guardian", otherGuardian, student1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked guardian: %v", err)
	}
	if err := s.DeleteClass(ctx, admin, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClass(ctx, admin, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete: %v", err)
	}
	exam1, err := s.SaveExam(ctx, admin, "", ExamInput{Name: "Annual", SessionID: session, ClassID: class1, SectionID: section1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishExam(ctx, admin, exam1.ID, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("empty publication: %v", err)
	}
	if page, err := s.ListExams(ctx, query, "student", studentUser, ""); err != nil || page.Total != 0 {
		t.Fatalf("draft visible: %+v %v", page, err)
	}
	slot := ExamSlotInput{SubjectID: subject1, ExamDate: "2188-05-10", StartTime: "10:00", EndTime: "12:00", Room: "Hall"}
	badDate := slot
	badDate.ExamDate = "2189-01-01"
	if _, err := s.SaveExamSlot(ctx, admin, exam1.ID, "", badDate); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out-of-session exam: %v", err)
	}
	exam1, err = s.SaveExamSlot(ctx, admin, exam1.ID, "", slot)
	if err != nil {
		t.Fatal(err)
	}
	if len(exam1.Slots) != 1 {
		t.Fatalf("exam slots: %+v", exam1)
	}
	exam2, err := s.SaveExam(ctx, admin, "", ExamInput{Name: "Half-yearly", SessionID: session, ClassID: class1, SectionID: section1})
	if err != nil {
		t.Fatal(err)
	}
	conflictSlot := slot
	conflictSlot.SubjectID = subject2
	conflictSlot.Room = "Other"
	if _, err := s.SaveExamSlot(ctx, admin, exam2.ID, "", conflictSlot); !errors.Is(err, ErrConflict) {
		t.Fatalf("exam section conflict: %v", err)
	}
	if _, err := s.SaveExam(ctx, admin, "", ExamInput{Name: "annual", SessionID: session, ClassID: class1, SectionID: section1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate exam: %v", err)
	}
	if _, err := s.PublishExam(ctx, admin, exam1.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveExamSlot(ctx, admin, exam1.ID, exam1.Slots[0].ID, slot); !errors.Is(err, ErrConflict) {
		t.Fatalf("published edit: %v", err)
	}
	if page, err := s.ListExams(ctx, query, "student", studentUser, ""); err != nil || page.Total != 1 {
		t.Fatalf("published student list: %+v %v", page, err)
	}
	if page, err := s.ListExams(ctx, query, "guardian", guardian, student1); err != nil || page.Total != 1 {
		t.Fatalf("published guardian list: %+v %v", page, err)
	}
	if _, err := s.GetExam(ctx, exam2.ID, "student", studentUser, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("draft detail visible: %v", err)
	}
	if _, err := s.GetExam(ctx, exam1.ID, "guardian", otherGuardian, student1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked exam: %v", err)
	}
	if _, err := s.GetExam(ctx, exam1.ID, "student", studentUser, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishExam(ctx, admin, exam1.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExam(ctx, admin, exam1.ID); err != nil {
		t.Fatal(err)
	}

	// Distinct transactions exercise advisory locking and conflict checks together.
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	// The committed fixture is cleaned below, after the concurrent transaction test.
	defer cleanupRoutineFixture(db, session, []string{class1, class2}, []string{subject1, subject2}, []string{teacher1, teacher2}, []string{guardianID}, []string{admin, guardian, otherGuardian, studentUser, teacherUser, teacher2User})
	parallel := NewService(NewRepository(db))
	raceInput := in
	raceInput.StartTime = "13:00"
	raceInput.EndTime = "14:00"
	var wg sync.WaitGroup
	var results [2]error
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, results[i] = parallel.SaveClass(ctx, admin, "", raceInput) }(i)
	}
	wg.Wait()
	if (results[0] == nil) == (results[1] == nil) || (results[0] != nil && !errors.Is(results[0], ErrConflict)) || (results[1] != nil && !errors.Is(results[1], ErrConflict)) {
		t.Fatalf("concurrent overlapping creates: %v %v", results[0], results[1])
	}
}

func cleanupRoutineFixture(db *gorm.DB, session string, classes, subjects, teachers, guardians, users []string) {
	_ = db.Exec("DELETE FROM exam_routine_slots WHERE routine_id IN (SELECT id FROM exam_routines WHERE session_id=?)", session).Error
	_ = db.Exec("DELETE FROM exam_routines WHERE session_id=?", session).Error
	_ = db.Exec("DELETE FROM class_routines WHERE session_id=?", session).Error
	_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", session).Error
	_ = db.Exec("DELETE FROM students WHERE session_id=?", session).Error
	_ = db.Exec("DELETE FROM guardians WHERE id IN ?", guardians).Error
	_ = db.Exec("DELETE FROM teacher_assignments WHERE session_id=?", session).Error
	_ = db.Exec("DELETE FROM employees WHERE id IN ?", teachers).Error
	_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", session).Error
	_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id IN ?", classes).Error
	_ = db.Exec("DELETE FROM academic_subjects WHERE id IN ?", subjects).Error
	_ = db.Exec("DELETE FROM academic_classes WHERE id IN ?", classes).Error
	_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", session).Error
	_ = db.Exec("DELETE FROM users WHERE id IN ?", users).Error
}
