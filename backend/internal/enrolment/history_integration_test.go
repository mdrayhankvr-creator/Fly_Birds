package enrolment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"gorm.io/gorm"
)

// openTx migrates the test database and returns a transaction that is rolled
// back when the test ends, so nothing it writes is kept.
func openTx(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
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
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	return ctx, tx
}

// school is a small fixture: one session with one class and two sections,
// one subject, an admin and one student in section A.
type school struct {
	t                  *testing.T
	tx                 *gorm.DB
	stamp              string
	admin              string
	session            string
	classID, nextClass string
	sectionA, sectionB string
	subject            string
	student            string
}

func newSchool(t *testing.T, tx *gorm.DB, year int) *school {
	t.Helper()
	s := &school{t: t, tx: tx, stamp: fmt.Sprintf("%d", time.Now().UnixNano())}
	s.admin = s.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,'admin','M15 admin') RETURNING id", "m15-admin-"+s.stamp+"@example.test", "unused")
	s.session = s.newSession(year)
	s.classID = s.id("INSERT INTO academic_classes(name,display_order) VALUES (?,1) RETURNING id", "M15 Six "+s.stamp)
	s.nextClass = s.id("INSERT INTO academic_classes(name,display_order) VALUES (?,2) RETURNING id", "M15 Seven "+s.stamp)
	s.sectionA = s.section(s.session, s.classID, "A")
	s.sectionB = s.section(s.session, s.classID, "B")
	s.subject = s.id("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", "M15 Bangla "+s.stamp, "বাংলা", "M15"+s.stamp[len(s.stamp)-10:])
	s.exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", s.classID, s.subject)
	s.student = s.id("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,roll_no,status) VALUES (?,?,?,?,?,1,'active') RETURNING id", "M15-"+s.stamp, "Student One", s.session, s.classID, s.sectionA)
	return s
}

func (s *school) id(q string, args ...any) string {
	s.t.Helper()
	var id string
	if err := s.tx.Raw(q, args...).Row().Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *school) exec(q string, args ...any) {
	s.t.Helper()
	if err := s.tx.Exec(q, args...).Error; err != nil {
		s.t.Fatal(err)
	}
}

func (s *school) newSession(year int) string {
	return s.id("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
}

func (s *school) section(session, class, code string) string {
	return s.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", session, class, code, code)
}

// examWithMark creates a draft exam for section A with one sheet and records
// the student's mark, then a published result row for that exam.
func (s *school) examWithMark(marks string) (exam, mark string) {
	s.t.Helper()
	exam = s.id("INSERT INTO exams(name,exam_type,session_id,class_id,section_id,created_by,updated_by) VALUES (?, 'half_yearly',?,?,?,?,?) RETURNING id", "Half Yearly "+s.stamp, s.session, s.classID, s.sectionA, s.admin, s.admin)
	sheet := s.id("INSERT INTO exam_subjects(exam_id,class_id,subject_id,full_marks,pass_marks) VALUES (?,?,?,100,33) RETURNING id", exam, s.classID, s.subject)
	mark = s.id("INSERT INTO exam_marks(exam_id,exam_subject_id,student_id,session_id,class_id,section_id,marks,grade,grade_point,passed,entered_by,updated_by) VALUES (?,?,?,?,?,?,?,'A',4,true,?,?) RETURNING id",
		exam, sheet, s.student, s.session, s.classID, s.sectionA, marks, s.admin, s.admin)
	s.exec("INSERT INTO exam_results(exam_id,student_id,session_id,class_id,section_id,total_marks,full_marks,gpa,passed,class_position,section_position) VALUES (?,?,?,?,?,?,100,4,true,1,1)",
		exam, s.student, s.session, s.classID, s.sectionA, marks)
	return exam, mark
}

func pgCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

// Before M15 a student with any mark or result could not change section,
// class or session: exam_marks and exam_results referenced the student's
// current (session, class, section), so the update failed with 23503. A
// section transfer must now succeed and leave the mark and the result in the
// section where they were recorded.
func TestPostgresSectionTransferKeepsMarks(t *testing.T) {
	_, tx := openTx(t)
	s := newSchool(t, tx, 2181)
	exam, mark := s.examWithMark("72.50")

	err := tx.Exec("UPDATE students SET section_id=?, roll_no=4 WHERE id=?", s.sectionB, s.student).Error
	if err != nil {
		t.Fatalf("moving a student with marks to another section failed (code %s): %v", pgCode(err), err)
	}

	var markSection, resultSection, marks string
	if err := tx.Raw("SELECT section_id, marks::text FROM exam_marks WHERE id=?", mark).Row().Scan(&markSection, &marks); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT section_id FROM exam_results WHERE exam_id=? AND student_id=?", exam, s.student).Row().Scan(&resultSection); err != nil {
		t.Fatal(err)
	}
	if markSection != s.sectionA || resultSection != s.sectionA || marks != "72.50" {
		t.Fatalf("history rewritten: mark section %s, result section %s, marks %s", markSection, resultSection, marks)
	}
}

