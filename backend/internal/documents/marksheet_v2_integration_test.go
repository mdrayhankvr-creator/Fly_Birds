package documents

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/classteacher"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

// signaturePNG is a dark stroke on transparent paper, prepared as an upload
// would be.
func signaturePNG(t *testing.T, shade uint8) ([]byte, string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 300, 120))
	for x := 20; x < 280; x++ {
		y := 60 + (x%40-20)/2
		for d := -2; d <= 2; d++ {
			img.SetNRGBA(x, y+d, color.NRGBA{shade, shade, 80, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, version, err := staff.ProcessSignature("image/png", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return out, version
}

// TestPostgresMarksheetV2 covers what marksheet v2 adds to a published
// result: parents and birth date, combined papers and the optional
// subject's own rows, positions out of the class, failed subjects, term
// attendance without off days and holidays, the class teacher's comment
// (written, reviewed, changed) and the signatures placed automatically.
func TestPostgresMarksheetV2(t *testing.T) {
	ctx, db, done := openDB(t)
	defer done()
	tx := db.Begin()
	defer tx.Rollback()
	s := newSchool(t, ctx, tx, fixtureOptions{combined: true})
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	store, err := files.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeRenderer{}
	svc := NewService(tx, s.res, store, fake, nil)
	svc.BaseURL = "https://school.example.test"
	svc.Now = func() time.Time { return time.Date(2401, 6, 1, 12, 0, 0, 0, time.UTC) }
	admin := Actor{s.admin, "admin"}

	// The student's details, the head teacher's name and the term's dates.
	exec(`UPDATE students SET father_name_en = 'Abdul Karim', father_name_bn = 'আবদুল করিম', mother_name_en = 'Rahima Begum', mother_name_bn = 'রহিমা বেগম',
 date_of_birth = '2389-03-05', student_code = 'RMS-0001' WHERE id = ?`, s.students[0])
	exec("UPDATE school_settings SET head_teacher_name_en = 'Head Master', head_teacher_name_bn = 'প্রধান শিক্ষক মহোদয়' WHERE id = 1")
	exec("UPDATE exam_terms SET starts_on = '2401-01-01', ends_on = '2401-01-31' WHERE id = ?", s.term)
	exec("UPDATE employees SET name_en = 'Nasrin Sultana', name_bn = 'নাসরিন সুলতানা' WHERE user_id = ?", s.teacher)

	// Attendance: a published two-day holiday (Wednesday and Thursday) and
	// the weekly off day (Friday) are not working days.
	exec(`INSERT INTO school_events (title_en, kind, starts_on, ends_on, audience_roles, status, created_by, updated_by)
 VALUES ('Holiday', 'holiday', '2401-01-10', '2401-01-11', ARRAY['student'], 'published', ?, ?)`, s.admin, s.admin)
	working, offFriday := 0, map[string]bool{}
	for d := time.Date(2401, 1, 1, 0, 0, 0, 0, time.UTC); d.Month() == time.January; d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Friday {
			offFriday[d.Format(time.DateOnly)] = true
			continue
		}
		if d.Day() == 10 || d.Day() == 11 {
			continue
		}
		working++
	}
	marked := 0
	for d := time.Date(2401, 1, 1, 0, 0, 0, 0, time.UTC); marked < 12; d = d.AddDate(0, 0, 1) {
		day := d.Format(time.DateOnly)
		if offFriday[day] || d.Day() == 10 || d.Day() == 11 {
			continue
		}
		status := "present"
		if marked == 3 {
			status = "late"
		}
		if marked >= 10 {
			status = "absent"
		}
		exec(`INSERT INTO student_attendance (student_id, session_id, class_id, section_id, attendance_date, status, source, recorded_by, updated_by)
 VALUES (?, ?, ?, ?, ?, ?, 'manual', ?, ?)`, s.students[0], s.session, s.classID, s.section, day, status, s.admin, s.admin)
		marked++
	}

	// The class teacher, assigned by the administrator, with a signature.
	ct := classteacher.NewService(tx)
	var empID string
	if err := tx.Raw("SELECT id FROM employees WHERE user_id = ?", s.teacher).Row().Scan(&empID); err != nil {
		t.Fatal(err)
	}
	if _, err := ct.Assign(ctx, classteacher.Actor{ID: s.head, Role: "head_teacher"}, s.section, empID); !errors.Is(err, classteacher.ErrForbidden) {
		t.Fatalf("the head teacher assigned a class teacher: %v", err)
	}
	if got, err := ct.Assign(ctx, classteacher.Actor{ID: s.admin, Role: "admin"}, s.section, empID); err != nil || got.EmployeeID == nil || *got.EmployeeID != empID {
		t.Fatalf("assign: %+v %v", got, err)
	}
	sig, sigVersion := signaturePNG(t, 20)
	exec("INSERT INTO employee_signatures (employee_id, content, version) VALUES (?, ?, ?)", empID, sig, sigVersion)

	// Comments: the class teacher writes; another teacher may not; the head
	// teacher reviews.
	presets, err := ct.Presets(ctx, false)
	if err != nil || len(presets) < 3 {
		t.Fatalf("presets: %d %v", len(presets), err)
	}
	cta, outa, heada := classteacher.Actor{ID: s.teacher, Role: "teacher"}, classteacher.Actor{ID: s.outsider, Role: "teacher"}, classteacher.Actor{ID: s.head, Role: "head_teacher"}
	if _, err := ct.Save(ctx, outa, classteacher.CommentInput{Scope: "term", ScopeID: s.term, StudentID: s.students[0], PresetID: &presets[0].ID}); !errors.Is(err, classteacher.ErrForbidden) {
		t.Fatalf("another teacher commented: %v", err)
	}
	if _, err := ct.Save(ctx, cta, classteacher.CommentInput{Scope: "term", ScopeID: s.term, StudentID: s.students[0], PresetID: &presets[0].ID, CustomText: "both"}); !errors.Is(err, classteacher.ErrInvalid) {
		t.Fatalf("a preset and own words together: %v", err)
	}
	if c, err := ct.Save(ctx, cta, classteacher.CommentInput{Scope: "term", ScopeID: s.term, StudentID: s.students[0], PresetID: &presets[0].ID}); err != nil || c.Status != "written" {
		t.Fatalf("preset comment: %+v %v", c, err)
	}
	long := "Rafi has worked steadily all term and helps his classmates in group work; he should practise written answers in Science every evening and read more widely to strengthen his vocabulary before the annual examination."
	if _, err := ct.Save(ctx, cta, classteacher.CommentInput{Scope: "term", ScopeID: s.term, StudentID: s.students[1], CustomText: long}); err != nil {
		t.Fatalf("own comment: %v", err)
	}
	sheet, err := ct.Sheet(ctx, heada, "term", s.term, s.section)
	if err != nil || !sheet.CanReview || len(sheet.Students) != 3 {
		t.Fatalf("head teacher's sheet: %+v %v", sheet, err)
	}
	if _, err := ct.Sheet(ctx, outa, "term", s.term, s.section); !errors.Is(err, classteacher.ErrForbidden) {
		t.Fatalf("another teacher read the comments: %v", err)
	}
	if _, err := ct.Review(ctx, cta, classteacher.ReviewInput{Scope: "term", ScopeID: s.term, SectionID: s.section}); !errors.Is(err, classteacher.ErrForbidden) {
		t.Fatalf("the class teacher reviewed: %v", err)
	}
	if n, err := ct.Review(ctx, heada, classteacher.ReviewInput{Scope: "term", ScopeID: s.term, SectionID: s.section}); err != nil || n != 2 {
		t.Fatalf("review: %d %v", n, err)
	}
	mine, err := ct.MySections(ctx, cta, "")
	if err != nil || len(mine) != 1 || mine[0].SectionID != s.section || len(mine[0].Scopes) < 2 {
		t.Fatalf("my sections: %+v %v", mine, err)
	}

	s.publish(t, ctx, "")
	var versions int64
	tx.Raw("SELECT count(*) FROM result_versions WHERE scope = 'term' AND scope_id = ?", s.term).Scan(&versions)

	// The marksheet of student 0 (with the fourth subject and the combined
	// papers) and of student 1 (who fails S1).
	request := func(i int, l string) (Document, sheetStudent) {
		t.Helper()
		doc, err := svc.Request(ctx, admin, Request{Kind: "marksheet", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, StudentID: s.students[i], Language: l})
		if err != nil || doc.Status != "done" {
			t.Fatalf("marksheet %d %s: %+v %v", i, l, doc, err)
		}
		data := fake.last().Data.(sheetData)
		return doc, data.Students[0]
	}
	doc0, st := request(0, "en")
	data := fake.last().Data.(sheetData)
	if data.Heading != "Academic Transcript" || data.HeadingAlt != "ফলাফল পত্র" || data.SessionName != "2401" {
		t.Fatalf("headings: %q %q %q", data.Heading, data.HeadingAlt, data.SessionName)
	}
	if st.Father != "Abdul Karim" || st.Mother != "Rahima Begum" || st.DOB != "5 March 2389" {
		t.Fatalf("details: %q %q %q", st.Father, st.Mother, st.DOB)
	}
	// The student ID is the school's student ID (as on the ID card and the
	// login slip), not the admission number.
	if st.StudentID != "RMS-0001" {
		t.Fatalf("student ID: %q", st.StudentID)
	}
	// 9 of the 12 marked days were present, 1 late (counts as present), 2
	// absent: 10 days present.
	if st.Held != fmt.Sprintf("%d", working) || st.PresentDays != "10" || st.AbsentDays != fmt.Sprintf("%d", working-10) {
		t.Fatalf("attendance: held %q present %q absent %q (working %d)", st.Held, st.PresentDays, st.AbsentDays, working)
	}
	if st.Comment != presets[0].TextEN || st.ClassPos != "1st of 3" || st.SectionPos != "1st of 3" || st.FailedCount != "0" {
		t.Fatalf("summary: comment %q pos %q/%q failed %q", st.Comment, st.ClassPos, st.SectionPos, st.FailedCount)
	}
	if len(st.Signatures) != 3 || st.Signatures[0].Name != "Nasrin Sultana" || st.Signatures[0].Image != "ctsig-"+sigVersion+".png" ||
		st.Signatures[1].Name != "Head Master" || !st.Signatures[2].Date || st.Signatures[2].Image != "" {
		t.Fatalf("signatures: %+v", st.Signatures)
	}
	if _, ok := fake.last().Assets["ctsig-"+sigVersion+".png"]; !ok {
		t.Fatalf("the class teacher's signature is not given to the renderer")
	}
	var combined *sheetRow
	for i := range st.Rows {
		if len(st.Rows[i].Papers) > 1 {
			combined = &st.Rows[i]
		}
	}
	if combined == nil || len(combined.Papers) != 2 || !strings.HasPrefix(combined.Papers[0].Subject, "B1 ") || !strings.HasPrefix(combined.Papers[1].Subject, "B2 ") ||
		combined.Papers[0].Parts[0].Obt != "70" || combined.Papers[1].Parts[0].Obt != "62" || combined.Parts[0].Obt != "132" {
		t.Fatalf("combined papers: %+v", combined)
	}
	if len(st.Optional) != 1 || !st.Optional[0].Optional || len(st.Rows) != 3 {
		t.Fatalf("optional rows: %d required, %+v", len(st.Rows), st.Optional)
	}

	// Bangla: Bangla names, digits and the comment preset's Bangla text.
	_, bn := request(0, "bn")
	if bn.Father != "আবদুল করিম" || bn.Comment != presets[0].TextBN || bn.ClassPos != "৩ জনের মধ্যে ১ম" || bn.PresentDays != "১০" || bn.Signatures[0].Name != "নাসরিন সুলতানা" {
		t.Fatalf("bangla: %q %q %q %q", bn.Father, bn.Comment, bn.ClassPos, bn.PresentDays)
	}

	_, failing := request(1, "en")
	if failing.FailedCount != "1" || failing.Comment != long || failing.Result != "Failed" || failing.Father != "—" || failing.Held != "—" {
		t.Fatalf("student 1: failed %q comment %q father %q held %q", failing.FailedCount, failing.Comment, failing.Father, failing.Held)
	}

	// The same request again is the cached document, not rendered again.
	calls := fake.count()
	if again, err := svc.Request(ctx, admin, Request{Kind: "marksheet", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, StudentID: s.students[0], Language: "en"}); err != nil || again.ID != doc0.ID || fake.count() != calls {
		t.Fatalf("cache: %s vs %s, renders %d, %v", again.ID, doc0.ID, fake.count()-calls, err)
	}

	// A changed comment makes a new document; the published result stays.
	if _, err := ct.Save(ctx, cta, classteacher.CommentInput{Scope: "term", ScopeID: s.term, StudentID: s.students[0], CustomText: "A new comment."}); err != nil {
		t.Fatal(err)
	}
	doc1, st1 := request(0, "en")
	if doc1.ID == doc0.ID || st1.Comment != "A new comment." {
		t.Fatalf("a changed comment kept the document: %s %q", doc1.ID, st1.Comment)
	}
	var after int64
	tx.Raw("SELECT count(*) FROM result_versions WHERE scope = 'term' AND scope_id = ?", s.term).Scan(&after)
	if after != versions {
		t.Fatalf("a comment changed the published results: %d → %d versions", versions, after)
	}
	sheet, _ = ct.Sheet(ctx, heada, "term", s.term, s.section)
	for _, c := range sheet.Students {
		if c.StudentID == s.students[0] && c.Status != "written" {
			t.Fatalf("a changed comment stays reviewed: %+v", c)
		}
	}

	// A new signature makes a new document too.
	sig2, v2 := signaturePNG(t, 60)
	exec("UPDATE employee_signatures SET content = ?, version = ? WHERE employee_id = ?", sig2, v2, empID)
	if doc2, st2 := request(0, "en"); doc2.ID == doc1.ID || st2.Signatures[0].Image != "ctsig-"+v2+".png" {
		t.Fatalf("a new signature kept the document: %s %+v", doc2.ID, st2.Signatures[0])
	}

	// Without a class teacher the slot stays blank; the comment is still
	// printed.
	if _, err := ct.Assign(ctx, classteacher.Actor{ID: s.admin, Role: "admin"}, s.section, ""); err != nil {
		t.Fatal(err)
	}
	if _, st3 := request(0, "en"); st3.Signatures[0].Name != "" || st3.Signatures[0].Image != "" || st3.Comment != "A new comment." {
		t.Fatalf("no class teacher: %+v %q", st3.Signatures[0], st3.Comment)
	}

	// A school that starts recording attendance later in the term: the days
	// before the section's first record are not counted as absent.
	exec("DELETE FROM student_attendance WHERE section_id = ? AND attendance_date < '2401-01-08'", s.section)
	var first string
	var present int
	tx.Raw("SELECT to_char(min(attendance_date), 'YYYY-MM-DD') FROM student_attendance WHERE section_id = ?", s.section).Row().Scan(&first)
	tx.Raw("SELECT count(*) FROM student_attendance WHERE student_id = ? AND status IN ('present', 'late')", s.students[0]).Row().Scan(&present)
	later := 0
	for d, _ := time.Parse(time.DateOnly, first); d.Month() == time.January; d = d.AddDate(0, 0, 1) {
		if !offFriday[d.Format(time.DateOnly)] && d.Day() != 10 && d.Day() != 11 {
			later++
		}
	}
	if _, st4 := request(0, "en"); later >= working || st4.Held != fmt.Sprintf("%d", later) || st4.PresentDays != fmt.Sprintf("%d", present) {
		t.Fatalf("attendance from the first record (%s): held %q present %q, want %d of %d", first, st4.Held, st4.PresentDays, present, later)
	}
}
