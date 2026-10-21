package exams

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

type workflowFixture struct {
	ctx                                   context.Context
	tx                                    *gorm.DB
	svc                                   *Service
	stamp, session, classID, section      string
	admin, headTeacher, teacher, teacher2 string
	students                              []string
	subjects                              map[string]string // code → subject id
}

// newWorkflowFixture creates a class with S1, S2 (required) and S3 (optional),
// three active students, a teacher for S1 and S2 and another for S3.
func newWorkflowFixture(t *testing.T, ctx context.Context, tx *gorm.DB, year int) *workflowFixture {
	t.Helper()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	f := &workflowFixture{ctx: ctx, tx: tx, svc: NewService(NewRepository(tx)), stamp: fmt.Sprintf("%d", time.Now().UnixNano()), subjects: map[string]string{}}
	user := func(role string) string {
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", "e3-"+role+"-"+f.stamp+"@example.test", "unused", role, "E3 "+role)
	}
	f.admin, f.headTeacher = user("admin"), user("head_teacher")
	teacherUser := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,'teacher','E3 teacher') RETURNING id", "e3-t1-"+f.stamp+"@example.test", "unused")
	teacher2User := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,'teacher','E3 teacher 2') RETURNING id", "e3-t2-"+f.stamp+"@example.test", "unused")
	f.teacher, f.teacher2 = teacherUser, teacher2User
	f.session = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	f.classID = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class E3 "+f.stamp)
	f.section = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", f.session, f.classID)
	for i, code := range []string{"S1", "S2", "S3"} {
		f.subjects[code] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", code+" "+f.stamp, "বিষয়", "E3"+code+f.stamp)
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", f.classID, f.subjects[code], i < 2, i == 2)
	}
	emp1 := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','T1','Teacher','2020-01-01') RETURNING id", "E3A"+f.stamp, teacherUser)
	emp2 := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','T2','Teacher','2020-01-01') RETURNING id", "E3B"+f.stamp, teacher2User)
	for code, emp := range map[string]string{"S1": emp1, "S2": emp1, "S3": emp2} {
		exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", emp, f.session, f.classID, f.subjects[code])
	}
	for i := 0; i < 3; i++ {
		f.students = append(f.students, insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", fmt.Sprintf("E3-%d-%s", i, f.stamp), fmt.Sprintf("Student %d", i), f.session, f.classID, f.section))
	}
	return f
}

// cleanup removes a committed fixture so it cannot affect other packages'
// tests on the same database (they count sessions and active administrators).
// Audit events stay; their actor becomes NULL.
func (f *workflowFixture) cleanup(t *testing.T, db *gorm.DB) {
	t.Helper()
	subjects := []string{}
	for _, id := range f.subjects {
		subjects = append(subjects, id)
	}
	users := []string{f.admin, f.headTeacher, f.teacher, f.teacher2}
	for _, step := range []struct {
		query string
		arg   any
	}{
		{"DELETE FROM exams WHERE session_id=?", f.session}, // cascades to subjects, marks and results
		{"DELETE FROM teacher_assignments WHERE session_id=?", f.session},
		{"DELETE FROM students WHERE session_id=?", f.session},
		{"DELETE FROM employees WHERE user_id IN ?", users},
		{"DELETE FROM academic_class_subjects WHERE class_id=?", f.classID},
		{"DELETE FROM academic_subjects WHERE id IN ?", subjects},
		{"DELETE FROM academic_sections WHERE session_id=?", f.session},
		{"DELETE FROM academic_classes WHERE id=?", f.classID},
		{"DELETE FROM academic_sessions WHERE id=?", f.session},
		{"DELETE FROM users WHERE id IN ?", users},
	} {
		if err := db.Exec(step.query, step.arg).Error; err != nil {
			t.Errorf("cleanup %q: %v", step.query, err)
		}
	}
}

