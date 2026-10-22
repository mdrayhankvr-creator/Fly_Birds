package blueprints

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

// fixture is a class with S1, S2 (required) and S3 (fourth), one section
// with three students, in a session of its own.
type fixture struct {
	ctx                       context.Context
	tx                        *gorm.DB
	svc                       *Service
	stamp, admin, class, sect string
	session, nextSession      string
	subjects                  map[string]string
	students                  []string
}

func newFixture(t *testing.T, year int) (*fixture, func()) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	sqlDB, _ := db.DB()
	done := func() { tx.Rollback(); sqlDB.Close(); cancel() }
	f := &fixture{ctx: ctx, tx: tx, svc: NewService(NewRepository(tx)), stamp: fmt.Sprint(time.Now().UnixNano()), subjects: map[string]string{}}
	insert := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(q, args...).Row().Scan(&id); err != nil {
			done()
			t.Fatal(err)
		}
		return id
	}
	f.admin = insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'E5 admin') RETURNING id", "e5-admin-"+f.stamp+"@example.test")
	f.session = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	f.nextSession = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year+1), fmt.Sprintf("%d-01-01", year+1), fmt.Sprintf("%d-12-31", year+1))
	f.class = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class E5 "+f.stamp)
	f.sect = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", f.session, f.class)
	for i, code := range []string{"S1", "S2", "S3"} {
		f.subjects[code] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", code+" "+f.stamp, "বিষয়", "E5"+code+f.stamp[len(f.stamp)-8:])
		if err := tx.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", f.class, f.subjects[code], i < 2, i == 2).Error; err != nil {
			done()
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		f.students = append(f.students, insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id",
			fmt.Sprintf("E5-%d-%s", i, f.stamp), fmt.Sprintf("Student %d", i), f.session, f.class, f.sect))
	}
	return f, done
}

