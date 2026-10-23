package exams

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// What students, guardians and staff read: published versions only (E6).
// Report cards come from the snapshot, never from the live marks, so a
// correction in progress, a changed grading scheme or a changed optional
// subject cannot change a published card.

func floatOf(s *string) *float64 { return parseFloat(s) }

// currentAssessmentVersion is a student's current version of an exam.
func currentAssessmentVersion(tx *gorm.DB, examID, studentID string) (*versionRow, error) {
	var v versionRow
	err := tx.Table("result_versions v").Select(versionColumns).
		Where("v.scope = 'assessment' AND v.scope_id = ? AND v.student_id = ? AND v.status = 'published'", examID, studentID).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

// cardFromVersion builds the report card of a version; staff also get the
// statuses' reasons and the marks' versions.
func cardFromVersion(card *Card, v versionRow, scope string) error {
	snap, err := v.snapshot()
	if err != nil {
		return err
	}
	st := snap.Student
	card.Result = Result{ExamID: v.ScopeID, StudentID: v.StudentID, StudentName: st.NameEN, StudentNameBN: st.NameBN, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo}
	card.VersionID, card.Version, card.PublishedAt = v.ID, v.Version, v.PublishedAt
	card.Revised = v.SupersedesID != nil
	if v.Outcome == "withheld" {
		card.Withheld = true
		return nil
	}
	if f := floatOf(v.TotalMarks); f != nil {
		card.Result.TotalMarks = *f
	}
	if f := floatOf(v.FullMarks); f != nil {
		card.Result.FullMarks = *f
	}
	if f := floatOf(v.GPA); f != nil {
		card.Result.GPA = *f
	}
	card.Result.Passed = v.Outcome == "passed"
	if v.ClassPosition != nil {
		card.Result.ClassPosition = *v.ClassPosition
	}
	if v.SectionPosition != nil {
		card.Result.SectionPosition = *v.SectionPosition
	}
	reasons, versions := map[string]string{}, map[string]int{}
	if scope == "admin" {
		var in AssessmentInputs
		if err := json.Unmarshal(snap.Inputs, &in); err == nil {
			for _, l := range in.Lines {
				reasons[l.SheetID], versions[l.SheetID] = l.Reason, l.RowVersion
			}
		}
	}
	for _, l := range snap.Outputs.Subjects {
		row := MarkRow{ExamSubjectID: l.SheetID, StudentID: v.StudentID, StudentName: st.NameEN, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo,
			SubjectName: l.NameEN, SubjectNameBN: l.NameBN, Marks: floatOf(l.Marks), Grade: l.Grade, GradePoint: floatOf(l.Point), Passed: l.Passed,
			Status: l.Status, StatusReason: reasons[l.SheetID], RowVersion: versions[l.SheetID]}
		if len(l.Components) > 0 {
			row.Components = map[string]float64{}
			for _, c := range l.Components {
				if f := floatOf(c.Marks); f != nil {
					row.Components[c.Key] = *f
				}
			}
		}
		card.Subjects = append(card.Subjects, row)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Term and annual results for students and guardians

// PublishedLine is one subject of a published term or annual result as a
// family sees it: no class-test contributions, no component detail.
type PublishedLine struct {
	Key      string               `json:"key"`
	NameEN   string               `json:"name_en"`
	NameBN   string               `json:"name_bn"`
	Optional bool                 `json:"optional"`
	Status   string               `json:"status"`
	Marks    *string              `json:"marks"`
	Full     string               `json:"full"`
	Grade    *string              `json:"grade"`
	Point    *string              `json:"point"`
	Passed   *bool                `json:"passed"`
	Reasons  []string             `json:"reasons"`
	Terms    []PublishedTermShare `json:"terms,omitempty"`
}

// PublishedTermShare is one term's part in an annual subject line.
type PublishedTermShare struct {
	TermID   string `json:"term_id"`
	TermName string `json:"term_name"`
	Status   string `json:"status"`
	Marks    string `json:"marks"`
	Full     string `json:"full"`
	Weight   int    `json:"weight"`
}

// PublishedResult is a published term or annual result.
type PublishedResult struct {
	ID              string          `json:"id"`
	Scope           string          `json:"scope"`
	Version         int             `json:"version"`
	Revised         bool            `json:"revised"`
	PublishedAt     string          `json:"published_at"`
	SessionName     string          `json:"session_name"`
	ClassName       string          `json:"class_name"`
	SectionName     string          `json:"section_name"`
	TermID          *string         `json:"term_id"`
	TermName        string          `json:"term_name"`
	TermSequence    int             `json:"term_sequence"`
	Method          string          `json:"method"`
	MethodTerms     []MethodTerm    `json:"method_terms"`
	Withheld        bool            `json:"withheld"`
	Outcome         string          `json:"outcome"`
	GPA             *string         `json:"gpa"`
	Total           *string         `json:"total"`
	Full            *string         `json:"full"`
	ClassPosition   *int            `json:"class_position"`
	SectionPosition *int            `json:"section_position"`
	Student         SnapshotStudent `json:"student"`
	Subjects        []PublishedLine `json:"subjects"`
}

// MethodTerm names a term of an annual method ("25 % First Term").
type MethodTerm struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

// publishedOf is the family view of a version: withheld shows no marks.
func publishedOf(v versionRow, rules string) (PublishedResult, error) {
	snap, err := v.snapshot()
	if err != nil {
		return PublishedResult{}, err
	}
	p := PublishedResult{ID: v.ID, Scope: v.Scope, Version: v.Version, Revised: v.SupersedesID != nil, PublishedAt: v.PublishedAt,
		SessionName: snap.Student.SessionName, ClassName: snap.Student.ClassName, SectionName: snap.Student.SectionName,
		TermID: snap.Context.TermID, TermName: snap.Context.TermName, TermSequence: snap.Context.TermSequence, Method: snap.Context.Method,
		MethodTerms: []MethodTerm{}, Outcome: v.Outcome, Student: snap.Student, Subjects: []PublishedLine{}}
	p.Student.ID = v.StudentID
	termNames := map[string]string{}
	if v.Scope == ScopeAnnual {
		var r AnnualRulesSnapshot
		if err := json.Unmarshal([]byte(rules), &r); err == nil {
			for _, t := range r.Annual.Terms {
				termNames[t.ID] = t.Name
				w := t.Weight
				if r.Annual.Method != "weighted" {
					w = 0
				}
				p.MethodTerms = append(p.MethodTerms, MethodTerm{ID: t.ID, Name: t.Name, Weight: w})
			}
		}
	}
	if v.Outcome == "withheld" {
		p.Withheld = true
		return p, nil
	}
	p.GPA, p.Total, p.Full, p.ClassPosition, p.SectionPosition = v.GPA, v.TotalMarks, v.FullMarks, v.ClassPosition, v.SectionPosition
	for _, l := range snap.Outputs.Subjects {
		line := PublishedLine{Key: l.Key, NameEN: l.NameEN, NameBN: l.NameBN, Optional: l.Optional, Status: l.Status, Marks: l.Marks, Full: l.Full,
			Grade: l.Grade, Point: l.Point, Passed: l.Passed, Reasons: []string{}}
		for _, r := range l.Reasons {
			code := r.Code
			if r.Component != "" {
				code += ":" + r.Component
			}
			line.Reasons = append(line.Reasons, code)
		}
		for _, t := range l.Terms {
			line.Terms = append(line.Terms, PublishedTermShare{TermID: t.TermID, TermName: termNames[t.TermID], Status: t.Status, Marks: t.Marks, Full: t.Full, Weight: t.Weight})
		}
		p.Subjects = append(p.Subjects, line)
	}
	return p, nil
}

// PublishedResults lists a student's current published term and annual
// results, newest first. scope is student, guardian or admin.
func (r *Results) PublishedResults(ctx context.Context, scope, actor, studentID string) ([]PublishedResult, error) {
	if !uuid.MatchString(actor) || (studentID != "" && !uuid.MatchString(studentID)) {
		return nil, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	studentID, err := (&GormRepository{db: r.db}).scopeStudent(tx, scope, actor, studentID)
	if err != nil {
		return nil, err
	}
	rows := []versionRow{}
	if err := tx.Table("result_versions v").Select(versionColumns).
		Where("v.student_id = ? AND v.scope IN ('term','annual') AND v.status = 'published'", studentID).
		Order("v.published_at DESC, v.id").Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := map[string]string{}
	for _, row := range rows {
		if _, ok := rules[row.PublicationID]; ok {
			continue
		}
		var text string
		if err := tx.Raw("SELECT rules::text FROM result_publications WHERE id = ?", row.PublicationID).Row().Scan(&text); err != nil {
			return nil, err
		}
		rules[row.PublicationID] = text
	}
	out := make([]PublishedResult, 0, len(rows))
	for _, row := range rows {
		p, err := publishedOf(row, rules[row.PublicationID])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
