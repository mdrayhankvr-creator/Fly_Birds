package exams

import (
	"context"
	"encoding/json"
)

// The E4 marks workspace reads: a teacher's work queue across exams, one
// sheet with everything the workspace needs (and what the actor may do), and
// the sheet's edit history from the audit trail. Writes stay on the E3 routes.

// QueueItem is one sheet in a teacher's work queue.
type QueueItem struct {
	ExamID       string  `json:"exam_id"`
	ExamName     string  `json:"exam_name"`
	ExamType     string  `json:"exam_type"`
	ExamStatus   string  `json:"exam_status"`
	SessionName  string  `json:"session_name"`
	ClassName    string  `json:"class_name"`
	SectionName  string  `json:"section_name"`
	SheetID      string  `json:"sheet_id"`
	SubjectID    string  `json:"subject_id"`
	SubjectEN    string  `json:"subject_name_en"`
	SubjectBN    string  `json:"subject_name_bn"`
	FullMarks    float64 `json:"full_marks"`
	Status       string  `json:"status"`
	Students     int     `json:"students"`
	Entered      int     `json:"entered"`
	Missing      int     `json:"missing"`
	ReturnReason string  `json:"return_reason"`
	UpdatedAt    *string `json:"updated_at"`
}

// SheetPermissions are what the actor may do on a sheet now, and why not.
type SheetPermissions struct {
	CanEdit   bool     `json:"can_edit"`
	CanSubmit bool     `json:"can_submit"`
	CanReview bool     `json:"can_review"` // approve or return
	Statuses  []string `json:"statuses"`   // statuses the actor may set
	// ReadOnlyReason explains a read-only sheet: exam_published, exam_locked,
	// sheet_submitted, sheet_approved or review_only.
	ReadOnlyReason string `json:"read_only_reason,omitempty"`
}

// Workspace is one sheet with its exam context and rows.
type Workspace struct {
	Exam        Exam             `json:"exam"`
	Sheet       Sheet            `json:"sheet"`
	Rows        []MarkRow        `json:"rows"`
	Permissions SheetPermissions `json:"permissions"`
}

// HistoryEntry is one audited change of a sheet.
type HistoryEntry struct {
	At          string          `json:"at"`
	Action      string          `json:"action"`
	Actor       string          `json:"actor"`
	StudentName string          `json:"student_name"`
	AdmissionNo string          `json:"admission_no"`
	Metadata    json.RawMessage `json:"metadata"`
}

func (s *Service) MySheets(ctx context.Context, actor string) ([]QueueItem, error) {
	if !uuid.MatchString(actor) {
		return nil, ErrInvalid
	}
	return s.repo.MySheets(ctx, actor)
}

func (s *Service) Workspace(ctx context.Context, examID, sheetID, actor, role, scope string) (Workspace, error) {
	if !uuid.MatchString(examID) || !uuid.MatchString(sheetID) || !uuid.MatchString(actor) {
		return Workspace{}, ErrInvalid
	}
	return s.repo.Workspace(ctx, examID, sheetID, actor, role, scope)
}

func (s *Service) SheetHistory(ctx context.Context, examID, sheetID, actor, role, scope string) ([]HistoryEntry, error) {
	if !uuid.MatchString(examID) || !uuid.MatchString(sheetID) || !uuid.MatchString(actor) {
		return nil, ErrInvalid
	}
	if _, err := s.repo.Workspace(ctx, examID, sheetID, actor, role, scope); err != nil {
		return nil, err // the same access rules as the sheet itself
	}
	return s.repo.SheetHistory(ctx, examID, sheetID)
}

// MySheets lists the sheets of the teacher's assigned subjects: draft exams
// of any session, and published or locked exams of the active session. Work
// that needs the teacher comes first.
func (r *GormRepository) MySheets(ctx context.Context, actor string) ([]QueueItem, error) {
	out := []QueueItem{}
	err := r.db.WithContext(ctx).Raw(`
SELECT e.id AS exam_id, e.name AS exam_name, e.exam_type, e.status AS exam_status, se.name AS session_name, c.name AS class_name, s.name AS section_name,
       es.id AS sheet_id, es.subject_id, sub.name_en AS subject_en, sub.name_bn AS subject_bn, es.full_marks, es.sheet_status AS status, es.return_reason,
       st.students, coalesce(m.entered, 0) AS entered, greatest(st.students - coalesce(m.entered, 0), 0) AS missing,
       to_char(m.updated_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD HH24:MI') AS updated_at
FROM exam_subjects es
JOIN exams e ON e.id = es.exam_id
JOIN academic_sessions se ON se.id = e.session_id
JOIN academic_classes c ON c.id = e.class_id
JOIN academic_sections s ON s.id = e.section_id
JOIN academic_subjects sub ON sub.id = es.subject_id
JOIN LATERAL (SELECT count(*) AS students FROM students x WHERE x.session_id = e.session_id AND x.class_id = e.class_id AND x.section_id = e.section_id AND x.status = 'active') st ON true
LEFT JOIN LATERAL (SELECT count(*) AS entered, max(mk.updated_at) AS updated_at FROM exam_marks mk JOIN students x ON x.id = mk.student_id AND x.status = 'active' WHERE mk.exam_subject_id = es.id) m ON true
WHERE EXISTS (SELECT 1 FROM employees emp JOIN teacher_assignments ta ON ta.employee_id = emp.id
              WHERE emp.user_id = ? AND emp.is_active AND ta.session_id = e.session_id AND ta.class_id = e.class_id AND ta.subject_id = es.subject_id)
  AND (e.status = 'draft' OR se.is_active)
ORDER BY (e.status <> 'draft'),
         CASE es.sheet_status WHEN 'returned' THEN 0 WHEN 'in_progress' THEN 1 WHEN 'not_started' THEN 2 WHEN 'submitted' THEN 3 ELSE 4 END,
         e.created_at DESC, c.name, s.name, sub.code
LIMIT 300`, actor).Scan(&out).Error
	return out, err
}

