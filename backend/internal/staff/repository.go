package staff

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }
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
func (r *GormRepository) allowed(tx *gorm.DB, actor, permission string) error {
	var count int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.id=? AND u.is_active=true AND u.role='admin' AND rp.permission_id=?", actor, permission).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}

const employeeSelect = "id,employee_code,user_id,kind,name_en,name_bn,designation,phone,to_char(joined_on,'YYYY-MM-DD') AS joined_on,to_char(left_on,'YYYY-MM-DD') AS left_on,is_active"

func (r *GormRepository) employee(tx *gorm.DB, id string) (Employee, error) {
	var item Employee
	err := tx.Table("employees").Select(employeeSelect).Where("id=?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Employee{}, ErrNotFound
	}
	return item, err
}
func (r *GormRepository) ListEmployees(ctx context.Context, q Query) (Page[Employee], error) {
	base := r.db.WithContext(ctx).Table("employees")
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("employee_code ILIKE ? OR name_en ILIKE ? OR name_bn ILIKE ? OR designation ILIKE ?", term, term, term, term)
	}
	if q.Kind != "" {
		base = base.Where("kind=?", q.Kind)
	}
	if q.Status != "" {
		base = base.Where("is_active=?", q.Status == "active")
	}
	page := Page[Employee]{Items: []Employee{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&page.Total).Error; err != nil {
		return Page[Employee]{}, err
	}
	err := base.Select(employeeSelect).Order("name_en ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func (r *GormRepository) GetEmployee(ctx context.Context, id string) (Employee, error) {
	return r.employee(r.db.WithContext(ctx), id)
}
func (r *GormRepository) OwnEmployee(ctx context.Context, user string) (Employee, error) {
	var id string
	err := r.db.WithContext(ctx).Table("employees AS e").Joins("JOIN users AS u ON u.id=e.user_id").Select("e.id").Where("e.user_id=? AND u.is_active=true AND ((e.kind='teacher' AND u.role IN ('teacher','head_teacher')) OR (e.kind='non_teaching' AND u.role IN ('admin','accountant')))", user).Take(&id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Employee{}, ErrNotFound
	}
	if err != nil {
		return Employee{}, err
	}
	return r.GetEmployee(ctx, id)
}
func (r *GormRepository) SaveEmployee(ctx context.Context, actor, id string, value Employee) (Employee, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.allowed(tx, actor, "staff:manage"); err != nil {
			return err
		}
		if value.UserID != nil {
			var linked struct {
				Role     string
				IsActive bool
			}
			err := tx.Table("users").Select("role,is_active").Where("id=?", *value.UserID).Take(&linked).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if !linked.IsActive || !(value.Kind == "teacher" && (linked.Role == "teacher" || linked.Role == "head_teacher") || value.Kind == "non_teaching" && (linked.Role == "accountant" || linked.Role == "admin")) {
				return ErrConflict
			}
		}
		if id == "" {
			err := tx.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,phone,joined_on,left_on,is_active) VALUES (?,?,?,?,?,?,?,?,?,?) RETURNING id", value.EmployeeCode, value.UserID, value.Kind, value.NameEN, value.NameBN, value.Designation, value.Phone, value.JoinedOn, value.LeftOn, value.IsActive).Scan(&value.ID).Error
			if err != nil {
				return err
			}
		} else {
			var previous Employee
			previous, err := r.employee(tx, id)
			if err != nil {
				return err
			}
			if previous.Kind == "teacher" && value.Kind != "teacher" {
				var count int64
				if err := tx.Table("teacher_assignments").Where("employee_id=?", id).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					return ErrConflict
				}
			}
			update := tx.Table("employees").Where("id=?", id).Updates(map[string]any{"employee_code": value.EmployeeCode, "user_id": value.UserID, "kind": value.Kind, "name_en": value.NameEN, "name_bn": value.NameBN, "designation": value.Designation, "phone": value.Phone, "joined_on": value.JoinedOn, "left_on": value.LeftOn, "is_active": value.IsActive, "updated_at": time.Now().UTC()})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 0 {
				return ErrNotFound
			}
			value.ID = id
		}
		return audit.Record(tx, &actor, "staff.employee_saved", "employee", value.ID, map[string]any{"created": id == "", "active": value.IsActive, "kind": value.Kind})
	})
	if err != nil {
		return Employee{}, dbError(err)
	}
	return r.GetEmployee(ctx, value.ID)
}

