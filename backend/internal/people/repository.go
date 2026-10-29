package people

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }
func databaseError(err error) error {
	if errors.Is(historyError(err), ErrHistory) {
		return ErrHistory
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrDuplicate
		case "23503", "23514", "23502", "22P02":
			return ErrInvalid
		}
	}
	return err
}
func requireAdmin(tx *gorm.DB, a Actor, permission string) error {
	var count int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.id=? AND u.is_active AND u.role='admin' AND rp.permission_id=?", a.ID, permission).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}
func requireScopedGrant(tx *gorm.DB, a Actor) error {
	permission := ""
	switch a.Role {
	case "admin", "head_teacher":
		permission = "students:read"
	case "student":
		permission = "students:read_self"
	case "guardian":
		permission = "guardians:read_children"
	default:
		return ErrForbidden
	}
	var n int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.id=? AND u.is_active AND rp.permission_id=?", a.ID, permission).Count(&n).Error
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrForbidden
	}
	return nil
}

const studentColumns = "s.id,s.admission_no,s.name_en,s.name_bn,COALESCE(to_char(s.date_of_birth,'YYYY-MM-DD'),'') AS date_of_birth,s.status,s.session_id,s.class_id,s.section_id,s.roll_no,s.user_id,a.name AS session_name,c.name AS class_name,sec.name AS section_name,(p.student_id IS NOT NULL) AS has_photo,s.student_code,s.gender,s.blood_group,s.father_name_en,s.father_name_bn,s.mother_name_en,s.mother_name_bn,s.present_address,s.permanent_address,COALESCE(to_char(s.admission_date,'YYYY-MM-DD'),'') AS admission_date"

