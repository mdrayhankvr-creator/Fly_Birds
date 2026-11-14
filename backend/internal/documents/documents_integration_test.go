package documents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
)

// fakeRenderer records what each document was given and returns a tiny PDF
// stand-in with the right page count.
type fakeRenderer struct {
	mu    sync.Mutex
	calls []Render
}

func (f *fakeRenderer) Render(_ context.Context, r Render) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	pages := 1
	switch d := r.Data.(type) {
	case sheetData:
		pages = len(d.Students)
	case map[string]any:
		raw, _ := json.Marshal(d["parts"])
		var parts []struct{ Pages int }
		_ = json.Unmarshal(raw, &parts)
		pages = 0
		for _, p := range parts {
			pages += p.Pages
		}
	}
	raw, _ := json.Marshal(r.Data)
	// DOCS_DUMP_DIR keeps each render's inputs, to try the templates with the
	// real renderer by hand.
	if dir := os.Getenv("DOCS_DUMP_DIR"); dir != "" {
		at := filepath.Join(dir, fmt.Sprintf("%02d-%s", len(f.calls), strings.TrimSuffix(r.Template, ".typ")))
		_ = os.MkdirAll(at, 0o755)
		_ = os.WriteFile(filepath.Join(at, "data.json"), raw, 0o644)
		for name, b := range r.Assets {
			_ = os.WriteFile(filepath.Join(at, name), b, 0o644)
		}
	}
	return []byte(fmt.Sprintf("%%PDF-1.7\n<< /Type /Pages /Count %d >>\n%s\n%%%%EOF", pages, raw)), nil
}