const assignmentSelect = "a.id,a.employee_id,a.session_id,a.class_id,a.subject_id,s.name AS session_name,c.name AS class_name,u.name_en AS subject_name_en,u.name_bn AS subject_name_bn"

func (r *GormRepository) assignment(tx *gorm.DB, id string) (Assignment, error) {
	var a Assignment
	err := tx.Table("teacher_assignments AS a").Joins("JOIN academic_sessions AS s ON s.id=a.session_id").Joins("JOIN academic_classes AS c ON c.id=a.class_id").Joins("JOIN academic_subjects AS u ON u.id=a.subject_id").Select(assignmentSelect).Where("a.id=?", id).Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Assignment{}, ErrNotFound
	}
	return a, err
}
func (r *GormRepository) ListAssignments(ctx context.Context, employee string) ([]Assignment, error) {
	if _, err := r.GetEmployee(ctx, employee); err != nil {
		return nil, err
	}
	items := []Assignment{}
	err := r.db.WithContext(ctx).Table("teacher_assignments AS a").Joins("JOIN academic_sessions AS s ON s.id=a.session_id").Joins("JOIN academic_classes AS c ON c.id=a.class_id").Joins("JOIN academic_subjects AS u ON u.id=a.subject_id").Select(assignmentSelect).Where("a.employee_id=?", employee).Order("s.start_date DESC,c.display_order,u.code").Scan(&items).Error
	return items, err
}
func (r *GormRepository) AddAssignment(ctx context.Context, actor string, a Assignment) (Assignment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.allowed(tx, actor, "staff:manage"); err != nil {
			return err
		}
		e, err := r.employee(tx, a.EmployeeID)
		if err != nil {
			return err
		}
		if e.Kind != "teacher" || !e.IsActive {
			return ErrConflict
		}
		var count int64
		err = tx.Table("academic_sessions AS s").Joins("JOIN academic_classes AS c ON c.id=?", a.ClassID).Joins("JOIN academic_subjects AS u ON u.id=?", a.SubjectID).Joins("JOIN academic_class_subjects AS m ON m.class_id=c.id AND m.subject_id=u.id").Where("s.id=? AND c.is_active=true AND u.is_active=true", a.SessionID).Count(&count).Error
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
		if err := tx.Raw("INSERT INTO teacher_assignments (employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?) RETURNING id", a.EmployeeID, a.SessionID, a.ClassID, a.SubjectID).Scan(&a.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "staff.assignment_created", "teacher_assignment", a.ID, map[string]any{"employee_id": a.EmployeeID, "session_id": a.SessionID, "class_id": a.ClassID, "subject_id": a.SubjectID})
	})
	if err != nil {
		return Assignment{}, dbError(err)
	}
	return r.assignment(r.db.WithContext(ctx), a.ID)
}
func (r *GormRepository) DeleteAssignment(ctx context.Context, actor, employee, id string) error {
	return dbError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.allowed(tx, actor, "staff:manage"); err != nil {
			return err
		}
		deleted := tx.Exec("DELETE FROM teacher_assignments WHERE id=? AND employee_id=?", id, employee)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "staff.assignment_deleted", "teacher_assignment", id, map[string]any{"employee_id": employee})
	}))
}

const leaveSelect = "id,employee_id,to_char(starts_on,'YYYY-MM-DD') AS starts_on,to_char(ends_on,'YYYY-MM-DD') AS ends_on,leave_type,reason,status,reviewed_by"

