package exams

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// Verification compares every published (or locked) result with what the
// exact engine calculates from the stored marks, without changing anything.
// It is the engine behind cmd/exam-verify.

// Difference kinds.
const (
	// BoundaryCorrection: the stored value is what the pre-E2 float engine
	// produced; the exact engine differs because a mark sits exactly on a
	// band edge. Correct by unpublishing and republishing the exam.
	BoundaryCorrection = "boundary_correction"
	// Inconsistent: neither engine reproduces the stored value from today's
	// marks and grading scheme (the scheme or a fourth-subject setting changed
	// after publication, or data was changed outside the application).
	Inconsistent = "inconsistent"
)

// Difference is one stored value that the exact engine would calculate
// differently.
type Difference struct {
	Kind        string `json:"kind"`
	Level       string `json:"level"` // mark, result or position
	ExamID      string `json:"exam_id"`
	Exam        string `json:"exam"`
	Session     string `json:"session"`
	Class       string `json:"class"`
	Section     string `json:"section"`
	StudentID   string `json:"student_id"`
	AdmissionNo string `json:"admission_no"`
	Student     string `json:"student"`
	Subject     string `json:"subject,omitempty"`
	Field       string `json:"field"`
	Stored      string `json:"stored"`
	Exact       string `json:"exact"`
	Detail      string `json:"detail,omitempty"`
}

// VerifyReport is the outcome of a verification run.
type VerifyReport struct {
	Exams               int `json:"exams"`
	Results             int `json:"results"`
	Marks               int `json:"marks"`
	BoundaryCorrections int `json:"boundary_corrections"`
	Inconsistent        int `json:"inconsistent"`
	// Correcting: results skipped because a correction is open (E6).
	Correcting  int          `json:"correcting"`
	Differences []Difference `json:"differences"`
}

// VerifyOptions narrows a run; the zero value checks every published exam.
type VerifyOptions struct {
	SessionID string
}

// Verify runs the check in one read-only, repeatable-read transaction, so
// it sees a consistent snapshot and PostgreSQL refuses any write.
func Verify(ctx context.Context, db *gorm.DB, opts VerifyOptions) (VerifyReport, error) {
	tx := beginReadOnly(ctx, db)
	if tx.Error != nil {
		return VerifyReport{}, tx.Error
	}
	defer tx.Rollback()
	return verifyIn(tx, opts)
}

// beginReadOnly starts the transaction Verify reads in: READ ONLY, so
// PostgreSQL itself refuses any write, and REPEATABLE READ, so every query
// sees the same snapshot. The caller must roll it back.
func beginReadOnly(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
}

type verifyExam struct {
	ID, Name, ExamType, SessionID, ClassID, SectionID string
	SessionName, ClassName, SectionName               string
	TermID, AssessmentKey, BlueprintVersionID         *string
}

type verifyMark struct {
	MarkID                                            string
	ExamID, StudentID, SubjectID, SubjectName, Status string
	Marks                                             *float64
	FullMarks, PassMarks                              float64
	IsFourth                                          bool
	Grade                                             *string
	GradePoint                                        *float64
	Passed                                            *bool
}

type verifyResult struct {
	ExamID, StudentID, SectionID, AdmissionNo, StudentName string
	TotalMarks, FullMarks, GPA                             float64
	Passed                                                 bool
	ClassPosition, SectionPosition                         int
}

