package exams

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/analytics"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/fees"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/promotion"
	"gorm.io/gorm"
)

// historyPrint hashes every stored row the old year is made of: marks and
// their components, results, result versions and publications, attendance
// and fee dues. Promotion must change none of them.
func historyPrint(t *testing.T, tx *gorm.DB) string {
	t.Helper()
	var v string
	q := ""
	for i, table := range []string{"exam_marks", "exam_component_marks", "exam_results", "result_versions", "result_publications", "student_attendance", "student_fees", "fee_adjustments"} {
		if i > 0 {
			q += " || '|' || "
		}
		q += fmt.Sprintf("(SELECT md5(coalesce(string_agg(x::text, ',' ORDER BY x::text), '')) FROM %s x)", table)
	}
	if err := tx.Raw("SELECT " + q).Row().Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// readViews renders what people read about the old year: every student's
// report cards (as the office sees them), both sections' tabulations and the
// session's exam analytics.
func readViews(t *testing.T, c *termClass, tx *gorm.DB, students []string) string {
	t.Helper()
	ctx := c.ctx
	out := map[string]any{}
	for _, st := range students {
		cards, err := c.svc.Cards(ctx, "admin", c.admin, st, "")
		if err != nil {
			t.Fatal(err)
		}
		out["cards "+st] = cards
	}
	for _, k := range []string{"AEX1", "BEX1"} {
		tab, err := c.svc.Tabulation(ctx, c.exam[k])
		if err != nil {
			t.Fatal(err)
		}
		out["tab "+k] = tab
	}
	ex, err := analytics.NewService(analytics.NewRepository(tx)).Exams(ctx, analytics.Scope{}, analytics.Filter{SessionID: c.session})
	if err != nil {
		t.Fatal(err)
	}
	out["analytics"] = ex
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func publishScope(t *testing.T, c *termClass, ref ScopeRef) ReviewView {
	t.Helper()
	ctx := c.ctx
	rev, err := c.res.Review(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	in := ApproveInput{ScopeRef: ref, InputsSHA256: rev.InputsSHA256}
	if rev.Published != nil {
		in.Reason = "Corrected after year-end promotion"
	}
	ok, err := c.res.Approve(ctx, c.headTeacher, in)
	if err != nil {
		t.Fatalf("approve %s: %s %+v %v", ref.Scope, rev.State, rev.Blockers, err)
	}
	pub, err := c.res.Publish(ctx, c.admin, PublishInput{ScopeRef: ref, PublicationID: ok.Approval.ID})
	if err != nil {
		t.Fatalf("publish %s: %v", ref.Scope, err)
	}
	return pub
}

// TestPostgresPromotionKeepsTheOldYear runs a whole year-end on the E6 class:
// publish the year (exams, term, annual), prepare and approve a promotion
// batch, apply it, prove nothing recorded or shown for the old year changed,
// correct and republish the old year after promotion, and revert.
func TestPostgresPromotionKeepsTheOldYear(t *testing.T) {
	ctx, db, done := openTestDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	c := newTermClass(t, ctx, tx, 2305)
	f := c.workflowFixture
	all := append(append([]string{}, f.students...), c.studentB)
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

	// Student 1 fails paper S1 outright (CQ 0, MCQ 0), so the year has a
	// failed student.
	c.save(t, ctx, "AEX1", f.admin, MarkEntry{ExamSubjectID: c.sheet["AEX1S1"], StudentID: f.students[1], Status: StatusPresent,
		Components: map[string]float64{"CQ": 0, "MCQ": 0}, RowVersion: 1})

	// The year: exams, the term and the annual result published; attendance
	// and a fee due recorded; a concession for that session.
	for _, k := range []string{"AEX1", "BEX1", "ACT1", "BCT1"} {
		if _, err := f.svc.SetState(ctx, f.admin, c.exam[k], "published"); err != nil {
			t.Fatal(err)
		}
	}
	publishScope(t, c, ScopeRef{Scope: ScopeTerm, ScopeID: c.term, ClassID: f.classID})
	publishScope(t, c, ScopeRef{Scope: ScopeAnnual, ScopeID: f.session, ClassID: f.classID})
	exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,'2305-03-01','present','manual',?,?)",
		f.students[0], f.session, f.classID, f.section, f.admin, f.admin)
	var category, structure string
	scan(&category, "INSERT INTO fee_categories(code,name_en,name_bn,frequency,created_by,updated_by) VALUES (?,?,?,'monthly',?,?) RETURNING id", "M15"+f.stamp[len(f.stamp)-8:], "Tuition", "বেতন", f.admin, f.admin)
	scan(&structure, "INSERT INTO fee_structures(session_id,class_id,category_id,amount,due_day,created_by,updated_by) VALUES (?,?,?,1000,10,?,?) RETURNING id", f.session, f.classID, category, f.admin, f.admin)
	feeRepo := fees.NewRepository(tx)
	if _, err := feeRepo.SaveConcession(ctx, f.admin, fees.ConcessionInput{StudentID: f.students[0], CategoryID: category, Percent: 50, Reason: "Sibling"}); err != nil {
		t.Fatal(err)
	}
	if _, err := feeRepo.GenerateDues(ctx, f.admin, fees.GenerateInput{SessionID: f.session, ClassID: f.classID, Period: "2305-02"}); err != nil {
		t.Fatal(err)
	}
	var discounted string
	scan(&discounted, "SELECT discount_amount::text FROM student_fees WHERE student_id = ? AND session_id = ?", f.students[0], f.session)
	if discounted != "500.00" {
		t.Fatalf("the old session's concession: %s", discounted)
	}
	// Student 2 can sign in; their login follows withdrawal.
	var s2User string
	scan(&s2User, "INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'M15 student') RETURNING id", "m15-s2-"+f.stamp+"@example.test")
	exec("UPDATE students SET user_id = ? WHERE id = ?", s2User, f.students[2])

	beforeRows, beforeViews := historyPrint(t, tx), readViews(t, c, tx, all)

	// The next year, the next class and its sections.
	next := f.session2(t, 2306)
	var seven, sevenA, sevenB, sixA, sixB string
	scan(&seven, "INSERT INTO academic_classes(name,display_order) VALUES (?, 2) RETURNING id", "Class M15 next "+f.stamp)
	scan(&sevenA, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", next, seven)
	scan(&sevenB, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", next, seven)
	scan(&sixA, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", next, f.classID)
	scan(&sixB, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", next, f.classID)

	ps := promotion.NewService(tx)
	admin := promotion.Actor{ID: f.admin, Role: "admin"}
	head := promotion.Actor{ID: f.headTeacher, Role: "head_teacher"}
	if _, err := ps.SetProgression(ctx, admin, f.classID, promotion.ProgressionInput{NextClassID: &seven}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.SetProgression(ctx, admin, seven, promotion.ProgressionInput{NextClassID: &f.classID}); !errors.Is(err, promotion.ErrInvalid) {
		t.Fatalf("a progression cycle: %v", err)
	}
	b, err := ps.Create(ctx, admin, promotion.CreateInput{FromSessionID: f.session, ToSessionID: next, ClassID: f.classID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Create(ctx, admin, promotion.CreateInput{FromSessionID: f.session, ToSessionID: next, ClassID: f.classID}); !errors.Is(err, promotion.ErrOpenBatch) {
		t.Fatalf("a second open batch: %v", err)
	}
	if len(b.Items) != 4 {
		t.Fatalf("items: %+v", b.Items)
	}
	// Recommendations follow each student's published annual result.
	var failed string
	for _, it := range b.Items {
		var outcome string
		scan(&outcome, "SELECT outcome FROM result_versions WHERE scope = 'annual' AND scope_id = ? AND student_id = ? AND status = 'published'", f.session, it.StudentID)
		want := map[string]string{"passed": promotion.Promote, "failed": promotion.Repeat}[outcome]
		if it.Recommendation != want || it.Decision == nil || *it.Decision != want || it.AnnualOutcome == nil || *it.AnnualOutcome != outcome {
			t.Fatalf("student %s (%s): %+v", it.StudentID, outcome, it)
		}
		if outcome == "failed" {
			failed = it.StudentID
		}
		wantClass, wantSection := seven, map[string]string{f.section: sevenA, c.sectionB: sevenB}[it.SourceSectionID]
		if want == promotion.Repeat {
			wantClass, wantSection = f.classID, map[string]string{f.section: sixA, c.sectionB: sixB}[it.SourceSectionID]
		}
		if *it.TargetClassID != wantClass || *it.TargetSectionID != wantSection || !sameRoll(it.SourceRollNo, it.TargetRollNo) {
			t.Fatalf("placement of %s: %+v", it.StudentID, it)
		}
	}
	if failed == "" {
		t.Fatal("the fixture should have one failed student")
	}
	if len(b.Blockers) != 0 {
		t.Fatalf("blockers: %+v", b.Blockers)
	}
	// Student 2 leaves: a decision against the recommendation needs a reason.
	withdraw := promotion.Withdraw
	if _, err := ps.UpdateItem(ctx, admin, b.ID, f.students[2], promotion.ItemInput{Decision: &withdraw}); !errors.Is(err, promotion.ErrReasonRequired) {
		t.Fatalf("an override without a reason: %v", err)
	}
	reason := "Family moved to Chattogram"
	if b, err = ps.UpdateItem(ctx, admin, b.ID, f.students[2], promotion.ItemInput{Decision: &withdraw, Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	if b, err = ps.Arrange(ctx, admin, b.ID, promotion.ArrangeInput{RollPolicy: "merit"}); err != nil {
		t.Fatal(err)
	}

	// Only the head teacher approves, and only the inputs reviewed.
	if _, err := ps.Approve(ctx, admin, b.ID, promotion.HashInput{InputsSHA256: b.InputsSHA256}); !errors.Is(err, promotion.ErrForbidden) {
		t.Fatalf("the administrator approved: %v", err)
	}
	if _, err := ps.Approve(ctx, head, b.ID, promotion.HashInput{InputsSHA256: fmt.Sprintf("%064d", 0)}); !errors.Is(err, promotion.ErrStale) {
		t.Fatalf("approving other inputs: %v", err)
	}
	if _, err := ps.Apply(ctx, admin, b.ID); !errors.Is(err, promotion.ErrState) {
		t.Fatalf("applying a draft: %v", err)
	}
	approved, err := ps.Approve(ctx, head, b.ID, promotion.HashInput{InputsSHA256: b.InputsSHA256})
	if err != nil || approved.Status != "approved" {
		t.Fatalf("approve: %+v %v", approved.Status, err)
	}
	// An edit after approval returns the batch to draft.
	roll := 40
	if b, err = ps.UpdateItem(ctx, admin, b.ID, f.students[0], promotion.ItemInput{TargetRollNo: &roll}); err != nil || b.Status != "draft" || b.RollPolicy != "manual" {
		t.Fatalf("edit after approval: %s %s %v", b.Status, b.RollPolicy, err)
	}
	if _, err := ps.Approve(ctx, head, b.ID, promotion.HashInput{InputsSHA256: b.InputsSHA256}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Apply(ctx, head, b.ID); !errors.Is(err, promotion.ErrForbidden) {
		t.Fatalf("the head teacher applied: %v", err)
	}
	applied, err := ps.Apply(ctx, admin, b.ID)
	if err != nil || applied.Status != "applied" {
		t.Fatalf("apply: %v", err)
	}
	again, err := ps.Apply(ctx, admin, b.ID)
	if err != nil || again.Status != "applied" || again.AppliedAt == nil || *again.AppliedAt != *applied.AppliedAt {
		t.Fatalf("apply again: %v", err)
	}
	var audits int64
	scan(&audits, "SELECT count(*) FROM audit_events WHERE action = 'promotion.batch_applied' AND target_id = ?", b.ID)
	if audits != 1 {
		t.Fatalf("apply audited %d times", audits)
	}

	// Nothing recorded or shown for the old year changed.
	if historyPrint(t, tx) != beforeRows {
		t.Fatal("promotion changed a mark, result, version, attendance or fee row")
	}
	if readViews(t, c, tx, all) != beforeViews {
		t.Fatal("promotion changed a report card, tabulation or analytics of the old year")
	}
	// Where everyone is now.
	type place struct {
		Session, Class, Status, State string
		Roll                          *int
	}
	where := func(st string) place {
		var p place
		if err := tx.Raw("SELECT session_id AS session, class_id AS class, status, roll_no AS roll FROM students WHERE id = ?", st).Scan(&p).Error; err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, it := range applied.Items {
		p := where(it.StudentID)
		var years int64
		scan(&years, "SELECT count(*) FROM student_enrolments WHERE student_id = ?", it.StudentID)
		switch *it.Decision {
		case promotion.Promote:
			if p.Session != next || p.Class != seven || p.Status != "active" || years != 2 {
				t.Fatalf("promoted %s: %+v %d years", it.StudentID, p, years)
			}
		case promotion.Repeat:
			if p.Session != next || p.Class != f.classID || p.Status != "active" || years != 2 || it.StudentID != failed {
				t.Fatalf("repeater %s: %+v %d years", it.StudentID, p, years)
			}
		case promotion.Withdraw:
			if p.Session != f.session || p.Status != "inactive" || years != 1 {
				t.Fatalf("withdrawn %s: %+v %d years", it.StudentID, p, years)
			}
		}
	}
	if p := where(f.students[0]); p.Roll == nil || *p.Roll != 40 {
		t.Fatalf("manual roll: %+v", p)
	}
	var active, suspended bool
	scan(&active, "SELECT is_active FROM users WHERE id = ?", s2User)
	scan(&suspended, "SELECT login_suspended_at IS NOT NULL FROM students WHERE id = ?", f.students[2])
	if active || !suspended {
		t.Fatalf("withdrawn login active=%v suspended=%v", active, suspended)
	}
	var onRoll int64
	scan(&onRoll, "SELECT count(*) FROM student_rolls WHERE session_id = ? AND class_id = ?", f.session, f.classID)
	if onRoll != 4 {
		t.Fatalf("the old year's roll: %d (promoted, repeating and the year-end leaver stay on it)", onRoll)
	}
	// Concessions do not carry over; the review lists the old one.
	review, err := feeRepo.ReviewConcessions(ctx, f.session, next)
	if err != nil || len(review) != 1 || review[0].StudentID != f.students[0] || review[0].ToPercent != nil {
		t.Fatalf("concession review: %+v %v", review, err)
	}
	var nextStructure string
	scan(&nextStructure, "INSERT INTO fee_structures(session_id,class_id,category_id,amount,due_day,created_by,updated_by) VALUES (?,?,?,1200,10,?,?) RETURNING id", next, seven, category, f.admin, f.admin)
	if _, err := feeRepo.GenerateDues(ctx, f.admin, fees.GenerateInput{SessionID: next, ClassID: seven, Period: "2306-02"}); err != nil {
		t.Fatal(err)
	}
	scan(&discounted, "SELECT discount_amount::text FROM student_fees WHERE student_id = ? AND session_id = ?", f.students[0], next)
	if discounted != "0.00" {
		t.Fatalf("a concession carried into the new session: %s", discounted)
	}
	exec("DELETE FROM fee_adjustments WHERE student_fee_id IN (SELECT id FROM student_fees WHERE session_id = ?)", next)
	exec("DELETE FROM student_fees WHERE session_id = ?", next)

	// The old year can still be corrected and republished after promotion.
	corr, err := c.res.OpenCorrection(ctx, f.admin, c.exam["AEX1"], failed, "CQ script re-marked after promotion")
	if err != nil {
		t.Fatal(err)
	}
	var fix []MarkEntry
	for _, cell := range corr.Cells {
		if cell.ExamSubjectID == c.sheet["AEX1S1"] {
			fix = append(fix, MarkEntry{ExamSubjectID: cell.ExamSubjectID, StudentID: failed, Status: StatusPresent, Components: map[string]float64{"CQ": 25, "MCQ": 40}, RowVersion: cell.RowVersion})
		}
	}
	if res, err := f.svc.SaveMarkEntries(ctx, f.admin, "admin", c.exam["AEX1"], fix); err != nil || len(res.Errors) != 0 {
		t.Fatalf("correction marks: %+v %v", res, err)
	}
	if pub := publishScope(t, c, ScopeRef{Scope: ScopeAssessment, ScopeID: c.exam["AEX1"]}); pub.Published == nil || pub.Published.Kind != "correction" {
		t.Fatalf("assessment correction: %+v", pub.Published)
	}
	publishScope(t, c, ScopeRef{Scope: ScopeTerm, ScopeID: c.term, ClassID: f.classID})
	annual := publishScope(t, c, ScopeRef{Scope: ScopeAnnual, ScopeID: f.session, ClassID: f.classID})
	if annual.Published == nil || *annual.Published.Revision != 2 || len(annual.Students) != 4 {
		t.Fatalf("annual republished after promotion: %+v, %d students", annual.Published, len(annual.Students))
	}
	// The student who left at year-end completed the year: still in it.
	kept := false
	for _, st := range annual.Students {
		kept = kept || st.StudentID == f.students[2]
	}
	if !kept {
		t.Fatal("the year-end leaver dropped out of the republished annual result")
	}
	var roll1 *int
	scan(&roll1, "SELECT (snapshot->'student'->>'roll_no')::int FROM result_versions WHERE scope = 'annual' AND scope_id = ? AND student_id = ? AND status = 'published'", f.session, failed)
	var oldRoll *int
	scan(&oldRoll, "SELECT roll_no FROM student_enrolments WHERE student_id = ? AND session_id = ?", failed, f.session)
	if !sameRoll(roll1, oldRoll) {
		t.Fatalf("republished snapshot roll %v, old-year roll %v", roll1, oldRoll)
	}
	// The applied batch now flags nothing, and the head teacher's batch view
	// shows the student's old annual version.
	if v, err := ps.Get(ctx, b.ID); err != nil || v.Status != "applied" {
		t.Fatalf("applied batch: %v", err)
	}

	// Revert: refused once the new year has a record, allowed after.
	exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) SELECT id, session_id, class_id, section_id, '2306-03-01', 'present', 'manual', ?, ? FROM students WHERE id = ?",
		f.admin, f.admin, f.students[1])
	if _, err := ps.Revert(ctx, admin, b.ID, promotion.RevertInput{Reason: "Wrong class map"}); !errors.Is(err, promotion.ErrRevertRefused) {
		t.Fatalf("revert after activity: %v", err)
	}
	exec("DELETE FROM student_attendance WHERE student_id = ? AND session_id = ?", f.students[1], next)
	if _, err := ps.Revert(ctx, admin, b.ID, promotion.RevertInput{Reason: ""}); !errors.Is(err, promotion.ErrReasonRequired) {
		t.Fatalf("revert without a reason: %v", err)
	}
	reverted, err := ps.Revert(ctx, admin, b.ID, promotion.RevertInput{Reason: "Wrong class map"})
	if err != nil || reverted.Status != "reverted" {
		t.Fatalf("revert: %v", err)
	}
	for _, st := range all {
		p := where(st)
		var years int64
		scan(&years, "SELECT count(*) FROM student_enrolments WHERE student_id = ?", st)
		if p.Session != f.session || p.Class != f.classID || p.Status != "active" || years != 1 {
			t.Fatalf("after revert %s: %+v %d years", st, p, years)
		}
	}
	scan(&active, "SELECT is_active FROM users WHERE id = ?", s2User)
	if !active {
		t.Fatal("revert did not restore the suspended login")
	}
	exec("SET CONSTRAINTS ALL IMMEDIATE")
	// A new batch may be made after a revert.
	if _, err := ps.Create(ctx, admin, promotion.CreateInput{FromSessionID: f.session, ToSessionID: next, ClassID: f.classID}); err != nil {
		t.Fatalf("a batch after revert: %v", err)
	}
}

func sameRoll(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// session2 adds another session to the fixture's school.
func (f *workflowFixture) session2(t *testing.T, year int) string {
	t.Helper()
	var id string
	if err := f.tx.Raw("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)).Row().Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