// refused runs q in a savepoint, expects it to fail with code (checking
// deferred constraints too) and rolls the savepoint back so the test
// transaction can continue.
func (s *school) refused(code, q string, args ...any) {
	s.t.Helper()
	s.exec("SAVEPOINT refused")
	err := s.tx.Exec(q, args...).Error
	if err == nil {
		err = s.tx.Exec("SET CONSTRAINTS ALL IMMEDIATE").Error
	}
	s.exec("ROLLBACK TO SAVEPOINT refused")
	if pgCode(err) != code {
		s.t.Fatalf("%s: want SQLSTATE %s, got %v", q, code, err)
	}
}

// attend records one attendance day for the student in section A.
func (s *school) attend(date, status string) {
	s.exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,?,?,'manual',?,?)",
		s.student, s.session, s.classID, s.sectionA, date, status, s.admin, s.admin)
}

// fingerprint hashes every stored mark, result and attendance row of the
// student, so a test can prove none of them changed.
func (s *school) fingerprint() string {
	var v string
	if err := s.tx.Raw(`SELECT (SELECT md5(coalesce(string_agg(m::text, '|' ORDER BY m.id), '')) FROM exam_marks m WHERE m.student_id = ?)
 || (SELECT md5(coalesce(string_agg(r::text, '|' ORDER BY r.exam_id), '')) FROM exam_results r WHERE r.student_id = ?)
 || (SELECT md5(coalesce(string_agg(a::text, '|' ORDER BY a.id), '')) FROM student_attendance a WHERE a.student_id = ?)`, s.student, s.student, s.student).Row().Scan(&v); err != nil {
		s.t.Fatal(err)
	}
	return v
}

