package exams

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

// TestPostgresBlueprintResults runs one term of a blueprint class end to end:
//   - CT 1 (20, averaged, worth 20) + term exam (100, worth 80);
//   - S1 in the term exam has CQ 60 (pass 20) + MCQ 40 (pass 13);
//   - S2 and S4 are papers combined as PAP under the each-paper rule;
//   - S3 is the optional subject, chosen by one student;
//   - dense ranking; two sections.
//
// It checks mark entry with components and choices, publication with the
// version's rules, hidden class tests, class positions joined by term, the
// provisional term preview and exam-verify.
func TestPostgresBlueprintResults(t *testing.T) {
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
	f := newWorkflowFixture(t, ctx, tx, 2201)
	svc := f.svc
	bp := svc.Blueprints()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	scan := func(dst any, q string, args ...any) {
		t.Helper()
		if err := tx.Raw(q, args...).Row().Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	// A fourth subject S4 (required) and a second section with one student.
	var s4, sectionB, studentB string
	scan(&s4, "INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", "S4 "+f.stamp, "বিষয়", "E5S4"+f.stamp[len(f.stamp)-8:])
	exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", f.classID, s4)
	scan(&sectionB, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", f.session, f.classID)
	scan(&studentB, "INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", "E5R-B-"+f.stamp, "Student B", f.session, f.classID, sectionB)

	// The blueprint.
	term, err := bp.SaveTerm(ctx, f.admin, "", blueprints.TermInput{SessionID: f.session, Name: "First Term", Kind: "term"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := bp.CreateBlueprint(ctx, f.admin, f.session, f.classID, "")
	if err != nil {
		t.Fatal(err)
	}
	d := draft.Definition
	d.Assessments = append([]blueprints.Assessment{{Key: "CT1", Name: "CT 1", TermID: term.ID, Kind: "class_test", Visibility: "hidden"}}, d.Assessments...)
	for i := range d.Subjects {
		s := &d.Subjects[i]
		s.Sheets = append(s.Sheets, blueprints.Sheet{Assessment: "CT1", Pass: 7, Components: []blueprints.Component{{Key: "TOTAL", Full: 20}}})
		s.Terms[0].Contributions = []blueprints.Contribution{{From: []string{"CT1"}, Combine: "average", Worth: 20}, {From: []string{"EX1"}, Combine: "single", Worth: 80}}
		switch s.SubjectID {
		case f.subjects["S1"]:
			pq, pm := 20.0, 13.0
			s.Sheets[0].Components = []blueprints.Component{{Key: "CQ", Full: 60, Pass: &pq}, {Key: "MCQ", Full: 40, Pass: &pm}}
		case f.subjects["S2"], s4:
			s.CombinedKey = "PAP"
		}
	}
	d.PaperRule, d.Policy.Ranking = "each_paper", "dense"
	if v, err := bp.SaveDraft(ctx, f.admin, draft.ID, d); err != nil || len(v.Issues) != 0 {
		t.Fatalf("draft: %+v %v", v.Issues, err)
	}
	if _, err := bp.Activate(ctx, f.admin, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := bp.SaveSetup(ctx, f.admin, term.ID, blueprints.Setup{ClassIDs: []string{f.classID}, Due: map[string]string{"CT1": "2201-02-01", "EX1": "2201-03-01"}}); err != nil {
		t.Fatal(err)
	}
	created, err := bp.CreateTerm(ctx, f.admin, term.ID)
	if err != nil || created.ExamsCreated != 4 {
		t.Fatalf("create: %+v %v", created, err)
	}
	// Exams and sheets by section and assessment.
	exam := map[string]string{}
	sheet := map[string]string{}
	rows := []struct{ ExamID, SectionID, Key, SheetID, SubjectID string }{}
	if err := tx.Raw(`SELECT e.id AS exam_id, e.section_id, e.assessment_key AS key, es.id AS sheet_id, es.subject_id FROM exams e JOIN exam_subjects es ON es.exam_id = e.id WHERE e.term_id = ?`, term.ID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	section := map[string]string{f.section: "A", sectionB: "B"}
	subjectCode := map[string]string{f.subjects["S1"]: "S1", f.subjects["S2"]: "S2", f.subjects["S3"]: "S3", s4: "S4"}
	for _, r := range rows {
		exam[section[r.SectionID]+r.Key] = r.ExamID
		sheet[section[r.SectionID]+r.Key+subjectCode[r.SubjectID]] = r.SheetID
	}

	// Only the first student takes the optional subject.
	if _, err := bp.SaveChoices(ctx, f.admin, f.session, f.classID, []blueprints.Choice{{StudentID: f.students[0], SubjectID: f.subjects["S3"]}}); err != nil {
		t.Fatal(err)
	}

	save := func(examKey string, entries []MarkEntry) MarksResult {
		t.Helper()
		res, err := svc.SaveMarkEntries(ctx, f.admin, "admin", exam[examKey], entries)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	present := func(sheetKey, student string, marks float64) MarkEntry {
		return MarkEntry{ExamSubjectID: sheet[sheetKey], StudentID: student, Status: StatusPresent, Marks: &marks}
	}
	comps := func(student string, cq, mcq float64) MarkEntry {
		return MarkEntry{ExamSubjectID: sheet["AEX1S1"], StudentID: student, Status: StatusPresent, Components: map[string]float64{"CQ": cq, "MCQ": mcq}}
	}
	// Component and enrolment rules on entry.
	bad := save("AEX1", []MarkEntry{
		present("AEX1S1", f.students[0], 80), // a component sheet needs components
		{ExamSubjectID: sheet["AEX1S1"], StudentID: f.students[1], Status: StatusPresent, Marks: num(90), Components: map[string]float64{"CQ": 50, "MCQ": 30}}, // 90 ≠ 80
		comps(f.students[2], 61, 10),         // CQ above 60
		present("AEX1S3", f.students[1], 50), // did not choose S3
		{ExamSubjectID: sheet["AEX1S2"], StudentID: f.students[0], Status: StatusPresent, Marks: num(70), Components: map[string]float64{"X": 1}},
	})
	codes := map[string]bool{}
	for _, e := range bad.Errors {
		codes[e.Code] = true
	}
	for _, c := range []string{"components_required", "marks_mismatch", "above_full_marks", "not_enrolled", "components_not_allowed"} {
		if !codes[c] {
			t.Fatalf("missing %s in %+v", c, bad.Errors)
		}
	}
	if len(bad.Saved) != 0 {
		t.Fatalf("nothing should save: %+v", bad.Saved)
	}

	// Term exam: S1 by components; papers S2 + S4; S3 for the one who chose it.
	ok := save("AEX1", []MarkEntry{
		comps(f.students[0], 50, 30), comps(f.students[1], 15, 40), comps(f.students[2], 40, 20),
		present("AEX1S2", f.students[0], 70), present("AEX1S4", f.students[0], 70),
		present("AEX1S2", f.students[1], 60), present("AEX1S4", f.students[1], 60),
		present("AEX1S2", f.students[2], 40), present("AEX1S4", f.students[2], 30),
		present("AEX1S3", f.students[0], 90),
	})
	if len(ok.Errors) != 0 || len(ok.Saved) != 10 || ok.Saved[0].Components == nil {
		t.Fatalf("term exam: %+v", ok)
	}
	save("BEX1", []MarkEntry{{ExamSubjectID: sheet["BEX1S1"], StudentID: studentB, Status: StatusPresent, Components: map[string]float64{"CQ": 55, "MCQ": 35}},
		present("BEX1S2", studentB, 80), present("BEX1S4", studentB, 80)})
	ct := []MarkEntry{}
	for _, st := range f.students {
		for _, code := range []string{"S1", "S2", "S4"} {
			ct = append(ct, present("ACT1"+code, st, 16))
		}
	}
	ct = append(ct, present("ACT1S3", f.students[0], 18))
	if res := save("ACT1", ct); len(res.Errors) != 0 {
		t.Fatalf("CT: %+v", res.Errors)
	}
	save("BCT1", []MarkEntry{present("BCT1S1", studentB, 20), present("BCT1S2", studentB, 20), present("BCT1S4", studentB, 20)})

	// The workspace shows components; the optional sheet counts one student.
	ws, err := svc.Workspace(ctx, exam["AEX1"], sheet["AEX1S1"], f.admin, "admin", "admin")
	if err != nil || len(ws.Components) != 2 || ws.Rows[0].Components["CQ"] == 0 {
		t.Fatalf("workspace: %+v %v", ws.Components, err)
	}
	opt, err := svc.Workspace(ctx, exam["AEX1"], sheet["AEX1S3"], f.admin, "admin", "admin")
	if err != nil || opt.Sheet.Students != 1 || len(opt.Rows) != 1 || opt.Sheet.Missing != 0 {
		t.Fatalf("optional sheet: %+v rows %d %v", opt.Sheet, len(opt.Rows), err)
	}

	// Publish both term exams and the class tests.
	for _, key := range []string{"AEX1", "BEX1", "ACT1", "BCT1"} {
		if _, err := svc.SetState(ctx, f.admin, exam[key], "published"); err != nil {
			t.Fatalf("publish %s: %v", key, err)
		}
	}
	grade := func(sheetKey, student string) (g string, passed bool) {
		t.Helper()
		if err := tx.Raw("SELECT coalesce(grade,''), coalesce(passed,false) FROM exam_marks WHERE exam_subject_id = ? AND student_id = ?", sheet[sheetKey], student).Row().Scan(&g, &passed); err != nil {
			t.Fatal(err)
		}
		return
	}
	// Student 1: CQ 15 < 20 fails S1 whatever its 55.
	if g, p := grade("AEX1S1", f.students[1]); g != "F" || p {
		t.Fatalf("component failure: %s %v", g, p)
	}
	// Student 2: papers 40 + 30 = 70 / 200 = 35 % (D on the combined rule)
	// but S4 30 < 33 under each-paper: both papers store F.
	if g2, _ := grade("AEX1S2", f.students[2]); g2 != "F" {
		t.Fatalf("each paper S2: %s", g2)
	}
	if g4, _ := grade("AEX1S4", f.students[2]); g4 != "F" {
		t.Fatalf("each paper S4: %s", g4)
	}
	// Student 0: papers 140 / 200 = 70 % → A on both; S1 80 → A+; S3 90 → A+ (optional).
	if g, _ := grade("AEX1S2", f.students[0]); g != "A" {
		t.Fatalf("combined A: %s", g)
	}
	result := func(examKey, student string) (gpa float64, passed bool, class, sec int) {
		t.Helper()
		if err := tx.Raw("SELECT gpa::float8, passed, class_position, section_position FROM exam_results WHERE exam_id = ? AND student_id = ?", exam[examKey], student).Row().Scan(&gpa, &passed, &class, &sec); err != nil {
			t.Fatal(err)
		}
		return
	}
	// GPA: (5 + 4 + bonus 3) / 2 = 6 → 5.00.
	if gpa, passed, _, _ := result("AEX1", f.students[0]); gpa != 5 || !passed {
		t.Fatalf("student 0: %v %v", gpa, passed)
	}
	if gpa, passed, _, _ := result("AEX1", f.students[1]); gpa != 0 || passed {
		t.Fatalf("student 1: %v %v", gpa, passed)
	}
	// Student B (section B): S1 90 A+, papers 160/200 80 % A+ → 5.00, like
	// student 0, whose total is higher (310 with the optional subject, against
	// 250). Positions join both sections' exams of the term (E1-05): student 0
	// first, B second, then the failed students by total.
	_, _, classB, secB := result("BEX1", studentB)
	_, _, class0, sec0 := result("AEX1", f.students[0])
	_, _, class1, _ := result("AEX1", f.students[1])
	if class0 != 1 || classB != 2 || class1 != 3 || sec0 != 1 || secB != 1 {
		t.Fatalf("positions: B %d/%d, 0 %d/%d, 1 %d", classB, secB, class0, sec0, class1)
	}

	// Class tests stay hidden from the student; the term exam is shown.
	var studentUser string
	scan(&studentUser, "INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'E5 student') RETURNING id", "e5-student-"+f.stamp+"@example.test")
	exec("UPDATE students SET user_id = ? WHERE id = ?", studentUser, f.students[0])
	cards, err := svc.Cards(ctx, "student", studentUser, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Exam.ID != exam["AEX1"] {
		t.Fatalf("student sees only the term exam: %d cards", len(cards))
	}
	if _, err := svc.Card(ctx, "student", studentUser, "", exam["ACT1"]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a class test card: %v", err)
	}
	if staff, err := svc.Cards(ctx, "admin", f.admin, f.students[0], ""); err != nil || len(staff) != 2 {
		t.Fatalf("staff see both: %d %v", len(staff), err)
	}

	// The provisional term preview: student 0's S1 is CT 16/20 → 16.00 plus
	// 80/100 × 80 → 64.00 = 80.00, A+.
	preview, err := bp.TermPreview(ctx, term.ID, f.classID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.State != "ready" || len(preview.Students) != 4 || !preview.Provisional {
		t.Fatalf("preview: state %s students %d pending %+v", preview.State, len(preview.Students), preview.Pending)
	}
	var first blueprints.PreviewStudent
	for _, st := range preview.Students {
		if st.StudentID == f.students[0] {
			first = st
		}
	}
	s1 := first.Result.Subjects[0]
	if s1.Marks != "80.00" || s1.Grade != "A+" || first.Result.Outcome != "passed" {
		t.Fatalf("preview S1: %+v %s", s1, first.Result.Outcome)
	}

	// exam-verify reproduces every published result.
	report, err := verifyIn(tx, VerifyOptions{SessionID: f.session})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Differences) != 0 || report.Results != 8 {
		t.Fatalf("verify: %d results, %+v", report.Results, report.Differences)
	}
}
