package routine

import (
	"context"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }
func missing(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
func lock(tx *gorm.DB) error {
	var ignored any
	return tx.Raw("SELECT pg_advisory_xact_lock(517006)").Row().Scan(&ignored)
}

const clockSQL = "lpad((r.start_minute/60)::text,2,'0')||':'||lpad((r.start_minute%60)::text,2,'0') AS start_time,lpad((r.end_minute/60)::text,2,'0')||':'||lpad((r.end_minute%60)::text,2,'0') AS end_time"
const classSelect = "r.id,r.session_id,r.class_id,r.section_id,r.subject_id,r.teacher_id,r.day_of_week,r.room,c.name AS class_name,sec.name AS section_name,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,e.name_en AS teacher_name," + clockSQL

func (r *GormRepository) classByID(tx *gorm.DB, id string) (ClassSlot, error) {
	var out ClassSlot
	err := tx.Table("class_routines AS r").Joins("JOIN academic_classes c ON c.id=r.class_id").Joins("JOIN academic_sections sec ON sec.id=r.section_id").Joins("JOIN academic_subjects sub ON sub.id=r.subject_id").Joins("JOIN employees e ON e.id=r.teacher_id").Select(classSelect).Where("r.id=?", id).Take(&out).Error
	return out, missing(err)
}
func (r *GormRepository) SaveClass(ctx context.Context, actor, id string, in ClassInput, start, end int) (ClassSlot, error) {
	var out ClassSlot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if id != "" {
			if _, err := r.classByID(tx, id); err != nil {
				return err
			}
		}
		var valid int64
		err := tx.Table("teacher_assignments a").Joins("JOIN employees e ON e.id=a.employee_id").Joins("JOIN academic_classes c ON c.id=a.class_id").Joins("JOIN academic_sections sec ON sec.id=? AND sec.class_id=a.class_id AND sec.session_id=a.session_id", in.SectionID).Joins("JOIN academic_subjects sub ON sub.id=a.subject_id").Where("a.employee_id=? AND a.session_id=? AND a.class_id=? AND a.subject_id=? AND e.kind='teacher' AND e.is_active=true AND c.is_active=true AND sec.is_active=true AND sub.is_active=true", in.TeacherID, in.SessionID, in.ClassID, in.SubjectID).Count(&valid).Error
		if err != nil {
			return err
		}
		if valid == 0 {
			return ErrInvalid
		}
		var overlap int64
		err = tx.Table("class_routines AS cr").Joins("JOIN academic_sessions existing_session ON existing_session.id=cr.session_id").Joins("JOIN academic_sessions selected_session ON selected_session.id=?", in.SessionID).Where("existing_session.start_date<=selected_session.end_date AND existing_session.end_date>=selected_session.start_date AND cr.day_of_week=? AND cr.start_minute<? AND cr.end_minute>? AND cr.id::text IS DISTINCT FROM ? AND (cr.section_id=? OR cr.teacher_id=? OR lower(cr.room)=lower(?))", in.DayOfWeek, end, start, id, in.SectionID, in.TeacherID, in.Room).Count(&overlap).Error
		if err != nil {
			return err
		}
		if overlap > 0 {
			return ErrConflict
		}
		if id == "" {
			err = tx.Raw("INSERT INTO class_routines(session_id,class_id,section_id,subject_id,teacher_id,day_of_week,start_minute,end_minute,room,created_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?,?,?) RETURNING id", in.SessionID, in.ClassID, in.SectionID, in.SubjectID, in.TeacherID, in.DayOfWeek, start, end, in.Room, actor, actor).Scan(&id).Error
		} else {
			err = tx.Exec("UPDATE class_routines SET session_id=?,class_id=?,section_id=?,subject_id=?,teacher_id=?,day_of_week=?,start_minute=?,end_minute=?,room=?,updated_by=?,updated_at=now() WHERE id=?", in.SessionID, in.ClassID, in.SectionID, in.SubjectID, in.TeacherID, in.DayOfWeek, start, end, in.Room, actor, id).Error
		}
		if err != nil {
			return err
		}
		out, err = r.classByID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.class_saved", "class_routine", id, map[string]any{"session_id": in.SessionID, "section_id": in.SectionID, "day": in.DayOfWeek})
	})
	return out, err
}
func (r *GormRepository) DeleteClass(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM class_routines WHERE id=?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "routine.class_deleted", "class_routine", id, nil)
	})
}
func (r *GormRepository) ListClasses(ctx context.Context, q Query, scope, actor, studentID string) (Page[ClassSlot], error) {
	db := r.db.WithContext(ctx)
	if scope == "guardian" {
		if err := linkedChild(db, actor, studentID); err != nil {
			return Page[ClassSlot]{}, err
		}
	}
	base := db.Table("class_routines AS r").Joins("JOIN academic_classes c ON c.id=r.class_id").Joins("JOIN academic_sections sec ON sec.id=r.section_id").Joins("JOIN academic_subjects sub ON sub.id=r.subject_id").Joins("JOIN employees e ON e.id=r.teacher_id")
	switch scope {
	case "teacher":
		base = base.Where("e.user_id=?", actor)
	case "student":
		base = base.Where("EXISTS (SELECT 1 FROM students s WHERE s.user_id=? AND s.status='active' AND s.session_id=r.session_id AND s.section_id=r.section_id)", actor)
	case "guardian":
		base = base.Where("EXISTS (SELECT 1 FROM students s WHERE s.id=? AND s.status='active' AND s.session_id=r.session_id AND s.section_id=r.section_id)", studentID)
	}
	if q.SessionID != "" {
		base = base.Where("r.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("r.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("r.section_id=?", q.SectionID)
	}
	if q.Search != "" {
		v := "%" + q.Search + "%"
		base = base.Where("c.name ILIKE ? OR sec.name ILIKE ? OR sub.name_en ILIKE ? OR sub.name_bn ILIKE ? OR e.name_en ILIKE ? OR r.room ILIKE ?", v, v, v, v, v, v)
	}
	out := Page[ClassSlot]{Items: []ClassSlot{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := base.Select(classSelect).Order("r.day_of_week,r.start_minute,r.section_id,r.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&out.Items).Error
	return out, err
}
func linkedChild(db *gorm.DB, actor, studentID string) error {
	var n int64
	err := db.Table("student_guardians sg").Joins("JOIN guardians g ON g.id=sg.guardian_id").Where("sg.student_id=? AND g.user_id=?", studentID, actor).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) examByID(tx *gorm.DB, id string) (Exam, error) {
	var out Exam
	err := tx.Table("exam_routines AS r").Joins("JOIN academic_classes c ON c.id=r.class_id").Joins("JOIN academic_sections sec ON sec.id=r.section_id").Select("r.id,r.name,r.session_id,r.class_id,r.section_id,r.is_published,c.name AS class_name,sec.name AS section_name").Where("r.id=?", id).Take(&out).Error
	return out, missing(err)
}
func (r *GormRepository) SaveExam(ctx context.Context, actor, id string, in ExamInput) (Exam, error) {
	var out Exam
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if id != "" {
			old, err := r.examByID(tx, id)
			if err != nil {
				return err
			}
			if old.IsPublished {
				return ErrConflict
			}
			var slots int64
			if err := tx.Table("exam_routine_slots").Where("routine_id=?", id).Count(&slots).Error; err != nil {
				return err
			}
			if slots > 0 && (old.SessionID != in.SessionID || old.ClassID != in.ClassID || old.SectionID != in.SectionID) {
				return ErrConflict
			}
		}
		var n int64
		if err := tx.Table("academic_sections sec").Joins("JOIN academic_classes c ON c.id=sec.class_id").Where("sec.id=? AND sec.class_id=? AND sec.session_id=? AND sec.is_active=true AND c.is_active=true", in.SectionID, in.ClassID, in.SessionID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrInvalid
		}
		if err := tx.Table("exam_routines").Where("session_id=? AND section_id=? AND lower(name)=lower(?) AND id::text IS DISTINCT FROM ?", in.SessionID, in.SectionID, in.Name, id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		var err error
		if id == "" {
			err = tx.Raw("INSERT INTO exam_routines(name,session_id,class_id,section_id,created_by,updated_by) VALUES (?,?,?,?,?,?) RETURNING id", in.Name, in.SessionID, in.ClassID, in.SectionID, actor, actor).Scan(&id).Error
		} else {
			err = tx.Exec("UPDATE exam_routines SET name=?,session_id=?,class_id=?,section_id=?,updated_by=?,updated_at=now() WHERE id=?", in.Name, in.SessionID, in.ClassID, in.SectionID, actor, id).Error
		}
		if err != nil {
			return err
		}
		out, err = r.examByID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.exam_saved", "exam_routine", id, map[string]any{"session_id": in.SessionID, "section_id": in.SectionID})
	})
	return out, err
}
func (r *GormRepository) PublishExam(ctx context.Context, actor, id string, published bool) (Exam, error) {
	var out Exam
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		old, err := r.examByID(tx, id)
		if err != nil {
			return err
		}
		if published {
			var n int64
			if err := tx.Table("exam_routine_slots").Where("routine_id=?", id).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
		}
		if old.IsPublished == published {
			out = old
			return nil
		}
		if err := tx.Exec("UPDATE exam_routines SET is_published=?,updated_by=?,updated_at=now() WHERE id=?", published, actor, id).Error; err != nil {
			return err
		}
		out, err = r.examByID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.exam_publication", "exam_routine", id, map[string]any{"published": published})
	})
	return out, err
}
func (r *GormRepository) DeleteExam(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		old, err := r.examByID(tx, id)
		if err != nil {
			return err
		}
		if old.IsPublished {
			return ErrConflict
		}
		if err := tx.Exec("DELETE FROM exam_routines WHERE id=?", id).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.exam_deleted", "exam_routine", id, nil)
	})
}
func (r *GormRepository) scopedExams(db *gorm.DB, scope, actor, studentID string) (*gorm.DB, error) {
	if scope == "guardian" {
		if err := linkedChild(db, actor, studentID); err != nil {
			return nil, err
		}
	}
	base := db.Table("exam_routines AS r").Joins("JOIN academic_classes c ON c.id=r.class_id").Joins("JOIN academic_sections sec ON sec.id=r.section_id")
	if scope == "student" {
		base = base.Where("r.is_published=true AND EXISTS (SELECT 1 FROM students s WHERE s.user_id=? AND s.status='active' AND s.section_id=r.section_id AND s.session_id=r.session_id)", actor)
	}
	if scope == "guardian" {
		base = base.Where("r.is_published=true AND EXISTS (SELECT 1 FROM students s WHERE s.id=? AND s.status='active' AND s.section_id=r.section_id AND s.session_id=r.session_id)", studentID)
	}
	return base, nil
}
func (r *GormRepository) ListExams(ctx context.Context, q Query, scope, actor, studentID string) (Page[Exam], error) {
	base, err := r.scopedExams(r.db.WithContext(ctx), scope, actor, studentID)
	if err != nil {
		return Page[Exam]{}, err
	}
	if q.SessionID != "" {
		base = base.Where("r.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("r.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("r.section_id=?", q.SectionID)
	}
	if q.Search != "" {
		v := "%" + q.Search + "%"
		base = base.Where("r.name ILIKE ? OR c.name ILIKE ? OR sec.name ILIKE ?", v, v, v)
	}
	out := Page[Exam]{Items: []Exam{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err = base.Select("r.id,r.name,r.session_id,r.class_id,r.section_id,r.is_published,c.name AS class_name,sec.name AS section_name").Order("r.name,r.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&out.Items).Error
	return out, err
}
func (r *GormRepository) GetExam(ctx context.Context, id, scope, actor, studentID string) (Exam, error) {
	db := r.db.WithContext(ctx)
	base, err := r.scopedExams(db, scope, actor, studentID)
	if err != nil {
		return Exam{}, err
	}
	var out Exam
	err = base.Select("r.id,r.name,r.session_id,r.class_id,r.section_id,r.is_published,c.name AS class_name,sec.name AS section_name").Where("r.id=?", id).Take(&out).Error
	if err != nil {
		return Exam{}, missing(err)
	}
	const timeColumns = "lpad((s.start_minute/60)::text,2,'0')||':'||lpad((s.start_minute%60)::text,2,'0') AS start_time,lpad((s.end_minute/60)::text,2,'0')||':'||lpad((s.end_minute%60)::text,2,'0') AS end_time"
	out.Slots = []ExamSlot{}
	err = db.Table("exam_routine_slots AS s").Joins("JOIN academic_subjects sub ON sub.id=s.subject_id").Select("s.id,s.routine_id,s.subject_id,s.room,to_char(s.exam_date,'YYYY-MM-DD') AS exam_date,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,"+timeColumns).Where("s.routine_id=?", id).Order("s.exam_date,s.start_minute,s.id").Scan(&out.Slots).Error
	return out, err
}
func (r *GormRepository) SaveExamSlot(ctx context.Context, actor, routineID, id string, in ExamSlotInput, start, end int) (Exam, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		exam, err := r.examByID(tx, routineID)
		if err != nil {
			return err
		}
		if exam.IsPublished {
			return ErrConflict
		}
		var bounds struct{ StartDate, EndDate time.Time }
		if err := tx.Table("academic_sessions").Select("start_date,end_date").Where("id=?", exam.SessionID).Take(&bounds).Error; err != nil {
			return missing(err)
		}
		date, _ := time.Parse("2006-01-02", in.ExamDate)
		if date.Before(bounds.StartDate) || date.After(bounds.EndDate) {
			return ErrInvalid
		}
		var n int64
		if err := tx.Table("academic_class_subjects m").Joins("JOIN academic_subjects sub ON sub.id=m.subject_id").Where("m.class_id=? AND m.subject_id=? AND sub.is_active=true", exam.ClassID, in.SubjectID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrInvalid
		}
		if id != "" {
			var existing string
			if err := tx.Table("exam_routine_slots").Select("routine_id").Where("id=?", id).Take(&existing).Error; err != nil {
				return missing(err)
			}
			if existing != routineID {
				return ErrNotFound
			}
		}
		if err := tx.Table("exam_routine_slots").Where("routine_id=? AND subject_id=? AND id::text IS DISTINCT FROM ?", routineID, in.SubjectID, id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		conflicts := tx.Table("exam_routine_slots s").Joins("JOIN exam_routines r ON r.id=s.routine_id").Where("s.exam_date=? AND s.start_minute<? AND s.end_minute>? AND s.id::text IS DISTINCT FROM ? AND (r.section_id=? OR lower(s.room)=lower(?))", in.ExamDate, end, start, id, exam.SectionID, in.Room)
		if err := conflicts.Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		if id == "" {
			err = tx.Raw("INSERT INTO exam_routine_slots(routine_id,class_id,subject_id,exam_date,start_minute,end_minute,room,created_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?) RETURNING id", routineID, exam.ClassID, in.SubjectID, in.ExamDate, start, end, in.Room, actor, actor).Scan(&id).Error
		} else {
			err = tx.Exec("UPDATE exam_routine_slots SET subject_id=?,exam_date=?,start_minute=?,end_minute=?,room=?,updated_by=?,updated_at=now() WHERE id=?", in.SubjectID, in.ExamDate, start, end, in.Room, actor, id).Error
		}
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.exam_slot_saved", "exam_routine_slot", id, map[string]any{"routine_id": routineID, "date": in.ExamDate})
	})
	if err != nil {
		return Exam{}, err
	}
	return r.GetExam(ctx, routineID, "admin", "", "")
}
func (r *GormRepository) DeleteExamSlot(ctx context.Context, actor, routineID, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		exam, err := r.examByID(tx, routineID)
		if err != nil {
			return err
		}
		if exam.IsPublished {
			return ErrConflict
		}
		res := tx.Exec("DELETE FROM exam_routine_slots WHERE id=? AND routine_id=?", id, routineID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "routine.exam_slot_deleted", "exam_routine_slot", id, map[string]any{"routine_id": routineID})
	})
}

var _ Repository = (*GormRepository)(nil)