func (f *fakeRenderer) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) }
func (f *fakeRenderer) last() Render {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// school is one class with a published-ready term:
//   - S1: CQ 60 + MCQ 40 in the term exam; S2: an ordinary paper; S3: the
//     optional fourth subject, taken by student 0;
//   - a class test worth 20 of every subject's 100;
//   - three students in section A; student 1 fails S1 on CQ;
//   - teacher (assigned to the class), outsider (a teacher of nothing here),
//     a student account for student 0 and another for student 1.
type school struct {
	tx                                     *gorm.DB
	ex                                     *exams.Service
	res                                    *exams.Results
	bp                                     *blueprints.Service
	stamp, session, classID, section, term string
	admin, head, teacher, outsider         string
	accountant, studentUser, otherUser     string
	students                               []string
	subjects                               map[string]string
	exam                                   map[string]string // EX1, CT1
	sheet                                  map[string]string // EX1S1 …
}

// fixtureOptions add to the school: combined adds a required subject taken
// as two papers, B1 and B2, graded together (combined key BAN).
type fixtureOptions struct{ combined bool }

func newSchool(t *testing.T, ctx context.Context, tx *gorm.DB, opts ...fixtureOptions) *school {
	t.Helper()
	var opt fixtureOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	insert := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(q, args...).Row().Scan(&id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return id
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ex := exams.NewService(exams.NewRepository(tx))
	s := &school{tx: tx, ex: ex, res: ex.Results(), bp: ex.Blueprints(), stamp: fmt.Sprint(time.Now().UnixNano()), subjects: map[string]string{}, exam: map[string]string{}, sheet: map[string]string{}}
	user := func(role, name string) string {
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", "e7-"+name+"-"+s.stamp+"@example.test", "unused", role, "E7 "+name)
	}
	s.admin, s.head, s.teacher, s.outsider = user("admin", "admin"), user("head_teacher", "head"), user("teacher", "teacher"), user("teacher", "outsider")
	s.accountant, s.studentUser, s.otherUser = user("accountant", "accounts"), user("student", "student"), user("student", "other")
	s.session = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2401','2401-01-01','2401-12-31') RETURNING id")
	s.classID = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class E7 "+s.stamp)
	s.section = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", s.session, s.classID)
	codes := []string{"S1", "S2", "S3"}
	if opt.combined {
		codes = append(codes, "B1", "B2")
	}
	for i, code := range codes {
		s.subjects[code] = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", code+" "+s.stamp, "বিজ্ঞান "+code, "E7"+code+s.stamp[len(s.stamp)-8:])
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", s.classID, s.subjects[code], i != 2, i == 2)
	}
	emp := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','T','Teacher','2020-01-01') RETURNING id", "E7A"+s.stamp, s.teacher)
	insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','O','Teacher','2020-01-01') RETURNING id", "E7B"+s.stamp, s.outsider)
	exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", emp, s.session, s.classID, s.subjects["S1"])
	for i := 0; i < 3; i++ {
		s.students = append(s.students, insert("INSERT INTO students(admission_no,name_en,name_bn,roll_no,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,?,?,'active') RETURNING id",
			fmt.Sprintf("E7-%d-%s", i, s.stamp), fmt.Sprintf("Student %d", i), fmt.Sprintf("শিক্ষার্থী %d", i), i+1, s.session, s.classID, s.section))
	}
	exec("UPDATE students SET user_id = ? WHERE id = ?", s.studentUser, s.students[0])
	exec("UPDATE students SET user_id = ? WHERE id = ?", s.otherUser, s.students[1])

	term, err := s.bp.SaveTerm(ctx, s.admin, "", blueprints.TermInput{SessionID: s.session, Name: "Second Term", Kind: "term"})
	if err != nil {
		t.Fatal(err)
	}
	s.term = term.ID
	draft, err := s.bp.CreateBlueprint(ctx, s.admin, s.session, s.classID, "")
	if err != nil {
		t.Fatal(err)
	}
	d := draft.Definition
	d.Assessments = append([]blueprints.Assessment{{Key: "CT1", Name: "CT 1", TermID: term.ID, Kind: "class_test", Visibility: "hidden"}}, d.Assessments...)
	for i := range d.Assessments {
		if d.Assessments[i].Key == "EX1" {
			d.Assessments[i].Visibility = "after_term_publication"
		}
	}
	for i := range d.Subjects {
		sub := &d.Subjects[i]
		sub.Sheets = append(sub.Sheets, blueprints.Sheet{Assessment: "CT1", Pass: 7, Components: []blueprints.Component{{Key: "TOTAL", Full: 20}}})
		sub.Terms[0].Contributions = []blueprints.Contribution{{From: []string{"CT1"}, Combine: "average", Worth: 20}, {From: []string{"EX1"}, Combine: "single", Worth: 80}}
		if sub.SubjectID == s.subjects["S1"] {
			pq, pm := 20.0, 13.0
			sub.Sheets[0].Components = []blueprints.Component{{Key: "CQ", Full: 60, Pass: &pq}, {Key: "MCQ", Full: 40, Pass: &pm}}
		}
		if opt.combined && (sub.SubjectID == s.subjects["B1"] || sub.SubjectID == s.subjects["B2"]) {
			sub.CombinedKey = "BAN"
		}
	}
	if v, err := s.bp.SaveDraft(ctx, s.admin, draft.ID, d); err != nil || len(v.Issues) != 0 {
		t.Fatalf("draft: %+v %v", v.Issues, err)
	}
	if _, err := s.bp.Activate(ctx, s.admin, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.bp.SaveSetup(ctx, s.admin, term.ID, blueprints.Setup{ClassIDs: []string{s.classID}, Due: map[string]string{"CT1": "2401-02-01", "EX1": "2401-03-01"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.bp.CreateTerm(ctx, s.admin, term.ID); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ ExamID, Key, SheetID, SubjectID string }{}
	if err := tx.Raw(`SELECT e.id AS exam_id, e.assessment_key AS key, es.id AS sheet_id, es.subject_id FROM exams e JOIN exam_subjects es ON es.exam_id = e.id WHERE e.term_id = ?`, term.ID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	code := map[string]string{}
	for k, id := range s.subjects {
		code[id] = k
	}
	for _, r := range rows {
		s.exam[r.Key] = r.ExamID
		s.sheet[r.Key+code[r.SubjectID]] = r.SheetID
	}
	if _, err := s.bp.SaveChoices(ctx, s.admin, s.session, s.classID, []blueprints.Choice{{StudentID: s.students[0], SubjectID: s.subjects["S3"]}}); err != nil {
		t.Fatal(err)
	}
	present := func(key, st string, m float64) exams.MarkEntry {
		return exams.MarkEntry{ExamSubjectID: s.sheet[key], StudentID: st, Status: exams.StatusPresent, Marks: &m}
	}
	comps := func(st string, cq, mcq float64) exams.MarkEntry {
		return exams.MarkEntry{ExamSubjectID: s.sheet["EX1S1"], StudentID: st, Status: exams.StatusPresent, Components: map[string]float64{"CQ": cq, "MCQ": mcq}}
	}
	s.save(t, ctx, "EX1", comps(s.students[0], 50, 30), comps(s.students[1], 15, 40), comps(s.students[2], 40, 20),
		present("EX1S2", s.students[0], 70), present("EX1S2", s.students[1], 60), present("EX1S2", s.students[2], 40), present("EX1S3", s.students[0], 90))
	if opt.combined {
		s.save(t, ctx, "EX1", present("EX1B1", s.students[0], 70), present("EX1B2", s.students[0], 62), present("EX1B1", s.students[1], 50), present("EX1B2", s.students[1], 45),
			present("EX1B1", s.students[2], 40), present("EX1B2", s.students[2], 44))
	}
	ct := []exams.MarkEntry{}
	for _, st := range s.students {
		ct = append(ct, present("CT1S1", st, 16), present("CT1S2", st, 16))
		if opt.combined {
			ct = append(ct, present("CT1B1", st, 15), present("CT1B2", st, 14))
		}
	}
	ct = append(ct, present("CT1S3", s.students[0], 18))
	s.save(t, ctx, "CT1", ct...)
	for _, k := range []string{"EX1", "CT1"} {
		if _, err := s.ex.SetState(ctx, s.admin, s.exam[k], "published"); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func (s *school) save(t *testing.T, ctx context.Context, key string, entries ...exams.MarkEntry) {
	t.Helper()
	res, err := s.ex.SaveMarkEntries(ctx, s.admin, "admin", s.exam[key], entries)
	if err != nil || len(res.Errors) != 0 {
		t.Fatalf("save %s: %+v %v", key, res.Errors, err)
	}
}

func (s *school) ref() exams.ScopeRef {
	return exams.ScopeRef{Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID}
}

// publish reviews, approves and publishes the term as it stands.
func (s *school) publish(t *testing.T, ctx context.Context, reason string) {
	t.Helper()
	review, err := s.res.Review(ctx, s.ref())
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.res.Approve(ctx, s.head, exams.ApproveInput{ScopeRef: s.ref(), InputsSHA256: review.InputsSHA256, Reason: reason})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := s.res.Publish(ctx, s.admin, exams.PublishInput{ScopeRef: s.ref(), PublicationID: ok.Approval.ID}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func openDB(t *testing.T) (context.Context, *gorm.DB, func()) {
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

func TestPostgresResultDocuments(t *testing.T) {
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
	admin, head := Actor{s.admin, "admin"}, Actor{s.head, "head_teacher"}
	teacher, outsider, accountant := Actor{s.teacher, "teacher"}, Actor{s.outsider, "teacher"}, Actor{s.accountant, "accountant"}
	studentActor, otherStudent := Actor{s.studentUser, "student"}, Actor{s.otherUser, "student"}
	bulk := Request{Kind: "marksheets", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, Language: "en"}

	// 1. Published only: nothing before publication; a provisional preview
	// for the head teacher and the administrator only, watermarked.
	if _, err := svc.Request(ctx, admin, bulk); !errors.Is(err, ErrNotPublished) {
		t.Fatalf("before publication: %v", err)
	}
	prov := bulk
	prov.Provisional = true
	if _, err := svc.Request(ctx, teacher, prov); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a teacher's provisional preview: %v", err)
	}
	pd, err := svc.Request(ctx, head, prov)
	if err != nil || pd.Status != "done" || !pd.Provisional {
		t.Fatalf("provisional preview: %+v %v", pd, err)
	}
	pdata := fake.last().Data.(sheetData)
	if !pdata.Provisional || pdata.Labels["provisional"] != "PROVISIONAL — NOT PUBLISHED" || pdata.Students[0].QR != "" || !strings.Contains(pdata.Students[0].Footer, "not published") {
		t.Fatalf("provisional data: %+v", pdata.Students[0])
	}
	var tokens int64
	tx.Raw("SELECT count(*) FROM result_verifications").Scan(&tokens)
	if tokens != 0 {
		t.Fatal("a provisional preview must not create verification tokens")
	}

	s.publish(t, ctx, "")

	// 2. The class's marksheets, from the published versions.
	calls := fake.count()
	doc, err := svc.Request(ctx, admin, bulk)
	if err != nil || doc.Status != "done" || doc.Pages == nil || *doc.Pages != 3 || doc.ProgressDone != 3 {
		t.Fatalf("marksheets: %+v %v", doc, err)
	}
	data := fake.last().Data.(sheetData)
	if strings.Join(data.Columns, ",") != "Exam,CQ,MCQ" || !data.ShowCT || len(data.Students) != 3 || len(data.Grading) == 0 {
		t.Fatalf("layout: %v ct=%v students=%d", data.Columns, data.ShowCT, len(data.Students))
	}
	first := data.Students[0] // roll 1: student 0, with the fourth subject
	var s1, s3 *sheetRow
	for i := range first.Rows {
		if strings.HasPrefix(first.Rows[i].Subject, "S1 ") {
			s1 = &first.Rows[i]
		}
	}
	// The optional subject has its own rows after the required ones.
	if len(first.Optional) == 1 && strings.HasPrefix(first.Optional[0].Subject, "S3 ") {
		s3 = &first.Optional[0]
	}
	if s1 == nil || s3 == nil || len(first.Rows) != 2 || s1.Parts[0].Obt != "—" || s1.Parts[1] != (sheetCell{Obt: "50", Full: "60"}) || s1.Parts[2] != (sheetCell{Obt: "30", Full: "40"}) || s1.CT != "16" || !s3.Optional || s3.Highest != s3.Total {
		t.Fatalf("rows: %+v optional %+v", first.Rows, first.Optional)
	}
	if first.Name != "Student 0" || first.NameAlt != "শিক্ষার্থী 0" || first.Result != "Passed" || first.QR == "" || first.GPA == "—" || first.GPAWithout == "—" || !strings.Contains(first.Footer, "Document code") {
		t.Fatalf("student 0: %+v", first)
	}
	failing := data.Students[1]
	if failing.Result != "Failed" || !failing.Failed || failing.GPA != "0.00" {
		t.Fatalf("student 1: %+v", failing)
	}
	// Highest mark in the class per subject: student 0's S1 (50+30 → 80 × 0.8 + 16).
	if s1.Highest != s1.Total {
		t.Fatalf("highest S1: %s vs %s", s1.Highest, s1.Total)
	}
	if fake.count() != calls+1 {
		t.Fatalf("renders: %d", fake.count()-calls)
	}

	// 3. Cached: the same request gives the same document without rendering.
	again, err := svc.Request(ctx, head, bulk)
	if err != nil || again.ID != doc.ID || fake.count() != calls+1 {
		t.Fatalf("cache: %s vs %s, renders %d", again.ID, doc.ID, fake.count()-calls)
	}

	// 4. Who may have what (the server decides).
	if _, err := svc.Request(ctx, teacher, Request{Kind: "tabulation", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, Language: "bn"}); err != nil {
		t.Fatalf("the class's teacher: %v", err)
	}
	if tab := fake.last().Data.(classData); tab.Kind != "tabulation" || len(tab.Tabulation.Rows) != 3 || tab.Labels["tabulation"] != "টেবুলেশন শিট" || tab.Tabulation.Rows[0].Roll != "১" || tab.Tabulation.Rows[0].Letter == "" {
		t.Fatalf("tabulation: %+v", tab.Tabulation.Rows)
	}
	// The strip under the sheet, in Bangla: how many appeared, passed and
	// failed, how many reached the top grade, and the pass rate. All three
	// of this class's students sat the exam; a withheld or absent one would
	// be on the sheet without being counted as having appeared.
	totals := fake.last().Data.(classData).Tabulation.Totals
	if len(totals) != 5 {
		t.Fatalf("the tabulation strip: %+v", totals)
	}
	got := map[string]string{}
	for _, tile := range totals {
		got[tile.Label] = tile.Value
	}
	if got["অংশগ্রহণকারী"] != "৩" || got["উত্তীর্ণ"] != "২" || got["অকৃতকার্য"] != "১" || got["পাসের হার"] != "৬৬.৭%" || got["A+"] != "১" {
		t.Fatalf("the tabulation strip: %+v", got)
	}
	if !totals[len(totals)-1].Strong {
		t.Fatalf("the pass rate should stand out: %+v", totals[len(totals)-1])
	}
	for name, a := range map[string]Actor{"outsider": outsider, "accountant": accountant, "student": studentActor} {
		if _, err := svc.Request(ctx, a, bulk); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s asked for the class's marksheets: %v", name, err)
		}
		if _, err := svc.Get(ctx, a, doc.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s read the class's document: %v", name, err)
		}
	}
	// The list carries every published result the teacher may print: the
	// term's, and each published exam of it (transcript v3).
	list, err := svc.Classes(ctx, teacher)
	if err != nil {
		t.Fatalf("teacher's classes: %v", err)
	}
	scopes := map[string]int{}
	for _, c := range list {
		scopes[c.Scope]++
		if c.Students != 3 || len(c.Sections) != 1 {
			t.Fatalf("teacher's classes: %+v", c)
		}
	}
	if scopes[exams.ScopeTerm] != 1 || scopes[exams.ScopeAssessment] == 0 {
		t.Fatalf("teacher's classes by scope: %+v (%d entries)", scopes, len(list))
	}
	if list, err := svc.Classes(ctx, outsider); err != nil || len(list) != 0 {
		t.Fatalf("outsider's classes: %+v %v", list, err)
	}
	if _, err := svc.Classes(ctx, studentActor); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a student listed classes: %v", err)
	}
	for _, kind := range []string{"merit_list", "failed_list", "summary"} {
		if d, err := svc.Request(ctx, head, Request{Kind: kind, Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, Language: "en"}); err != nil || d.Status != "done" {
			t.Fatalf("%s: %+v %v", kind, d, err)
		}
		cd := fake.last().Data.(classData)
		switch kind {
		case "merit_list":
			if len(cd.Merit) != 2 || cd.Merit[0].Position != "1" {
				t.Fatalf("merit: %+v", cd.Merit)
			}
		case "failed_list":
			if len(cd.Failed) != 1 || !strings.Contains(cd.Failed[0].Subjects, "S1 ") {
				t.Fatalf("failed: %+v", cd.Failed)
			}
		case "summary":
			if cd.Summary.Tiles[1].Value != "2" || cd.Summary.Tiles[2].Value != "1" || cd.Summary.Tiles[4].Value != "66.7%" {
				t.Fatalf("summary: %+v", cd.Summary.Tiles)
			}
		}
	}

	// 5. A student downloads only their own current marksheet.
	var v0, v1 string
	tx.Raw("SELECT id FROM result_versions WHERE scope = 'term' AND scope_id = ? AND student_id = ? AND status = 'published'", s.term, s.students[0]).Row().Scan(&v0)
	tx.Raw("SELECT id FROM result_versions WHERE scope = 'term' AND scope_id = ? AND student_id = ? AND status = 'published'", s.term, s.students[1]).Row().Scan(&v1)
	mine, err := svc.StudentMarksheet(ctx, studentActor, v0, "bn")
	if err != nil || mine.Status != "done" || mine.StudentID == nil || *mine.StudentID != s.students[0] {
		t.Fatalf("own marksheet: %+v %v", mine, err)
	}
	if _, rc, err := svc.Open(ctx, studentActor, mine.ID); err != nil {
		t.Fatalf("open own marksheet: %v", err)
	} else {
		rc.Close()
	}
	if _, err := svc.StudentMarksheet(ctx, studentActor, v1, "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another student's marksheet: %v", err)
	}
	if _, err := svc.Get(ctx, otherStudent, mine.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another student opened it: %v", err)
	}
	if _, err := svc.Get(ctx, studentActor, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a student opened the class's marksheets: %v", err)
	}
	if _, err := svc.StudentMarksheet(ctx, admin, v0, "en"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("the student route for staff: %v", err)
	}

	// 6. Verification: the QR's token shows the result is current, with no
	// personal data.
	var token string
	if err := tx.Raw("SELECT token FROM result_verifications WHERE version_id = ?", v0).Row().Scan(&token); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Verify(ctx, token)
	if err != nil || v.State != "current" || v.Scope != "term" || v.Title != "Second Term" || v.Version != 1 || v.Code != documentCode(token) {
		t.Fatalf("verify: %+v %v", v, err)
	}
	raw, _ := json.Marshal(v)
	for _, personal := range []string{"Student 0", "শিক্ষার্থী", s.students[0], "E7-0-", "gpa", "marks"} {
		if strings.Contains(string(raw), personal) {
			t.Fatalf("verification shows %q: %s", personal, raw)
		}
	}
	if _, err := svc.Verify(ctx, "AAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err := svc.Verify(ctx, "not-a-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed token: %v", err)
	}

	// 7. A correction publishes new versions: the next request makes a new
	// document, the old file is deleted, and the old QR says "superseded".
	corr, err := s.res.OpenCorrection(ctx, s.admin, s.exam["EX1"], s.students[1], "CQ script re-marked")
	if err != nil {
		t.Fatal(err)
	}
	fixed := exams.MarkEntry{ExamSubjectID: s.sheet["EX1S1"], StudentID: s.students[1], Status: exams.StatusPresent, Components: map[string]float64{"CQ": 30, "MCQ": 40}, RowVersion: corr.Cells[0].RowVersion}
	s.save(t, ctx, "EX1", fixed)
	aRef := exams.ScopeRef{Scope: exams.ScopeAssessment, ScopeID: s.exam["EX1"]}
	rev, err := s.res.Review(ctx, aRef)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.res.Approve(ctx, s.head, exams.ApproveInput{ScopeRef: aRef, InputsSHA256: rev.InputsSHA256})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.res.Publish(ctx, s.admin, exams.PublishInput{ScopeRef: aRef, PublicationID: ok.Approval.ID}); err != nil {
		t.Fatal(err)
	}
	// Until the term revision is published, the cached document stands.
	if same, err := svc.Request(ctx, admin, bulk); err != nil || same.ID != doc.ID {
		t.Fatalf("before the term revision: %+v %v", same.ID, err)
	}
	s.publish(t, ctx, "S1 CQ re-marked")
	fresh, err := svc.Request(ctx, admin, bulk)
	if err != nil || fresh.ID == doc.ID || fresh.Status != "done" {
		t.Fatalf("after the correction: %+v %v", fresh, err)
	}
	if after := fake.last().Data.(sheetData); after.Students[1].Result != "Passed" {
		t.Fatalf("the corrected student: %+v", after.Students[1])
	}
	if _, err := svc.Get(ctx, admin, doc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the old document should be pruned: %v", err)
	}
	if _, err := store.Open(ctx, *doc.StorageKey); !errors.Is(err, files.ErrNotFound) {
		t.Fatalf("the old file should be deleted: %v", err)
	}
	var old1 string
	tx.Raw("SELECT token FROM result_verifications WHERE version_id = ?", v1).Row().Scan(&old1)
	if ov, err := svc.Verify(ctx, old1); err != nil || ov.State != "superseded" {
		t.Fatalf("the corrected student's old QR: %+v %v", ov, err)
	}
	if _, err := svc.StudentMarksheet(ctx, otherStudent, v1, "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a superseded version's marksheet: %v", err)
	}

	// 8. School details from school settings: the EIIN and the address in the
	// document's language; changing them makes new documents; no EIIN, no line.
	if err := tx.Exec("UPDATE school_settings SET eiin = '123456', address = 'Mirpur, Dhaka', address_bn = 'মিরপুর, ঢাকা', phone = '01700 000000', email = 'office@school.example.test' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	withEIIN, err := svc.Request(ctx, admin, bulk)
	if err != nil || withEIIN.ID == fresh.ID {
		t.Fatalf("after the school details changed: %+v %v", withEIIN.ID, err)
	}
	if sch := fake.last().Data.(sheetData).School; sch.EIIN != "EIIN: 123456" || sch.Contact != "Mirpur, Dhaka · 01700 000000 · office@school.example.test" {
		t.Fatalf("English header: %+v", sch)
	}
	if _, err := svc.Request(ctx, teacher, Request{Kind: "tabulation", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, Language: "bn"}); err != nil {
		t.Fatal(err)
	}
	if sch := fake.last().Data.(classData).School; sch.EIIN != "EIIN: ১২৩৪৫৬" || !strings.HasPrefix(sch.Contact, "মিরপুর, ঢাকা · ০১৭০০ ০০০০০০") {
		t.Fatalf("Bangla header: %+v", sch)
	}
	tx.Exec("UPDATE school_settings SET eiin = '', address_bn = '' WHERE id = 1")
	if _, err := svc.Request(ctx, admin, Request{Kind: "merit_list", Scope: exams.ScopeTerm, ScopeID: s.term, ClassID: s.classID, Language: "bn"}); err != nil {
		t.Fatal(err)
	}
	if sch := fake.last().Data.(classData).School; sch.EIIN != "" || !strings.HasPrefix(sch.Contact, "Mirpur, Dhaka") {
		t.Fatalf("without EIIN or a Bangla address: %+v", sch)
	}

	// 9. Large documents go to the background worker, in parts, with progress.
	svc.Chunk, svc.InlineLimit = 2, 1
	bn := bulk
	bn.Language = "bn"
	queued, err := svc.Request(ctx, admin, bn)
	if err != nil || queued.Status != "queued" || queued.ProgressTotal != 3 {
		t.Fatalf("queued: %+v %v", queued, err)
	}
	if n, err := svc.Work(ctx); err != nil || n != 1 {
		t.Fatalf("work: %d %v", n, err)
	}
	finished, err := svc.Get(ctx, admin, queued.ID)
	if err != nil || finished.Status != "done" || finished.ProgressDone != 3 || *finished.Pages != 3 || fake.last().Template != "merge.typ" {
		t.Fatalf("worker: %+v %v %s", finished, err, fake.last().Template)
	}
}

func TestFormatting(t *testing.T) {
	bn, en := lang("bn"), lang("en")
	if studentCode(" RMS260001 ", "2026-0001") != "RMS260001" || studentCode("", "2026-0001") != "2026-0001" {
		t.Fatal("student ID: the school's ID, or the admission number")
	}
	m := "45.50"
	if bn.marks(&m) != "৪৫.৫" || en.marks(&m) != "45.5" {
		t.Fatalf("marks: %s %s", bn.marks(&m), en.marks(&m))
	}
	w := "80.00"
	if en.marks(&w) != "80" || bn.int(2026) != "২০২৬" {
		t.Fatal("whole marks or digits")
	}
	if got := bn.date("2026-10-31T18:30:00Z"); got != "১ নভেম্বর ২০২৬" {
		t.Fatalf("Bangla date in Dhaka time: %s", got)
	}
	if bn.code("101") != "১০১" || bn.code("DEMO-001") != "DEMO-001" || bn.code("B1GM9O") != "B1GM9O" || en.code("101") != "101" {
		t.Fatal("codes: Bangla digits only for all-digit codes")
	}
	if documentCode("abc") == documentCode("abd") || len(documentCode("abc")) != 9 {
		t.Fatal("document code")
	}
	if initials("Green Valley Demo School") != "GV" || accentOf("red") != "#1d4ed8" || accentOf("#0F766E") != "#0F766E" {
		t.Fatal("branding helpers")
	}
	if strings.Join(orderComponents([]string{"PRAC", "MCQ", "CQ", "ORAL"}), ",") != "CQ,MCQ,PRAC,ORAL" {
		t.Fatal("component order")
	}
}
