package exams

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

// examFingerprint hashes every mark, grade, sheet and result value of a
// session, so an upgrade can be shown to change none of them.
func examFingerprint(t *testing.T, tx *gorm.DB, session string) string {
	t.Helper()
	var out string
	err := tx.Raw(`SELECT md5(
  coalesce((SELECT string_agg(concat_ws('|', m.id, m.exam_subject_id, m.student_id, m.marks, m.grade, m.grade_point, m.passed, m.status, m.status_reason, m.row_version), ',' ORDER BY m.id)
              FROM exam_marks m WHERE m.session_id = ?), '') || '#' ||
  coalesce((SELECT string_agg(concat_ws('|', r.exam_id, r.student_id, r.total_marks, r.full_marks, r.gpa, r.passed, r.class_position, r.section_position, r.calculated_at), ',' ORDER BY r.exam_id, r.student_id)
              FROM exam_results r WHERE r.session_id = ?), '') || '#' ||
  coalesce((SELECT string_agg(concat_ws('|', es.id, es.full_marks, es.pass_marks, es.sheet_status, es.reviewed_at), ',' ORDER BY es.id)
              FROM exam_subjects es JOIN exams e ON e.id = es.exam_id WHERE e.session_id = ?), '') || '#' ||
  coalesce((SELECT string_agg(concat_ws('|', e.id, e.name, e.exam_type, e.status, e.published_at), ',' ORDER BY e.id)
              FROM exams e WHERE e.session_id = ?), ''))`, session, session, session, session).Row().Scan(&out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPostgresLegacyBlueprintLink runs the upgrade's linking step on exams
// made the pre-E5 way: terms and a legacy blueprint appear, every exam is
// linked, class tests become hidden from students, and no mark, grade, sheet
// or result value changes.
func TestPostgresLegacyBlueprintLink(t *testing.T) {
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
	f := newWorkflowFixture(t, ctx, tx, 2197)
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A second section with one student, sitting the same half-yearly exam.
	var sectionB, studentB string
	if err := tx.Raw("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", f.session, f.classID).Row().Scan(&sectionB); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", "E5B-"+f.stamp, "Student B", f.session, f.classID, sectionB).Row().Scan(&studentB); err != nil {
		t.Fatal(err)
	}
	publish := func(name, kind, section string, students []string, subjects map[string][2]float64, mark float64) Exam {
		t.Helper()
		e, err := f.svc.SaveExam(ctx, f.admin, "", ExamInput{Name: name, ExamType: kind, SessionID: f.session, ClassID: f.classID, SectionID: section})
		if err != nil {
			t.Fatal(err)
		}
		for code, fp := range subjects {
			if e, err = f.svc.SaveSubject(ctx, f.admin, e.ID, SubjectInput{SubjectID: f.subjects[code], FullMarks: fp[0], PassMarks: fp[1]}); err != nil {
				t.Fatal(err)
			}
		}
		entries := []MarkInput{}
		for _, s := range e.Subjects {
			for _, st := range students {
				entries = append(entries, MarkInput{ExamSubjectID: s.ID, StudentID: st, Marks: mark})
			}
		}
		if err := f.svc.SaveMarks(ctx, f.admin, "admin", e.ID, entries); err != nil {
			t.Fatal(err)
		}
		if e, err = f.svc.SetState(ctx, f.admin, e.ID, "published"); err != nil {
			t.Fatal(err)
		}
		return e
	}
	all := map[string][2]float64{"S1": {51, 16.83}, "S2": {100, 33}, "S3": {100, 33}}
	hyA := publish("Half Yearly", "half_yearly", f.section, f.students, all, 40)
	hyB := publish("half yearly", "half_yearly", sectionB, []string{studentB}, all, 45)
	ct := publish("CT 1", "class_test", f.section, f.students, map[string][2]float64{"S1": {20, 7}}, 15)
	draft, err := f.svc.SaveExam(ctx, f.admin, "", ExamInput{Name: "Annual", ExamType: "annual", SessionID: f.session, ClassID: f.classID, SectionID: f.section})
	if err != nil {
		t.Fatal(err)
	}
	// The way the upgrade sees them: no term yet.
	exec("UPDATE exams SET term_id=NULL, assessment_key=NULL, blueprint_version_id=NULL WHERE session_id=?", f.session)
	// One transaction gives every row the same time; the upgrade orders terms
	// by each exam's creation, so give them the order they were made in.
	for i, id := range []string{hyA.ID, hyB.ID, ct.ID, draft.ID} {
		exec("UPDATE exams SET created_at = now() + make_interval(mins => ?) WHERE id=?", i, id)
	}

	before := examFingerprint(t, tx, f.session)
	var raw string
	if err := tx.Raw("SELECT exam_link_legacy()::text").Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if after := examFingerprint(t, tx, f.session); after != before {
		t.Fatal("linking changed a mark, grade, sheet or result")
	}
	var first map[string]int
	if err := json.Unmarshal([]byte(raw), &first); err != nil || first["class_tests_hidden"] < 1 {
		t.Fatalf("the audit counts published class tests now hidden: %s %v", raw, err)
	}

	type link struct {
		ID, Name, TermID, AssessmentKey, Kind, Visibility, VersionID string
		Visible                                                      bool
	}
	links := map[string]link{}
	rows := []link{}
	if err := tx.Raw(`SELECT e.id, e.name, coalesce(e.term_id::text,'') AS term_id, coalesce(e.assessment_key,'') AS assessment_key,
       coalesce(ba.kind,'') AS kind, coalesce(ba.student_visibility,'') AS visibility, coalesce(e.blueprint_version_id::text,'') AS version_id,
       exam_visible_to_students(e.id) AS visible
  FROM exams e LEFT JOIN blueprint_assessments ba ON ba.version_id = e.blueprint_version_id AND ba.key = e.assessment_key
 WHERE e.session_id = ?`, f.session).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		links[r.ID] = r
		if r.TermID == "" || r.AssessmentKey == "" || r.VersionID == "" {
			t.Fatalf("exam %q not linked: %+v", r.Name, r)
		}
	}
	a, b := links[hyA.ID], links[hyB.ID]
	if a.TermID != b.TermID || a.AssessmentKey != b.AssessmentKey || a.VersionID != b.VersionID {
		t.Fatalf("both sections' half-yearly exams belong to one term and assessment: %+v %+v", a, b)
	}
	if c := links[ct.ID]; c.Kind != "class_test" || c.Visibility != "hidden" || c.Visible || c.TermID == a.TermID {
		t.Fatalf("class test: %+v", c)
	}
	if !a.Visible || a.Kind != "term_exam" || a.Visibility != "with_assessment_publication" {
		t.Fatalf("half-yearly: %+v", a)
	}
	if links[draft.ID].TermID == a.TermID {
		t.Fatal("annual exam needs its own term")
	}

	// Terms in order of their first exam; the published ones in "results".
	terms := []struct {
		Name, Kind, Status string
		Sequence           int
	}{}
	if err := tx.Raw("SELECT name, kind, status, sequence FROM exam_terms WHERE session_id=? ORDER BY sequence", f.session).Scan(&terms).Error; err != nil {
		t.Fatal(err)
	}
	if len(terms) != 3 || terms[0].Name != "Half Yearly" || terms[0].Status != "results" || terms[1].Name != "CT 1" || terms[2].Kind != "annual" || terms[2].Status != "running" {
		t.Fatalf("terms: %+v", terms)
	}

	// The legacy version: active, frozen, final-only totals per subject.
	var version struct {
		Status    string
		Legacy    bool
		Snapshot  string
		Sha       string
		Subjects  int
		Optional  int
		S1Full    float64
		S1CTFull  float64
		Component string
	}
	if err := tx.Raw(`SELECT v.status, v.legacy, v.grading_scheme_snapshot::text AS snapshot, v.definition_sha256 AS sha,
       (SELECT count(*) FROM blueprint_subjects bs WHERE bs.version_id = v.id) AS subjects,
       (SELECT count(*) FROM blueprint_subjects bs WHERE bs.version_id = v.id AND bs.role = 'optional') AS optional,
       (SELECT bc.full_marks FROM blueprint_components bc JOIN blueprint_subjects bs ON bs.id = bc.blueprint_subject_id WHERE bs.version_id = v.id AND bs.subject_id = ? AND bc.assessment_key = ?) AS s1_full,
       (SELECT bc.full_marks FROM blueprint_components bc JOIN blueprint_subjects bs ON bs.id = bc.blueprint_subject_id WHERE bs.version_id = v.id AND bs.subject_id = ? AND bc.assessment_key = ?) AS s1_ct_full,
       (SELECT string_agg(DISTINCT bc.component_key, ',') FROM blueprint_components bc JOIN blueprint_subjects bs ON bs.id = bc.blueprint_subject_id WHERE bs.version_id = v.id) AS component
  FROM blueprint_versions v WHERE v.id = ?`, f.subjects["S1"], a.AssessmentKey, f.subjects["S1"], links[ct.ID].AssessmentKey, a.VersionID).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	var bands []map[string]any
	if err := json.Unmarshal([]byte(version.Snapshot), &bands); err != nil || len(bands) == 0 {
		t.Fatalf("grading snapshot %q: %v", version.Snapshot, err)
	}
	if version.Status != "active" || !version.Legacy || len(version.Sha) != 64 || version.Subjects != 3 || version.Optional != 1 ||
		version.S1Full != 51 || version.S1CTFull != 20 || version.Component != "TOTAL" {
		t.Fatalf("legacy version: %+v", version)
	}
	if err := tx.Exec("SAVEPOINT frozen").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE blueprint_versions SET definition='{}'::jsonb WHERE id=?", a.VersionID).Error; err == nil {
		t.Fatal("an active version must be frozen")
	}
	exec("ROLLBACK TO SAVEPOINT frozen")

	// An exam made the old way after the upgrade, named like an upgraded
	// term, joins that term and the class's legacy assessment, so its class
	// positions group with the other sections as before (E1-05).
	late, err := f.svc.SaveExam(ctx, f.admin, "", ExamInput{Name: "ct 1", ExamType: "class_test", SessionID: f.session, ClassID: f.classID, SectionID: sectionB})
	if err != nil {
		t.Fatal(err)
	}
	if late.TermID == nil || *late.TermID != links[ct.ID].TermID || *late.AssessmentKey != links[ct.ID].AssessmentKey || late.BlueprintVersionID == nil {
		t.Fatalf("a late class test: %+v", late)
	}
	// Renamed away from it, the exam leaves the term.
	if late, err = f.svc.SaveExam(ctx, f.admin, late.ID, ExamInput{Name: "Spot test", ExamType: "class_test", SessionID: f.session, ClassID: f.classID, SectionID: sectionB}); err != nil || late.TermID != nil {
		t.Fatalf("renamed: %+v %v", late, err)
	}
	exec("DELETE FROM exams WHERE id = ?", late.ID)

	// Running it again changes nothing.
	if err := tx.Raw("SELECT exam_link_legacy()::text").Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var counts map[string]int
	if err := json.Unmarshal([]byte(raw), &counts); err != nil {
		t.Fatal(err)
	}
	var termCount int64
	if err := tx.Table("exam_terms").Where("session_id=?", f.session).Count(&termCount).Error; err != nil || termCount != 3 {
		t.Fatalf("terms after a second run: %d %v", termCount, err)
	}
	if after := examFingerprint(t, tx, f.session); after != before {
		t.Fatal("a second run changed a mark, grade, sheet or result")
	}
}
