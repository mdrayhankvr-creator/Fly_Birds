package exams

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Corrections (E6). An administrator opens a correction for one student of
// a published (or locked) exam with a reason; that student's marks in the
// exam can then be changed by an administrator, nobody else's. The change
// reaches families only when the correction is reviewed, approved and
// published as a new version; cancelling it restores the published marks.

// Correction is one opened correction.
type Correction struct {
	ID            string           `json:"id"`
	ExamID        string           `json:"exam_id"`
	StudentID     string           `json:"student_id"`
	StudentName   string           `json:"student_name"`
	AdmissionNo   string           `json:"admission_no"`
	RollNo        *int             `json:"roll_no"`
	Reason        string           `json:"reason"`
	Status        string           `json:"status"`
	OpenedBy      string           `json:"opened_by"`
	OpenedAt      string           `json:"opened_at"`
	ClosedAt      *string          `json:"closed_at"`
	PublicationID *string          `json:"publication_id"`
	Cells         []CorrectionCell `json:"cells" gorm:"-"`
}

// CorrectionCell is one of the student's marks with its published value.
type CorrectionCell struct {
	ExamSubjectID   string          `json:"exam_subject_id"`
	SubjectID       string          `json:"subject_id"`
	SubjectName     string          `json:"subject_name"`
	SubjectNameBN   string          `json:"subject_name_bn"`
	FullMarks       float64         `json:"full_marks"`
	Status          string          `json:"status"`
	Marks           *float64        `json:"marks"`
	Reason          string          `json:"reason"`
	RowVersion      int             `json:"row_version"`
	Components      []CellComponent `json:"components" gorm:"-"`
	PublishedStatus string          `json:"published_status"`
	PublishedMarks  *string         `json:"published_marks"`
	PublishedGrade  *string         `json:"published_grade"`
}

// CellComponent is one component of a component sheet.
type CellComponent struct {
	Key   string   `json:"key"`
	Full  float64  `json:"full"`
	Marks *float64 `json:"marks"`
}

func corrections(tx *gorm.DB, examID string, openOnly bool) ([]Correction, error) {
	out := []Correction{}
	q := tx.Table("result_corrections c").Joins("JOIN students st ON st.id = c.student_id").
		Select(`c.id, c.exam_id, c.student_id, st.name_en AS student_name, st.admission_no, st.roll_no, c.reason, c.status,
 (SELECT coalesce(NULLIF(u.display_name, ''), u.email) FROM users u WHERE u.id = c.opened_by) AS opened_by,
 to_char(c.opened_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS opened_at, to_char(c.closed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS closed_at, c.publication_id`).
		Where("c.exam_id = ?", examID)
	if openOnly {
		q = q.Where("c.status = 'open'")
	}
	if err := q.Order("c.opened_at DESC, c.id").Find(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Cells = []CorrectionCell{}
		if out[i].Status != "open" {
			continue
		}
		cells, err := correctionCells(tx, examID, out[i].StudentID)
		if err != nil {
			return nil, err
		}
		out[i].Cells = cells
	}
	return out, nil
}

