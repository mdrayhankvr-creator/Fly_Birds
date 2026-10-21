package exams

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }
func dbErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22P02":
			return ErrInvalid
		}
	}
	return err
}
func lock(tx *gorm.DB) error {
	var unused any
	return tx.Raw("SELECT pg_advisory_xact_lock(517007)").Row().Scan(&unused)
}
func examByID(tx *gorm.DB, id string) (Exam, error) {
	var e Exam
	err := tx.Table("exams e").Joins("JOIN academic_sessions se ON se.id=e.session_id").Joins("JOIN academic_classes c ON c.id=e.class_id").Joins("JOIN academic_sections s ON s.id=e.section_id").Select("e.id,e.name,e.exam_type,e.session_id,e.class_id,e.section_id,e.routine_id,e.status,se.name AS session_name,c.name AS class_name,s.name AS section_name").Where("e.id=?", id).Take(&e).Error
	if err != nil {
		return Exam{}, dbErr(err)
	}
	e.Subjects = []Subject{}
	err = tx.Table("exam_subjects es").Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").Select("es.id,es.exam_id,es.subject_id,sub.name_en,sub.name_bn,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth").Where("es.exam_id=?", id).Order("sub.code").Find(&e.Subjects).Error
	return e, err
}
func (r *GormRepository) GetExam(ctx context.Context, id string) (Exam, error) {
	return examByID(r.db.WithContext(ctx), id)
}
func (r *GormRepository) SaveExam(ctx context.Context, actor, id string, in ExamInput) (Exam, error) {
	var out Exam
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if id != "" {
			old, err := examByID(tx, id)
			if err != nil {
				return err
			}
			if old.Status != "draft" || old.SessionID != in.SessionID || old.ClassID != in.ClassID || old.SectionID != in.SectionID {
				return ErrConflict
			}
		}
		var count int64
		if err := tx.Table("academic_sections s").Joins("JOIN academic_classes c ON c.id=s.class_id").Where("s.id=? AND s.class_id=? AND s.session_id=? AND s.is_active AND c.is_active", in.SectionID, in.ClassID, in.SessionID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrInvalid
		}
		if in.RoutineID != nil && *in.RoutineID != "" {
			if err := tx.Table("exam_routines").Where("id=? AND session_id=? AND class_id=? AND section_id=?", *in.RoutineID, in.SessionID, in.ClassID, in.SectionID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrInvalid
			}
		}
		if err := tx.Table("exams").Where("session_id=? AND section_id=? AND lower(name)=lower(?) AND id::text IS DISTINCT FROM ?", in.SessionID, in.SectionID, in.Name, id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrConflict
		}
		var routine any
		if in.RoutineID != nil && *in.RoutineID != "" {
			routine = *in.RoutineID
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO exams(name,exam_type,session_id,class_id,section_id,routine_id,created_by,updated_by) VALUES (?,?,?,?,?,?,?,?) RETURNING id", in.Name, in.ExamType, in.SessionID, in.ClassID, in.SectionID, routine, actor, actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			if err := tx.Exec("UPDATE exams SET name=?,exam_type=?,routine_id=?,updated_by=?,updated_at=now() WHERE id=?", in.Name, in.ExamType, routine, actor, id).Error; err != nil {
				return dbErr(err)
			}
		}
		var err error
		out, err = examByID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.saved", "exam", id, map[string]any{"session_id": in.SessionID, "section_id": in.SectionID})
	})
	return out, dbErr(err)
}
func (r *GormRepository) DeleteExam(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if _, err := lockExam(tx, id, "UPDATE"); err != nil {
			return err
		}
		e, err := examByID(tx, id)
		if err != nil {
			return err
		}
		if e.Status != "draft" {
			return ErrConflict
		}
		res := tx.Exec("DELETE FROM exams WHERE id=?", id)
		if res.Error != nil {
			return res.Error
		}
		return audit.Record(tx, &actor, "exam.deleted", "exam", id, nil)
	}))
}
func (r *GormRepository) ListExams(ctx context.Context, q Query, scope, actor string) (Page[Exam], error) {
	out := Page[Exam]{Items: []Exam{}, Page: q.Page, PageSize: q.PageSize}
	base := r.db.WithContext(ctx).Table("exams e").Joins("JOIN academic_sessions se ON se.id=e.session_id").Joins("JOIN academic_classes c ON c.id=e.class_id").Joins("JOIN academic_sections s ON s.id=e.section_id")
	if scope == "teacher" {
		base = base.Where("EXISTS (SELECT 1 FROM employees emp JOIN teacher_assignments ta ON ta.employee_id=emp.id JOIN exam_subjects es ON es.exam_id=e.id AND es.subject_id=ta.subject_id WHERE emp.user_id=? AND emp.is_active AND ta.session_id=e.session_id AND ta.class_id=e.class_id)", actor)
	}
	if q.SessionID != "" {
		base = base.Where("e.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("e.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("e.section_id=?", q.SectionID)
	}
	if q.Search != "" {
		base = base.Where("e.name ILIKE ?", "%"+q.Search+"%")
	}
	if err := base.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := base.Select("e.id,e.name,e.exam_type,e.session_id,e.class_id,e.section_id,e.routine_id,e.status,se.name AS session_name,c.name AS class_name,s.name AS section_name").Order("e.created_at DESC,e.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&out.Items).Error
	return out, err
}
func (r *GormRepository) SaveSubject(ctx context.Context, actor, examID string, in SubjectInput) (Exam, error) {
	var out Exam
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if _, err := lockExam(tx, examID, "UPDATE"); err != nil {
			return err
		}
		e, err := examByID(tx, examID)
		if err != nil {
			return err
		}
		if e.Status != "draft" {
			return ErrConflict
		}
		var fourth, count int64
		if err := tx.Table("academic_class_subjects acs").Joins("JOIN academic_subjects sub ON sub.id=acs.subject_id").Where("acs.class_id=? AND acs.subject_id=? AND sub.is_active", e.ClassID, in.SubjectID).Select("CASE WHEN acs.is_fourth_subject THEN 1 ELSE 0 END").Scan(&fourth).Error; err != nil {
			return err
		}
		if err := tx.Table("academic_class_subjects").Where("class_id=? AND subject_id=?", e.ClassID, in.SubjectID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrInvalid
		}
		if fourth == 1 {
			if err := tx.Table("exam_subjects es").Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").Where("es.exam_id=? AND acs.is_fourth_subject AND es.subject_id<>?", examID, in.SubjectID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrConflict
			}
		}
		if err := tx.Exec("INSERT INTO exam_subjects(exam_id,class_id,subject_id,full_marks,pass_marks) VALUES (?,?,?,?,?) ON CONFLICT(exam_id,subject_id) DO UPDATE SET full_marks=EXCLUDED.full_marks,pass_marks=EXCLUDED.pass_marks", examID, e.ClassID, in.SubjectID, in.FullMarks, in.PassMarks).Error; err != nil {
			return dbErr(err)
		}
		if err := tx.Table("exam_marks m").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").Where("m.exam_id=? AND es.subject_id=? AND m.marks>es.full_marks", examID, in.SubjectID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrConflict
		}
		out, err = examByID(tx, examID)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.subject_saved", "exam", examID, map[string]any{"subject_id": in.SubjectID, "full_marks": in.FullMarks, "pass_marks": in.PassMarks})
	})
	return out, dbErr(err)
}
func (r *GormRepository) DeleteSubject(ctx context.Context, actor, examID, subjectID string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if _, err := lockExam(tx, examID, "UPDATE"); err != nil {
			return err
		}
		e, err := examByID(tx, examID)
		if err != nil {
			return err
		}
		if e.Status != "draft" {
			return ErrConflict
		}
		res := tx.Exec("DELETE FROM exam_subjects WHERE exam_id=? AND subject_id=?", examID, subjectID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "exam.subject_deleted", "exam", examID, map[string]any{"subject_id": subjectID})
	}))
}
func (r *GormRepository) Roster(ctx context.Context, examID, actor, scope string) ([]MarkRow, error) {
	e, err := examByID(r.db.WithContext(ctx), examID)
	if err != nil {
		return nil, err
	}
	if scope == "teacher" {
		var n int64
		if err := r.db.WithContext(ctx).Table("employees emp").Joins("JOIN teacher_assignments ta ON ta.employee_id=emp.id").Joins("JOIN exam_subjects es ON es.subject_id=ta.subject_id AND es.exam_id=?", examID).Where("emp.user_id=? AND emp.is_active AND ta.session_id=? AND ta.class_id=?", actor, e.SessionID, e.ClassID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrForbidden
		}
	}
	rows := []MarkRow{}
	q := r.db.WithContext(ctx).Table("students s").Joins("CROSS JOIN exam_subjects es").Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").Joins("LEFT JOIN exam_marks m ON m.student_id=s.id AND m.exam_subject_id=es.id").Where("s.session_id=? AND s.class_id=? AND s.section_id=? AND s.status='active' AND es.exam_id=?", e.SessionID, e.ClassID, e.SectionID, e.ID)
	if scope == "teacher" {
		q = q.Where("EXISTS (SELECT 1 FROM employees emp JOIN teacher_assignments ta ON ta.employee_id=emp.id WHERE emp.user_id=? AND emp.is_active AND ta.session_id=? AND ta.class_id=? AND ta.subject_id=es.subject_id)", actor, e.SessionID, e.ClassID)
	}
	err = q.Select("es.id AS exam_subject_id,s.id AS student_id,s.name_en AS student_name,s.admission_no,s.roll_no,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,m.marks,m.grade,m.grade_point,m.passed,coalesce(m.status,'') AS status,coalesce(m.status_reason,'') AS status_reason,coalesce(m.row_version,0) AS row_version").Order("s.roll_no NULLS LAST,s.admission_no,sub.code").Find(&rows).Error
	return rows, err
}
func classRanks(tx *gorm.DB, e Exam) error {
	return tx.Exec(`WITH ranked AS (
 SELECT r.exam_id,r.student_id,
 rank() OVER (ORDER BY r.gpa DESC,r.total_marks DESC) AS class_rank,
 rank() OVER (PARTITION BY r.section_id ORDER BY r.gpa DESC,r.total_marks DESC) AS section_rank
 FROM exam_results r JOIN exams x ON x.id=r.exam_id
 WHERE x.session_id=? AND x.class_id=? AND lower(x.name)=lower(?) AND x.exam_type=? AND x.status IN ('published','locked')
 ) UPDATE exam_results r SET class_position=ranked.class_rank,section_position=ranked.section_rank
 FROM ranked WHERE r.exam_id=ranked.exam_id AND r.student_id=ranked.student_id`, e.SessionID, e.ClassID, e.Name, e.ExamType).Error
}
func (r *GormRepository) SetState(ctx context.Context, actor, id, state string) (Exam, error) {
	var out Exam
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if _, err := lockExam(tx, id, "UPDATE"); err != nil {
			return err
		}
		e, err := examByID(tx, id)
		if err != nil {
			return err
		}
		if e.Status == "locked" {
			return ErrConflict
		}
		if e.Status == state {
			out = e
			return nil
		}
		if state == "locked" && e.Status != "published" {
			return ErrConflict
		}
		if state == "published" && e.Status != "draft" {
			return ErrConflict
		}
		if state == "draft" && e.Status != "published" {
			return ErrConflict
		}
		var locked int64
		if state != "locked" {
			if err := tx.Table("exams").Where("id<>? AND session_id=? AND class_id=? AND lower(name)=lower(?) AND exam_type=? AND status='locked'", id, e.SessionID, e.ClassID, e.Name, e.ExamType).Count(&locked).Error; err != nil {
				return err
			}
			if locked > 0 {
				return ErrConflict
			}
		}
		if state == "published" {
			if len(e.Subjects) == 0 {
				return ErrConflict
			}
			fourth, required := 0, 0
			for _, s := range e.Subjects {
				if s.IsFourth {
					fourth++
				} else {
					required++
				}
			}
			if fourth > 1 || required == 0 {
				return ErrConflict
			}
			var pending int64
			if err := tx.Table("exam_subjects").Where("exam_id=? AND sheet_status IN ('submitted','returned')", id).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return ErrSheetsPending
			}
			students := []struct{ ID string }{}
			if err := tx.Table("students").Select("id").Where("session_id=? AND class_id=? AND section_id=? AND status='active'", e.SessionID, e.ClassID, e.SectionID).Order("id").Find(&students).Error; err != nil {
				return err
			}
			if len(students) == 0 {
				return ErrConflict
			}
			bands := []Band{}
			if err := tx.Table("grading_bands b").Joins("JOIN school_settings ss ON ss.grading_scheme_id=b.scheme_id AND ss.id=1").Select("b.grade,b.min_marks,b.max_marks,b.gpa").Order("b.min_marks DESC").Find(&bands).Error; err != nil {
				return err
			}
			if len(bands) == 0 {
				return ErrInvalid
			}
			for _, student := range students {
				rows := []struct {
					MarkID    string
					Marks     *float64
					Status    string
					FullMarks float64
					PassMarks float64
					IsFourth  bool
				}{}
				err := tx.Table("exam_marks m").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").Select("m.id AS mark_id,m.marks,m.status,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth").Where("m.exam_id=? AND m.student_id=?", id, student.ID).Order("es.subject_id").Find(&rows).Error
				if err != nil {
					return err
				}
				if len(rows) != len(e.Subjects) {
					return ErrMarksIncomplete
				}
				lines := make([]MarkLine, 0, len(rows))
				for _, row := range rows {
					if row.Status == StatusIncomplete {
						return ErrMarksIncomplete
					}
					lines = append(lines, MarkLine{Marks: row.Marks, FullMarks: row.FullMarks, PassMarks: row.PassMarks, Fourth: row.IsFourth, Status: row.Status})
				}
				// Exact engine with mark statuses; values are written as
				// two-decimal strings so no float reaches the numeric columns.
				score, err := ScoreLines(lines, bands)
				if err != nil {
					return err
				}
				if score.Withheld {
					continue // no grades and no result: the student sees "withheld"
				}
				for i, row := range rows {
					g := score.Lines[i]
					var point any
					if g.Point != nil {
						point = g.Point.String()
					}
					if err := tx.Exec("UPDATE exam_marks SET grade=?,grade_point=?::numeric,passed=? WHERE id=?", g.Grade, point, g.Passed, row.MarkID).Error; err != nil {
						return err
					}
				}
				result := score.Result
				if err := tx.Exec("INSERT INTO exam_results(exam_id,student_id,session_id,class_id,section_id,total_marks,full_marks,gpa,passed,class_position,section_position) VALUES (?,?,?,?,?,?::numeric,?::numeric,?::numeric,?,1,1)", id, student.ID, e.SessionID, e.ClassID, e.SectionID, result.Total.String(), result.Full.String(), result.GPA.String(), result.Passed).Error; err != nil {
					return dbErr(err)
				}
			}
			if err := tx.Exec("UPDATE exams SET status='published',published_at=now(),updated_by=?,updated_at=now() WHERE id=?", actor, id).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE exam_subjects SET sheet_status='approved',reviewed_by=?,reviewed_at=now(),return_reason='' WHERE exam_id=? AND sheet_status<>'approved'", actor, id).Error; err != nil {
				return err
			}
		} else if state == "draft" {
			if err := tx.Exec("DELETE FROM exam_results WHERE exam_id=?", id).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE exam_marks SET grade=NULL,grade_point=NULL,passed=NULL WHERE exam_id=?", id).Error; err != nil {
				return err
			}
			// Back to draft: sheets reopen for correction, as before E3.
			if err := tx.Exec("UPDATE exam_subjects SET sheet_status='in_progress',submitted_by=NULL,submitted_at=NULL,reviewed_by=NULL,reviewed_at=NULL,return_reason='' WHERE exam_id=?", id).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE exams SET status='draft',published_at=NULL,updated_by=?,updated_at=now() WHERE id=?", actor, id).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Exec("UPDATE exams SET status='locked',updated_by=?,updated_at=now() WHERE id=?", actor, id).Error; err != nil {
				return err
			}
		}
		if err := classRanks(tx, e); err != nil {
			return err
		}
		out, err = examByID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.state_changed", "exam", id, map[string]any{"from": e.Status, "to": state})
	})
	return out, dbErr(err)
}
func (r *GormRepository) Tabulation(ctx context.Context, id string) ([]Result, error) {
	if _, err := examByID(r.db.WithContext(ctx), id); err != nil {
		return nil, err
	}
	rows := []Result{}
	err := r.db.WithContext(ctx).Table("exam_results er").Joins("JOIN students s ON s.id=er.student_id").Select("er.exam_id,er.student_id,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,s.roll_no,er.total_marks,er.full_marks,er.gpa,er.passed,er.class_position,er.section_position").Where("er.exam_id=?", id).Order("er.section_position,s.admission_no").Find(&rows).Error
	return rows, err
}
func (r *GormRepository) scopeStudent(tx *gorm.DB, scope, actor, studentID string) (string, error) {
	if scope == "admin" {
		if studentID == "" {
			return "", ErrInvalid
		}
		return studentID, nil
	}
	var id string
	if scope == "student" {
		err := tx.Raw("SELECT id FROM students WHERE user_id=?", actor).Row().Scan(&id)
		if err != nil {
			return "", ErrNotFound
		}
		return id, nil
	}
	if scope == "guardian" {
		if studentID == "" {
			return "", ErrInvalid
		}
		err := tx.Raw("SELECT s.id FROM students s JOIN student_guardians sg ON sg.student_id=s.id JOIN guardians g ON g.id=sg.guardian_id WHERE s.id=? AND g.user_id=?", studentID, actor).Row().Scan(&id)
		if err != nil {
			return "", ErrNotFound
		}
		return id, nil
	}
	return "", ErrForbidden
}
func (r *GormRepository) Cards(ctx context.Context, scope, actor, studentID, examID string) ([]Card, error) {
	tx := r.db.WithContext(ctx)
	studentID, err := r.scopeStudent(tx, scope, actor, studentID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	q := tx.Table("exam_results er").Joins("JOIN exams e ON e.id=er.exam_id").Where("er.student_id=?", studentID).Select("er.exam_id").Order("e.published_at DESC,e.id DESC")
	if scope != "admin" {
		q = q.Where("e.status IN ('published','locked')")
	}
	if examID != "" {
		q = q.Where("er.exam_id=?", examID)
	}
	if err := q.Pluck("er.exam_id", &ids).Error; err != nil {
		return nil, err
	}
	withheld := []string{}
	wq := tx.Table("exams e").Where("e.status IN ('published','locked') AND EXISTS (SELECT 1 FROM exam_marks m WHERE m.exam_id=e.id AND m.student_id=? AND m.status='withheld') AND NOT EXISTS (SELECT 1 FROM exam_results er WHERE er.exam_id=e.id AND er.student_id=?)", studentID, studentID).Order("e.published_at DESC,e.id DESC")
	if examID != "" {
		wq = wq.Where("e.id=?", examID)
	}
	if err := wq.Pluck("e.id", &withheld).Error; err != nil {
		return nil, err
	}
	ids = append(ids, withheld...)
	out := make([]Card, 0, len(ids))
	for _, id := range ids {
		card, err := r.Card(ctx, scope, actor, studentID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, nil
}
func (r *GormRepository) Card(ctx context.Context, scope, actor, studentID, examID string) (Card, error) {
	tx := r.db.WithContext(ctx)
	studentID, err := r.scopeStudent(tx, scope, actor, studentID)
	if err != nil {
		return Card{}, err
	}
	card := Card{Subjects: []MarkRow{}}
	card.Exam, err = examByID(tx, examID)
	if err != nil {
		return Card{}, err
	}
	if scope != "admin" && card.Exam.Status == "draft" {
		return Card{}, ErrNotFound
	}
	err = tx.Table("exam_results er").Joins("JOIN students s ON s.id=er.student_id").Select("er.exam_id,er.student_id,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,s.roll_no,er.total_marks,er.full_marks,er.gpa,er.passed,er.class_position,er.section_position").Where("er.exam_id=? AND er.student_id=?", examID, studentID).Take(&card.Result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && card.Exam.Status != "draft" {
		// A withheld result: the student learns it is held back, nothing more.
		var n int64
		if err := tx.Table("exam_marks").Where("exam_id=? AND student_id=? AND status='withheld'", examID, studentID).Count(&n).Error; err != nil {
			return Card{}, err
		}
		if n == 0 {
			return Card{}, ErrNotFound
		}
		if err := tx.Table("students s").Select("? AS exam_id,s.id AS student_id,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,s.roll_no", examID).Where("s.id=?", studentID).Take(&card.Result).Error; err != nil {
			return Card{}, dbErr(err)
		}
		card.Withheld = true
		err = tx.Raw("SELECT name_en,name_bn,logo_url FROM school_settings WHERE id=1").Row().Scan(&card.SchoolNameEN, &card.SchoolNameBN, &card.LogoURL)
		return card, err
	}
	if err != nil {
		return Card{}, dbErr(err)
	}
	err = tx.Table("exam_marks m").Joins("JOIN exam_subjects es ON es.id=m.exam_subject_id").Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").Select("m.exam_subject_id,m.student_id,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,m.marks,m.grade,m.grade_point,m.passed,m.status,m.status_reason,m.row_version").Where("m.exam_id=? AND m.student_id=?", examID, studentID).Order("sub.code").Find(&card.Subjects).Error
	if err != nil {
		return Card{}, err
	}
	if scope != "admin" {
		for i := range card.Subjects {
			card.Subjects[i].StatusReason, card.Subjects[i].RowVersion = "", 0
		}
	}
	err = tx.Raw("SELECT name_en,name_bn,logo_url FROM school_settings WHERE id=1").Row().Scan(&card.SchoolNameEN, &card.SchoolNameBN, &card.LogoURL)
	return card, err
}
