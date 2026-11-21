package blueprints

import (
	"errors"
	"fmt"
	"testing"
)

func TestPostgresReligionCorrectionRequestsProtectOwnership(t *testing.T) {
	f, done := newFixture(t, 2474)
	defer done()
	if err := f.tx.Exec("UPDATE academic_sessions SET is_active=true WHERE id=?", f.session).Error; err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.ReligionChoices(f.ctx, f.session, f.class)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"], f.subjects["S2"]}, v.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	userNumber := 0
	insertUser := func(role string) string {
		t.Helper()
		userNumber++
		var id string
		if err := f.tx.Raw("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,'unused',?,'Test') RETURNING id", fmt.Sprintf("choice-%d-%s@example.test", userNumber, f.stamp), role).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	studentUser := insertUser("student")
	guardianUser := insertUser("guardian")
	stranger := insertUser("student")
	if err := f.tx.Exec("UPDATE students SET user_id=? WHERE id=?", studentUser, f.students[0]).Error; err != nil {
		t.Fatal(err)
	}
	var guardian string
	if err := f.tx.Raw("INSERT INTO guardians(name_en,user_id) VALUES ('Test guardian',?) RETURNING id", guardianUser).Row().Scan(&guardian); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship) VALUES (?,?,'parent')", f.students[0], guardian).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ReligionCorrectionOptions(f.ctx, stranger, "student", f.students[0], f.session); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stranger read options: %v", err)
	}
	options, err := f.svc.ReligionCorrectionOptions(f.ctx, guardianUser, "guardian", f.students[0], f.session)
	if err != nil || len(options.Options) != 2 || options.CurrentSubjectID != "" {
		t.Fatalf("guardian options: %+v %v", options, err)
	}
	if _, err := f.svc.RequestReligionCorrection(f.ctx, stranger, "student", f.students[0], f.session, f.subjects["S2"]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stranger requested: %v", err)
	}
	if _, err := f.svc.RequestReligionCorrection(f.ctx, guardianUser, "guardian", f.students[1], f.session, f.subjects["S2"]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked guardian requested: %v", err)
	}
	request, err := f.svc.RequestReligionCorrection(f.ctx, guardianUser, "guardian", f.students[0], f.session, f.subjects["S2"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RequestReligionCorrection(f.ctx, studentUser, "student", f.students[0], f.session, f.subjects["S1"]); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate pending request: %v", err)
	}
	if _, err := f.svc.ReligionCorrectionRequests(f.ctx, guardianUser); !errors.Is(err, ErrNotFound) {
		t.Fatalf("guardian listed requests: %v", err)
	}
	list, err := f.svc.ReligionCorrectionRequests(f.ctx, f.admin)
	if err != nil || len(list) != 1 || list[0].ID != request.ID {
		t.Fatalf("admin requests: %+v %v", list, err)
	}
	if err := f.svc.ResolveReligionCorrection(f.ctx, f.admin, request.ID, "resolved"); !errors.Is(err, ErrConflict) {
		t.Fatalf("resolved without actual choice: %v", err)
	}
	v, err = f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class, []ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S2"]}}, v.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ResolveReligionCorrection(f.ctx, f.admin, request.ID, "resolved"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ResolveReligionCorrection(f.ctx, f.admin, request.ID, "rejected"); !errors.Is(err, ErrConflict) {
		t.Fatalf("resolved request changed: %v", err)
	}
	if _, err := f.svc.RequestReligionCorrection(f.ctx, studentUser, "student", f.students[0], f.session, f.subjects["S1"]); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresReligionChoicesAreExplicitAndSeparateFromFourth(t *testing.T) {
	f, done := newFixture(t, 2471)
	defer done()
	if err := f.tx.Exec("UPDATE academic_sessions SET is_active=true WHERE id=?", f.session).Error; err != nil {
		t.Fatal(err)
	}

	v, err := f.svc.ReligionChoices(f.ctx, f.session, f.class)
	if err != nil || len(v.Options) != 0 || len(v.Students) != 3 {
		t.Fatalf("empty preview: %+v, %v", v, err)
	}
	if _, err := f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"], f.subjects["S2"]}, v.Digest, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfirmed options accepted: %v", err)
	}
	v, err = f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"], f.subjects["S2"]}, v.Digest, true)
	if err != nil || len(v.Options) != 2 || v.Unresolved != 3 {
		t.Fatalf("configured preview: %+v, %v", v, err)
	}
	if _, err := f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"]}, v.Digest, true); err != nil {
		t.Fatalf("unselected option should be removable: %v", err)
	}
	v, err = f.svc.ReligionChoices(f.ctx, f.session, f.class)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"], f.subjects["S2"]}, v.Digest, true); err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.ReligionChoices(f.ctx, f.session, f.class)
	if err != nil {
		t.Fatal(err)
	}
	checkTakes := func(student, subject string, want bool) {
		t.Helper()
		var got bool
		if err := f.tx.Raw("SELECT student_takes_subject(?,?,?,?)", student, f.session, f.class, f.subjects[subject]).Row().Scan(&got); err != nil || got != want {
			t.Fatalf("student %s takes %s: %v, %v; want %v", student, subject, got, err, want)
		}
	}
	checkTakes(f.students[0], "S1", false)
	checkTakes(f.students[0], "S2", false)
	checkTakes(f.students[0], "S3", true) // Religion choice cannot activate legacy fourth choices.
	if _, err := f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class,
		[]ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S1"]}, {StudentID: f.students[1], SubjectID: f.subjects["S2"]}}, "stale", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad digest accepted: %v", err)
	}
	if _, err := f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class,
		[]ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S1"]}}, v.Digest, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfirmed choice accepted: %v", err)
	}
	v, err = f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class,
		[]ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S1"]}, {StudentID: f.students[1], SubjectID: f.subjects["S2"]}}, v.Digest, true)
	if err != nil || v.Unresolved != 1 {
		t.Fatalf("saved choice preview: %+v, %v", v, err)
	}
	checkTakes(f.students[0], "S1", true)
	checkTakes(f.students[0], "S2", false)
	checkTakes(f.students[1], "S1", false)
	checkTakes(f.students[1], "S2", true)
	checkTakes(f.students[2], "S1", false)
	checkTakes(f.students[0], "S3", true)
	if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: f.subjects["S3"]}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveChoices(f.ctx, f.admin, f.session, f.class, []Choice{{StudentID: f.students[0], SubjectID: ""}}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := f.tx.Raw("SELECT count(*) FROM student_subject_choices WHERE student_id=? AND role='religion'", f.students[0]).Row().Scan(&count); err != nil || count != 1 {
		t.Fatalf("fourth-subject clear removed religion choice: %d, %v", count, err)
	}
	if _, err := f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class,
		[]ReligionChoice{{StudentID: f.students[2], SubjectID: f.subjects["S3"]}}, v.Digest, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("optional subject accepted as Religion Education: %v", err)
	}
	if _, err := f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"]}, v.Digest, true); err == nil {
		t.Fatal("option with existing student choice removed")
	}
}

