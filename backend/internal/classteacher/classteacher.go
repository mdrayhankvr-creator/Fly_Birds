// Package classteacher holds the class teacher of each section and their
// comments on students' term and annual results (marksheet v2, migration
// 1805). The administrator assigns class teachers. A class teacher writes
// one comment per student per result, from an editable preset or in their
// own words; the head teacher (or the administrator) reviews it. Comments
// are printed on marksheets and never change a published result.
package classteacher

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

var (
	ErrInvalid   = errors.New("invalid request")
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("not allowed")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Actor is who asks.
type Actor struct {
	ID   string
	Role string
}

func (a Actor) reviewer() bool { return a.Role == "admin" || a.Role == "head_teacher" }

// SectionTeacher is a section of a session with its class teacher (if any).
type SectionTeacher struct {
	SectionID    string  `json:"section_id"`
	SectionName  string  `json:"section_name"`
	ClassID      string  `json:"class_id"`
	ClassName    string  `json:"class_name"`
	SessionID    string  `json:"session_id"`
	EmployeeID   *string `json:"employee_id"`
	TeacherName  string  `json:"teacher_name"`
	TeacherBN    string  `json:"teacher_name_bn"`
	HasSignature bool    `json:"has_signature"`
}

// Preset is an editable preset comment.
type Preset struct {
	ID           string `json:"id"`
	TextEN       string `json:"text_en"`
	TextBN       string `json:"text_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}

// PresetInput creates or changes a preset.
type PresetInput struct {
	TextEN       string `json:"text_en"`
	TextBN       string `json:"text_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

// Scope is a result a comment belongs to: a term of the session, or the
// session's annual result (scope_id is then the session id).
type Scope struct {
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id"`
	Name    string `json:"name"`
}

// MySection is a section the actor may comment on, with its results.
type MySection struct {
	SectionID   string  `json:"section_id"`
	SectionName string  `json:"section_name"`
	ClassID     string  `json:"class_id"`
	ClassName   string  `json:"class_name"`
	SessionID   string  `json:"session_id"`
	SessionName string  `json:"session_name"`
	TeacherName string  `json:"teacher_name"`
	Scopes      []Scope `json:"scopes" gorm:"-"`
}

// Comment is one student's comment on one result.
type Comment struct {
	StudentID   string  `json:"student_id"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	AdmissionNo string  `json:"admission_no"`
	RollNo      *int    `json:"roll_no"`
	PresetID    *string `json:"preset_id"`
	CustomText  string  `json:"custom_text"`
	Status      string  `json:"status"` // "", written or reviewed
	WrittenBy   string  `json:"written_by"`
	WrittenAt   *string `json:"written_at"`
	ReviewedBy  string  `json:"reviewed_by"`
	ReviewedAt  *string `json:"reviewed_at"`
	GPA         *string `json:"gpa"`
	Outcome     string  `json:"outcome"`
}

// Sheet is a section's comments on one result.
type Sheet struct {
	Scope       string    `json:"scope"`
	ScopeID     string    `json:"scope_id"`
	SectionID   string    `json:"section_id"`
	SectionName string    `json:"section_name"`
	ClassName   string    `json:"class_name"`
	CanWrite    bool      `json:"can_write"`
	CanReview   bool      `json:"can_review"`
	Presets     []Preset  `json:"presets"`
	Students    []Comment `json:"students"`
}

// CommentInput writes or clears a comment: a preset, own text, or neither
// (clears it).
type CommentInput struct {
	Scope      string  `json:"scope"`
	ScopeID    string  `json:"scope_id"`
	StudentID  string  `json:"student_id"`
	PresetID   *string `json:"preset_id"`
	CustomText string  `json:"custom_text"`
}

// ReviewInput marks a section's written comments on a result reviewed.
type ReviewInput struct {
	Scope      string   `json:"scope"`
	ScopeID    string   `json:"scope_id"`
	SectionID  string   `json:"section_id"`
	StudentIDs []string `json:"student_ids"` // empty: every written comment
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func dbError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23503":
			return ErrNotFound
		case "23505", "23514", "23502", "22P02", "22007":
			return ErrInvalid
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func textOK(s string, max int) bool {
	return s == strings.TrimSpace(s) && utf8.RuneCountInString(s) <= max
}

// ---------------------------------------------------------------------------
// Class teachers

// Sections lists a session's sections with their class teachers.
func (s *Service) Sections(ctx context.Context, sessionID string) ([]SectionTeacher, error) {
	if !uuidPattern.MatchString(sessionID) {
		return nil, ErrInvalid
	}
	out := []SectionTeacher{}
	err := s.db.WithContext(ctx).Raw(`SELECT sec.id AS section_id, sec.name AS section_name, c.id AS class_id, c.name AS class_name, sec.session_id,
 ct.employee_id, coalesce(e.name_en, '') AS teacher_name, coalesce(e.name_bn, '') AS teacher_bn, (sig.employee_id IS NOT NULL) AS has_signature
 FROM academic_sections sec JOIN academic_classes c ON c.id = sec.class_id
 LEFT JOIN section_class_teachers ct ON ct.section_id = sec.id
 LEFT JOIN employees e ON e.id = ct.employee_id
 LEFT JOIN employee_signatures sig ON sig.employee_id = ct.employee_id
 WHERE sec.session_id = ? AND sec.is_active
 ORDER BY c.display_order, c.name, sec.display_order, sec.name`, sessionID).Scan(&out).Error
	return out, err
}

// Assign makes an active teacher the class teacher of a section, or clears
// it when employeeID is empty. Administrators only.
func (s *Service) Assign(ctx context.Context, a Actor, sectionID, employeeID string) (SectionTeacher, error) {
	if a.Role != "admin" {
		return SectionTeacher{}, ErrForbidden
	}
	if !uuidPattern.MatchString(sectionID) || (employeeID != "" && !uuidPattern.MatchString(employeeID)) {
		return SectionTeacher{}, ErrInvalid
	}
	var session string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sec struct{ ClassID, SessionID string }
		if err := tx.Raw("SELECT class_id, session_id FROM academic_sections WHERE id = ?", sectionID).Scan(&sec).Error; err != nil {
			return err
		}
		if sec.ClassID == "" {
			return ErrNotFound
		}
		session = sec.SessionID
		if employeeID == "" {
			if err := tx.Exec("DELETE FROM section_class_teachers WHERE section_id = ?", sectionID).Error; err != nil {
				return err
			}
			return audit.Record(tx, &a.ID, "class_teacher.cleared", "academic_section", sectionID, nil)
		}
		var ok int64
		if err := tx.Raw("SELECT count(*) FROM employees WHERE id = ? AND kind = 'teacher' AND is_active", employeeID).Scan(&ok).Error; err != nil {
			return err
		}
		if ok == 0 {
			return ErrInvalid
		}
		if err := tx.Exec(`INSERT INTO section_class_teachers (section_id, class_id, session_id, employee_id, assigned_by) VALUES (?, ?, ?, ?, ?)
 ON CONFLICT (section_id) DO UPDATE SET employee_id = EXCLUDED.employee_id, assigned_by = EXCLUDED.assigned_by, assigned_at = now()`,
			sectionID, sec.ClassID, sec.SessionID, employeeID, a.ID).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &a.ID, "class_teacher.assigned", "academic_section", sectionID, map[string]any{"employee_id": employeeID})
	})
	if err != nil {
		return SectionTeacher{}, err
	}
	rows, err := s.Sections(ctx, session)
	if err != nil {
		return SectionTeacher{}, err
	}
	for _, r := range rows {
		if r.SectionID == sectionID {
			return r, nil
		}
	}
	return SectionTeacher{}, ErrNotFound
}

// ---------------------------------------------------------------------------
// Presets

func (s *Service) Presets(ctx context.Context, all bool) ([]Preset, error) {
	out := []Preset{}
	q := "SELECT id, text_en, text_bn, display_order, is_active FROM result_comment_presets"
	if !all {
		q += " WHERE is_active"
	}
	err := s.db.WithContext(ctx).Raw(q + " ORDER BY display_order, created_at, id").Scan(&out).Error
	return out, err
}

// SavePreset creates (id "") or changes a preset. Administrators and the
// head teacher.
func (s *Service) SavePreset(ctx context.Context, a Actor, id string, in PresetInput) (Preset, error) {
	if !a.reviewer() {
		return Preset{}, ErrForbidden
	}
	in.TextEN, in.TextBN = strings.TrimSpace(in.TextEN), strings.TrimSpace(in.TextBN)
	if in.TextEN == "" || in.TextBN == "" || !textOK(in.TextEN, 300) || !textOK(in.TextBN, 300) || in.DisplayOrder < 0 || in.DisplayOrder > 10000 || (id != "" && !uuidPattern.MatchString(id)) {
		return Preset{}, ErrInvalid
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var out Preset
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if id == "" {
			err = tx.Raw(`INSERT INTO result_comment_presets (text_en, text_bn, display_order, is_active, updated_by) VALUES (?, ?, ?, ?, ?)
 RETURNING id, text_en, text_bn, display_order, is_active`, in.TextEN, in.TextBN, in.DisplayOrder, active, a.ID).Scan(&out).Error
		} else {
			err = tx.Raw(`UPDATE result_comment_presets SET text_en = ?, text_bn = ?, display_order = ?, is_active = ?, updated_by = ?, updated_at = now() WHERE id = ?
 RETURNING id, text_en, text_bn, display_order, is_active`, in.TextEN, in.TextBN, in.DisplayOrder, active, a.ID, id).Scan(&out).Error
		}
		if err != nil {
			return dbError(err)
		}
		if out.ID == "" {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "result_comment.preset_saved", "result_comment_preset", out.ID, map[string]any{"active": active})
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Comments

// classTeacherOf reports whether the actor is the class teacher of the
// section.
func (s *Service) classTeacherOf(tx *gorm.DB, a Actor, sectionID string) (bool, error) {
	if a.Role != "teacher" && a.Role != "head_teacher" {
		return false, nil
	}
	var n int64
	err := tx.Raw(`SELECT count(*) FROM section_class_teachers ct JOIN employees e ON e.id = ct.employee_id
 WHERE ct.section_id = ? AND e.user_id = ? AND e.is_active`, sectionID, a.ID).Scan(&n).Error
	return n > 0, err
}

// MySections lists the sections the actor may comment on (a class teacher's
// own) or review (head teacher and administrator: every section of the
// session), each with its results.
func (s *Service) MySections(ctx context.Context, a Actor, sessionID string) ([]MySection, error) {
	if !uuidPattern.MatchString(a.ID) || (sessionID != "" && !uuidPattern.MatchString(sessionID)) {
		return nil, ErrInvalid
	}
	db := s.db.WithContext(ctx)
	q := `SELECT sec.id AS section_id, sec.name AS section_name, c.id AS class_id, c.name AS class_name, ses.id AS session_id, ses.name AS session_name, coalesce(e.name_en, '') AS teacher_name
 FROM academic_sections sec JOIN academic_classes c ON c.id = sec.class_id JOIN academic_sessions ses ON ses.id = sec.session_id
 LEFT JOIN section_class_teachers ct ON ct.section_id = sec.id LEFT JOIN employees e ON e.id = ct.employee_id
 WHERE sec.is_active`
	args := []any{}
	if !a.reviewer() {
		if a.Role != "teacher" {
			return nil, ErrForbidden
		}
		q += " AND e.user_id = ? AND e.is_active"
		args = append(args, a.ID)
	} else if sessionID == "" {
		// Reviewers see the current session unless they choose one.
		q += " AND ses.id = (SELECT id FROM academic_sessions WHERE is_active LIMIT 1)"
	}
	if sessionID != "" {
		q += " AND ses.id = ?"
		args = append(args, sessionID)
	}
	q += " ORDER BY ses.start_date DESC, c.display_order, c.name, sec.display_order, sec.name"
	out := []MySection{}
	if err := db.Raw(q, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	scopes := map[string][]Scope{}
	for i := range out {
		sid := out[i].SessionID
		if _, ok := scopes[sid]; !ok {
			list := []Scope{}
			if err := db.Raw(`SELECT 'term' AS scope, id AS scope_id, name FROM exam_terms WHERE session_id = ? ORDER BY sequence, name`, sid).Scan(&list).Error; err != nil {
				return nil, err
			}
			list = append(list, Scope{Scope: "annual", ScopeID: sid, Name: out[i].SessionName})
			scopes[sid] = list
		}
		out[i].Scopes = scopes[sid]
	}
	return out, nil
}

// sessionOfScope is the session a result scope belongs to.
func sessionOfScope(tx *gorm.DB, scope, scopeID string) (string, error) {
	if !uuidPattern.MatchString(scopeID) {
		return "", ErrInvalid
	}
	var id string
	switch scope {
	case "term":
		if err := tx.Raw("SELECT session_id FROM exam_terms WHERE id = ?", scopeID).Scan(&id).Error; err != nil {
			return "", err
		}
	case "annual":
		if err := tx.Raw("SELECT id FROM academic_sessions WHERE id = ?", scopeID).Scan(&id).Error; err != nil {
			return "", err
		}
	default:
		return "", ErrInvalid
	}
	if id == "" {
		return "", ErrNotFound
	}
	return id, nil
}

// access is what the actor may do with a section's comments.
func (s *Service) access(tx *gorm.DB, a Actor, sectionID string) (write, review bool, err error) {
	write, err = s.classTeacherOf(tx, a, sectionID)
	if err != nil {
		return false, false, err
	}
	return write, a.reviewer(), nil
}

// Sheet is a section's students with their comments on one result.
func (s *Service) Sheet(ctx context.Context, a Actor, scope, scopeID, sectionID string) (Sheet, error) {
	if !uuidPattern.MatchString(sectionID) {
		return Sheet{}, ErrInvalid
	}
	db := s.db.WithContext(ctx)
	session, err := sessionOfScope(db, scope, scopeID)
	if err != nil {
		return Sheet{}, err
	}
	var sec struct{ Name, ClassName, SessionID string }
	if err := db.Raw("SELECT sec.name, c.name AS class_name, sec.session_id FROM academic_sections sec JOIN academic_classes c ON c.id = sec.class_id WHERE sec.id = ?", sectionID).Scan(&sec).Error; err != nil {
		return Sheet{}, err
	}
	if sec.Name == "" {
		return Sheet{}, ErrNotFound
	}
	if sec.SessionID != session {
		return Sheet{}, ErrInvalid
	}
	write, review, err := s.access(db, a, sectionID)
	if err != nil {
		return Sheet{}, err
	}
	if !write && !review {
		return Sheet{}, ErrForbidden
	}
	presets, err := s.Presets(ctx, false)
	if err != nil {
		return Sheet{}, err
	}
	out := Sheet{Scope: scope, ScopeID: scopeID, SectionID: sectionID, SectionName: sec.Name, ClassName: sec.ClassName, CanWrite: write || review, CanReview: review, Presets: presets, Students: []Comment{}}
	if err := db.Raw(`SELECT st.id AS student_id, st.name_en, st.name_bn, st.admission_no, st.roll_no,
 rc.preset_id, coalesce(rc.custom_text, '') AS custom_text, coalesce(rc.status, '') AS status,
 coalesce(wu.display_name, '') AS written_by, to_char(rc.written_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS written_at,
 coalesce(ru.display_name, '') AS reviewed_by, to_char(rc.reviewed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS reviewed_at,
 to_char(rv.gpa, 'FM0.00') AS gpa, coalesce(rv.outcome, '') AS outcome
 FROM students st
 LEFT JOIN result_comments rc ON rc.student_id = st.id AND rc.scope = ? AND rc.scope_id = ?
 LEFT JOIN users wu ON wu.id = rc.written_by LEFT JOIN users ru ON ru.id = rc.reviewed_by
 LEFT JOIN result_versions rv ON rv.student_id = st.id AND rv.scope = ? AND rv.scope_id = ? AND rv.status = 'published'
 WHERE st.section_id = ? AND st.session_id = ? AND st.status = 'active'
 ORDER BY st.roll_no NULLS LAST, st.admission_no`, scope, scopeID, scope, scopeID, sectionID, session).Scan(&out.Students).Error; err != nil {
		return Sheet{}, err
	}
	return out, nil
}

// Save writes (or clears) a student's comment. The class teacher writes;
// the head teacher and the administrator may correct one, which counts as
// reviewed. A teacher's change after review needs reviewing again.
func (s *Service) Save(ctx context.Context, a Actor, in CommentInput) (Comment, error) {
	in.CustomText = strings.TrimSpace(in.CustomText)
	if !uuidPattern.MatchString(in.StudentID) || !textOK(in.CustomText, 400) || (in.PresetID != nil && !uuidPattern.MatchString(*in.PresetID)) {
		return Comment{}, ErrInvalid
	}
	if in.PresetID != nil && in.CustomText != "" {
		return Comment{}, ErrInvalid
	}
	var student struct{ SectionID, SessionID string }
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, err := sessionOfScope(tx, in.Scope, in.ScopeID)
		if err != nil {
			return err
		}
		if err := tx.Raw("SELECT section_id, session_id FROM students WHERE id = ? AND status = 'active'", in.StudentID).Scan(&student).Error; err != nil {
			return err
		}
		if student.SectionID == "" {
			return ErrNotFound
		}
		if student.SessionID != session {
			return ErrInvalid
		}
		write, review, err := s.access(tx, a, student.SectionID)
		if err != nil {
			return err
		}
		if !write && !review {
			return ErrForbidden
		}
		if in.PresetID == nil && in.CustomText == "" {
			res := tx.Exec("DELETE FROM result_comments WHERE scope = ? AND scope_id = ? AND student_id = ?", in.Scope, in.ScopeID, in.StudentID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			return audit.Record(tx, &a.ID, "result_comment.cleared", "student", in.StudentID, map[string]any{"scope": in.Scope, "scope_id": in.ScopeID})
		}
		if in.PresetID != nil {
			var ok int64
			if err := tx.Raw("SELECT count(*) FROM result_comment_presets WHERE id = ? AND is_active", *in.PresetID).Scan(&ok).Error; err != nil {
				return err
			}
			if ok == 0 {
				return ErrInvalid
			}
		}
		// A reviewer's own words are reviewed at once.
		status := "written"
		var reviewer any
		if review && !write {
			status, reviewer = "reviewed", a.ID
		}
		if err := tx.Exec(`INSERT INTO result_comments (scope, scope_id, student_id, session_id, section_id, preset_id, custom_text, status, written_by, reviewed_by, reviewed_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = 'reviewed' THEN now() END)
 ON CONFLICT (scope, scope_id, student_id) DO UPDATE SET section_id = EXCLUDED.section_id, preset_id = EXCLUDED.preset_id, custom_text = EXCLUDED.custom_text,
   status = EXCLUDED.status, written_by = EXCLUDED.written_by, written_at = now(), reviewed_by = EXCLUDED.reviewed_by, reviewed_at = EXCLUDED.reviewed_at`,
			in.Scope, in.ScopeID, in.StudentID, session, student.SectionID, in.PresetID, in.CustomText, status, a.ID, reviewer, status).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &a.ID, "result_comment.written", "student", in.StudentID, map[string]any{"scope": in.Scope, "scope_id": in.ScopeID, "preset": in.PresetID != nil, "status": status})
	})
	if err != nil {
		return Comment{}, err
	}
	sheet, err := s.Sheet(ctx, a, in.Scope, in.ScopeID, student.SectionID)
	if err != nil {
		return Comment{}, err
	}
	for _, c := range sheet.Students {
		if c.StudentID == in.StudentID {
			return c, nil
		}
	}
	return Comment{}, ErrNotFound
}

// Review marks written comments of a section reviewed (head teacher and
// administrator). It returns how many it marked.
func (s *Service) Review(ctx context.Context, a Actor, in ReviewInput) (int64, error) {
	if !a.reviewer() {
		return 0, ErrForbidden
	}
	if !uuidPattern.MatchString(in.SectionID) || len(in.StudentIDs) > 500 {
		return 0, ErrInvalid
	}
	for _, id := range in.StudentIDs {
		if !uuidPattern.MatchString(id) {
			return 0, ErrInvalid
		}
	}
	var n int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := sessionOfScope(tx, in.Scope, in.ScopeID); err != nil {
			return err
		}
		q := tx.Exec
		var res *gorm.DB
		if len(in.StudentIDs) == 0 {
			res = q(`UPDATE result_comments SET status = 'reviewed', reviewed_by = ?, reviewed_at = now() WHERE scope = ? AND scope_id = ? AND section_id = ? AND status = 'written'`,
				a.ID, in.Scope, in.ScopeID, in.SectionID)
		} else {
			res = q(`UPDATE result_comments SET status = 'reviewed', reviewed_by = ?, reviewed_at = now() WHERE scope = ? AND scope_id = ? AND section_id = ? AND status = 'written' AND student_id IN ?`,
				a.ID, in.Scope, in.ScopeID, in.SectionID, in.StudentIDs)
		}
		if res.Error != nil {
			return res.Error
		}
		n = res.RowsAffected
		if n == 0 {
			return nil
		}
		return audit.Record(tx, &a.ID, "result_comment.reviewed", "academic_section", in.SectionID, map[string]any{"scope": in.Scope, "scope_id": in.ScopeID, "count": n})
	})
	return n, err
}