func studentBase(db *gorm.DB) *gorm.DB {
	return db.Table("students AS s").Joins("JOIN academic_sessions AS a ON a.id=s.session_id").Joins("JOIN academic_classes AS c ON c.id=s.class_id").Joins("JOIN academic_sections AS sec ON sec.id=s.section_id").Joins("LEFT JOIN student_photos AS p ON p.student_id=s.id")
}
func (r *GormRepository) ListStudents(ctx context.Context, q Query) (Page[Student], error) {
	base := studentBase(r.db.WithContext(ctx))
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.admission_no ILIKE ? OR s.name_en ILIKE ? OR s.name_bn ILIKE ? OR c.name ILIKE ? OR sec.name ILIKE ?", term, term, term, term, term)
	}
	if q.Status != "" {
		base = base.Where("s.status=?", q.Status)
	}
	if q.SessionID != "" {
		base = base.Where("s.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("s.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("s.section_id=?", q.SectionID)
	}
	out := Page[Student]{Items: []Student{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&out.Total).Error; err != nil {
		return Page[Student]{}, err
	}
	err := base.Select(studentColumns).Order("s.admission_no ASC,s.id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&out.Items).Error
	return out, err
}
func scopedStudent(tx *gorm.DB, a Actor) *gorm.DB {
	base := studentBase(tx)
	switch a.Role {
	case "admin", "head_teacher":
		return base
	case "student":
		return base.Where("s.user_id=?", a.ID)
	case "guardian":
		return base.Joins("JOIN student_guardians AS sg ON sg.student_id=s.id").Joins("JOIN guardians AS g ON g.id=sg.guardian_id").Where("g.user_id=?", a.ID)
	default:
		return base.Where("1=0")
	}
}
func (r *GormRepository) GetStudent(ctx context.Context, a Actor, id string) (Student, error) {
	if err := requireScopedGrant(r.db.WithContext(ctx), a); err != nil {
		return Student{}, err
	}
	var v Student
	err := scopedStudent(r.db.WithContext(ctx), a).Select(studentColumns).Where("s.id=?", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Student{}, ErrNotFound
	}
	if err != nil {
		return Student{}, err
	}
	return v, withPrimaryGuardian(r.db.WithContext(ctx), &v)
}
func (r *GormRepository) OwnStudent(ctx context.Context, a Actor) (Student, error) {
	if a.Role != "student" {
		return Student{}, ErrForbidden
	}
	if err := requireScopedGrant(r.db.WithContext(ctx), a); err != nil {
		return Student{}, err
	}
	var v Student
	err := scopedStudent(r.db.WithContext(ctx), a).Select(studentColumns).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Student{}, ErrNotFound
	}
	if err != nil {
		return Student{}, err
	}
	return v, withPrimaryGuardian(r.db.WithContext(ctx), &v)
}

// withPrimaryGuardian attaches the student's primary guardian, if one is
// linked, from the existing relationship (no separate copy is stored).
func withPrimaryGuardian(tx *gorm.DB, v *Student) error {
	var g GuardianSummary
	err := tx.Table("student_guardians AS sg").Joins("JOIN guardians AS g ON g.id=sg.guardian_id").Select("g.name_en,g.name_bn,sg.relationship,g.phone").Where("sg.student_id=? AND sg.is_primary", v.ID).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err == nil {
		v.PrimaryGuardian = &g
	}
	return err
}

// profileArgs lists the optional text details in column order; nil means "not sent".
func profileArgs(v StudentInput) []any {
	return []any{v.StudentCode, v.Gender, v.BloodGroup, v.FatherNameEN, v.FatherNameBN, v.MotherNameEN, v.MotherNameBN, v.PresentAddress, v.PermanentAddress}
}

func saveStudentTx(tx *gorm.DB, id string, v StudentInput) (string, error) {
	base := []any{v.AdmissionNo, v.NameEN, v.NameBN, v.DateOfBirth, v.Status, v.SessionID, v.ClassID, v.SectionID, v.RollNo, v.UserID}
	if id == "" {
		args := append(append(base, profileArgs(v)...), v.AdmissionDate)
		err := tx.Raw("INSERT INTO students (admission_no,name_en,name_bn,date_of_birth,status,session_id,class_id,section_id,roll_no,user_id,student_code,gender,blood_group,father_name_en,father_name_bn,mother_name_en,mother_name_bn,present_address,permanent_address,admission_date) VALUES (?,?,?,NULLIF(?,'')::date,?,?,?,?,?,NULLIF(?,'')::uuid,COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),COALESCE(?,''),NULLIF(?::text,'')::date) RETURNING id", args...).Scan(&id).Error
		return id, err
	}
	args := append(append(base, profileArgs(v)...), v.AdmissionDate, v.AdmissionDate, id)
	res := tx.Exec("UPDATE students SET admission_no=?,name_en=?,name_bn=?,date_of_birth=NULLIF(?,'')::date,status=?,session_id=?,class_id=?,section_id=?,roll_no=?,user_id=NULLIF(?,'')::uuid,student_code=COALESCE(?,student_code),gender=COALESCE(?,gender),blood_group=COALESCE(?,blood_group),father_name_en=COALESCE(?,father_name_en),father_name_bn=COALESCE(?,father_name_bn),mother_name_en=COALESCE(?,mother_name_en),mother_name_bn=COALESCE(?,mother_name_bn),present_address=COALESCE(?,present_address),permanent_address=COALESCE(?,permanent_address),admission_date=CASE WHEN ?::text IS NULL THEN admission_date ELSE NULLIF(?::text,'')::date END,updated_at=now() WHERE id=?", args...)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return id, nil
}
func (r *GormRepository) SaveStudent(ctx context.Context, a Actor, id string, v StudentInput) (Student, error) {
	created := id == ""
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		var err error
		id, err = saveStudentTx(tx, id, v)
		if err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "student.saved", "student", id, map[string]any{"created": created})
	})
	if err != nil {
		return Student{}, databaseError(err)
	}
	return r.GetStudent(ctx, a, id)
}
func (r *GormRepository) DeleteStudent(ctx context.Context, a Actor, id string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		result := tx.Exec("DELETE FROM students WHERE id=?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "student.deleted", "student", id, nil)
	})
	return databaseError(err)
}
func (r *GormRepository) ImportStudents(ctx context.Context, a Actor, rows []StudentInput) (int, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		for _, row := range rows {
			if _, err := saveStudentTx(tx, "", row); err != nil {
				return err
			}
		}
		return audit.Record(tx, &a.ID, "student.imported", "student_import", a.ID, map[string]any{"count": len(rows)})
	})
	if err != nil {
		return 0, databaseError(err)
	}
	return len(rows), nil
}
func (r *GormRepository) SavePhoto(ctx context.Context, a Actor, id string, p Photo) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "students:manage"); err != nil {
			return err
		}
		var n int64
		if err := tx.Table("students").Where("id=?", id).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if err := tx.Exec("INSERT INTO student_photos (student_id,content_type,content) VALUES (?,?,?) ON CONFLICT (student_id) DO UPDATE SET content_type=EXCLUDED.content_type,content=EXCLUDED.content,updated_at=now()", id, p.ContentType, p.Content).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "student.photo_saved", "student", id, map[string]any{"content_type": p.ContentType, "bytes": len(p.Content)})
	})
	return databaseError(err)
}
func (r *GormRepository) GetPhoto(ctx context.Context, a Actor, id string) (Photo, error) {
	if _, err := r.GetStudent(ctx, a, id); err != nil {
		return Photo{}, err
	}
	var p Photo
	err := r.db.WithContext(ctx).Table("student_photos").Select("content_type,content").Where("student_id=?", id).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Photo{}, ErrNotFound
	}
	return p, err
}
func (r *GormRepository) ListGuardians(ctx context.Context, q Query) (Page[Guardian], error) {
	base := r.db.WithContext(ctx).Table("guardians")
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("name_en ILIKE ? OR name_bn ILIKE ? OR phone ILIKE ? OR email ILIKE ?", term, term, term, term)
	}
	out := Page[Guardian]{Items: []Guardian{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&out.Total).Error; err != nil {
		return Page[Guardian]{}, err
	}
	err := base.Select("id,name_en,name_bn,phone,email,address,user_id").Order("name_en ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&out.Items).Error
	return out, err
}
func (r *GormRepository) GetGuardian(ctx context.Context, id string) (Guardian, error) {
	var v Guardian
	err := r.db.WithContext(ctx).Table("guardians").Select("id,name_en,name_bn,phone,email,address,user_id").Where("id=?", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Guardian{}, ErrNotFound
	}
	return v, err
}
func (r *GormRepository) OwnGuardian(ctx context.Context, a Actor) (Guardian, error) {
	if a.Role != "guardian" {
		return Guardian{}, ErrForbidden
	}
	var v Guardian
	err := r.db.WithContext(ctx).Table("guardians").Select("id,name_en,name_bn,phone,email,address,user_id").Where("user_id=?", a.ID).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Guardian{}, ErrNotFound
	}
	return v, err
}
func (r *GormRepository) SaveGuardian(ctx context.Context, a Actor, id string, v GuardianInput) (Guardian, error) {
	created := id == ""
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
			return err
		}
		if created {
			if err := tx.Raw("INSERT INTO guardians (name_en,name_bn,phone,email,address,user_id) VALUES (?,?,?,?,?,NULLIF(?,'')::uuid) RETURNING id", v.NameEN, v.NameBN, v.Phone, v.Email, v.Address, v.UserID).Scan(&id).Error; err != nil {
				return err
			}
		} else {
			result := tx.Exec("UPDATE guardians SET name_en=?,name_bn=?,phone=?,email=?,address=?,user_id=NULLIF(?,'')::uuid,updated_at=now() WHERE id=?", v.NameEN, v.NameBN, v.Phone, v.Email, v.Address, v.UserID, id)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		return audit.Record(tx, &a.ID, "guardian.saved", "guardian", id, map[string]any{"created": created})
	})
	if err != nil {
		return Guardian{}, databaseError(err)
	}
	return r.GetGuardian(ctx, id)
}
func (r *GormRepository) DeleteGuardian(ctx context.Context, a Actor, id string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
			return err
		}
		result := tx.Exec("DELETE FROM guardians WHERE id=?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "guardian.deleted", "guardian", id, nil)
	})
	return databaseError(err)
}
func (r *GormRepository) ListRelationships(ctx context.Context, id string) ([]Relationship, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("students").Where("id=?", id).Count(&n).Error; err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	items := []Relationship{}
	err := r.db.WithContext(ctx).Table("student_guardians AS sg").Joins("JOIN guardians AS g ON g.id=sg.guardian_id").Select("sg.student_id,sg.guardian_id,sg.relationship,sg.is_primary,g.name_en AS guardian_name,g.name_bn AS guardian_name_bn,g.phone").Where("sg.student_id=?", id).Order("sg.is_primary DESC,g.name_en ASC").Scan(&items).Error
	return items, err
}
func (r *GormRepository) Link(ctx context.Context, a Actor, id string, v LinkInput) (Relationship, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO student_guardians (student_id,guardian_id,relationship,is_primary) VALUES (?,?,?,?) ON CONFLICT (student_id,guardian_id) DO UPDATE SET relationship=EXCLUDED.relationship,is_primary=EXCLUDED.is_primary", id, v.GuardianID, v.Relationship, v.IsPrimary).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "student.guardian_linked", "student", id, map[string]any{"guardian_id": v.GuardianID, "is_primary": v.IsPrimary})
	})
	if err != nil {
		return Relationship{}, databaseError(err)
	}
	return Relationship{StudentID: id, GuardianID: v.GuardianID, Relationship: v.Relationship, IsPrimary: v.IsPrimary}, nil
}
func (r *GormRepository) Unlink(ctx context.Context, a Actor, id, guardian string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdmin(tx, a, "guardians:manage"); err != nil {
			return err
		}
		result := tx.Exec("DELETE FROM student_guardians WHERE student_id=? AND guardian_id=?", id, guardian)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "student.guardian_unlinked", "student", id, map[string]any{"guardian_id": guardian})
	})
	return databaseError(err)
}
func (r *GormRepository) Children(ctx context.Context, a Actor) ([]Student, error) {
	if a.Role != "guardian" {
		return nil, ErrForbidden
	}
	if err := requireScopedGrant(r.db.WithContext(ctx), a); err != nil {
		return nil, err
	}
	items := []Student{}
	err := scopedStudent(r.db.WithContext(ctx), a).Select(studentColumns).Order("s.admission_no ASC").Scan(&items).Error
	return items, err
}
func (r *GormRepository) GuardianStudents(ctx context.Context, id string) ([]Student, error) {
	var n int64
	if err := r.db.WithContext(ctx).Table("guardians").Where("id=?", id).Count(&n).Error; err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	items := []Student{}
	err := studentBase(r.db.WithContext(ctx)).Joins("JOIN student_guardians AS sg ON sg.student_id=s.id").Where("sg.guardian_id=?", id).Select(studentColumns).Order("s.admission_no ASC").Scan(&items).Error
	return items, err
}