func verifyIn(tx *gorm.DB, opts VerifyOptions) (VerifyReport, error) {
	report := VerifyReport{Differences: []Difference{}}
	if opts.SessionID != "" && !uuid.MatchString(opts.SessionID) {
		return report, ErrInvalid
	}
	bands := []Band{}
	if err := tx.Table("grading_bands b").Joins("JOIN school_settings ss ON ss.grading_scheme_id=b.scheme_id AND ss.id=1").Select("b.grade,b.min_marks,b.max_marks,b.gpa").Order("b.min_marks DESC").Find(&bands).Error; err != nil {
		return report, err
	}
	if len(bands) == 0 {
		return report, fmt.Errorf("the school has no grading bands")
	}
	published := "e.status IN ('published','locked')"
	session := func(q *gorm.DB) *gorm.DB {
		if opts.SessionID != "" {
			return q.Where("e.session_id=?", opts.SessionID)
		}
		return q
	}
	exams := []verifyExam{}
	if err := session(tx.Table("exams e").Joins("JOIN academic_sessions se ON se.id=e.session_id").Joins("JOIN academic_classes c ON c.id=e.class_id").Joins("JOIN academic_sections s ON s.id=e.section_id").Where(published)).
		Select("e.id,e.name,e.exam_type,e.session_id,e.class_id,e.section_id,se.name AS session_name,c.name AS class_name,s.name AS section_name,e.term_id,e.assessment_key,e.blueprint_version_id").
		Order("se.name,c.name,e.name,s.name,e.id").Find(&exams).Error; err != nil {
		return report, err
	}
	marks := []verifyMark{}
	if err := session(tx.Table("exam_marks m").Joins("JOIN exams e ON e.id=m.exam_id").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").Where(published).
		// Only subjects the student takes, as publication scores them (E5).
		Where("student_takes_subject(m.student_id, e.session_id, e.class_id, es.subject_id)")).
		Select("m.id AS mark_id,m.exam_id,m.student_id,es.subject_id,sub.name_en AS subject_name,m.status,m.marks,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth,m.grade,m.grade_point,m.passed").
		Order("m.exam_id,m.student_id,es.subject_id").Find(&marks).Error; err != nil {
		return report, err
	}
	results := []verifyResult{}
	if err := session(tx.Table("exam_results r").Joins("JOIN exams e ON e.id=r.exam_id").Joins("JOIN students st ON st.id=r.student_id").Where(published)).
		Select("r.exam_id,r.student_id,r.section_id,st.admission_no,st.name_en AS student_name,r.total_marks,r.full_marks,r.gpa,r.passed,r.class_position,r.section_position").
		Order("r.exam_id,st.admission_no,r.student_id").Find(&results).Error; err != nil {
		return report, err
	}
	report.Exams, report.Results, report.Marks = len(exams), len(results), len(marks)

	examByID := map[string]verifyExam{}
	rulesOf := map[string]examRules{}
	bandsOf := map[string][]Band{}
	for _, e := range exams {
		examByID[e.ID] = e
		rules, err := rulesFor(tx, Exam{ID: e.ID, SessionID: e.SessionID, ClassID: e.ClassID, BlueprintVersionID: e.BlueprintVersionID})
		if err != nil {
			return report, err
		}
		bandsOf[e.ID] = bands
		// Results are checked against the rules they were published with
		// (E6): a later change of the grading bands is not a difference.
		if stored, _, err := storedAssessmentRules(tx, e.ID, e.ClassID); err == nil {
			b, policy, mode := stored.engine()
			rules.Policy, rules.Dense, rules.Combined = policy, mode == calc.Dense, stored.Combined
			bandsOf[e.ID] = b
		} else if !errors.Is(err, ErrNotPublished) {
			return report, err
		}
		rulesOf[e.ID] = rules
	}
	// A student with an open correction is being corrected: their marks may
	// differ from what is published until the correction is published.
	correcting := map[[2]string]bool{}
	open := []struct{ ExamID, StudentID string }{}
	if err := tx.Raw("SELECT exam_id, student_id FROM result_corrections WHERE status = 'open'").Scan(&open).Error; err != nil {
		return report, err
	}
	for _, o := range open {
		correcting[[2]string{o.ExamID, o.StudentID}] = true
	}
	// Component marks of the published marks, by mark id, in one query.
	componentsOf := map[string][]ComponentLine{}
	comps := []struct {
		MarkID, Key string
		Marks       float64
		Pass        *float64
	}{}
	if err := session(tx.Table("exam_component_marks cm").Joins("JOIN exam_subject_components c ON c.exam_subject_id=cm.exam_subject_id AND c.component_key=cm.component_key").
		Joins("JOIN exam_marks m ON m.id=cm.exam_mark_id").Joins("JOIN exams e ON e.id=m.exam_id").Where(published)).
		Select("cm.exam_mark_id AS mark_id,cm.component_key AS key,cm.marks::float8 AS marks,c.pass_marks::float8 AS pass").Order("c.display_order").Find(&comps).Error; err != nil {
		return report, err
	}
	for _, c := range comps {
		componentsOf[c.MarkID] = append(componentsOf[c.MarkID], ComponentLine{Key: c.Key, Marks: c.Marks, Pass: c.Pass})
	}
	marksOf := map[[2]string][]verifyMark{}
	for _, m := range marks {
		key := [2]string{m.ExamID, m.StudentID}
		marksOf[key] = append(marksOf[key], m)
	}
	add := func(kind, level string, e verifyExam, r verifyResult, subject, field, stored, exact, detail string) {
		report.Differences = append(report.Differences, Difference{Kind: kind, Level: level, ExamID: e.ID, Exam: e.Name, Session: e.SessionName, Class: e.ClassName, Section: e.SectionName,
			StudentID: r.StudentID, AdmissionNo: r.AdmissionNo, Student: r.StudentName, Subject: subject, Field: field, Stored: stored, Exact: exact, Detail: detail})
		if kind == BoundaryCorrection {
			report.BoundaryCorrections++
		} else {
			report.Inconsistent++
		}
	}
	kindOf := func(legacyMatches bool) string {
		if legacyMatches {
			return BoundaryCorrection
		}
		return Inconsistent
	}

	// Each result and its marks.
	type ranking struct {
		exact, legacy calc.Ranked
		stored        verifyResult
	}
	groups := map[string][]ranking{}
	for _, r := range results {
		e := examByID[r.ExamID]
		if correcting[[2]string{r.ExamID, r.StudentID}] {
			report.Correcting++
			continue
		}
		bands := bandsOf[e.ID]
		rows := marksOf[[2]string{r.ExamID, r.StudentID}]
		if len(rows) == 0 {
			add(Inconsistent, "result", e, r, "", "marks", "none", "", "a published result has no marks")
			continue
		}
		lines := make([]MarkLine, 0, len(rows))
		subjects := make([]ScoredSubject, 0, len(rows))
		allPresent := true
		for _, m := range rows {
			lines = append(lines, MarkLine{Marks: m.Marks, FullMarks: m.FullMarks, PassMarks: m.PassMarks, Fourth: m.IsFourth, Status: m.Status,
				Combined: rulesOf[e.ID].Combined[m.SubjectID], Components: componentsOf[m.MarkID]})
			if m.Status != StatusPresent || m.Marks == nil {
				allPresent = false
				continue
			}
			subjects = append(subjects, ScoredSubject{Marks: *m.Marks, FullMarks: m.FullMarks, PassMarks: m.PassMarks, Fourth: m.IsFourth})
		}
		score, err := ScoreLinesWith(lines, bands, rulesOf[e.ID].Policy)
		if err != nil {
			add(Inconsistent, "result", e, r, "", "marks", "", "", "the stored marks cannot be graded with the current scheme")
			continue
		}
		if score.Withheld {
			add(Inconsistent, "result", e, r, "", "result", "published", "withheld", "a student whose result is withheld has a published result")
			continue
		}
		exact := score.Result
		// The pre-E2 engine never saw statuses; it explains a difference only
		// when every mark is present.
		var lgpa float64
		var lpassed, lok bool
		var lgrades []Grade
		if allPresent {
			lgpa, lpassed, lgrades, lok = legacyGPA(subjects, bands)
		}
		for i, m := range rows {
			g := score.Lines[i]
			stored := fmt.Sprintf("%s %s %s", deref(m.Grade), points(m.GradePoint), passedWord(m.Passed))
			want := "— — —"
			if g.Grade != nil {
				want = fmt.Sprintf("%s %s %s", *g.Grade, *g.Point, passedWord(g.Passed))
			}
			if stored == want {
				continue
			}
			legacy := lok && fmt.Sprintf("%s %s %s", lgrades[i].Name, fmt.Sprintf("%.2f", lgrades[i].Point), passedWord(&lgrades[i].Passed)) == stored
			detail := m.Status
			if m.Marks != nil {
				detail = fmt.Sprintf("%s of %s", formatMarks(*m.Marks), formatMarks(m.FullMarks))
			}
			if legacy {
				detail += " is exactly on a band edge"
			}
			add(kindOf(legacy), "mark", e, r, m.SubjectName, "grade", stored, want, detail)
		}
		storedGPA, _ := calc.PointsOf(r.GPA)
		storedTotal, _ := calc.MarksOf(r.TotalMarks)
		storedFull, _ := calc.MarksOf(r.FullMarks)
		if storedGPA != exact.GPA || r.Passed != exact.Passed {
			legacy := lok && math.Round(lgpa*100) == float64(storedGPA) && lpassed == r.Passed
			add(kindOf(legacy), "result", e, r, "", "gpa", fmt.Sprintf("%s %s", storedGPA, passedWord(&r.Passed)), fmt.Sprintf("%s %s", exact.GPA, passedWord(&exact.Passed)), "")
		}
		if storedTotal != exact.Total || storedFull != exact.Full {
			add(Inconsistent, "result", e, r, "", "total", fmt.Sprintf("%s / %s", storedTotal, storedFull), fmt.Sprintf("%s / %s", exact.Total, exact.Full), "")
		}
		// Class positions join a term's assessment across sections (E5),
		// or, for exams made outside a term, the name and type.
		group := strings.Join([]string{e.SessionID, e.ClassID, strings.ToLower(e.Name), e.ExamType}, "|")
		if e.TermID != nil && e.AssessmentKey != nil {
			group = strings.Join([]string{e.SessionID, e.ClassID, "term", *e.TermID, *e.AssessmentKey}, "|")
		}
		legacyGPAPoints := calc.Points(math.Round(lgpa * 100))
		if !lok { // statuses: the old engine has no value; rank with the exact one
			legacyGPAPoints, lpassed = exact.GPA, exact.Passed
		}
		groups[group] = append(groups[group], ranking{
			exact:  calc.Ranked{Key: r.ExamID + "|" + r.StudentID, GPA: exact.GPA, Total: exact.Total, Passed: exact.Passed},
			legacy: calc.Ranked{Key: r.ExamID + "|" + r.StudentID, GPA: legacyGPAPoints, Total: exact.Total, Passed: lpassed},
			stored: r,
		})
	}

	// Positions: class position across the sections of a class (exams joined
	// by session, class, name and type, as at publication) and section
	// position within the section.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		entries := groups[k]
		exactAll, legacyAll := []calc.Ranked{}, []calc.Ranked{}
		bySection := map[string][2][]calc.Ranked{}
		for _, en := range entries {
			exactAll, legacyAll = append(exactAll, en.exact), append(legacyAll, en.legacy)
			s := bySection[en.stored.SectionID]
			bySection[en.stored.SectionID] = [2][]calc.Ranked{append(s[0], en.exact), append(s[1], en.legacy)}
		}
		mode := calc.Competition
		if rulesOf[entries[0].stored.ExamID].Dense {
			mode = calc.Dense
		}
		exactClass, legacyClass := calc.Rank(exactAll, mode), calc.Rank(legacyAll, calc.Competition)
		exactSection, legacySection := map[string]int{}, map[string]int{}
		for _, s := range bySection {
			for key, v := range calc.Rank(s[0], mode) {
				exactSection[key] = v
			}
			for key, v := range calc.Rank(s[1], calc.Competition) {
				legacySection[key] = v
			}
		}
		for _, en := range entries {
			key, r := en.exact.Key, en.stored
			e := examByID[r.ExamID]
			if r.ClassPosition != exactClass[key] {
				add(kindOf(r.ClassPosition == legacyClass[key]), "position", e, r, "", "class_position", fmt.Sprint(r.ClassPosition), fmt.Sprint(exactClass[key]), "")
			}
			if r.SectionPosition != exactSection[key] {
				add(kindOf(r.SectionPosition == legacySection[key]), "position", e, r, "", "section_position", fmt.Sprint(r.SectionPosition), fmt.Sprint(exactSection[key]), "")
			}
		}
	}
	return report, nil
}