func TestPostgresTermsAndBlueprints(t *testing.T) {
	f, done := newFixture(t, 2198)
	defer done()
	ctx, svc := f.ctx, f.svc

	// Terms: in order, named once per session, deletable only while unused.
	var ids []string
	for _, n := range []struct{ name, kind string }{{"First Term", "term"}, {"Half-Yearly", "term"}, {"Annual", "annual"}} {
		term, err := svc.SaveTerm(ctx, f.admin, "", TermInput{SessionID: f.session, Name: n.name, Kind: n.kind, StartsOn: "2198-01-10", EndsOn: "2198-03-30"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, term.ID)
		if term.Sequence != len(ids) {
			t.Fatalf("sequence %d for %s", term.Sequence, n.name)
		}
	}
	if _, err := svc.SaveTerm(ctx, f.admin, "", TermInput{SessionID: f.session, Name: "first term", Kind: "term"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := svc.SaveTerm(ctx, f.admin, "", TermInput{SessionID: f.session, Name: "Bad dates", Kind: "term", StartsOn: "2198-05-01", EndsOn: "2198-04-01"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dates: %v", err)
	}
	extra, _ := svc.SaveTerm(ctx, f.admin, "", TermInput{SessionID: f.session, Name: "Spare", Kind: "term"})
	if err := svc.DeleteTerm(ctx, f.admin, extra.ID); err != nil {
		t.Fatalf("unused term: %v", err)
	}

	// A class with no blueprint starts from today's model, which activates.
	list, err := svc.ClassBlueprints(ctx, f.session)
	if err != nil {
		t.Fatal(err)
	}
	var mine *ClassBlueprint
	for i := range list {
		if list[i].ClassID == f.class {
			mine = &list[i]
		}
	}
	if mine == nil || mine.BlueprintID != nil {
		t.Fatalf("class without blueprint: %+v", mine)
	}
	draft, err := svc.CreateBlueprint(ctx, f.admin, f.session, f.class, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Definition.Assessments) != 3 || len(draft.Definition.Subjects) != 3 || draft.Definition.Subjects[2].Role != "optional" || len(draft.Issues) != 0 {
		t.Fatalf("seed: %+v issues %+v", draft.Definition, draft.Issues)
	}
	if _, err := svc.CreateBlueprint(ctx, f.admin, f.session, f.class, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("second draft: %v", err)
	}
	v1, err := svc.Activate(ctx, f.admin, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Status != "active" || len(v1.SHA256) != 64 || v1.Version != 1 {
		t.Fatalf("v1: %+v", v1)
	}
	if _, err := svc.SaveDraft(ctx, f.admin, v1.ID, v1.Definition); !errors.Is(err, ErrInUse) {
		t.Fatalf("an active version is frozen: %v", err)
	}
	if err := svc.DeleteTerm(ctx, f.admin, ids[0]); !errors.Is(err, ErrInUse) {
		t.Fatalf("a term in a blueprint: %v", err)
	}

	// v2: CT 1 + CT 2 averaged (20) and the term exam (80) in the first term.
	v2, err := svc.NewDraft(ctx, f.admin, v1.BlueprintID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := v2.Definition
	d.Assessments = append([]Assessment{
		{Key: "CT1", Name: "CT 1", TermID: ids[0], Kind: "class_test", Visibility: "hidden"},
		{Key: "CT2", Name: "CT 2", TermID: ids[0], Kind: "class_test", Visibility: "hidden"},
	}, d.Assessments...)
	for i := range d.Subjects {
		s := &d.Subjects[i]
		s.Sheets = append(s.Sheets, Sheet{Assessment: "CT1", Pass: 7, Components: total(20)}, Sheet{Assessment: "CT2", Pass: 7, Components: total(20)})
		s.Terms[0].Contributions = []Contribution{{From: []string{"CT1", "CT2"}, Combine: "average", Worth: 20}, {From: []string{"EX1"}, Combine: "single", Worth: 70}}
	}
	saved, err := svc.SaveDraft(ctx, f.admin, v2.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Issues) != 3 || saved.Issues[0].Code != "worth_sum" {
		t.Fatalf("issues: %+v", saved.Issues)
	}
	var invalid *ValidationError
	if _, err := svc.Activate(ctx, f.admin, v2.ID); !errors.As(err, &invalid) || len(invalid.Issues) != 3 {
		t.Fatalf("invalid activation: %v", err)
	}
	for i := range d.Subjects {
		d.Subjects[i].Terms[0].Contributions[1].Worth = 80
	}
	if _, err := svc.SaveDraft(ctx, f.admin, v2.ID, d); err != nil {
		t.Fatal(err)
	}
	active, err := svc.Activate(ctx, f.admin, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	for _, v := range active.Versions {
		statuses = append(statuses, fmt.Sprintf("v%d:%s", v.Version, v.Status))
	}
	if fmt.Sprint(statuses) != "[v2:active v1:retired]" {
		t.Fatalf("versions: %v", statuses)
	}
	var rows struct{ Assessments, Components int }
	f.tx.Raw(`SELECT (SELECT count(*) FROM blueprint_assessments WHERE version_id = ?) AS assessments,
       (SELECT count(*) FROM blueprint_components bc JOIN blueprint_subjects bs ON bs.id = bc.blueprint_subject_id WHERE bs.version_id = ?) AS components`, v2.ID, v2.ID).Scan(&rows)
	if rows.Assessments != 5 || rows.Components != 15 {
		t.Fatalf("normalised rows: %+v", rows)
	}

	// The worked sample reproduces example 1 of the calculation rules.
	sample, err := svc.Sample(ctx, v2.ID, SampleRequest{SubjectID: f.subjects["S1"], TermID: ids[0], Marks: map[string]SampleMark{
		"CT1": {Marks: f64(17)}, "CT2": {Marks: f64(14)}, "EX1": {Marks: f64(68)},
	}})
	if err != nil || sample.Marks != "69.90" || sample.Grade != "A-" || sample.Contributions[0].Value != "15.50" {
		t.Fatalf("sample: %+v %v", sample, err)
	}

	// Visibility is a display policy: it may change on an active version.
	vis, err := svc.SetVisibility(ctx, f.admin, v2.ID, "CT1", "with_assessment_publication")
	if err != nil {
		t.Fatal(err)
	}
	var shown string
	f.tx.Raw("SELECT student_visibility FROM blueprint_assessments WHERE version_id = ? AND key = 'CT1'", vis.ID).Row().Scan(&shown)
	if shown != "with_assessment_publication" {
		t.Fatalf("visibility %q", shown)
	}

	// Copy forward: next session's terms are made by order and mapped.
	next, err := svc.CreateBlueprint(ctx, f.admin, f.nextSession, f.class, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Terms) != 3 || next.Terms[0].Name != "First Term" || next.Definition.Assessments[0].TermID != next.Terms[0].ID || len(next.Issues) != 0 {
		t.Fatalf("copy forward: terms %+v issues %+v", next.Terms, next.Issues)
	}
	if next.Definition.Assessments[0].Visibility != "hidden" {
		t.Fatal("copy forward keeps the definition's own visibility")
	}
}

func f64(v float64) *float64 { return &v }

func TestPostgresSubjectChoices(t *testing.T) {
	f, done := newFixture(t, 2199)
	defer done()
	ctx, svc := f.ctx, f.svc
	takes := func(student, subject string) bool {
		t.Helper()
		var ok bool
		if err := f.tx.Raw("SELECT student_takes_subject(?, ?, ?, ?)", student, f.session, f.class, f.subjects[subject]).Row().Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	// No choices yet: the class's fourth subject applies to everyone.
	if !takes(f.students[1], "S3") || !takes(f.students[1], "S1") {
		t.Fatal("without choices every student takes every class subject")
	}
	view, err := svc.SaveChoices(ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: f.subjects["S3"]}})
	if err != nil {
		t.Fatal(err)
	}
	if !view.InUse || view.Counts[f.subjects["S3"]] != 1 || len(view.Optional) != 1 {
		t.Fatalf("choices: %+v", view)
	}
	if !takes(f.students[0], "S3") || takes(f.students[1], "S3") || !takes(f.students[1], "S1") {
		t.Fatal("with choices only students who chose a fourth subject take it")
	}
	if _, err := svc.SaveChoices(ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[1], SubjectID: f.subjects["S1"]}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a required subject cannot be a choice: %v", err)
	}
	if _, err := svc.SaveChoices(ctx, f.admin, f.nextSession, f.class, []Choice{{StudentID: f.students[1], SubjectID: f.subjects["S3"]}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a student of another session: %v", err)
	}
	if view, err = svc.SaveChoices(ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: ""}}); err != nil || view.InUse {
		t.Fatalf("clearing: %+v %v", view, err)
	}
	if !takes(f.students[1], "S3") {
		t.Fatal("with no choices left the class's fourth subject applies again")
	}
}

// The wizard creates a whole term: an exam per section per assessment and a
// sheet per subject, with components; it refuses while a due date is missing
// or a name clashes, and running it again creates nothing new.
func TestPostgresTermSetupWizard(t *testing.T) {
	f, done := newFixture(t, 2200)
	defer done()
	ctx, svc := f.ctx, f.svc
	exec := func(q string, args ...any) {
		t.Helper()
		if err := f.tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	var sectionB, teacher string
	f.tx.Raw("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", f.session, f.class).Row().Scan(&sectionB)
	user := f.admin // any user id works for an employee record
	f.tx.Raw("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','E5 Teacher','Teacher','2020-01-01') RETURNING id", "E5T"+f.stamp[len(f.stamp)-8:], user).Row().Scan(&teacher)
	exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", teacher, f.session, f.class, f.subjects["S1"])

	term, err := svc.SaveTerm(ctx, f.admin, "", TermInput{SessionID: f.session, Name: "First Term", Kind: "term"})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateBlueprint(ctx, f.admin, f.session, f.class, "")
	if err != nil {
		t.Fatal(err)
	}
	d := draft.Definition
	d.Assessments = append([]Assessment{
		{Key: "CT1", Name: "CT 1", TermID: term.ID, Kind: "class_test", Visibility: "hidden"},
		{Key: "CT2", Name: "CT 2", TermID: term.ID, Kind: "class_test", Visibility: "hidden"},
	}, d.Assessments...)
	for i := range d.Subjects {
		s := &d.Subjects[i]
		s.Sheets = append(s.Sheets, Sheet{Assessment: "CT1", Pass: 7, Components: total(20)}, Sheet{Assessment: "CT2", Pass: 7, Components: total(20)})
		s.Terms[0].Contributions = []Contribution{{From: []string{"CT1", "CT2"}, Combine: "average", Worth: 20}, {From: []string{"EX1"}, Combine: "single", Worth: 80}}
	}
	d.Subjects[1].Sheets[0].Components = []Component{{Key: "CQ", Full: 60, Pass: f64(20)}, {Key: "MCQ", Full: 40, Pass: f64(13)}}
	if v, err := svc.SaveDraft(ctx, f.admin, draft.ID, d); err != nil || len(v.Issues) != 0 {
		t.Fatalf("draft: %+v %v", v.Issues, err)
	}
	version, err := svc.Activate(ctx, f.admin, draft.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Choices saved, but one assessment has no due date yet: nothing created.
	setup := Setup{ClassIDs: []string{f.class}, Due: map[string]string{"CT1": "2200-02-01", "CT2": "2200-03-01"}, Assign: map[string]string{f.class + "|" + f.subjects["S2"]: teacher}}
	plan, err := svc.SaveSetup(ctx, f.admin, term.ID, setup)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Exams != 6 || plan.Sheets != 18 || plan.ToCreate != 18 || len(plan.Gaps) != 2 || len(plan.Assessments) != 3 {
		t.Fatalf("plan: exams %d sheets %d new %d gaps %+v assessments %+v", plan.Exams, plan.Sheets, plan.ToCreate, plan.Gaps, plan.Assessments)
	}
	if _, err := svc.CreateTerm(ctx, f.admin, term.ID); !errors.Is(err, ErrSetupBlocked) {
		t.Fatalf("missing due date: %v", err)
	}
	var none int64
	f.tx.Table("exams").Where("term_id = ?", term.ID).Count(&none)
	if none != 0 {
		t.Fatal("a refused setup must create nothing")
	}

	// An unrelated exam already named like a new one blocks too.
	exec("INSERT INTO exams(name,exam_type,session_id,class_id,section_id,created_by,updated_by) VALUES ('First Term · CT 1','class_test',?,?,?,?,?)", f.session, f.class, sectionB, f.admin, f.admin)
	setup.Due["EX1"] = "2200-04-01"
	if plan, err = svc.SaveSetup(ctx, f.admin, term.ID, setup); err != nil || len(plan.Conflicts) != 1 {
		t.Fatalf("conflict: %+v %v", plan.Conflicts, err)
	}
	if _, err := svc.CreateTerm(ctx, f.admin, term.ID); !errors.Is(err, ErrSetupBlocked) {
		t.Fatalf("name conflict: %v", err)
	}
	exec("DELETE FROM exams WHERE term_id IS NULL AND session_id = ?", f.session)

	res, err := svc.CreateTerm(ctx, f.admin, term.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExamsCreated != 6 || res.SheetsCreated != 18 || res.Assigned != 1 || res.Plan.Term.Status != "running" || res.Plan.ToCreate != 0 || len(res.Plan.Gaps) != 1 {
		t.Fatalf("create: %+v gaps %+v", res, res.Plan.Gaps)
	}
	var check struct {
		Linked, Components, Due int
		Name                    string
		Full                    float64
	}
	f.tx.Raw(`SELECT (SELECT count(*) FROM exams WHERE term_id = ? AND blueprint_version_id = ? AND assessment_key IN ('CT1','CT2','EX1')) AS linked,
       (SELECT count(*) FROM exam_subject_components c JOIN exam_subjects es ON es.id = c.exam_subject_id JOIN exams e ON e.id = es.exam_id WHERE e.term_id = ?) AS components,
       (SELECT count(*) FROM exams WHERE term_id = ? AND marks_due_on IS NOT NULL) AS due,
       (SELECT name FROM exams WHERE term_id = ? AND assessment_key = 'CT1' AND section_id = ?) AS name,
       (SELECT es.full_marks FROM exam_subjects es JOIN exams e ON e.id = es.exam_id WHERE e.term_id = ? AND e.assessment_key = 'EX1' AND es.subject_id = ? LIMIT 1) AS full`,
		term.ID, version.ID, term.ID, term.ID, term.ID, f.sect, term.ID, f.subjects["S2"]).Scan(&check)
	if check.Linked != 6 || check.Components != 4 || check.Due != 6 || check.Name != "First Term · CT 1" || check.Full != 100 {
		t.Fatalf("created exams: %+v", check)
	}
	// Idempotent.
	again, err := svc.CreateTerm(ctx, f.admin, term.ID)
	if err != nil || again.ExamsCreated != 0 || again.SheetsCreated != 0 {
		t.Fatalf("second create: %+v %v", again, err)
	}
}
