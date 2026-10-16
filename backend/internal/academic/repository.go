package academic

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

func academicDBError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrDuplicate
		case "23503":
			return ErrNotFound
		case "23514", "23502":
			return ErrInvalid
		}
	}
	return err
}
func requireManage(tx *gorm.DB, actor string) error {
	var count int64
	if err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.id=? AND u.is_active=true AND u.role='admin' AND rp.permission_id='academics:manage'", actor).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}
func pageBase(q Query) bool { return q.Status == "active" }

func (r *GormRepository) ListClasses(ctx context.Context, q Query) (Page[Class], error) {
	base := r.db.WithContext(ctx).Table("academic_classes")
	if q.Search != "" {
		base = base.Where("name ILIKE ?", "%"+q.Search+"%")
	}
	if q.Status != "" {
		base = base.Where("is_active=?", pageBase(q))
	}
	result := Page[Class]{Items: []Class{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page[Class]{}, err
	}
	err := base.Select("id,name,display_order,is_active").Order("display_order ASC,name ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}
func (r *GormRepository) GetClass(ctx context.Context, id string) (Class, error) {
	var item Class
	err := r.db.WithContext(ctx).Table("academic_classes").Select("id,name,display_order,is_active").Where("id=?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Class{}, ErrNotFound
	}
	return item, err
}
func (r *GormRepository) SaveClass(ctx context.Context, actor, id string, value Class) (Class, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO academic_classes (name,display_order,is_active) VALUES (?,?,?) RETURNING id", value.Name, value.DisplayOrder, value.IsActive).Scan(&value.ID).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Table("academic_classes").Where("id=?", id).Updates(map[string]any{"name": value.Name, "display_order": value.DisplayOrder, "is_active": value.IsActive, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 0 {
				return ErrNotFound
			}
			value.ID = id
		}
		return audit.Record(tx, &actor, "academic.class_saved", "academic_class", value.ID, map[string]any{"created": id == ""})
	})
	return value, academicDBError(err)
}
func (r *GormRepository) ListSubjects(ctx context.Context, q Query) (Page[Subject], error) {
	base := r.db.WithContext(ctx).Table("academic_subjects")
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("name_en ILIKE ? OR name_bn ILIKE ? OR code ILIKE ?", term, term, term)
	}
	if q.Status != "" {
		base = base.Where("is_active=?", pageBase(q))
	}
	result := Page[Subject]{Items: []Subject{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page[Subject]{}, err
	}
	err := base.Select("id,name_en,name_bn,code,is_active").Order("code ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}
func (r *GormRepository) GetSubject(ctx context.Context, id string) (Subject, error) {
	var item Subject
	err := r.db.WithContext(ctx).Table("academic_subjects").Select("id,name_en,name_bn,code,is_active").Where("id=?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Subject{}, ErrNotFound
	}
	return item, err
}
func (r *GormRepository) SaveSubject(ctx context.Context, actor, id string, value Subject) (Subject, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO academic_subjects (name_en,name_bn,code,is_active) VALUES (?,?,?,?) RETURNING id", value.NameEN, value.NameBN, value.Code, value.IsActive).Scan(&value.ID).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Table("academic_subjects").Where("id=?", id).Updates(map[string]any{"name_en": value.NameEN, "name_bn": value.NameBN, "code": value.Code, "is_active": value.IsActive, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 0 {
				return ErrNotFound
			}
			value.ID = id
		}
		return audit.Record(tx, &actor, "academic.subject_saved", "academic_subject", value.ID, map[string]any{"created": id == ""})
	})
	return value, academicDBError(err)
}

const mappingSelect = "m.id,m.class_id,m.subject_id,m.is_required,m.is_fourth_subject,s.name_en AS subject_name_en,s.name_bn AS subject_name_bn,s.code AS subject_code"

func (r *GormRepository) ListMappings(ctx context.Context, classID string, q Query) (Page[Mapping], error) {
	var exists int64
	if err := r.db.WithContext(ctx).Table("academic_classes").Where("id=?", classID).Count(&exists).Error; err != nil {
		return Page[Mapping]{}, err
	}
	if exists == 0 {
		return Page[Mapping]{}, ErrNotFound
	}
	base := r.db.WithContext(ctx).Table("academic_class_subjects AS m").Joins("JOIN academic_subjects AS s ON s.id=m.subject_id").Where("m.class_id=?", classID)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.name_en ILIKE ? OR s.name_bn ILIKE ? OR s.code ILIKE ?", term, term, term)
	}
	result := Page[Mapping]{Items: []Mapping{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page[Mapping]{}, err
	}
	err := base.Select(mappingSelect).Order("s.code ASC,m.id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}
func activeRecord(tx *gorm.DB, table, id string) error {
	var row struct{ IsActive bool }
	err := tx.Table(table).Select("is_active").Where("id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !row.IsActive {
		return ErrInactive
	}
	return nil
}
func mappingByPair(tx *gorm.DB, classID, subjectID string) (Mapping, error) {
	var value Mapping
	err := tx.Table("academic_class_subjects AS m").Joins("JOIN academic_subjects AS s ON s.id=m.subject_id").Select(mappingSelect).Where("m.class_id=? AND m.subject_id=?", classID, subjectID).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Mapping{}, ErrNotFound
	}
	return value, err
}
func (r *GormRepository) CreateMapping(ctx context.Context, actor string, value Mapping) (Mapping, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if err := activeRecord(tx, "academic_classes", value.ClassID); err != nil {
			return err
		}
		if err := activeRecord(tx, "academic_subjects", value.SubjectID); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO academic_class_subjects (class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", value.ClassID, value.SubjectID, value.IsRequired, value.IsFourthSubject).Error; err != nil {
			return err
		}
		var err error
		value, err = mappingByPair(tx, value.ClassID, value.SubjectID)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.mapping_created", "academic_class_subject", value.ID, map[string]any{"class_id": value.ClassID, "subject_id": value.SubjectID})
	})
	return value, academicDBError(err)
}
func (r *GormRepository) UpdateMapping(ctx context.Context, actor string, value Mapping) (Mapping, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if err := activeRecord(tx, "academic_classes", value.ClassID); err != nil {
			return err
		}
		if err := activeRecord(tx, "academic_subjects", value.SubjectID); err != nil {
			return err
		}
		updated := tx.Table("academic_class_subjects").Where("class_id=? AND subject_id=?", value.ClassID, value.SubjectID).Updates(map[string]any{"is_required": value.IsRequired, "is_fourth_subject": value.IsFourthSubject, "updated_at": time.Now().UTC()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return ErrNotFound
		}
		var err error
		value, err = mappingByPair(tx, value.ClassID, value.SubjectID)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.mapping_updated", "academic_class_subject", value.ID, map[string]any{"is_required": value.IsRequired, "is_fourth_subject": value.IsFourthSubject})
	})
	return value, academicDBError(err)
}
func (r *GormRepository) DeleteMapping(ctx context.Context, actor, classID, subjectID string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		value, err := mappingByPair(tx, classID, subjectID)
		if err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM academic_class_subjects WHERE id=?", value.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.mapping_deleted", "academic_class_subject", value.ID, map[string]any{"class_id": classID, "subject_id": subjectID})
	})
	return academicDBError(err)
}
