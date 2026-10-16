package academic

import (
	"context"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type SectionGormRepository struct{ db *gorm.DB }

func NewSectionRepository(db *gorm.DB) *SectionGormRepository { return &SectionGormRepository{db: db} }

const sectionSelect = "s.id,s.session_id,s.class_id,s.name,s.code,s.display_order,s.is_active,a.name AS session_name,c.name AS class_name"

func sectionBase(db *gorm.DB) *gorm.DB {
	return db.Table("academic_sections AS s").
		Joins("JOIN academic_sessions AS a ON a.id=s.session_id").
		Joins("JOIN academic_classes AS c ON c.id=s.class_id")
}

func (r *SectionGormRepository) ListSections(ctx context.Context, q SectionQuery) (Page[Section], error) {
	base := sectionBase(r.db.WithContext(ctx))
	if q.SessionID != "" {
		base = base.Where("s.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("s.class_id=?", q.ClassID)
	}
	if q.Status != "" {
		base = base.Where("s.is_active=?", q.Status == "active")
	}
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.name ILIKE ? OR s.code ILIKE ? OR c.name ILIKE ? OR a.name ILIKE ?", term, term, term, term)
	}
	result := Page[Section]{Items: []Section{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page[Section]{}, err
	}
	err := base.Select(sectionSelect).Order("a.start_date DESC,c.display_order ASC,s.display_order ASC,s.code ASC,s.id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}

func sectionByID(tx *gorm.DB, id string) (Section, error) {
	var value Section
	err := sectionBase(tx).Select(sectionSelect).Where("s.id=?", id).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Section{}, ErrNotFound
	}
	return value, err
}

func (r *SectionGormRepository) GetSection(ctx context.Context, id string) (Section, error) {
	return sectionByID(r.db.WithContext(ctx), id)
}

func (r *SectionGormRepository) SaveSection(ctx context.Context, actor, id string, value Section) (Section, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO academic_sections (session_id,class_id,name,code,display_order,is_active) VALUES (?,?,?,?,?,?) RETURNING id", value.SessionID, value.ClassID, value.Name, value.Code, value.DisplayOrder, value.IsActive).Scan(&value.ID).Error; err != nil {
				return err
			}
		} else {
			current, err := sectionByID(tx, id)
			if err != nil {
				return err
			}
			if current.SessionID != value.SessionID || current.ClassID != value.ClassID {
				return ErrInvalid
			}
			updated := tx.Table("academic_sections").Where("id=?", id).Updates(map[string]any{"name": value.Name, "code": value.Code, "display_order": value.DisplayOrder, "is_active": value.IsActive, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			value.ID = id
		}
		var err error
		value, err = sectionByID(tx, value.ID)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.section_saved", "academic_section", value.ID, map[string]any{"created": id == "", "session_id": value.SessionID, "class_id": value.ClassID})
	})
	return value, academicDBError(err)
}