// Moving a student into the next session keeps the old year intact: the
// stored rows are unchanged, the old exam's roster and tabulation, the old
// section's attendance reports and the history all still show the student
// where (and with the roll) they were that year.
func TestPostgresHistoryStaysAfterMovingSession(t *testing.T) {
	ctx, tx := openTx(t)
	s := newSchool(t, tx, 1998)
	exam, _ := s.examWithMark("64.00")
	s.attend("1998-03-02", "present")
	s.attend("1998-03-03", "absent")
	before := s.fingerprint()

	next := s.newSession(1999)
	nextSection := s.section(next, s.nextClass, "A")
	s.exec("UPDATE students SET session_id=?, class_id=?, section_id=?, roll_no=7 WHERE id=?", next, s.nextClass, nextSection, s.student)
	s.exec("SET CONSTRAINTS ALL IMMEDIATE")
	if after := s.fingerprint(); after != before {
		t.Fatal("marks, results or attendance changed when the student moved session")
	}

	hist, err := people.NewRepository(tx).Enrolments(ctx, people.Actor{ID: s.admin, Role: "admin"}, s.student)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || hist[0].SessionID != s.session || hist[0].State != "closed" || hist[0].Outcome == nil || *hist[0].Outcome != "moved" ||
		hist[0].SectionID != s.sectionA || hist[0].RollNo == nil || *hist[0].RollNo != 1 || hist[0].EndedOn == nil ||
		hist[1].SessionID != next || hist[1].State != "current" || hist[1].Source != "moved" || hist[1].RollNo == nil || *hist[1].RollNo != 7 {
		t.Fatalf("history: %+v", hist)
	}

	examRepo := exams.NewRepository(tx)
	rows, err := examRepo.Roster(ctx, exam, s.admin, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].StudentID != s.student || rows[0].RollNo == nil || *rows[0].RollNo != 1 {
		t.Fatalf("old exam roster: %+v", rows)
	}
	tab, err := examRepo.Tabulation(ctx, exam)
	if err != nil {
		t.Fatal(err)
	}
	if len(tab) != 1 || tab[0].RollNo == nil || *tab[0].RollNo != 1 {
		t.Fatalf("old tabulation: %+v", tab)
	}

	att := attendance.NewStudentRepository(tx)
	month, err := att.StudentMonthly(ctx, s.admin, attendance.StudentQuery{SessionID: s.session, SectionID: s.sectionA, Month: "1998-03", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if month.Total != 1 || len(month.Items) != 1 || month.Items[0].Present != 1 || month.Items[0].Absent != 1 {
		t.Fatalf("old monthly report: %+v", month)
	}
	day, err := att.StudentRoster(ctx, s.admin, attendance.StudentQuery{SessionID: s.session, SectionID: s.sectionA, Date: "1998-03-02", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if day.Total != 1 || day.Items[0].Status == nil || *day.Items[0].Status != "present" || day.Items[0].RollNo == nil || *day.Items[0].RollNo != 1 {
		t.Fatalf("old daily roster: %+v", day)
	}
	now, err := att.StudentRoster(ctx, s.admin, attendance.StudentQuery{SessionID: next, SectionID: nextSection, Date: "1999-03-01", Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if now.Total != 1 || now.Items[0].RollNo == nil || *now.Items[0].RollNo != 7 || now.Items[0].RecordID != nil {
		t.Fatalf("new section roster: %+v", now)
	}

	// The year the student left cannot be re-entered by an edit.
	s.refused("FB001", "UPDATE students SET session_id=?, class_id=?, section_id=?, roll_no=1 WHERE id=?", s.session, s.classID, s.sectionA, s.student)
}

// QA-06: after a section transfer, the old section's monthly report keeps
// the student's days there, and the new section's report has only theirs.
func TestPostgresSectionTransferKeepsAttendanceReport(t *testing.T) {
	ctx, tx := openTx(t)
	s := newSchool(t, tx, 1997)
	s.attend("1997-09-01", "present")
	s.attend("1997-09-02", "late")
	s.exec("UPDATE students SET section_id=? WHERE id=?", s.sectionB, s.student)
	s.exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,'1997-09-03','present','manual',?,?)",
		s.student, s.session, s.classID, s.sectionB, s.admin, s.admin)
	att := attendance.NewStudentRepository(tx)
	for section, want := range map[string]int64{s.sectionA: 2, s.sectionB: 1} {
		month, err := att.StudentMonthly(ctx, s.admin, attendance.StudentQuery{SessionID: s.session, SectionID: section, Month: "1997-09", Page: 1, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		if month.Total != 1 || month.Items[0].Total != want {
			t.Fatalf("section %s: %+v, want %d days", section, month, want)
		}
	}
}

// The enrolment triggers: admission, in-place correction of a year with no
// records, section transfers, refused class changes, the commit-time
// consistency check and append-only events.
func TestPostgresEnrolmentTriggers(t *testing.T) {
	_, tx := openTx(t)
	s := newSchool(t, tx, 2183)

	var n int64
	var source string
	if err := tx.Raw("SELECT count(*), max(source) FROM student_enrolments WHERE student_id=? AND state='current' AND session_id=? AND section_id=? AND roll_no=1", s.student, s.session, s.sectionA).Row().Scan(&n, &source); err != nil || n != 1 || source != "admission" {
		t.Fatalf("admission enrolment %d %q: %v", n, source, err)
	}

	// A student with nothing recorded is corrected in place: one enrolment.
	other := s.id("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", "M15-B-"+s.stamp, "Student Two", s.session, s.classID, s.sectionB)
	next := s.newSession(2184)
	nextSection := s.section(next, s.classID, "A")
	s.exec("UPDATE students SET session_id=?, section_id=? WHERE id=?", next, nextSection, other)
	if err := tx.Raw("SELECT count(*) FROM student_enrolments WHERE student_id=?", other).Row().Scan(&n); err != nil || n != 1 {
		t.Fatalf("corrected enrolments %d: %v", n, err)
	}
	if err := tx.Raw("SELECT count(*) FROM student_enrolment_events WHERE student_id=? AND kind='session_corrected'", other).Row().Scan(&n); err != nil || n != 1 {
		t.Fatalf("session_corrected events %d: %v", n, err)
	}

	// With a mark: section transfer is allowed and logged; class change is not.
	exam, _ := s.examWithMark("50.00")
	otherClassSection := s.section(s.session, s.nextClass, "A")
	s.refused("FB001", "UPDATE students SET class_id=?, section_id=? WHERE id=?", s.nextClass, otherClassSection, s.student)
	s.exec("UPDATE students SET section_id=?, roll_no=2 WHERE id=?", s.sectionB, s.student)
	if err := tx.Raw("SELECT count(*) FROM student_enrolment_events WHERE student_id=? AND kind IN ('section_changed','roll_changed')", s.student).Row().Scan(&n); err != nil || n != 2 {
		t.Fatalf("transfer events %d: %v", n, err)
	}
	// A correction of the old section's exam can still store the result;
	// a result in a section the student never belonged to is refused.
	s.exec("DELETE FROM exam_results WHERE exam_id=? AND student_id=?", exam, s.student)
	s.exec("INSERT INTO exam_results(exam_id,student_id,session_id,class_id,section_id,total_marks,full_marks,gpa,passed,class_position,section_position) VALUES (?,?,?,?,?,50,100,3,true,1,1)",
		exam, s.student, s.session, s.classID, s.sectionA)
	stranger := s.section(s.session, s.classID, "C")
	strangerExam := s.id("INSERT INTO exams(name,exam_type,session_id,class_id,section_id,created_by,updated_by) VALUES ('C exam','half_yearly',?,?,?,?,?) RETURNING id", s.session, s.classID, stranger, s.admin, s.admin)
	s.refused("23503", "INSERT INTO exam_results(exam_id,student_id,session_id,class_id,section_id,total_marks,full_marks,gpa,passed,class_position,section_position) VALUES (?,?,?,?,?,50,100,3,true,1,1)",
		strangerExam, s.student, s.session, s.classID, stranger)

	// Tampering with the current enrolment fails the commit-time check;
	// events never change.
	s.refused("23514", "UPDATE student_enrolments SET roll_no=99 WHERE student_id=? AND state='current'", s.student)
	s.refused("23514", "UPDATE student_enrolment_events SET to_value='x' WHERE student_id=?", s.student)

	// A student with no marks can still be deleted; the history goes too.
	s.exec("DELETE FROM students WHERE id=?", other)
	if err := tx.Raw("SELECT count(*) FROM student_enrolments WHERE student_id=?", other).Row().Scan(&n); err != nil || n != 0 {
		t.Fatalf("enrolments left after delete %d: %v", n, err)
	}
	// One with marks cannot, exactly as before.
	s.refused("23503", "DELETE FROM students WHERE id=?", s.student)
}