func (r *GormRepository) ownID(tx *gorm.DB, actor string) (string, error) {
	var id string
	err := tx.Table("employees AS e").Joins("JOIN users AS u ON u.id=e.user_id").Select("e.id").Where("e.user_id=? AND e.is_active=true AND u.is_active=true AND ((e.kind='teacher' AND u.role IN ('teacher','head_teacher')) OR (e.kind='non_teaching' AND u.role IN ('admin','accountant')))", actor).Take(&id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	return id, err
}
func (r *GormRepository) ListLeaves(ctx context.Context, actor, employee string, self bool) ([]Leave, error) {
	db := r.db.WithContext(ctx)
	if self {
		var err error
		employee, err = r.ownID(db, actor)
		if err != nil {
			return nil, err
		}
	} else {
		if _, err := r.GetEmployee(ctx, employee); err != nil {
			return nil, err
		}
	}
	items := []Leave{}
	err := db.Table("employee_leaves").Select(leaveSelect).Where("employee_id=?", employee).Order("starts_on DESC,id DESC").Scan(&items).Error
	return items, err
}
func (r *GormRepository) RequestLeave(ctx context.Context, actor, employee string, l Leave) (Leave, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if employee == "" {
			var err error
			employee, err = r.ownID(tx, actor)
			if err != nil {
				return err
			}
		} else if err := r.allowed(tx, actor, "staff_leaves:manage"); err != nil {
			return err
		}
		e, err := r.employee(tx, employee)
		if err != nil {
			return err
		}
		if !e.IsActive {
			return ErrConflict
		}
		l.EmployeeID = employee
		if err := tx.Raw("INSERT INTO employee_leaves (employee_id,starts_on,ends_on,leave_type,reason) VALUES (?,?,?,?,?) RETURNING id", employee, l.StartsOn, l.EndsOn, l.LeaveType, l.Reason).Scan(&l.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "staff.leave_requested", "employee_leave", l.ID, map[string]any{"employee_id": employee})
	})
	if err != nil {
		return Leave{}, dbError(err)
	}
	var out Leave
	err = r.db.WithContext(ctx).Table("employee_leaves").Select(leaveSelect).Where("id=?", l.ID).Take(&out).Error
	return out, err
}
func (r *GormRepository) DecideLeave(ctx context.Context, actor, id, status string) (Leave, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.allowed(tx, actor, "staff_leaves:manage"); err != nil {
			return err
		}
		update := tx.Exec("UPDATE employee_leaves SET status=?,reviewed_by=?,reviewed_at=now(),updated_at=now() WHERE id=? AND status='pending'", status, actor, id)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrConflict
		}
		return audit.Record(tx, &actor, "staff.leave_decided", "employee_leave", id, map[string]any{"status": status})
	})
	if err != nil {
		return Leave{}, dbError(err)
	}
	var out Leave
	err = r.db.WithContext(ctx).Table("employee_leaves").Select(leaveSelect).Where("id=?", id).Take(&out).Error
	return out, err
}

const salarySelect = "id,employee_id,to_char(effective_on,'YYYY-MM-DD') AS effective_on,basic_bdt,allowance_bdt,deduction_bdt,note"

func (r *GormRepository) ListSalaries(ctx context.Context, employee string) ([]Salary, error) {
	if _, err := r.GetEmployee(ctx, employee); err != nil {
		return nil, err
	}
	items := []Salary{}
	err := r.db.WithContext(ctx).Table("employee_salary_records").Select(salarySelect).Where("employee_id=?", employee).Order("effective_on DESC,id DESC").Scan(&items).Error
	return items, err
}
func (r *GormRepository) AddSalary(ctx context.Context, actor string, s Salary) (Salary, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.allowed(tx, actor, "staff_salary:manage"); err != nil {
			return err
		}
		if _, err := r.employee(tx, s.EmployeeID); err != nil {
			return err
		}
		if err := tx.Raw("INSERT INTO employee_salary_records (employee_id,effective_on,basic_bdt,allowance_bdt,deduction_bdt,note) VALUES (?,?,?,?,?,?) RETURNING id", s.EmployeeID, s.EffectiveOn, s.BasicBDT, s.AllowanceBDT, s.DeductionBDT, s.Note).Scan(&s.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "staff.salary_created", "employee_salary_record", s.ID, map[string]any{"employee_id": s.EmployeeID, "effective_on": s.EffectiveOn})
	})
	if err != nil {
		return Salary{}, dbError(err)
	}
	var out Salary
	err = r.db.WithContext(ctx).Table("employee_salary_records").Select(salarySelect).Where("id=?", s.ID).Take(&out).Error
	return out, err
}