// correctionCells are a student's current marks in an exam beside the
// published values.
func correctionCells(tx *gorm.DB, examID, studentID string) ([]CorrectionCell, error) {
	cells := []CorrectionCell{}
	if err := tx.Raw(`SELECT es.id AS exam_subject_id, es.subject_id, sub.name_en AS subject_name, sub.name_bn AS subject_name_bn, es.full_marks,
       coalesce(m.status, '') AS status, m.marks, coalesce(m.status_reason, '') AS reason, coalesce(m.row_version, 0) AS row_version
  FROM exam_subjects es JOIN exams e ON e.id = es.exam_id JOIN academic_subjects sub ON sub.id = es.subject_id
  LEFT JOIN exam_marks m ON m.exam_subject_id = es.id AND m.student_id = ?
 WHERE es.exam_id = ? AND student_takes_subject(?, e.session_id, e.class_id, es.subject_id)
 ORDER BY sub.code, es.subject_id`, studentID, examID, studentID).Scan(&cells).Error; err != nil {
		return nil, err
	}
	comps := []struct {
		SheetID, Key string
		Full         float64
		Marks        *float64
	}{}
	if err := tx.Raw(`SELECT c.exam_subject_id AS sheet_id, c.component_key AS key, c.full_marks AS full, cm.marks
  FROM exam_subject_components c JOIN exam_subjects es ON es.id = c.exam_subject_id
  LEFT JOIN exam_marks m ON m.exam_subject_id = c.exam_subject_id AND m.student_id = ?
  LEFT JOIN exam_component_marks cm ON cm.exam_mark_id = m.id AND cm.component_key = c.component_key
 WHERE es.exam_id = ? ORDER BY c.display_order, c.component_key`, studentID, examID).Scan(&comps).Error; err != nil {
		return nil, err
	}
	bySheet := map[string][]CellComponent{}
	for _, c := range comps {
		bySheet[c.SheetID] = append(bySheet[c.SheetID], CellComponent{Key: c.Key, Full: c.Full, Marks: c.Marks})
	}
	current, err := currentVersions(tx, ScopeAssessment, examID, "")
	if err != nil {
		return nil, err
	}
	published := map[string]OutLine{}
	if v, ok := current[studentID]; ok {
		snap, err := v.snapshot()
		if err != nil {
			return nil, err
		}
		var in AssessmentInputs
		_ = json.Unmarshal(snap.Inputs, &in)
		for _, l := range in.Lines {
			published[l.SheetID] = OutLine{Status: l.Status, Marks: l.Marks}
		}
		for _, l := range snap.Outputs.Subjects {
			if p, ok := published[l.SheetID]; ok {
				p.Grade = l.Grade
				published[l.SheetID] = p
			}
		}
	}
	for i := range cells {
		cells[i].Components = bySheet[cells[i].ExamSubjectID]
		if cells[i].Components == nil {
			cells[i].Components = []CellComponent{}
		}
		if p, ok := published[cells[i].ExamSubjectID]; ok {
			cells[i].PublishedStatus, cells[i].PublishedMarks, cells[i].PublishedGrade = p.Status, p.Marks, p.Grade
		}
	}
	return cells, nil
}

// Corrections lists an exam's corrections, newest first.
func (r *Results) Corrections(ctx context.Context, examID string) ([]Correction, error) {
	if !uuid.MatchString(examID) {
		return nil, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	if _, err := examByID(tx, examID); err != nil {
		return nil, err
	}
	out, err := corrections(tx, examID, false)
	return out, dbErr(err)
}

// OpenCorrection opens a correction for one student of a published exam.
func (r *Results) OpenCorrection(ctx context.Context, actor, examID, studentID, reason string) (Correction, error) {
	reason = strings.TrimSpace(reason)
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !uuid.MatchString(studentID) || !utf8.ValidString(reason) {
		return Correction{}, ErrInvalid
	}
	if n := utf8.RuneCountInString(reason); n < 3 || n > 500 {
		return Correction{}, ErrReasonRequired
	}
	var out Correction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		e, err := examByID(tx, examID)
		if err != nil {
			return err
		}
		if e.Status == "draft" {
			return ErrNotPublished
		}
		var versions int64
		if err := tx.Table("result_versions").Where("scope = 'assessment' AND scope_id = ? AND student_id = ? AND status = 'published'", examID, studentID).Count(&versions).Error; err != nil {
			return err
		}
		if versions == 0 {
			return ErrNotFound // only a published student's result can be corrected
		}
		var id string
		if err := tx.Raw("INSERT INTO result_corrections (exam_id, student_id, reason, opened_by) VALUES (?, ?, ?, ?) RETURNING id", examID, studentID, reason, actor).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		action := "exam.correction_opened"
		if e.Status == "locked" {
			action = "exam.locked_correction_opened"
		}
		if err := recordAudit(tx, actor, action, "exam", examID, map[string]any{"correction_id": id, "student_id": studentID, "reason": reason}); err != nil {
			return err
		}
		list, err := corrections(tx, examID, true)
		if err != nil {
			return err
		}
		for _, c := range list {
			if c.ID == id {
				out = c
			}
		}
		return nil
	})
	return out, dbErr(err)
}

