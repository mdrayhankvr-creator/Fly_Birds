package attendance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }
func (r *GormRepository) ListSessions(ctx context.Context, q Query) (Page[Session], error) {
	base := r.db.WithContext(ctx).Table("academic_sessions")
	if q.Search != "" {
		base = base.Where("name ILIKE ?", "%"+q.Search+"%")
	}
	page := Page[Session]{Items: []Session{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&page.Total).Error; err != nil {
		return Page[Session]{}, err
	}
	err := base.Select("id,name,to_char(start_date,'YYYY-MM-DD') AS start_date,to_char(end_date,'YYYY-MM-DD') AS end_date").Order("start_date DESC,id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func dbError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505":
			return ErrDuplicate
		case "23503":
			return ErrNotFound
		case "23514", "23502", "22P02", "22007":
			return ErrInvalid
		}
	}
	return err
}
func manager(tx *gorm.DB, actor string) (string, error) {
	var role string
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Select("u.role").Where("u.id=? AND u.is_active=true AND u.role IN ('admin','head_teacher') AND rp.permission_id='attendance:teachers_manage'", actor).Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrForbidden
	}
	return role, err
}
func ownTeacher(tx *gorm.DB, actor string) (string, error) {
	var id string
	err := tx.Table("employees AS e").Joins("JOIN users AS u ON u.id=e.user_id").Select("e.id").Where("e.user_id=? AND e.kind='teacher' AND u.is_active=true AND u.role IN ('teacher','head_teacher')", actor).Take(&id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return id, err
}
func sessionDates(tx *gorm.DB, id string) (string, string, error) {
	var row struct {
		StartDate string
		EndDate   string
	}
	err := tx.Table("academic_sessions").Select("to_char(start_date,'YYYY-MM-DD') AS start_date,to_char(end_date,'YYYY-MM-DD') AS end_date").Where("id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", ErrNotFound
	}
	return row.StartDate, row.EndDate, err
}
func validateDate(tx *gorm.DB, sessionID, date string) error {
	start, end, err := sessionDates(tx, sessionID)
	if err != nil {
		return err
	}
	if date < start || date > end {
		return ErrConflict
	}
	return nil
}
func validateEmployee(tx *gorm.DB, employeeID, date, status string) error {
	var row struct {
		Kind     string
		IsActive bool
		UserID   *string
		JoinedOn string
		LeftOn   *string
	}
	err := tx.Table("employees").Select("kind,is_active,user_id,to_char(joined_on,'YYYY-MM-DD') AS joined_on,to_char(left_on,'YYYY-MM-DD') AS left_on").Where("id=?", employeeID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.Kind != "teacher" || date < row.JoinedOn || (row.LeftOn != nil && date > *row.LeftOn) {
		return ErrConflict
	}
	if status == "leave" {
		var count int64
		err = tx.Table("employee_leaves").Where("employee_id=? AND status='approved' AND starts_on<=? AND ends_on>=?", employeeID, date, date).Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return ErrConflict
		}
	}
	return nil
}

const recordSelect = "a.id,a.session_id,a.employee_id,to_char(a.attendance_date,'YYYY-MM-DD') AS attendance_date,a.status,a.note,a.recorded_by,a.updated_by,a.created_at,a.updated_at,s.name AS session_name,e.employee_code,e.name_en AS employee_name_en,e.name_bn AS employee_name_bn"

func record(tx *gorm.DB, id string) (Record, error) {
	var value Record
	err := tx.Table("teacher_attendance AS a").Joins("JOIN academic_sessions AS s ON s.id=a.session_id").Joins("JOIN employees AS e ON e.id=a.employee_id").Select(recordSelect).Where("a.id=?", id).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Record{}, ErrNotFound
	}
	return value, err
}
func (r *GormRepository) Roster(ctx context.Context, q Query) (Page[RosterItem], error) {
	db := r.db.WithContext(ctx)
	if err := validateDate(db, q.SessionID, q.Date); err != nil {
		return Page[RosterItem]{}, err
	}
	base := db.Table("employees AS e").Joins("LEFT JOIN teacher_attendance AS a ON a.employee_id=e.id AND a.session_id=? AND a.attendance_date=?", q.SessionID, q.Date).Where("e.kind='teacher' AND e.joined_on<=? AND (e.left_on IS NULL OR e.left_on>=?) AND (e.is_active=true OR a.id IS NOT NULL)", q.Date, q.Date)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("e.employee_code ILIKE ? OR e.name_en ILIKE ? OR e.name_bn ILIKE ?", term, term, term)
	}
	page := Page[RosterItem]{Items: []RosterItem{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&page.Total).Error; err != nil {
		return Page[RosterItem]{}, err
	}
	err := base.Select("e.id AS employee_id,e.employee_code,e.name_en,e.name_bn,a.id AS record_id,a.status,a.note").Order("e.name_en ASC,e.id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func (r *GormRepository) Record(ctx context.Context, actor string, value Record) (Record, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := manager(tx, actor)
		if err != nil {
			return err
		}
		if err := validateDate(tx, value.SessionID, value.AttendanceDate); err != nil {
			return err
		}
		if err := validateEmployee(tx, value.EmployeeID, value.AttendanceDate, value.Status); err != nil {
			return err
		}
		if role == "head_teacher" {
			var count int64
			if err := tx.Table("employees").Where("id=? AND user_id=?", value.EmployeeID, actor).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrForbidden
			}
		}
		if err := tx.Raw("INSERT INTO teacher_attendance (session_id,employee_id,attendance_date,status,note,recorded_by,updated_by) VALUES (?,?,?,?,?,?,?) RETURNING id", value.SessionID, value.EmployeeID, value.AttendanceDate, value.Status, value.Note, actor, actor).Scan(&value.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "attendance.teacher_recorded", "teacher_attendance", value.ID, map[string]any{"employee_id": value.EmployeeID, "session_id": value.SessionID, "attendance_date": value.AttendanceDate, "status": value.Status})
	})
	if err != nil {
		return Record{}, dbError(err)
	}
	return record(r.db.WithContext(ctx), value.ID)
}
func (r *GormRepository) Correct(ctx context.Context, actor, id string, input CorrectionInput) (Record, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := manager(tx, actor)
		if err != nil {
			return err
		}
		var before Record
		err = tx.Table("teacher_attendance").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,session_id,employee_id,to_char(attendance_date,'YYYY-MM-DD') AS attendance_date,status").Where("id=?", id).Take(&before).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role == "head_teacher" {
			var count int64
			if err := tx.Table("employees").Where("id=? AND user_id=?", before.EmployeeID, actor).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrForbidden
			}
		}
		if err := validateEmployee(tx, before.EmployeeID, before.AttendanceDate, input.Status); err != nil {
			return err
		}
		if err := tx.Table("teacher_attendance").Where("id=?", id).Updates(map[string]any{"status": input.Status, "note": input.Note, "updated_by": actor, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "attendance.teacher_corrected", "teacher_attendance", id, map[string]any{"old_status": before.Status, "new_status": input.Status})
	})
	if err != nil {
		return Record{}, dbError(err)
	}
	return record(r.db.WithContext(ctx), id)
}
func (r *GormRepository) OwnHistory(ctx context.Context, actor string, q Query) (Page[Record], error) {
	db := r.db.WithContext(ctx)
	id, err := ownTeacher(db, actor)
	if err != nil {
		return Page[Record]{}, err
	}
	base := db.Table("teacher_attendance AS a").Joins("JOIN academic_sessions AS s ON s.id=a.session_id").Joins("JOIN employees AS e ON e.id=a.employee_id").Where("a.employee_id=?", id)
	if q.SessionID != "" {
		base = base.Where("a.session_id=?", q.SessionID)
	}
	if q.From != "" {
		base = base.Where("a.attendance_date>=?", q.From)
	}
	if q.To != "" {
		base = base.Where("a.attendance_date<=?", q.To)
	}
	page := Page[Record]{Items: []Record{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&page.Total).Error; err != nil {
		return Page[Record]{}, err
	}
	err = base.Select(recordSelect).Order("a.attendance_date DESC,a.id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func monthBounds(month string) (string, string) {
	start, _ := time.Parse("2006-01", month)
	return start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02")
}
func (r *GormRepository) Monthly(ctx context.Context, q Query) (Page[Summary], error) {
	db := r.db.WithContext(ctx)
	start, next := monthBounds(q.Month)
	sessionStart, sessionEnd, err := sessionDates(db, q.SessionID)
	if err != nil {
		return Page[Summary]{}, err
	}
	if next <= sessionStart || start > sessionEnd {
		return Page[Summary]{}, ErrConflict
	}
	base := db.Table("employees AS e").Joins("LEFT JOIN teacher_attendance AS a ON a.employee_id=e.id AND a.session_id=? AND a.attendance_date>=? AND a.attendance_date<?", q.SessionID, start, next).Where("e.kind='teacher' AND e.joined_on<? AND (e.left_on IS NULL OR e.left_on>=?) AND (e.is_active=true OR a.id IS NOT NULL)", next, start)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("e.employee_code ILIKE ? OR e.name_en ILIKE ? OR e.name_bn ILIKE ?", term, term, term)
	}
	page := Page[Summary]{Items: []Summary{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Distinct("e.id").Count(&page.Total).Error; err != nil {
		return Page[Summary]{}, err
	}
	err = base.Select("e.id AS employee_id,e.employee_code,e.name_en,e.name_bn,count(a.id) FILTER (WHERE a.status='present') AS present,count(a.id) FILTER (WHERE a.status='absent') AS absent,count(a.id) FILTER (WHERE a.status='leave') AS leave,count(a.id) AS total").Group("e.id,e.employee_code,e.name_en,e.name_bn").Order("e.name_en,e.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func (r *GormRepository) OwnMonthly(ctx context.Context, actor string, q Query) (Summary, error) {
	db := r.db.WithContext(ctx)
	id, err := ownTeacher(db, actor)
	if err != nil {
		return Summary{}, err
	}
	start, next := monthBounds(q.Month)
	sessionStart, sessionEnd, err := sessionDates(db, q.SessionID)
	if err != nil {
		return Summary{}, err
	}
	if next <= sessionStart || start > sessionEnd {
		return Summary{}, ErrConflict
	}
	var item Summary
	err = db.Table("employees AS e").Joins("LEFT JOIN teacher_attendance AS a ON a.employee_id=e.id AND a.session_id=? AND a.attendance_date>=? AND a.attendance_date<?", q.SessionID, start, next).Select("e.id AS employee_id,e.employee_code,e.name_en,e.name_bn,count(a.id) FILTER (WHERE a.status='present') AS present,count(a.id) FILTER (WHERE a.status='absent') AS absent,count(a.id) FILTER (WHERE a.status='leave') AS leave,count(a.id) AS total").Where("e.id=?", id).Group("e.id,e.employee_code,e.name_en,e.name_bn").Take(&item).Error
	return item, err
}
