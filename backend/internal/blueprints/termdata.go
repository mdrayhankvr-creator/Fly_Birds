package blueprints

import (
	"encoding/json"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// TermData is everything a class's term results are calculated from: the
// blueprint version the term's exams were created from, the grading bands,
// every active student's marks and the sheets that are not approved yet.
// The staff preview and result publication (E6) both read it, so a published
// term result is exactly what reviewers saw.
type TermData struct {
	Term          TermView
	ClassID       string
	ClassName     string
	State         string // ready, pending, no_exams or mixed_versions
	VersionID     string
	Version       int
	Legacy        bool
	DefinitionSHA string
	Definition    Definition
	Subjects      []SubjectRef
	Bands         []BandRef
	// BandsSource: "blueprint_version" (the bands snapshotted when the version
	// was activated) or "school" (legacy versions, as their exams publish).
	BandsSource string
	Students    []TermStudent
	Sheets      int
	Approved    int
	Pending     []PendingSheet
}

// TermStudent is one student's input to a term result.
type TermStudent struct {
	ID          string
	Name        string
	NameBN      string
	AdmissionNo string
	RollNo      *int
	SectionID   string
	SectionName string
	Input       Student
	// NotTaken lists the class subjects the student does not take.
	NotTaken []string
}

// Scheme is the exact grading scheme of the bands.
func SchemeOf(bands []BandRef) (calc.Scheme, error) {
	out := make([]calc.Band, 0, len(bands))
	for _, b := range bands {
		p, err := calc.PointsOf(b.GPA)
		if err != nil {
			return calc.Scheme{}, err
		}
		out = append(out, calc.Band{Grade: b.Grade, Min: b.Min, Max: b.Max, Point: p})
	}
	return calc.NewScheme(out)
}

// VersionBands returns the bands a version's results are graded with: its
// activation snapshot, or the school's current bands for a legacy version.
func VersionBands(tx *gorm.DB, versionID string) ([]BandRef, string, error) {
	var row struct {
		Legacy   bool
		Snapshot *string
	}
	if err := tx.Raw("SELECT legacy, grading_scheme_snapshot::text AS snapshot FROM blueprint_versions WHERE id = ?", versionID).Scan(&row).Error; err != nil {
		return nil, "", err
	}
	if !row.Legacy && row.Snapshot != nil {
		bands := []BandRef{}
		if err := json.Unmarshal([]byte(*row.Snapshot), &bands); err != nil {
			return nil, "", err
		}
		if len(bands) > 0 {
			return bands, "blueprint_version", nil
		}
	}
	bands, err := SchoolBands(tx)
	return bands, "school", err
}

// SchoolBands are the school's current grading bands.
func SchoolBands(tx *gorm.DB) ([]BandRef, error) {
	bands := []BandRef{}
	err := tx.Raw(`SELECT b.grade, b.min_marks AS min, b.max_marks AS max, b.gpa::float8 AS gpa
  FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1 ORDER BY b.min_marks DESC`).Scan(&bands).Error
	return bands, err
}

// LoadTerm reads a class's term: its version, bands, students, marks and
// sheet readiness.
func (r *Repository) LoadTerm(tx *gorm.DB, termID, classID string) (TermData, error) {
	term, err := r.term(tx, termID)
	if err != nil {
		return TermData{}, err
	}
	p := TermData{Term: term, ClassID: classID, Subjects: []SubjectRef{}, Students: []TermStudent{}, Pending: []PendingSheet{}}
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
	p.Legacy, p.DefinitionSHA = row.Legacy, row.Sha
	if p.Definition, err = r.definitionOf(tx, row); err != nil {
		return p, err
	}
	_, subjects, _, _, err := r.context(tx, term.SessionID, classID)
	if err != nil {
		return p, err
	}
	p.Subjects = subjects
	if p.Bands, p.BandsSource, err = VersionBands(tx, p.VersionID); err != nil {
		return p, err
	}

	students := []struct {
		ID, Name, NameBN, AdmissionNo, SectionID, SectionName string
		RollNo                                                *int
	}{}
	if err := tx.Raw(`SELECT st.id, st.name_en AS name, coalesce(st.name_bn, '') AS name_bn, st.admission_no, r.roll_no, r.section_id, sec.name AS section_name
  FROM student_rolls r JOIN students st ON st.id = r.student_id JOIN academic_sections sec ON sec.id = r.section_id
 WHERE r.session_id = ? AND r.class_id = ?
 ORDER BY sec.display_order, sec.name, r.roll_no NULLS LAST, st.admission_no, st.id`, term.SessionID, classID).Scan(&students).Error; err != nil {
		return p, err
	}
	marks, err := termMarks(tx, termID, classID)
	if err != nil {
		return p, err
	}
	notTaken := map[string][]string{}
	pairs := []struct{ StudentID, SubjectID string }{}
	if err := tx.Raw(`SELECT r.student_id, acs.subject_id FROM student_rolls r JOIN academic_class_subjects acs ON acs.class_id = r.class_id
 WHERE r.session_id = ? AND r.class_id = ? AND NOT student_takes_subject(r.student_id, r.session_id, r.class_id, acs.subject_id)
 ORDER BY acs.subject_id`, term.SessionID, classID).Scan(&pairs).Error; err != nil {
		return p, err
	}
	for _, pr := range pairs {
		notTaken[pr.StudentID] = append(notTaken[pr.StudentID], pr.SubjectID)
	}
	for _, st := range students {
		skip := map[string]bool{}
		for _, s := range notTaken[st.ID] {
			skip[s] = true
		}
		nt := notTaken[st.ID]
		if nt == nil {
			nt = []string{}
		}
		p.Students = append(p.Students, TermStudent{ID: st.ID, Name: st.Name, NameBN: st.NameBN, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo,
			SectionID: st.SectionID, SectionName: st.SectionName, NotTaken: nt,
			Input: Student{ID: st.ID, SectionID: st.SectionID, Marks: marks[st.ID], Takes: func(subject string) bool { return !skip[subject] }}})
	}

	// Readiness: every sheet of the term approved.
	sheets := []struct {
		ExamID, SheetID, SectionName, Assessment, SubjectName, Status string
		Missing                                                       int
	}{}
	if err := tx.Raw(`SELECT e.id AS exam_id, es.id AS sheet_id, sec.name AS section_name, e.assessment_key AS assessment, sub.name_en AS subject_name, es.sheet_status AS status,
       (SELECT count(*) FROM student_rolls st WHERE st.session_id = e.session_id AND st.class_id = e.class_id AND st.section_id = e.section_id
           AND student_takes_subject(st.student_id, st.session_id, st.class_id, es.subject_id)
           AND NOT EXISTS (SELECT 1 FROM exam_marks m WHERE m.exam_subject_id = es.id AND m.student_id = st.student_id)) AS missing
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
	if len(p.Pending) == 0 {
		p.State = "ready"
	}
	return p, nil
}

// Inputs returns the students' inputs in order.
func (p TermData) Inputs() []Student {
	in := make([]Student, 0, len(p.Students))
	for _, st := range p.Students {
		in = append(in, st.Input)
	}
	return in
}

// DefinitionOf returns a version's definition (a legacy version's is rebuilt
// from its rows) and whether it is legacy.
func (r *Repository) DefinitionOf(tx *gorm.DB, versionID string) (Definition, bool, error) {
	row, err := r.versionRow(tx, versionID, false)
	if err != nil {
		return Definition{}, false, err
	}
	d, err := r.definitionOf(tx, row)
	return d, row.Legacy, err
}
