package exams

import (
	"context"
	"database/sql"
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
	Exams               int          `json:"exams"`
	Results             int          `json:"results"`
	Marks               int          `json:"marks"`
	BoundaryCorrections int          `json:"boundary_corrections"`
	Inconsistent        int          `json:"inconsistent"`
	Differences         []Difference `json:"differences"`
}

// VerifyOptions narrows a run; the zero value checks every published exam.
type VerifyOptions struct {
	SessionID string
}

// Verify runs the check in one read-only, repeatable-read transaction, so
// it sees a consistent snapshot and PostgreSQL refuses any write.
func Verify(ctx context.Context, db *gorm.DB, opts VerifyOptions) (VerifyReport, error) {
	tx := db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return VerifyReport{}, tx.Error
	}
	defer tx.Rollback()
	return verifyIn(tx, opts)
}

type verifyExam struct {
	ID, Name, ExamType, SessionID, ClassID, SectionID string
	SessionName, ClassName, SectionName               string
}

type verifyMark struct {
	ExamID, StudentID, SubjectID, SubjectName string
	Marks, FullMarks, PassMarks               float64
	IsFourth                                  bool
	Grade                                     *string
	GradePoint                                *float64
	Passed                                    *bool
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
		Select("e.id,e.name,e.exam_type,e.session_id,e.class_id,e.section_id,se.name AS session_name,c.name AS class_name,s.name AS section_name").
		Order("se.name,c.name,e.name,s.name,e.id").Find(&exams).Error; err != nil {
		return report, err
	}
	marks := []verifyMark{}
	if err := session(tx.Table("exam_marks m").Joins("JOIN exams e ON e.id=m.exam_id").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").Where(published)).
		Select("m.exam_id,m.student_id,es.subject_id,sub.name_en AS subject_name,m.marks,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth,m.grade,m.grade_point,m.passed").
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
	for _, e := range exams {
		examByID[e.ID] = e
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
		rows := marksOf[[2]string{r.ExamID, r.StudentID}]
		if len(rows) == 0 {
			add(Inconsistent, "result", e, r, "", "marks", "none", "", "a published result has no marks")
			continue
		}
		subjects := make([]ScoredSubject, 0, len(rows))
		for _, m := range rows {
			subjects = append(subjects, ScoredSubject{Marks: m.Marks, FullMarks: m.FullMarks, PassMarks: m.PassMarks, Fourth: m.IsFourth})
		}
		exact, err := Score(subjects, bands)
		if err != nil {
			add(Inconsistent, "result", e, r, "", "marks", "", "", "the stored marks cannot be graded with the current scheme")
			continue
		}
		lgpa, lpassed, lgrades, lok := legacyGPA(subjects, bands)
		for i, m := range rows {
			g := exact.Grades[i]
			stored := fmt.Sprintf("%s %s %s", deref(m.Grade), points(m.GradePoint), passedWord(m.Passed))
			want := fmt.Sprintf("%s %s %s", g.Grade, g.Point, passedWord(&g.Passed))
			if stored == want {
				continue
			}
			legacy := lok && fmt.Sprintf("%s %s %s", lgrades[i].Name, fmt.Sprintf("%.2f", lgrades[i].Point), passedWord(&lgrades[i].Passed)) == stored
			detail := fmt.Sprintf("%s of %s", formatMarks(m.Marks), formatMarks(m.FullMarks))
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
		group := strings.Join([]string{e.SessionID, e.ClassID, strings.ToLower(e.Name), e.ExamType}, "|")
		legacyGPAPoints := calc.Points(math.Round(lgpa * 100))
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
		exactClass, legacyClass := calc.Rank(exactAll, calc.Competition), calc.Rank(legacyAll, calc.Competition)
		exactSection, legacySection := map[string]int{}, map[string]int{}
		for _, s := range bySection {
			for key, v := range calc.Rank(s[0], calc.Competition) {
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