// CancelCorrection closes an open correction without publishing and puts
// the student's marks back to their published values.
func (r *Results) CancelCorrection(ctx context.Context, actor, examID, correctionID string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !uuid.MatchString(correctionID) {
		return ErrInvalid
	}
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		var c struct{ StudentID, Status string }
		if err := tx.Raw("SELECT student_id, status FROM result_corrections WHERE id = ? AND exam_id = ? FOR UPDATE", correctionID, examID).Scan(&c).Error; err != nil {
			return err
		}
		if c.StudentID == "" {
			return ErrNotFound
		}
		if c.Status != "open" {
			return ErrConflict
		}
		current, err := currentVersions(tx, ScopeAssessment, examID, "")
		if err != nil {
			return err
		}
		restored := 0
		if v, ok := current[c.StudentID]; ok {
			snap, err := v.snapshot()
			if err != nil {
				return err
			}
			var in AssessmentInputs
			if err := json.Unmarshal(snap.Inputs, &in); err != nil {
				return err
			}
			for _, l := range in.Lines {
				res := tx.Exec(`UPDATE exam_marks SET status = ?, marks = ?::numeric, status_reason = ?, row_version = row_version + 1, updated_by = ?, updated_at = now()
 WHERE id = ? AND (status IS DISTINCT FROM ? OR marks IS DISTINCT FROM ?::numeric OR status_reason IS DISTINCT FROM ?)`,
					l.Status, l.Marks, l.Reason, actor, l.MarkID, l.Status, l.Marks, l.Reason)
				if res.Error != nil {
					return dbErr(res.Error)
				}
				restored += int(res.RowsAffected)
				// The published grade comes back with the published mark.
				var grade, point *string
				var passed *bool
				for _, o := range snap.Outputs.Subjects {
					if o.SheetID == l.SheetID {
						grade, point, passed = o.Grade, o.Point, o.Passed
					}
				}
				if err := tx.Exec("UPDATE exam_marks SET grade = ?, grade_point = ?::numeric, passed = ? WHERE id = ?", grade, point, passed, l.MarkID).Error; err != nil {
					return err
				}
				if len(l.Components) > 0 {
					if err := tx.Exec("DELETE FROM exam_component_marks WHERE exam_mark_id = ?", l.MarkID).Error; err != nil {
						return err
					}
					for _, comp := range l.Components {
						if comp.Marks == nil {
							continue
						}
						if err := tx.Exec("INSERT INTO exam_component_marks (exam_mark_id, exam_subject_id, component_key, marks) VALUES (?, ?, ?, ?::numeric)", l.MarkID, l.SheetID, comp.Key, *comp.Marks).Error; err != nil {
							return dbErr(err)
						}
					}
				}
			}
		}
		if err := tx.Exec("UPDATE result_corrections SET status = 'cancelled', closed_by = ?, closed_at = now() WHERE id = ?", actor, correctionID).Error; err != nil {
			return err
		}
		return recordAudit(tx, actor, "exam.correction_cancelled", "exam", examID, map[string]any{"correction_id": correctionID, "student_id": c.StudentID, "marks_restored": restored})
	}))
}

// openCorrectionStudents are the students whose marks an open correction
// lets an administrator change in a published exam.
func openCorrectionStudents(tx *gorm.DB, examID string) (map[string]string, error) {
	rows := []struct{ ID, StudentID string }{}
	if err := tx.Raw("SELECT id, student_id FROM result_corrections WHERE exam_id = ? AND status = 'open'", examID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.StudentID] = r.ID
	}
	return out, nil
}
