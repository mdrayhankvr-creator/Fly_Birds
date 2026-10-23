package exams

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

// termClass is a blueprint class with one term (CT 1, hidden, and a term
// exam shown after the term result), two sections and every mark entered:
//   - section A: students 0, 1, 2; section B: one student;
//   - S1 has CQ 60 (pass 20) + MCQ 40 (pass 13) in the term exam;
//   - S2 and S4 are papers combined under the each-paper rule;
//   - S3 is optional, chosen by student 0; dense ranking.
type termClass struct {
	*workflowFixture
	db          *gorm.DB
	bp          *blueprints.Service
	res         *Results
	term        string
	s4          string
	sectionB    string
	studentB    string
	exam        map[string]string // AEX1, BEX1, ACT1, BCT1
	sheet       map[string]string // AEX1S1 …
	blueprintID string
	versionID   string
}

func newTermClass(t *testing.T, ctx context.Context, tx *gorm.DB, year int) *termClass {
	t.Helper()
	f := newWorkflowFixture(t, ctx, tx, year)
	c := &termClass{workflowFixture: f, db: tx, bp: f.svc.Blueprints(), res: f.svc.Results(), exam: map[string]string{}, sheet: map[string]string{}}
	scan := func(dst any, q string, args ...any) {
		t.Helper()
		if err := tx.Raw(q, args...).Row().Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	scan(&c.s4, "INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", "S4 "+f.stamp, "বিষয়", "E6S4"+f.stamp[len(f.stamp)-8:])
	exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", f.classID, c.s4)
	scan(&c.sectionB, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", f.session, f.classID)
	scan(&c.studentB, "INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", "E6-B-"+f.stamp, "Student B", f.session, f.classID, c.sectionB)

	term, err := c.bp.SaveTerm(ctx, f.admin, "", blueprints.TermInput{SessionID: f.session, Name: "First Term", Kind: "term"})
	if err != nil {
		t.Fatal(err)
	}
	c.term = term.ID
	draft, err := c.bp.CreateBlueprint(ctx, f.admin, f.session, f.classID, "")
	if err != nil {
		t.Fatal(err)
	}
	c.blueprintID = draft.BlueprintID
	d := draft.Definition
	d.Assessments = append([]blueprints.Assessment{{Key: "CT1", Name: "CT 1", TermID: term.ID, Kind: "class_test", Visibility: "hidden"}}, d.Assessments...)
	for i := range d.Assessments {
		if d.Assessments[i].Key == "EX1" {
			d.Assessments[i].Visibility = "after_term_publication"
		}
	}
	for i := range d.Subjects {
		s := &d.Subjects[i]
		s.Sheets = append(s.Sheets, blueprints.Sheet{Assessment: "CT1", Pass: 7, Components: []blueprints.Component{{Key: "TOTAL", Full: 20}}})
		s.Terms[0].Contributions = []blueprints.Contribution{{From: []string{"CT1"}, Combine: "average", Worth: 20}, {From: []string{"EX1"}, Combine: "single", Worth: 80}}
		switch s.SubjectID {
		case f.subjects["S1"]:
			pq, pm := 20.0, 13.0
			s.Sheets[0].Components = []blueprints.Component{{Key: "CQ", Full: 60, Pass: &pq}, {Key: "MCQ", Full: 40, Pass: &pm}}
		case f.subjects["S2"], c.s4:
			s.CombinedKey = "PAP"
		}
	}
	d.PaperRule, d.Policy.Ranking = "each_paper", "dense"
	if v, err := c.bp.SaveDraft(ctx, f.admin, draft.ID, d); err != nil || len(v.Issues) != 0 {
		t.Fatalf("draft: %+v %v", v.Issues, err)
	}
	if _, err := c.bp.Activate(ctx, f.admin, draft.ID); err != nil {
		t.Fatal(err)
	}
	c.versionID = draft.ID
	if _, err := c.bp.SaveSetup(ctx, f.admin, term.ID, blueprints.Setup{ClassIDs: []string{f.classID}, Due: map[string]string{"CT1": "2201-02-01", "EX1": "2201-03-01"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.bp.CreateTerm(ctx, f.admin, term.ID); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ ExamID, SectionID, Key, SheetID, SubjectID string }{}
	if err := tx.Raw(`SELECT e.id AS exam_id, e.section_id, e.assessment_key AS key, es.id AS sheet_id, es.subject_id FROM exams e JOIN exam_subjects es ON es.exam_id = e.id WHERE e.term_id = ?`, term.ID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	section := map[string]string{f.section: "A", c.sectionB: "B"}
	code := map[string]string{f.subjects["S1"]: "S1", f.subjects["S2"]: "S2", f.subjects["S3"]: "S3", c.s4: "S4"}
	for _, r := range rows {
		c.exam[section[r.SectionID]+r.Key] = r.ExamID
		c.sheet[section[r.SectionID]+r.Key+code[r.SubjectID]] = r.SheetID
	}
	if _, err := c.bp.SaveChoices(ctx, f.admin, f.session, f.classID, []blueprints.Choice{{StudentID: f.students[0], SubjectID: f.subjects["S3"]}}); err != nil {
		t.Fatal(err)
	}
	present := func(key, st string, m float64) MarkEntry {
		return MarkEntry{ExamSubjectID: c.sheet[key], StudentID: st, Status: StatusPresent, Marks: &m}
	}
	comps := func(key, st string, cq, mcq float64) MarkEntry {
		return MarkEntry{ExamSubjectID: c.sheet[key], StudentID: st, Status: StatusPresent, Components: map[string]float64{"CQ": cq, "MCQ": mcq}}
	}
	c.save(t, ctx, "AEX1", f.admin, comps("AEX1S1", f.students[0], 50, 30), comps("AEX1S1", f.students[1], 15, 40), comps("AEX1S1", f.students[2], 40, 20),
		present("AEX1S2", f.students[0], 70), present("AEX1S4", f.students[0], 70), present("AEX1S2", f.students[1], 60), present("AEX1S4", f.students[1], 60),
		present("AEX1S2", f.students[2], 40), present("AEX1S4", f.students[2], 30), present("AEX1S3", f.students[0], 90))
	c.save(t, ctx, "BEX1", f.admin, comps("BEX1S1", c.studentB, 55, 35), present("BEX1S2", c.studentB, 80), present("BEX1S4", c.studentB, 80))
	ct := []MarkEntry{}
	for _, st := range f.students {
		for _, s := range []string{"S1", "S2", "S4"} {
			ct = append(ct, present("ACT1"+s, st, 16))
		}
	}
	ct = append(ct, present("ACT1S3", f.students[0], 18))
	c.save(t, ctx, "ACT1", f.admin, ct...)
	c.save(t, ctx, "BCT1", f.admin, present("BCT1S1", c.studentB, 20), present("BCT1S2", c.studentB, 20), present("BCT1S4", c.studentB, 20))
	return c
}

func (c *termClass) save(t *testing.T, ctx context.Context, examKey, actor string, entries ...MarkEntry) MarksResult {
	t.Helper()
	res, err := c.svc.SaveMarkEntries(ctx, actor, "admin", c.exam[examKey], entries)
	if err != nil {
		t.Fatalf("save %s: %v", examKey, err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("save %s: %+v", examKey, res.Errors)
	}
	return res
}

func openTestDB(t *testing.T) (context.Context, *gorm.DB, func()) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		cancel()
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	return ctx, db, func() { _ = sqlDB.Close(); cancel() }
}

func versionsOf(t *testing.T, tx *gorm.DB, scope, scopeID, student string) []versionRow {
	t.Helper()
	rows := []versionRow{}
	if err := tx.Table("result_versions v").Select(versionColumns).Where("v.scope = ? AND v.scope_id = ? AND v.student_id = ?", scope, scopeID, student).Order("v.version").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestPostgresResultVersions walks the E6 workflow on one class: exam
// publication into versions, position updates across sections, term review,
// approval and publication, immutability, grading-band and blueprint
// changes, an optional-subject change, a correction with a superseded
// version, withheld and incomplete results, cancellation, withdrawal and the
// annual result.
func TestPostgresResultVersions(t *testing.T) {
	ctx, db, done := openTestDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	c := newTermClass(t, ctx, tx, 2301)
	f := c.workflowFixture
	s0, s1, s2, sb := f.students[0], f.students[1], f.students[2], c.studentB
	var studentUser string
	if err := tx.Raw("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'E6 student') RETURNING id", "e6-student-"+f.stamp+"@example.test").Row().Scan(&studentUser); err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE students SET user_id = ? WHERE id = ?", studentUser, s1).Error; err != nil {
		t.Fatal(err)
	}

	// A. Publishing an exam makes every student's result version 1.
	if _, err := f.svc.SetState(ctx, f.admin, c.exam["AEX1"], "published"); err != nil {
		t.Fatal(err)
	}
	v0 := versionsOf(t, tx, ScopeAssessment, c.exam["AEX1"], s0)
	if len(v0) != 1 || v0[0].Status != "published" || v0[0].Outcome != "passed" || *v0[0].ClassPosition != 1 || len(v0[0].InputsSHA256) != 64 {
		t.Fatalf("version 1: %+v", v0)
	}
	// Section B's exam joins the class ranking: section A's class positions
	// change, as a position update (version 2), never in place.
	before := versionsOf(t, tx, ScopeAssessment, c.exam["AEX1"], s1)[0]
	if _, err := f.svc.SetState(ctx, f.admin, c.exam["BEX1"], "published"); err != nil {
		t.Fatal(err)
	}
	after := versionsOf(t, tx, ScopeAssessment, c.exam["AEX1"], s1)
	if len(after) != 2 || after[0].Status != "superseded" || after[1].Status != "published" || *after[1].ClassPosition != *before.ClassPosition+1 {
		t.Fatalf("position update: %+v", after)
	}
	if after[0].Snapshot != before.Snapshot || after[0].ID != before.ID {
		t.Fatal("a superseded version must keep its snapshot")
	}
	var projected int
	if err := tx.Raw("SELECT class_position FROM exam_results WHERE exam_id = ? AND student_id = ?", c.exam["AEX1"], s1).Row().Scan(&projected); err != nil || projected != *after[1].ClassPosition {
		t.Fatalf("projection: %d %v", projected, err)
	}
	for _, k := range []string{"ACT1", "BCT1"} {
		if _, err := f.svc.SetState(ctx, f.admin, c.exam[k], "published"); err != nil {
			t.Fatal(err)
		}
	}

	// Published versions are immutable in the database itself.
	exec := func(q string, args ...any) error { return tx.Exec("SAVEPOINT imm").Error }
	_ = exec
	for _, q := range []string{
		"UPDATE result_versions SET gpa = 1 WHERE id = '" + v0[0].ID + "'",
		"UPDATE result_versions SET snapshot = '{}' WHERE id = '" + v0[0].ID + "'",
		"DELETE FROM result_versions WHERE id = '" + v0[0].ID + "'",
		"DELETE FROM result_publications WHERE id = '" + v0[0].PublicationID + "'",
		"UPDATE result_publications SET rules = '{}' WHERE id = '" + v0[0].PublicationID + "'",
	} {
		if err := tx.Exec("SAVEPOINT imm").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(q).Error; err == nil {
			t.Fatalf("allowed: %s", q)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT imm").Error; err != nil {
			t.Fatal(err)
		}
	}

	// The term exam is shown to students only after the term result (its
	// visibility); the class test never.
	if cards, err := f.svc.Cards(ctx, "student", studentUser, "", ""); err != nil || len(cards) != 0 {
		t.Fatalf("before the term result: %d %v", len(cards), err)
	}

	// The term: review (provisional), approve exactly what was reviewed,
	// publish.
	ref := ScopeRef{Scope: ScopeTerm, ScopeID: c.term, ClassID: f.classID}
	review, err := c.res.Review(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if review.State != "ready" || len(review.Students) != 4 || !review.CanApprove || review.Published != nil || len(review.Blockers) != 0 {
		t.Fatalf("term review: %s %d %+v", review.State, len(review.Students), review.Blockers)
	}
	if _, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: ref, InputsSHA256: "0000000000000000000000000000000000000000000000000000000000000000"}); !errors.Is(err, ErrReviewStale) {
		t.Fatalf("approve other inputs: %v", err)
	}
	approved, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: ref, InputsSHA256: review.InputsSHA256})
	if err != nil || approved.State != "approved" || approved.Approval == nil || approved.Approval.ApprovedBy == nil {
		t.Fatalf("approve: %+v %v", approved.Approval, err)
	}
	published, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: ref, PublicationID: approved.Approval.ID})
	if err != nil || published.State != "published" || published.Published == nil || *published.Published.Revision != 1 || published.Published.Changed != 4 {
		t.Fatalf("publish: %+v %v", published.Published, err)
	}
	// I. Publishing the same approval twice is refused.
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: ref, PublicationID: approved.Approval.ID}); !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("published twice: %v", err)
	}
	term0 := versionsOf(t, tx, ScopeTerm, c.term, s0)
	if len(term0) != 1 || term0[0].Outcome != "passed" {
		t.Fatalf("term version: %+v", term0)
	}
	snap0, _ := term0[0].snapshot()
	if snap0.Outputs.Subjects[0].Marks == nil || *snap0.Outputs.Subjects[0].Marks != "80.00" || *snap0.Outputs.Subjects[0].Grade != "A+" {
		t.Fatalf("term S1 of student 0 (CT 16/20 → 16.00 + 80 × 0.8 → 64.00): %+v", snap0.Outputs.Subjects[0])
	}
	// Students see the term result (no class-test contributions) and now the
	// term exam; never the class test.
	terms, err := c.res.PublishedResults(ctx, "student", studentUser, "")
	if err != nil || len(terms) != 1 || terms[0].Scope != ScopeTerm || terms[0].Withheld || len(terms[0].Subjects) == 0 {
		t.Fatalf("student term results: %+v %v", terms, err)
	}
	raw, _ := json.Marshal(terms)
	if json.Valid(raw) && (contains(string(raw), "contributions") || contains(string(raw), "CT1")) {
		t.Fatalf("class-test detail leaked: %s", raw)
	}
	cards, err := f.svc.Cards(ctx, "student", studentUser, "", "")
	if err != nil || len(cards) != 1 || cards[0].Exam.ID != c.exam["AEX1"] || cards[0].Version != 2 || !cards[0].Revised {
		t.Fatalf("student cards after the term: %+v %v", cards, err)
	}
	cardBefore, _ := json.Marshal(cards[0])

	// C. A grading-band change does not touch anything published, and the
	// term is not "changed" (its rules are those of its publication).
	if err := tx.Exec("UPDATE grading_bands SET gpa = gpa - 0.5 WHERE gpa >= 1 AND scheme_id = (SELECT grading_scheme_id FROM school_settings WHERE id = 1)").Error; err != nil {
		t.Fatal(err)
	}
	cards, _ = f.svc.Cards(ctx, "student", studentUser, "", "")
	if got, _ := json.Marshal(cards[0]); string(got) != string(cardBefore) {
		t.Fatalf("a band change altered a published card:\n%s\n%s", cardBefore, got)
	}
	if again, err := c.res.Review(ctx, ref); err != nil || again.State != "published" || again.Changed != 0 {
		t.Fatalf("term after a band change: %s %d %v", again.State, again.Changed, err)
	}

	// B. A new blueprint version after publication: the term's exams keep
	// theirs, so nothing published or reviewed changes.
	next, err := c.bp.NewDraft(ctx, f.admin, c.blueprintID, c.versionID)
	if err != nil {
		t.Fatal(err)
	}
	nd := next.Definition
	nd.Policy.GPARounding, nd.PaperRule = "truncate", "combined"
	if v, err := c.bp.SaveDraft(ctx, f.admin, next.ID, nd); err != nil || len(v.Issues) != 0 {
		t.Fatalf("new version: %+v %v", v.Issues, err)
	}
	if _, err := c.bp.Activate(ctx, f.admin, next.ID); err != nil {
		t.Fatal(err)
	}
	if again, err := c.res.Review(ctx, ref); err != nil || again.State != "published" || again.Changed != 0 {
		t.Fatalf("term after a new blueprint version: %s %d %v", again.State, again.Changed, err)
	}
	if v := versionsOf(t, tx, ScopeTerm, c.term, s0); len(v) != 1 || v[0].Snapshot != term0[0].Snapshot {
		t.Fatal("a blueprint change altered a published snapshot")
	}

	// G. An optional-subject change after publication: what is published
	// stays; the review shows the change, which needs approval with a reason.
	if _, err := c.bp.SaveChoices(ctx, f.admin, f.session, f.classID, []blueprints.Choice{{StudentID: s0, SubjectID: ""}}); err != nil {
		t.Fatal(err)
	}
	if v := versionsOf(t, tx, ScopeTerm, c.term, s0); len(v) != 1 || v[0].Status != "published" {
		t.Fatalf("published term after a choice change: %+v", v)
	}
	// With no choice left, the class's fourth subject applies to everyone
	// again (E5), so the others now lack S3 marks: nothing can be approved,
	// and what is published stays as it is.
	changed, err := c.res.Review(ctx, ref)
	if err != nil || changed.State != "blocked" || changed.Outcomes["incomplete"] != 3 {
		t.Fatalf("review after a choice change: %s %+v %v", changed.State, changed.Outcomes, err)
	}
	if _, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: ref, InputsSHA256: changed.InputsSHA256, Reason: "choice"}); !errors.Is(err, ErrResultsNotReady) {
		t.Fatalf("approve after a choice change: %v", err)
	}
	if v := versionsOf(t, tx, ScopeTerm, c.term, s0); v[0].Snapshot != term0[0].Snapshot {
		t.Fatal("a choice change altered a published snapshot")
	}
	// Put the choice back: nothing to approve any more.
	if _, err := c.bp.SaveChoices(ctx, f.admin, f.session, f.classID, []blueprints.Choice{{StudentID: s0, SubjectID: f.subjects["S3"]}}); err != nil {
		t.Fatal(err)
	}
	if again, err := c.res.Review(ctx, ref); err != nil || again.State != "published" {
		t.Fatalf("choice restored: %s %v", again.State, err)
	}

	// D. A correction: student 1's CQ was 15 (fails S1); it should be 25.
	aRef := ScopeRef{Scope: ScopeAssessment, ScopeID: c.exam["AEX1"]}
	if _, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], s1, "x"); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("a correction without a reason: %v", err)
	}
	corr, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], s1, "CQ script re-marked")
	if err != nil || corr.Status != "open" || len(corr.Cells) != 3 {
		t.Fatalf("open correction: %+v %v", corr, err)
	}
	if _, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], s1, "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("two open corrections: %v", err)
	}
	cq := MarkEntry{ExamSubjectID: c.sheet["AEX1S1"], StudentID: s1, Status: StatusPresent, Components: map[string]float64{"CQ": 25, "MCQ": 40}, RowVersion: corr.Cells[0].RowVersion}
	if _, err := f.svc.SaveMarkEntries(ctx, f.teacher, "teacher", c.exam["AEX1"], []MarkEntry{cq}); !errors.Is(err, ErrExamNotDraft) {
		t.Fatalf("a teacher changed a published mark: %v", err)
	}
	other := MarkEntry{ExamSubjectID: c.sheet["AEX1S1"], StudentID: s2, Status: StatusPresent, Components: map[string]float64{"CQ": 59, "MCQ": 40}, RowVersion: 1}
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], []MarkEntry{other}); err != nil || len(res.Errors) != 1 || res.Errors[0].Code != "correction_required" {
		t.Fatalf("a student without a correction: %+v %v", res, err)
	}
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], []MarkEntry{cq}); err != nil || len(res.Errors) != 0 {
		t.Fatalf("the correction: %+v %v", res, err)
	}
	// Until published, the family still sees the published version.
	cards, _ = f.svc.Cards(ctx, "student", studentUser, "", "")
	if got, _ := json.Marshal(cards[0]); string(got) != string(cardBefore) {
		t.Fatal("an unpublished correction reached the student")
	}
	rev, err := c.res.Review(ctx, aRef)
	if err != nil || rev.State != "changes" || rev.Changed < 1 || len(rev.Corrections) != 1 {
		t.Fatalf("revision review: %s %d %v", rev.State, rev.Changed, err)
	}
	var mine ReviewStudent
	for _, st := range rev.Students {
		if st.StudentID == s1 {
			mine = st
		}
	}
	if mine.Change != "changed" || mine.Previous == nil || mine.Outputs.Outcome != "passed" || mine.Previous.Outcome != "failed" {
		t.Fatalf("student 1's change: %+v", mine)
	}
	// Approval binds to the inputs: a change after approval blocks publishing.
	ok, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: aRef, InputsSHA256: rev.InputsSHA256})
	if err != nil || ok.Approval == nil || ok.Approval.Reason != "CQ script re-marked" {
		t.Fatalf("approve the correction: %+v %v", ok.Approval, err)
	}
	cq.Components, cq.RowVersion = map[string]float64{"CQ": 26, "MCQ": 40}, cq.RowVersion+1
	f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], []MarkEntry{cq})
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: aRef, PublicationID: ok.Approval.ID}); !errors.Is(err, ErrApprovalStale) {
		t.Fatalf("publish a stale approval: %v", err)
	}
	rev, _ = c.res.Review(ctx, aRef)
	ok, err = c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: aRef, InputsSHA256: rev.InputsSHA256, Reason: "CQ script re-marked (26)"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: aRef, PublicationID: ok.Approval.ID}); err != nil {
		t.Fatal(err)
	}
	// E. The previous version stays readable to staff; F. the student sees
	// only the new one, marked revised.
	hist, err := c.res.History(ctx, ScopeAssessment, c.exam["AEX1"], s1)
	if err != nil || len(hist) != 3 || hist[0].Status != "published" || hist[0].Kind != "correction" || hist[1].Status != "superseded" || hist[0].Reason != "CQ script re-marked (26)" {
		t.Fatalf("history: %+v %v", hist, err)
	}
	oldDetail, err := c.res.Version(ctx, hist[1].ID)
	if err != nil || !oldDetail.Verification.HashMatches || !oldDetail.Verification.Reproduced {
		t.Fatalf("old version detail: %+v %v", oldDetail.Verification, err)
	}
	newDetail, err := c.res.Version(ctx, hist[0].ID)
	if err != nil || !newDetail.Verification.Reproduced || newDetail.Verification.Differences == nil {
		t.Fatalf("new version detail: %+v %v", newDetail.Verification, err)
	}
	cards, _ = f.svc.Cards(ctx, "student", studentUser, "", "")
	if len(cards) != 1 || cards[0].VersionID != hist[0].ID || !cards[0].Result.Passed || !cards[0].Revised {
		t.Fatalf("student card after the correction: %+v", cards)
	}
	var corrStatus string
	if err := tx.Raw("SELECT status FROM result_corrections WHERE id = ?", corr.ID).Row().Scan(&corrStatus); err != nil || corrStatus != "applied" {
		t.Fatalf("correction status: %s %v", corrStatus, err)
	}
	// The term result follows only through its own approved revision.
	termRev, err := c.res.Review(ctx, ref)
	if err != nil || termRev.State != "changes" || termRev.Changed < 1 {
		t.Fatalf("term after the correction: %s %d %v", termRev.State, termRev.Changed, err)
	}
	termOK, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: ref, InputsSHA256: termRev.InputsSHA256, Reason: "S1 CQ re-marked for student 1"})
	if err != nil {
		t.Fatal(err)
	}
	termPub, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: ref, PublicationID: termOK.Approval.ID})
	if err != nil || *termPub.Published.Revision != 2 || termPub.Published.Changed != termRev.Changed {
		t.Fatalf("term revision: %+v %v", termPub.Published, err)
	}
	if v := versionsOf(t, tx, ScopeTerm, c.term, s0); v[0].Snapshot != term0[0].Snapshot {
		t.Fatal("an unchanged student's term version was rewritten")
	}

	// H. Withheld through a correction: the student then sees "withheld".
	corr2, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], s1, "Exam committee review")
	if err != nil {
		t.Fatal(err)
	}
	cell := corr2.Cells[1]
	marks := 60.0
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], []MarkEntry{{ExamSubjectID: cell.ExamSubjectID, StudentID: s1, Status: StatusWithheld, Marks: &marks, Reason: "Committee", RowVersion: cell.RowVersion}}); err != nil || len(res.Errors) != 0 {
		t.Fatalf("withhold: %+v %v", res, err)
	}
	rev, _ = c.res.Review(ctx, aRef)
	ok, err = c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: aRef, InputsSHA256: rev.InputsSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: aRef, PublicationID: ok.Approval.ID}); err != nil {
		t.Fatal(err)
	}
	cards, _ = f.svc.Cards(ctx, "student", studentUser, "", "")
	if len(cards) != 1 || !cards[0].Withheld || len(cards[0].Subjects) != 0 || cards[0].Result.GPA != 0 {
		t.Fatalf("withheld card: %+v", cards)
	}
	var rows int64
	tx.Table("exam_results").Where("exam_id = ? AND student_id = ?", c.exam["AEX1"], s1).Count(&rows)
	if rows != 0 {
		t.Fatal("a withheld result has a projection row")
	}
	termRev, _ = c.res.Review(ctx, ref)
	termOK, err = c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: ref, InputsSHA256: termRev.InputsSHA256, Reason: "Withheld by the committee"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: ref, PublicationID: termOK.Approval.ID}); err != nil {
		t.Fatal(err)
	}
	terms, _ = c.res.PublishedResults(ctx, "student", studentUser, "")
	if len(terms) != 1 || !terms[0].Withheld || len(terms[0].Subjects) != 0 || terms[0].GPA != nil {
		t.Fatalf("withheld term result: %+v", terms)
	}

	// Incomplete blocks: a correction that leaves a mark incomplete cannot be
	// approved; cancelling the correction restores the published marks.
	corr3, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], s2, "Script missing")
	if err != nil {
		t.Fatal(err)
	}
	cell = corr3.Cells[1]
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], []MarkEntry{{ExamSubjectID: cell.ExamSubjectID, StudentID: s2, Status: StatusIncomplete, RowVersion: cell.RowVersion}}); err != nil || len(res.Errors) != 0 {
		t.Fatalf("incomplete: %+v %v", res, err)
	}
	rev, _ = c.res.Review(ctx, aRef)
	if rev.State != "blocked" || rev.CanApprove {
		t.Fatalf("an incomplete correction: %s", rev.State)
	}
	if _, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: aRef, InputsSHA256: rev.InputsSHA256}); !errors.Is(err, ErrResultsNotReady) {
		t.Fatalf("approve an incomplete result: %v", err)
	}
	if err := c.res.CancelCorrection(ctx, f.admin, c.exam["AEX1"], corr3.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := tx.Raw("SELECT status FROM exam_marks WHERE exam_subject_id = ? AND student_id = ?", cell.ExamSubjectID, s2).Row().Scan(&status); err != nil || status != StatusPresent {
		t.Fatalf("restored mark: %s %v", status, err)
	}
	if again, err := c.res.Review(ctx, aRef); err != nil || again.State != "published" {
		t.Fatalf("after cancelling: %s %v", again.State, err)
	}

	// K. exam-verify reproduces every projection row from the stored marks.
	report, err := verifyIn(tx, VerifyOptions{SessionID: f.session})
	if err != nil || len(report.Differences) != 0 {
		t.Fatalf("verify: %+v %v", report.Differences, err)
	}

	// The annual result (final only of the one term) from published terms.
	annualRef := ScopeRef{Scope: ScopeAnnual, ScopeID: f.session, ClassID: f.classID}
	annual, err := c.res.Review(ctx, annualRef)
	if err != nil || annual.State != "ready" || len(annual.Students) != 4 {
		t.Fatalf("annual review: %s %+v %v", annual.State, annual.Blockers, err)
	}
	var a0 ReviewStudent
	for _, st := range annual.Students {
		if st.StudentID == s0 {
			a0 = st
		}
	}
	if a0.Outputs.Subjects[0].Marks == nil || *a0.Outputs.Subjects[0].Marks != "80.00" || len(a0.Outputs.Subjects[0].Terms) != 1 {
		t.Fatalf("annual S1 of student 0: %+v", a0.Outputs.Subjects[0])
	}
	annualOK, err := c.res.Approve(ctx, f.headTeacher, ApproveInput{ScopeRef: annualRef, InputsSHA256: annual.InputsSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.res.Publish(ctx, f.admin, PublishInput{ScopeRef: annualRef, PublicationID: annualOK.Approval.ID}); err != nil {
		t.Fatal(err)
	}
	if v := versionsOf(t, tx, ScopeAnnual, f.session, sb); len(v) != 1 || v[0].Outcome != "passed" {
		t.Fatalf("annual version of student B: %+v", v)
	}
	overview, err := c.res.Overview(ctx, f.session)
	if err != nil || len(overview.Terms) != 1 || overview.Terms[0].Classes[0].State != "published" || overview.Annual[0].State != "published" {
		t.Fatalf("overview: %+v %v", overview, err)
	}

	// Withdrawal keeps the versions (withdrawn) and republication adds one.
	if _, err := f.svc.SetStateReason(ctx, f.admin, c.exam["BEX1"], "draft", "Wrong marks scheme"); err != nil {
		t.Fatal(err)
	}
	wb := versionsOf(t, tx, ScopeAssessment, c.exam["BEX1"], sb)
	if len(wb) != 1 || wb[0].Status != "withdrawn" || wb[0].CloseReason != "Wrong marks scheme" {
		t.Fatalf("withdrawn: %+v", wb)
	}
	if err := f.svc.DeleteExam(ctx, f.admin, c.exam["BEX1"]); !errors.Is(err, ErrHasVersions) {
		t.Fatalf("delete with history: %v", err)
	}
	if _, err := f.svc.SetState(ctx, f.admin, c.exam["BEX1"], "published"); err != nil {
		t.Fatal(err)
	}
	if wb = versionsOf(t, tx, ScopeAssessment, c.exam["BEX1"], sb); len(wb) != 2 || wb[1].Status != "published" || wb[1].Version != 2 {
		t.Fatalf("republished: %+v", wb)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestPostgresResultVersionsMigrate runs the upgrade's step on a result
// published the pre-E6 way (stored rows, no versions): version 1 records
// exactly the stored values, nothing existing changes, the report card shows
// the same values and a second run adds nothing.
func TestPostgresResultVersionsMigrate(t *testing.T) {
	ctx, db, done := openTestDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	f := newWorkflowFixture(t, ctx, tx, 2302)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	e, err := f.svc.SaveExam(ctx, f.admin, "", ExamInput{Name: "Half Yearly", ExamType: "half_yearly", SessionID: f.session, ClassID: f.classID, SectionID: f.section})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"S1", "S2", "S3"} {
		if e, err = f.svc.SaveSubject(ctx, f.admin, e.ID, SubjectInput{SubjectID: f.subjects[code], FullMarks: 100, PassMarks: 33}); err != nil {
			t.Fatal(err)
		}
	}
	sheet := map[string]string{}
	for _, s := range e.Subjects {
		for code, id := range f.subjects {
			if id == s.SubjectID {
				sheet[code] = s.ID
			}
		}
	}
	m := func(v float64) *float64 { return &v }
	entries := []MarkEntry{}
	for i, marks := range [][3]float64{{85, 75, 65}, {50, 20, 40}, {60, 60, 60}} {
		for j, code := range []string{"S1", "S2", "S3"} {
			entry := MarkEntry{ExamSubjectID: sheet[code], StudentID: f.students[i], Status: StatusPresent, Marks: m(marks[j])}
			if i == 2 && code == "S1" {
				entry.Status, entry.Reason = StatusWithheld, "Committee"
			}
			entries = append(entries, entry)
		}
	}
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", e.ID, entries); err != nil || len(res.Errors) != 0 {
		t.Fatalf("marks: %+v %v", res, err)
	}
	// Published the pre-E6 way: grades and results stored, no version.
	grades := map[string][3]string{f.students[0]: {"A+|5.00|t", "A|4.00|t", "A-|3.50|t"}, f.students[1]: {"B|3.00|t", "F|0.00|f", "C|2.00|t"}}
	for st, gs := range grades {
		for j, code := range []string{"S1", "S2", "S3"} {
			parts := splitBar(gs[j])
			exec("UPDATE exam_marks SET grade = ?, grade_point = ?::numeric, passed = ? WHERE exam_subject_id = ? AND student_id = ?", parts[0], parts[1], parts[2] == "t", sheet[code], st)
		}
	}
	exec(`INSERT INTO exam_results (exam_id, student_id, session_id, class_id, section_id, total_marks, full_marks, gpa, passed, class_position, section_position) VALUES
 (?, ?, ?, ?, ?, 225, 300, 5.00, true, 1, 1), (?, ?, ?, ?, ?, 110, 300, 0.00, false, 2, 2)`,
		e.ID, f.students[0], f.session, f.classID, f.section, e.ID, f.students[1], f.session, f.classID, f.section)
	exec("UPDATE exams SET status = 'published', published_at = now() - interval '3 days' WHERE id = ?", e.ID)

	before := examFingerprint(t, tx, f.session)
	var raw string
	if err := tx.Raw("SELECT result_versions_migrate()::text").Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if after := examFingerprint(t, tx, f.session); after != before {
		t.Fatal("the upgrade changed a mark, grade, sheet or result")
	}
	var counts map[string]int
	if err := json.Unmarshal([]byte(raw), &counts); err != nil || counts["versions"] < 3 || counts["withheld"] < 1 {
		t.Fatalf("counts: %s %v", raw, err)
	}
	v := versionsOf(t, tx, ScopeAssessment, e.ID, f.students[0])
	if len(v) != 1 || v[0].Version != 1 || *v[0].GPA != "5.00" || *v[0].TotalMarks != "225.00" || *v[0].ClassPosition != 1 {
		t.Fatalf("migrated version: %+v", v)
	}
	var linked int64
	tx.Table("exam_results").Where("exam_id = ? AND result_version_id IS NOT NULL", e.ID).Count(&linked)
	if linked != 2 {
		t.Fatalf("projection rows linked: %d", linked)
	}
	card, err := f.svc.Card(ctx, "admin", f.admin, f.students[0], e.ID)
	if err != nil || card.Result.GPA != 5 || card.Result.TotalMarks != 225 || len(card.Subjects) != 3 || *card.Subjects[0].Grade != "A+" || card.Revised {
		t.Fatalf("card from the migrated version: %+v %v", card, err)
	}
	withheld, err := f.svc.Card(ctx, "admin", f.admin, f.students[2], e.ID)
	if err != nil || !withheld.Withheld || len(withheld.Subjects) != 0 {
		t.Fatalf("withheld card: %+v %v", withheld, err)
	}
	detail, err := f.svc.Results().Version(ctx, v[0].ID)
	if err != nil || !detail.Verification.HashMatches || !detail.Verification.Reproduced || detail.Version.Source != "migration" {
		t.Fatalf("migrated version detail: %+v %v", detail.Verification, err)
	}
	if err := tx.Raw("SELECT result_versions_migrate()::text").Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &counts); err != nil || counts["versions"] != 0 || counts["publications"] != 0 {
		t.Fatalf("second run: %s %v", raw, err)
	}
}

func splitBar(s string) []string {
	out, cur := []string{}, ""
	for _, r := range s {
		if r == '|' {
			out, cur = append(out, cur), ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