// exam creates a draft exam with the three subjects (S1 out of 51, others 100).
func (f *workflowFixture) exam(t *testing.T, name string) (Exam, map[string]string) {
	t.Helper()
	e, err := f.svc.SaveExam(f.ctx, f.admin, "", ExamInput{Name: name, ExamType: "half_yearly", SessionID: f.session, ClassID: f.classID, SectionID: f.section})
	if err != nil {
		t.Fatal(err)
	}
	for code, full := range map[string][2]float64{"S1": {51, 16.83}, "S2": {100, 33}, "S3": {100, 33}} {
		if e, err = f.svc.SaveSubject(f.ctx, f.admin, e.ID, SubjectInput{SubjectID: f.subjects[code], FullMarks: full[0], PassMarks: full[1]}); err != nil {
			t.Fatal(err)
		}
	}
	sheets := map[string]string{}
	for _, s := range e.Subjects {
		for code, id := range f.subjects {
			if s.SubjectID == id {
				sheets[code] = s.ID
			}
		}
	}
	return e, sheets
}

func num(v float64) *float64 { return &v }

func TestPostgresSheetWorkflowAndStatuses(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	tx := db.Begin()
	defer tx.Rollback()
	f := newWorkflowFixture(t, ctx, tx, 2195)
	svc, s1, s2, s3 := f.svc, f.students[0], f.students[1], f.students[2]
	exam, sheet := f.exam(t, "Half Yearly")

	view, err := svc.Sheets(ctx, exam.ID, f.admin, "admin")
	if err != nil || len(view.Sheets) != 3 || view.Readiness.Ready {
		t.Fatalf("new exam sheets: %+v %v", view, err)
	}
	for _, s := range view.Sheets {
		if s.Status != SheetNotStarted || s.Missing != 3 || s.Students != 3 {
			t.Fatalf("new sheet %+v", s)
		}
	}

	// Cells are saved independently; each invalid one is reported with a code.
	res, err := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{
		{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(16.83)},
		{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(52)},
		{ExamSubjectID: sheet["S1"], StudentID: s3, Status: StatusAbsent, Marks: num(10)},
		{ExamSubjectID: sheet["S2"], StudentID: s1, Status: StatusExempt, Reason: "medical"},
		{ExamSubjectID: sheet["S3"], StudentID: s1, Status: StatusPresent, Marks: num(70)},
		{ExamSubjectID: sheet["S2"], StudentID: s2, Status: StatusPresent},
		{ExamSubjectID: sheet["S2"], StudentID: s3, Status: StatusPresent, Marks: num(33.333)},
		{ExamSubjectID: sheet["S2"], StudentID: s3, Status: "late", Marks: nil, RowVersion: 0}, // duplicate cell
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("a duplicate cell must refuse the request: %v", err)
	}
	res, err = svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{
		{ExamSubjectID: sheet["S1"], StudentID: s1, Status: "late"},
		{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(52)},
		{ExamSubjectID: sheet["S1"], StudentID: s3, Status: StatusAbsent, Marks: num(10)},
		{ExamSubjectID: sheet["S2"], StudentID: s1, Status: StatusExempt, Reason: "medical"},
		{ExamSubjectID: sheet["S3"], StudentID: s1, Status: StatusPresent, Marks: num(70)},
		{ExamSubjectID: sheet["S2"], StudentID: s2, Status: StatusPresent},
		{ExamSubjectID: sheet["S2"], StudentID: s3, Status: StatusPresent, Marks: num(33.333)},
	})
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]string{}
	for _, e := range res.Errors {
		codes[e.ExamSubjectID+":"+e.StudentID] = e.Code
	}
	for key, want := range map[string]string{
		sheet["S1"] + ":" + s2: "above_full_marks",
		sheet["S1"] + ":" + s3: "marks_not_allowed",
		sheet["S2"] + ":" + s1: "status_not_allowed",
		sheet["S3"] + ":" + s1: "not_assigned",
		sheet["S2"] + ":" + s2: "marks_required",
		sheet["S2"] + ":" + s3: "invalid_marks",
		sheet["S1"] + ":" + s1: "invalid_status",
	} {
		if codes[key] != want {
			t.Errorf("cell %s: %q, want %q (all: %v)", key, codes[key], want, codes)
		}
	}
	if len(res.Saved) != 0 || len(res.Errors) != 7 {
		t.Fatalf("saved %d, errors %d: %+v", len(res.Saved), len(res.Errors), res.Errors)
	}

	// Valid cells; versions start at 1 and a stale version never overwrites.
	res, err = svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{
		{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(16.83)},
		{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(40)},
		{ExamSubjectID: sheet["S1"], StudentID: s3, Status: StatusAbsent},
		{ExamSubjectID: sheet["S2"], StudentID: s1, Status: StatusPresent, Marks: num(80)},
		{ExamSubjectID: sheet["S2"], StudentID: s2, Status: StatusPresent, Marks: num(60)},
		{ExamSubjectID: sheet["S2"], StudentID: s3, Status: StatusIncomplete},
	})
	if err != nil || len(res.Saved) != 6 || len(res.Errors) != 0 || res.Saved[0].RowVersion != 1 {
		t.Fatalf("valid cells: %+v %v", res, err)
	}
	stale, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(45), RowVersion: 0}})
	if len(stale.Errors) != 1 || stale.Errors[0].Code != "version_conflict" || stale.Errors[0].Current == nil || *stale.Errors[0].Current.Marks != 40 || stale.Errors[0].Current.RowVersion != 1 {
		t.Fatalf("stale write: %+v", stale)
	}
	fresh, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(45), RowVersion: 1}})
	if len(fresh.Saved) != 1 || fresh.Saved[0].RowVersion != 2 {
		t.Fatalf("current write: %+v", fresh)
	}
	same, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s2, Status: StatusPresent, Marks: num(45), RowVersion: 2}})
	if len(same.Saved) != 1 || same.Saved[0].RowVersion != 2 {
		t.Fatalf("an unchanged cell keeps its version: %+v", same)
	}
	// Exempt and withheld: administrators only, with a reason.
	noReason, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S3"], StudentID: s2, Status: StatusExempt}})
	if len(noReason.Errors) != 1 || noReason.Errors[0].Code != "reason_required" {
		t.Fatalf("exempt without reason: %+v", noReason)
	}
	if res, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S3"], StudentID: s2, Status: StatusExempt, Reason: "Medical certificate"}}); len(res.Saved) != 1 {
		t.Fatalf("exempt with reason: %+v", res)
	}

	view, _ = svc.Sheets(ctx, exam.ID, f.admin, "admin")
	byCode := map[string]Sheet{}
	for _, s := range view.Sheets {
		for code, id := range sheet {
			if s.ID == id {
				byCode[code] = s
			}
		}
	}
	if byCode["S1"].Status != SheetInProgress || byCode["S1"].Absent != 1 || byCode["S1"].Missing != 0 || byCode["S2"].Incomplete != 1 || byCode["S3"].Missing != 2 || byCode["S3"].Exempt != 1 {
		t.Fatalf("sheet counts %+v", byCode)
	}
	blockers := map[string]bool{}
	for _, b := range view.Readiness.Blockers {
		blockers[b.Code+":"+b.SheetID] = true
	}
	if !blockers["marks_incomplete:"+sheet["S2"]] || !blockers["marks_missing:"+sheet["S3"]] || view.Readiness.Ready {
		t.Fatalf("readiness %+v", view.Readiness)
	}

	// The teacher's view lists only assigned sheets.
	mine, err := svc.Sheets(ctx, exam.ID, f.teacher2, "teacher")
	if err != nil || len(mine.Sheets) != 1 || mine.Sheets[0].ID != sheet["S3"] {
		t.Fatalf("teacher 2 sheets: %+v %v", mine, err)
	}

	// Submit: complete sheets only, by the assigned teacher; then read-only.
	if _, err := svc.SheetAction(ctx, f.teacher2, "teacher", exam.ID, sheet["S3"], ActionSubmit, ""); !errors.Is(err, ErrSheetIncomplete) {
		t.Fatalf("incomplete submit: %v", err)
	}
	if _, err := svc.SheetAction(ctx, f.teacher2, "teacher", exam.ID, sheet["S1"], ActionSubmit, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("submit by an unassigned teacher: %v", err)
	}
	submitted, err := svc.SheetAction(ctx, f.teacher, "teacher", exam.ID, sheet["S1"], ActionSubmit, "")
	if err != nil || submitted.Status != SheetSubmitted || submitted.SubmittedBy != "E3 teacher" || submitted.SubmittedAt == nil {
		t.Fatalf("submit: %+v %v", submitted, err)
	}
	locked, _ := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(20), RowVersion: 1}})
	if len(locked.Errors) != 1 || locked.Errors[0].Code != "sheet_locked" {
		t.Fatalf("write after submit: %+v", locked)
	}
	if err := svc.SaveMarks(ctx, f.admin, "admin", exam.ID, []MarkInput{{ExamSubjectID: sheet["S1"], StudentID: s1, Marks: 20}}); !errors.Is(err, ErrSheetState) || !errors.Is(err, ErrConflict) {
		t.Fatalf("legacy write after submit: %v", err)
	}

	// Review: a reason is required to return; teachers cannot review.
	if _, err := svc.SheetAction(ctx, f.headTeacher, "head_teacher", exam.ID, sheet["S1"], ActionReturn, "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("return without reason: %v", err)
	}
	if _, err := svc.SheetAction(ctx, f.teacher, "teacher", exam.ID, sheet["S1"], ActionApprove, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("approve by a teacher: %v", err)
	}
	returned, err := svc.SheetAction(ctx, f.headTeacher, "head_teacher", exam.ID, sheet["S1"], ActionReturn, "Check Student 1's script")
	if err != nil || returned.Status != SheetReturned || returned.ReturnReason != "Check Student 1's script" || returned.ReviewedBy != "E3 head_teacher" {
		t.Fatalf("return: %+v %v", returned, err)
	}
	if res, _ := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(16.83), RowVersion: 1}}); len(res.Saved) != 1 {
		t.Fatalf("a returned sheet is writable: %+v", res)
	}

	// Publication waits for review and for incomplete marks.
	if _, err := svc.SheetAction(ctx, f.teacher, "teacher", exam.ID, sheet["S1"], ActionSubmit, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, f.admin, exam.ID, "published"); !errors.Is(err, ErrSheetsPending) {
		t.Fatalf("publish with a submitted sheet: %v", err)
	}
	if _, err := svc.SheetAction(ctx, f.headTeacher, "head_teacher", exam.ID, sheet["S1"], ActionApprove, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, f.admin, exam.ID, "published"); !errors.Is(err, ErrMarksIncomplete) {
		t.Fatalf("publish with an incomplete mark: %v", err)
	}
	if res, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{
		{ExamSubjectID: sheet["S2"], StudentID: s3, Status: StatusWithheld, Marks: num(50), Reason: "Under review by the exam committee", RowVersion: 1},
		{ExamSubjectID: sheet["S3"], StudentID: s1, Status: StatusPresent, Marks: num(70)},
		{ExamSubjectID: sheet["S3"], StudentID: s3, Status: StatusPresent, Marks: num(90)},
	}); len(res.Saved) != 3 {
		t.Fatalf("admin completes the sheets: %+v", res)
	}
	published, err := svc.SetState(ctx, f.admin, exam.ID, "published")
	if err != nil || published.Status != "published" {
		t.Fatalf("publish: %+v %v", published, err)
	}
	view, _ = svc.Sheets(ctx, exam.ID, f.admin, "admin")
	for _, s := range view.Sheets {
		if s.Status != SheetApproved {
			t.Fatalf("after publication every sheet is approved: %+v", s)
		}
	}

	// Results: s1 has D (16.83/51) + A+ (80) + optional A (70, bonus 2):
	// (1 + 5 + 2) / 2 = 4.00. s2 is exempt from S3 (optional) and has
	// A+ (45/51 is 88 %) + A- (60): 4.25; s3 is withheld: no result.
	type result struct {
		GPA        float64
		Passed     bool
		TotalMarks string
		FullMarks  string
	}
	results := map[string]result{}
	rows := []struct {
		StudentID             string
		GPA                   float64
		Passed                bool
		TotalMarks, FullMarks string
	}{}
	if err := tx.Raw("SELECT student_id,gpa,passed,total_marks::text AS total_marks,full_marks::text AS full_marks FROM exam_results WHERE exam_id=?", exam.ID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		results[r.StudentID] = result{r.GPA, r.Passed, r.TotalMarks, r.FullMarks}
	}
	if len(results) != 2 || results[s1] != (result{4, true, "166.83", "251.00"}) || results[s2] != (result{4.25, true, "105.00", "151.00"}) {
		t.Fatalf("results %+v", results)
	}
	// s1 absent case: fail with an F stored for the absent subject.
	exam2, sheet2 := f.exam(t, "Class Test")
	entries := []MarkEntry{}
	for _, st := range f.students {
		for code, id := range sheet2 {
			e := MarkEntry{ExamSubjectID: id, StudentID: st, Status: StatusPresent, Marks: num(40)}
			if st == s1 && code == "S2" {
				e = MarkEntry{ExamSubjectID: id, StudentID: st, Status: StatusAbsent}
			}
			entries = append(entries, e)
		}
	}
	if res, err := svc.SaveMarkEntries(ctx, f.admin, "admin", exam2.ID, entries); err != nil || len(res.Errors) != 0 {
		t.Fatalf("class test marks: %+v %v", res, err)
	}
	if _, err := svc.SetState(ctx, f.admin, exam2.ID, "published"); err != nil {
		t.Fatal(err)
	}
	var absent struct {
		Grade      string
		GradePoint float64
		Passed     bool
		GPA        float64
		Ok         bool
	}
	if err := tx.Raw("SELECT m.grade,m.grade_point,m.passed,r.gpa,r.passed AS ok FROM exam_marks m JOIN exam_results r ON r.exam_id=m.exam_id AND r.student_id=m.student_id WHERE m.exam_id=? AND m.student_id=? AND m.status='absent'", exam2.ID, s1).Scan(&absent).Error; err != nil {
		t.Fatal(err)
	}
	if absent.Grade != "F" || absent.Passed || absent.GPA != 0 || absent.Ok {
		t.Fatalf("absent in a required subject fails: %+v", absent)
	}

	// Students: statuses without the staff reason; withheld as a card without marks.
	var s3User, s2User string
	s3User = insertUser(t, tx, "student", "e3-s3-"+f.stamp)
	s2User = insertUser(t, tx, "student", "e3-s2-"+f.stamp)
	if err := tx.Exec("UPDATE students SET user_id=? WHERE id=?", s3User, s3).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE students SET user_id=? WHERE id=?", s2User, s2).Error; err != nil {
		t.Fatal(err)
	}
	withheld, err := svc.Card(ctx, "student", s3User, "", exam.ID)
	if err != nil || !withheld.Withheld || len(withheld.Subjects) != 0 || withheld.Result.GPA != 0 {
		t.Fatalf("withheld card: %+v %v", withheld, err)
	}
	cards, err := svc.Cards(ctx, "student", s3User, "", "")
	if err != nil || len(cards) != 2 {
		t.Fatalf("withheld student's cards: %d %v", len(cards), err)
	}
	card, err := svc.Card(ctx, "student", s2User, "", exam.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range card.Subjects {
		if sub.Status == StatusExempt && (sub.StatusReason != "" || sub.Grade != nil || sub.Marks != nil) {
			t.Fatalf("exempt subject on a student card: %+v", sub)
		}
		if sub.RowVersion != 0 {
			t.Fatalf("row version on a student card: %+v", sub)
		}
	}

	// exam-verify understands statuses: no differences.
	report, err := verifyIn(tx, VerifyOptions{SessionID: f.session})
	if err != nil || len(report.Differences) != 0 {
		t.Fatalf("verify with statuses: %+v %v", report.Differences, err)
	}

	// Unpublishing reopens the sheets; republishing gives the same results.
	if _, err := svc.SetState(ctx, f.admin, exam.ID, "draft"); err != nil {
		t.Fatal(err)
	}
	view, _ = svc.Sheets(ctx, exam.ID, f.admin, "admin")
	for _, s := range view.Sheets {
		if s.Status != SheetInProgress || s.ReviewedAt != nil {
			t.Fatalf("after unpublishing: %+v", s)
		}
	}
	if _, err := svc.SetState(ctx, f.admin, exam.ID, "published"); err != nil {
		t.Fatal(err)
	}
	// The legacy PUT contract: all or nothing.
	exam3, sheet3 := f.exam(t, "Test Three")
	err = svc.SaveMarks(ctx, f.admin, "admin", exam3.ID, []MarkInput{{ExamSubjectID: sheet3["S2"], StudentID: s1, Marks: 70}, {ExamSubjectID: sheet3["S1"], StudentID: s1, Marks: 52}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("legacy batch with a bad mark: %v", err)
	}
	var n int64
	tx.Table("exam_marks").Where("exam_id=?", exam3.ID).Count(&n)
	if n != 0 {
		t.Fatalf("legacy batch saved %d marks despite an error", n)
	}
	var audits int64
	tx.Table("audit_events").Where("target_id=? AND action IN ('exam.sheet_submitted','exam.sheet_returned','exam.sheet_approved')", exam.ID).Count(&audits)
	if audits != 4 {
		t.Fatalf("sheet audit events %d, want 4", audits)
	}
}

func insertUser(t *testing.T, tx *gorm.DB, role, email string) string {
	t.Helper()
	var id string
	if err := tx.Raw("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", email+"@example.test", "unused", role, role).Row().Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPostgresConcurrentMarkWrites needs committed data: two writers with the
// same version on one cell cannot both win; writers on different sheets do
// not wait for a school-wide lock.
func TestPostgresConcurrentMarkWrites(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	// A committed fixture in a session year nobody else uses.
	var f *workflowFixture
	for attempt := 0; attempt < 20 && f == nil; attempt++ {
		year := 3000 + rand.Intn(6000)
		var used int64
		db.Table("academic_sessions").Where("name=?", fmt.Sprint(year)).Count(&used)
		if used == 0 {
			f = newWorkflowFixture(t, ctx, db, year)
		}
	}
	if f == nil {
		t.Fatal("no free session year")
	}
	defer f.cleanup(t, db) // before the deferred sqlDB.Close (t.Cleanup would run after it)
	exam, sheet := f.exam(t, "Concurrency")
	s1 := f.students[0]
	if res, err := f.svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(30)}}); err != nil || len(res.Saved) != 1 {
		t.Fatalf("seed cell: %+v %v", res, err)
	}
	var wg sync.WaitGroup
	results := make([]MarksResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S1"], StudentID: s1, Status: StatusPresent, Marks: num(float64(31 + i)), RowVersion: 1}})
		}(i)
	}
	wg.Wait()
	saved, conflicts := 0, 0
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		saved += len(results[i].Saved)
		for _, e := range results[i].Errors {
			if e.Code == "version_conflict" {
				conflicts++
			}
		}
	}
	if saved != 1 || conflicts != 3 {
		t.Fatalf("same cell, same version, four writers: %d saved, %d conflicts", saved, conflicts)
	}
	var version int
	db.Raw("SELECT row_version FROM exam_marks WHERE exam_subject_id=? AND student_id=?", sheet["S1"], s1).Scan(&version)
	if version != 2 {
		t.Fatalf("row version %d, want 2", version)
	}
	// Two teachers on different sheets of the same exam, at the same time.
	var wg2 sync.WaitGroup
	var e1, e2 error
	var r1, r2 MarksResult
	wg2.Add(2)
	go func() {
		defer wg2.Done()
		r1, e1 = f.svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S2"], StudentID: s1, Status: StatusPresent, Marks: num(70)}})
	}()
	go func() {
		defer wg2.Done()
		r2, e2 = f.svc.SaveMarkEntries(ctx, f.teacher2, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S3"], StudentID: s1, Status: StatusPresent, Marks: num(80)}})
	}()
	wg2.Wait()
	if e1 != nil || e2 != nil || len(r1.Saved) != 1 || len(r2.Saved) != 1 {
		t.Fatalf("parallel sheets: %+v %v / %+v %v", r1, e1, r2, e2)
	}
}
