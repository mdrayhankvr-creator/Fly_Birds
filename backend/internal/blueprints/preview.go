package blueprints

import (
	"context"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// A term preview computes a class's term results from the marks entered so
// far, with the blueprint version the term's exams were created from. It is
// provisional and for staff only: nothing is stored or published, and
// students never see it. Publishing term results needs result versions (E6).

// PreviewStudent is one student's provisional term result.
type PreviewStudent struct {
	StudentID   string        `json:"student_id"`
	Name        string        `json:"name"`
	AdmissionNo string        `json:"admission_no"`
	RollNo      *int          `json:"roll_no"`
	SectionID   string        `json:"section_id"`
	SectionName string        `json:"section_name"`
	Result      StudentResult `json:"result"`
}

// PendingSheet is a sheet of the term that is not approved yet.
type PendingSheet struct {
	ExamID      string `json:"exam_id"`
	SheetID     string `json:"sheet_id"`
	SectionName string `json:"section_name"`
	Assessment  string `json:"assessment"`
	SubjectName string `json:"subject_name"`
	Status      string `json:"status"`
	Missing     int    `json:"missing"`
}

// TermPreview is a class's provisional term results.
type TermPreview struct {
	Term        TermView         `json:"term"`
	ClassID     string           `json:"class_id"`
	ClassName   string           `json:"class_name"`
	State       string           `json:"state"` // ready, pending, no_exams, mixed_versions
	VersionID   string           `json:"version_id"`
	Version     int              `json:"version"`
	Definition  *Definition      `json:"definition"`
	Subjects    []SubjectRef     `json:"subjects"`
	Students    []PreviewStudent `json:"students"`
	Sheets      int              `json:"sheets"`
	Approved    int              `json:"approved"`
	Pending     []PendingSheet   `json:"pending"`
	Outcomes    map[string]int   `json:"outcomes"`
	Provisional bool             `json:"provisional"`
}

func (r *Repository) TermPreview(ctx context.Context, termID, classID string) (TermPreview, error) {
	tx := r.db.WithContext(ctx)
	term, err := r.term(tx, termID)
	if err != nil {
		return TermPreview{}, err
	}
	p := TermPreview{Term: term, ClassID: classID, Subjects: []SubjectRef{}, Students: []PreviewStudent{}, Pending: []PendingSheet{}, Outcomes: map[string]int{}, Provisional: true}
	if err := tx.Raw("SELECT name FROM academic_classes WHERE id = ?", classID).Row().Scan(&p.ClassName); err != nil {
		return p, ErrNotFound
	}
	versions := []struct {
		ID      string
		Version int
	}{}
	if err := tx.Raw(`SELECT DISTINCT v.id, v.version FROM exams e JOIN blueprint_versions v ON v.id = e.blueprint_version_id
 WHERE e.term_id = ? AND e.class_id = ? ORDER BY v.version`, termID, classID).Scan(&versions).Error; err != nil {
		return p, err
	}
	switch len(versions) {
	case 0:
		p.State = "no_exams"
		return p, nil
	case 1:
	default:
		p.State = "mixed_versions" // the term's exams come from different versions
		return p, nil
	}
	p.VersionID, p.Version = versions[0].ID, versions[0].Version
	row, err := r.versionRow(tx, p.VersionID, false)
	if err != nil {
		return p, err
	}
	d, err := r.definitionOf(tx, row)
	if err != nil {
		return p, err
	}
	p.Definition = &d
	_, subjects, _, c, err := r.context(tx, term.SessionID, classID)
	if err != nil {
		return p, err
	}
	p.Subjects = subjects
	scheme, err := calc.NewScheme(c.Bands)
	if err != nil {
		return p, ErrInvalid
	}

	students := []struct {
		ID, Name, AdmissionNo, SectionID, SectionName string
		RollNo                                        *int
	}{}
	if err := tx.Raw(`SELECT st.id, st.name_en AS name, st.admission_no, st.roll_no, st.section_id, sec.name AS section_name
  FROM students st JOIN academic_sections sec ON sec.id = st.section_id
 WHERE st.session_id = ? AND st.class_id = ? AND st.status = 'active'
 ORDER BY sec.display_order, sec.name, st.roll_no NULLS LAST, st.admission_no`, term.SessionID, classID).Scan(&students).Error; err != nil {
		return p, err
	}
	marks, err := termMarks(tx, termID, classID)
	if err != nil {
		return p, err
	}
	notTaken := map[string]bool{}
	pairs := []struct{ StudentID, SubjectID string }{}
	if err := tx.Raw(`SELECT st.id AS student_id, acs.subject_id FROM students st JOIN academic_class_subjects acs ON acs.class_id = st.class_id
 WHERE st.session_id = ? AND st.class_id = ? AND st.status = 'active' AND NOT student_takes_subject(st.id, st.session_id, st.class_id, acs.subject_id)`,
		term.SessionID, classID).Scan(&pairs).Error; err != nil {
		return p, err
	}
	for _, pr := range pairs {
		notTaken[pr.StudentID+"|"+pr.SubjectID] = true
	}
	in := make([]Student, 0, len(students))
	for _, st := range students {
		id := st.ID
		in = append(in, Student{ID: id, SectionID: st.SectionID, Marks: marks[id], Takes: func(subject string) bool { return !notTaken[id+"|"+subject] }})
	}
	results, err := TermResults(d, termID, in, scheme)
	if err != nil {
		return p, err
	}
	for i, st := range students {
		p.Students = append(p.Students, PreviewStudent{StudentID: st.ID, Name: st.Name, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo, SectionID: st.SectionID, SectionName: st.SectionName, Result: results[i]})
		p.Outcomes[results[i].Outcome]++
	}

	// Readiness: every sheet of the term approved.
	sheets := []struct {
		ExamID, SheetID, SectionName, Assessment, SubjectName, Status string
		Missing                                                       int
	}{}
	if err := tx.Raw(`SELECT e.id AS exam_id, es.id AS sheet_id, sec.name AS section_name, e.assessment_key AS assessment, sub.name_en AS subject_name, es.sheet_status AS status,
       (SELECT count(*) FROM students st WHERE st.session_id = e.session_id AND st.class_id = e.class_id AND st.section_id = e.section_id AND st.status = 'active'
           AND student_takes_subject(st.id, st.session_id, st.class_id, es.subject_id)
           AND NOT EXISTS (SELECT 1 FROM exam_marks m WHERE m.exam_subject_id = es.id AND m.student_id = st.id)) AS missing
  FROM exams e JOIN exam_subjects es ON es.exam_id = e.id JOIN academic_sections sec ON sec.id = e.section_id JOIN academic_subjects sub ON sub.id = es.subject_id
 WHERE e.term_id = ? AND e.class_id = ? ORDER BY sec.display_order, sec.name, e.assessment_key, sub.code`, termID, classID).Scan(&sheets).Error; err != nil {
		return p, err
	}
	for _, s := range sheets {
		p.Sheets++
		if s.Status == "approved" {
			p.Approved++
			continue
		}
		p.Pending = append(p.Pending, PendingSheet{ExamID: s.ExamID, SheetID: s.SheetID, SectionName: s.SectionName, Assessment: s.Assessment, SubjectName: s.SubjectName, Status: s.Status, Missing: s.Missing})
	}
	p.State = "pending"
	if len(p.Pending) == 0 && p.Outcomes["incomplete"] == 0 {
		p.State = "ready"
	}
	return p, nil
}

// termMarks loads every mark of a class's term: student → assessment →
// subject, with each sheet's full and pass marks and component marks.
func termMarks(tx *gorm.DB, termID, classID string) (map[string]map[string]map[string]Mark, error) {
	rows := []struct {
		MarkID, StudentID, Assessment, SubjectID, Status string
		Marks                                            *float64
		Full, Pass                                       float64
	}{}
	if err := tx.Raw(`SELECT m.id AS mark_id, m.student_id, e.assessment_key AS assessment, es.subject_id, m.status, m.marks::float8 AS marks,
       es.full_marks::float8 AS full, es.pass_marks::float8 AS pass
  FROM exam_marks m JOIN exams e ON e.id = m.exam_id JOIN exam_subjects es ON es.id = m.exam_subject_id
 WHERE e.term_id = ? AND e.class_id = ?`, termID, classID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	comps := []struct {
		MarkID, Key string
		Marks, Full float64
		Pass        *float64
	}{}
	if err := tx.Raw(`SELECT cm.exam_mark_id AS mark_id, cm.component_key AS key, cm.marks::float8 AS marks, c.full_marks::float8 AS full, c.pass_marks::float8 AS pass
  FROM exam_component_marks cm JOIN exam_subject_components c ON c.exam_subject_id = cm.exam_subject_id AND c.component_key = cm.component_key
  JOIN exam_marks m ON m.id = cm.exam_mark_id JOIN exams e ON e.id = m.exam_id
 WHERE e.term_id = ? AND e.class_id = ? ORDER BY c.display_order`, termID, classID).Scan(&comps).Error; err != nil {
		return nil, err
	}
	byMark := map[string][]ComponentMark{}
	for _, c := range comps {
		byMark[c.MarkID] = append(byMark[c.MarkID], ComponentMark{Key: c.Key, Marks: c.Marks, Full: c.Full, Pass: c.Pass})
	}
	out := map[string]map[string]map[string]Mark{}
	for _, r := range rows {
		if out[r.StudentID] == nil {
			out[r.StudentID] = map[string]map[string]Mark{}
		}
		if out[r.StudentID][r.Assessment] == nil {
			out[r.StudentID][r.Assessment] = map[string]Mark{}
		}
		out[r.StudentID][r.Assessment][r.SubjectID] = Mark{Status: r.Status, Marks: r.Marks, Full: r.Full, Pass: r.Pass, Components: byMark[r.MarkID]}
	}
	return out, nil
}
