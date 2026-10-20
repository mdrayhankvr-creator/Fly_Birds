package exams

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

// TestPostgresExactPublicationAndVerify publishes an exam with a mark exactly
// on a band edge (16.83 of 51 = 33 %) through the real publish path, then
// makes the stored values look like the pre-E2 float engine's and checks
// that the verifier reports exactly those differences, classified, without
// changing a row.
func TestPostgresExactPublicationAndVerify(t *testing.T) {
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
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	admin := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", "e2-admin-"+stamp+"@example.test", "unused", "admin", "admin")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2193','2193-01-01','2193-12-31') RETURNING id")
	classID := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class E2 "+stamp)
	sectionA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, classID)
	sectionB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, classID)
	subjects := map[string]string{}
	for i, code := range []string{"S1", "S2", "S3"} {
		subjects[code] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", code+" "+stamp, "বিষয়", fmt.Sprintf("E2%s%s", code, stamp))
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", classID, subjects[code], i < 2, i == 2)
	}
	student := func(no, name, section string) string {
		return insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", no+stamp, name, session, classID, section)
	}
	a1, a2, b1 := student("A1-", "Edge Student", sectionA), student("A2-", "Second Student", sectionA), student("B1-", "Section B Student", sectionB)

	svc := NewService(NewRepository(tx))
	publish := func(section string, marks map[string][3]float64) string {
		t.Helper()
		exam, err := svc.SaveExam(ctx, admin, "", ExamInput{Name: "Half Yearly", ExamType: "half_yearly", SessionID: session, ClassID: classID, SectionID: section})
		if err != nil {
			t.Fatal(err)
		}
		for code, full := range map[string][2]float64{"S1": {51, 16.83}, "S2": {100, 33}, "S3": {100, 33}} {
			if exam, err = svc.SaveSubject(ctx, admin, exam.ID, SubjectInput{SubjectID: subjects[code], FullMarks: full[0], PassMarks: full[1]}); err != nil {
				t.Fatal(err)
			}
		}
		subjectRow := map[string]string{}
		for _, s := range exam.Subjects {
			subjectRow[s.SubjectID] = s.ID
		}
		entries := []MarkInput{}
		for studentID, m := range marks {
			for i, code := range []string{"S1", "S2", "S3"} {
				entries = append(entries, MarkInput{ExamSubjectID: subjectRow[subjects[code]], StudentID: studentID, Marks: m[i]})
			}
		}
		if err := svc.SaveMarks(ctx, admin, "admin", exam.ID, entries); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetState(ctx, admin, exam.ID, "published"); err != nil {
			t.Fatal(err)
		}
		return exam.ID
	}
	examA := publish(sectionA, map[string][3]float64{a1: {16.83, 100, 100}, a2: {30, 60, 50}})
	publish(sectionB, map[string][3]float64{b1: {30, 60, 90}})

	// The exact engine published 16.83/51 as D (33 %), not F.
	type stored struct {
		Grade      string
		GradePoint float64
		Passed     bool
	}
	var edge stored
	if err := tx.Raw("SELECT m.grade,m.grade_point,m.passed FROM exam_marks m JOIN exam_subjects es ON es.id=m.exam_subject_id WHERE m.student_id=? AND es.subject_id=?", a1, subjects["S1"]).Scan(&edge).Error; err != nil {
		t.Fatal(err)
	}
	if edge.Grade != "D" || edge.GradePoint != 1 || !edge.Passed {
		t.Fatalf("16.83/51 published as %+v, want D 1.00 passed", edge)
	}
	type result struct {
		GPA                            float64
		Passed                         bool
		TotalMarks                     string
		ClassPosition, SectionPosition int
	}
	resultOf := func(id string) result {
		var r result
		if err := tx.Raw("SELECT gpa,passed,total_marks::text AS total_marks,class_position,section_position FROM exam_results WHERE student_id=?", id).Scan(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r
	}
	// a1: (1.00 + 5.00 + bonus 3.00) / 2 = 4.50; a2: (3.00 + 3.50 + 1.00) / 2 = 3.75; b1: (3.00 + 3.50 + 3.00) / 2 = 4.75.
	for id, want := range map[string]result{a1: {4.5, true, "216.83", 2, 1}, a2: {3.75, true, "140.00", 3, 2}, b1: {4.75, true, "180.00", 1, 1}} {
		if got := resultOf(id); got != want {
			t.Fatalf("published result %+v, want %+v", got, want)
		}
	}
	empty, err := verifyIn(tx, VerifyOptions{SessionID: session})
	if err != nil || len(empty.Differences) != 0 || empty.Exams != 2 || empty.Results != 3 || empty.Marks != 9 {
		t.Fatalf("verify after an exact publication: %+v %v", empty, err)
	}

	// What the pre-E2 engine stored: 16.83/51 graded F, so a1 failed and the
	// merit order changed. b1's GPA is changed to a value no engine produces.
	exec("UPDATE exam_marks m SET grade='F',grade_point=0,passed=false FROM exam_subjects es WHERE es.id=m.exam_subject_id AND m.student_id=? AND es.subject_id=?", a1, subjects["S1"])
	exec("UPDATE exam_results SET gpa=0,passed=false,class_position=3,section_position=2 WHERE student_id=?", a1)
	exec("UPDATE exam_results SET class_position=2,section_position=1 WHERE student_id=?", a2)
	exec("UPDATE exam_results SET gpa=4.99 WHERE student_id=?", b1)

	fingerprint := func() string {
		var s string
		if err := tx.Raw(`SELECT md5(coalesce(string_agg(x, '|' ORDER BY x), '')) FROM (
			SELECT m::text AS x FROM exam_marks m JOIN exams e ON e.id=m.exam_id WHERE e.session_id=?
			UNION ALL SELECT r::text FROM exam_results r WHERE r.session_id=?
			UNION ALL SELECT e::text FROM exams e WHERE e.session_id=?) t`, session, session, session).Scan(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := fingerprint()
	report, err := verifyIn(tx, VerifyOptions{SessionID: session})
	if err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(); after != before {
		t.Fatal("the verifier changed examination rows")
	}
	got := map[string]string{}
	for _, d := range report.Differences {
		got[d.Level+" "+d.Field+" "+strings.TrimSuffix(d.AdmissionNo, stamp)] = d.Kind + ": " + d.Stored + " → " + d.Exact
	}
	want := map[string]string{
		"mark grade A1-":                BoundaryCorrection + ": F 0.00 failed → D 1.00 passed",
		"result gpa A1-":                BoundaryCorrection + ": 0.00 failed → 4.50 passed",
		"position class_position A1-":   BoundaryCorrection + ": 3 → 2",
		"position section_position A1-": BoundaryCorrection + ": 2 → 1",
		"position class_position A2-":   BoundaryCorrection + ": 2 → 3",
		"position section_position A2-": BoundaryCorrection + ": 1 → 2",
		"result gpa B1-":                Inconsistent + ": 4.99 passed → 4.75 passed",
	}
	if len(got) != len(want) || report.BoundaryCorrections != 6 || report.Inconsistent != 1 {
		t.Fatalf("differences %v (boundary %d, inconsistent %d), want %v", got, report.BoundaryCorrections, report.Inconsistent, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
	for _, d := range report.Differences {
		if d.Level == "mark" && !strings.Contains(d.Detail, "16.83 of 51.00 is exactly on a band edge") {
			t.Errorf("mark detail %q", d.Detail)
		}
		if d.ExamID == examA && d.Session != "2193" {
			t.Errorf("context %+v", d)
		}
	}
}

// TestPostgresVerifyNeedsNoWrites runs the public Verify over a connection
// on which PostgreSQL refuses every write, across the whole test database.
func TestPostgresVerifyNeedsNoWrites(t *testing.T) {
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
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	ro, err := database.Open(ctx, readOnlyDSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	roSQL, _ := ro.DB()
	defer roSQL.Close()
	if err := ro.Exec("CREATE TEMP TABLE e2_write_probe(x int)").Error; err == nil {
		t.Fatal("the read-only connection accepted a write")
	}
	report, err := Verify(ctx, ro, VerifyOptions{})
	if err != nil {
		t.Fatalf("verify on a read-only connection: %v", err)
	}
	t.Logf("whole database: %d exams, %d results, %d marks, %d differences", report.Exams, report.Results, report.Marks, len(report.Differences))
	if _, err := Verify(ctx, ro, VerifyOptions{SessionID: "not-a-uuid"}); err == nil {
		t.Fatal("an invalid session id was accepted")
	}
}

// readOnlyDSN asks PostgreSQL to make every transaction of the connection
// read-only (pgx passes unknown URL parameters as run-time parameters).
func readOnlyDSN(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "default_transaction_read_only=on"
}