// Workspace returns one sheet for the actor. Teachers (scope "teacher") must
// be assigned to the subject; staff reach it through exams:read.
func (r *GormRepository) Workspace(ctx context.Context, examID, sheetID, actor, role, scope string) (Workspace, error) {
	tx := r.db.WithContext(ctx)
	var w Workspace
	exam, err := examByID(tx, examID)
	if err != nil {
		return w, err
	}
	view, err := sheetsIn(tx, examID, actor, scope)
	if err != nil {
		return w, err
	}
	found := false
	for _, s := range view.Sheets {
		if s.ID == sheetID {
			w.Sheet, found = s, true
		}
	}
	if !found {
		if scope == "teacher" {
			return w, ErrForbidden
		}
		return w, ErrNotFound
	}
	exam.Subjects = nil
	w.Exam = exam
	w.Rows = []MarkRow{}
	if err := tx.Table("students s").
		Joins("JOIN exam_subjects es ON es.id=?", sheetID).
		Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").
		Joins("LEFT JOIN exam_marks m ON m.student_id=s.id AND m.exam_subject_id=es.id").
		Where("s.session_id=? AND s.class_id=? AND s.section_id=? AND s.status='active'", exam.SessionID, exam.ClassID, exam.SectionID).
		Select(`es.id AS exam_subject_id,s.id AS student_id,s.name_en AS student_name,s.admission_no,s.roll_no,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,
 m.marks,m.grade,m.grade_point,m.passed,coalesce(m.status,'') AS status,coalesce(m.status_reason,'') AS status_reason,coalesce(m.row_version,0) AS row_version`).
		Order("s.roll_no NULLS LAST,s.admission_no").Scan(&w.Rows).Error; err != nil {
		return w, err
	}
	w.Permissions = permissionsFor(exam.Status, w.Sheet.Status, role)
	return w, nil
}

// permissionsFor mirrors the E3 rules the write routes enforce, so the
// workspace can explain a read-only sheet instead of failing on save.
func permissionsFor(examStatus, sheetStatus, role string) SheetPermissions {
	p := SheetPermissions{Statuses: []string{}}
	reviewer := role == "admin" || role == "head_teacher"
	switch {
	case examStatus == "published":
		p.ReadOnlyReason = "exam_published"
	case examStatus == "locked":
		p.ReadOnlyReason = "exam_locked"
	default:
		p.CanReview = reviewer && (sheetStatus == SheetSubmitted || sheetStatus == SheetApproved)
		switch {
		case role != "admin" && role != "teacher":
			p.ReadOnlyReason = "review_only"
		case sheetStatus == SheetSubmitted:
			p.ReadOnlyReason = "sheet_submitted"
		case sheetStatus == SheetApproved:
			p.ReadOnlyReason = "sheet_approved"
		default:
			p.CanEdit, p.CanSubmit = true, true
		}
	}
	if p.CanEdit {
		p.Statuses = []string{StatusPresent, StatusAbsent, StatusIncomplete}
		if role == "admin" {
			p.Statuses = append(p.Statuses, StatusExempt, StatusWithheld)
		}
	}
	return p
}

// SheetHistory returns the sheet's last 200 audited changes, newest first.
func (r *GormRepository) SheetHistory(ctx context.Context, examID, sheetID string) ([]HistoryEntry, error) {
	out := []HistoryEntry{}
	err := r.db.WithContext(ctx).Raw(`
SELECT to_char(a.created_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD HH24:MI:SS') AS at, a.action,
       coalesce(nullif(u.display_name, ''), u.email, '') AS actor,
       coalesce(st.name_en, '') AS student_name, coalesce(st.admission_no, '') AS admission_no, a.metadata
FROM audit_events a
LEFT JOIN users u ON u.id = a.actor_user_id
LEFT JOIN students st ON st.id::text = a.metadata->>'student_id'
WHERE a.target_type = 'exam' AND a.target_id = ?
  AND ((a.action = 'exam.mark_saved' AND a.metadata->>'exam_subject_id' = ?)
    OR (a.action IN ('exam.sheet_submitted', 'exam.sheet_returned', 'exam.sheet_approved') AND a.metadata->>'sheet_id' = ?))
ORDER BY a.created_at DESC, a.id DESC
LIMIT 200`, examID, sheetID, sheetID).Scan(&out).Error
	return out, err
}
