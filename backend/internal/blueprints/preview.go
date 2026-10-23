package blueprints

import (
	"context"

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
	d, err := r.LoadTerm(r.db.WithContext(ctx), termID, classID)
	if err != nil {
		return TermPreview{}, err
	}
	p := TermPreview{Term: d.Term, ClassID: classID, ClassName: d.ClassName, State: d.State, VersionID: d.VersionID, Version: d.Version,
		Subjects: d.Subjects, Students: []PreviewStudent{}, Sheets: d.Sheets, Approved: d.Approved, Pending: d.Pending, Outcomes: map[string]int{}, Provisional: true}
	if d.State == "no_exams" || d.State == "mixed_versions" {
		return p, nil
	}
	p.Definition = &d.Definition
	scheme, err := SchemeOf(d.Bands)
	if err != nil {
		return p, ErrInvalid
	}
	results, err := TermResults(d.Definition, termID, d.Inputs(), scheme)
	if err != nil {
		return p, err
	}
	for i, st := range d.Students {
		p.Students = append(p.Students, PreviewStudent{StudentID: st.ID, Name: st.Name, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo, SectionID: st.SectionID, SectionName: st.SectionName, Result: results[i]})
		p.Outcomes[results[i].Outcome]++
	}
	if d.State == "ready" && p.Outcomes["incomplete"] > 0 {
		p.State = "pending"
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
