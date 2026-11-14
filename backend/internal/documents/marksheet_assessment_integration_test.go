package documents

import (
	"errors"
	"io"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
)

// A marksheet of an exam's own published result ('assessment' scope) must be
// stored and rendered like a term one. result_documents (1800) only allowed
// 'term' and 'annual', so until 1812 every such request died on INSERT with a
// check violation, which the handler turned into a 500: on the showcase both
// "Download PDF (English)" and "(বাংলা)" on a published class-test result
// said the document could not be prepared.
func TestPostgresAssessmentMarksheet(t *testing.T) {
	ctx, db, done := openDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	s := newSchool(t, ctx, tx)
	store, err := files.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeRenderer{}
	svc := NewService(tx, s.res, store, fake, nil)
	svc.BaseURL = "https://school.example.test"
	admin := Actor{s.admin, "admin"}
	studentActor, otherStudent := Actor{s.studentUser, "student"}, Actor{s.otherUser, "student"}

	// EX1's own result is published with the exam; it becomes visible to
	// students once the term is published.
	s.publish(t, ctx, "term published")
	if visible, err := s.res.StudentsMaySee(ctx, s.exam["EX1"]); err != nil || !visible {
		t.Fatalf("the exam should be visible to students: %v %v", visible, err)
	}

	// 1. Staff print one student's exam marksheet.
	req := Request{Kind: "marksheet", Scope: exams.ScopeAssessment, ScopeID: s.exam["EX1"], ClassID: s.classID, StudentID: s.students[0], Language: "en"}
	doc, err := svc.Request(ctx, admin, req)
	if err != nil {
		t.Fatalf("staff marksheet of an exam's result: %v", err)
	}
	if doc.Status != "done" || doc.Scope != exams.ScopeAssessment || doc.Error != "" {
		t.Fatalf("staff marksheet: %+v", doc)
	}
	if fake.count() == 0 {
		t.Fatal("nothing was rendered")
	}

	// 2. The student's own download, in both languages, is prepared and sent.
	var version string
	if err := tx.Raw("SELECT id FROM result_versions WHERE scope = 'assessment' AND scope_id = ? AND student_id = ? AND status = 'published'",
		s.exam["EX1"], s.students[0]).Row().Scan(&version); err != nil {
		t.Fatalf("the student's published exam version: %v", err)
	}
	for _, language := range []string{"en", "bn"} {
		mine, err := svc.StudentMarksheet(ctx, studentActor, version, language)
		if err != nil {
			t.Fatalf("the student's own %s marksheet: %v", language, err)
		}
		if mine.Status != "done" || mine.Language != language || mine.Scope != exams.ScopeAssessment {
			t.Fatalf("the student's own %s marksheet: %+v", language, mine)
		}
		_, rc, err := svc.Open(ctx, studentActor, mine.ID)
		if err != nil {
			t.Fatalf("open the %s marksheet: %v", language, err)
		}
		pdf, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || len(pdf) == 0 {
			t.Fatalf("read the %s marksheet: %d bytes %v", language, len(pdf), err)
		}
	}

	// 3. The row is stored under the exam's own scope, and asking again gives
	// the cached document rather than a second one.
	var stored int64
	tx.Raw("SELECT count(*) FROM result_documents WHERE scope = 'assessment' AND scope_id = ? AND student_id = ?",
		s.exam["EX1"], s.students[0]).Scan(&stored)
	if stored != 2 { // en and bn
		t.Fatalf("stored exam marksheets: %d", stored)
	}
	before := fake.count()
	again, err := svc.Request(ctx, admin, req)
	if err != nil || again.ID != doc.ID || fake.count() != before {
		t.Fatalf("the cached exam marksheet: %+v %v (renders %d → %d)", again, err, before, fake.count())
	}

	// 4. The scope stays a student's own: another student's exam result is not
	// found, and class documents remain term- and annual-only.
	if _, err := svc.StudentMarksheet(ctx, otherStudent, version, "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another student's exam marksheet: %v", err)
	}
	for _, kind := range []string{"tabulation", "merit_list", "failed_list", "summary"} {
		class := Request{Kind: kind, Scope: exams.ScopeAssessment, ScopeID: s.exam["EX1"], ClassID: s.classID, Language: "en"}
		if _, err := svc.Request(ctx, admin, class); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s of an exam's result: %v", kind, err)
		}
	}
}