func TestPostgresReligionChoiceBlocksPublishedResultEdits(t *testing.T) {
	f, done := newFixture(t, 2472)
	defer done()
	if err := f.tx.Exec("UPDATE academic_sessions SET is_active=true WHERE id=?", f.session).Error; err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.ReligionChoices(f.ctx, f.session, f.class)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"], f.subjects["S2"]}, v.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class, []ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S1"]}}, v.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	var publication string
	if err := f.tx.Raw(`INSERT INTO result_publications(scope,scope_id,session_id,class_id,section_id,revision,kind,status,rules,rules_sha256,inputs_sha256,published_at)
VALUES ('annual',?,?,?, ?,1,'publication','published','{}','test',repeat('0',64),now()) RETURNING id`, f.session, f.session, f.class, f.sect).Row().Scan(&publication); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.Exec(`INSERT INTO result_versions(publication_id,scope,scope_id,session_id,class_id,section_id,student_id,version,status,outcome,total_marks,full_marks,gpa,inputs_sha256,snapshot,published_at)
VALUES (?,'annual',?,?,?,?,?,1,'published','passed',80,100,4,repeat('0',64),'{"format":"1","inputs":{},"outputs":{}}',now())`, publication, f.session, f.session, f.class, f.sect, f.students[0]).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveReligionChoices(f.ctx, f.admin, f.session, f.class, []ReligionChoice{{StudentID: f.students[0], SubjectID: f.subjects["S2"]}}, v.Digest, true); err == nil {
		t.Fatal("published choice changed")
	}
	if err := f.tx.Exec("DELETE FROM student_subject_choices WHERE student_id=? AND session_id=? AND role='religion'", f.students[0], f.session).Error; err == nil {
		t.Fatal("published choice deleted directly")
	}
	if _, err := f.svc.SaveReligionOptions(f.ctx, f.admin, f.session, f.class, []string{f.subjects["S1"]}, v.Digest, true); err == nil {
		t.Fatal("published option removed")
	}
}