func deref(s *string) string {
	if s == nil {
		return "—"
	}
	return *s
}

func points(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *p)
}

func passedWord(b *bool) string {
	switch {
	case b == nil:
		return "—"
	case *b:
		return "passed"
	default:
		return "failed"
	}
}

func formatMarks(v float64) string { return fmt.Sprintf("%.2f", v) }

// legacyGradeFor and legacyGPA are the float engine used before E2, kept
// only to classify differences: a stored value it reproduces is a boundary
// correction, anything else is an inconsistency. Never use them to grade.
func legacyGradeFor(marks, full, pass float64, bands []Band) (Grade, bool) {
	if !finiteMoney(marks) || !finiteMoney(full) || !finiteMoney(pass) || full <= 0 || marks > full || pass > full {
		return Grade{}, false
	}
	pct := marks * 100 / full
	for _, b := range bands {
		if pct >= float64(b.MinMarks) && (pct < float64(b.MaxMarks+1) || b.MaxMarks == 100 && pct <= 100) {
			return Grade{Name: b.Grade, Point: b.GPA, Passed: marks >= pass && b.GPA > 0}, true
		}
	}
	return Grade{}, false
}

func legacyGPA(subjects []ScoredSubject, bands []Band) (float64, bool, []Grade, bool) {
	required, optional := 0, 0
	sum, bonus, passed := 0.0, 0.0, true
	grades := make([]Grade, 0, len(subjects))
	for _, s := range subjects {
		g, ok := legacyGradeFor(s.Marks, s.FullMarks, s.PassMarks, bands)
		if !ok {
			return 0, false, nil, false
		}
		grades = append(grades, g)
		if s.Fourth {
			optional++
			bonus = math.Max(0, g.Point-2)
			continue
		}
		required++
		sum += g.Point
		if !g.Passed {
			passed = false
		}
	}
	if required == 0 || optional > 1 {
		return 0, false, nil, false
	}
	if !passed {
		return 0, false, grades, true
	}
	return math.Round(math.Min(5, (sum+bonus)/float64(required))*100) / 100, true, grades, true
}
