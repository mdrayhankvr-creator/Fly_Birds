package school

import (
	"context"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type Repository interface {
	Settings(context.Context) (SchoolSettings, error)
	UpdateSettings(context.Context, string, SettingsInput) (SchoolSettings, error)
	Grading(context.Context) ([]GradeBand, error)
	ReplaceGrading(context.Context, string, []GradeBand) ([]GradeBand, error)
	Modules(context.Context, string, int, int) (ModulePage, error)
	ModuleEnabled(context.Context, string) (bool, error)
	UpdateModule(context.Context, string, string, bool, bool) (Module, error)
}
type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Settings(ctx context.Context) (SchoolSettings, error) {
	var settings SchoolSettings
	err := r.db.WithContext(ctx).First(&settings, "id = ?", SingleSchoolID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return SchoolSettings{}, ErrNotFound
	}
	return settings, err
}
func schoolLock(tx *gorm.DB) error { return tx.Exec("SELECT pg_advisory_xact_lock(?)", 517003).Error }
func schoolGuard(tx *gorm.DB, actor, permission string) error {
	var count int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id = u.role").
		Where("u.id = ? AND u.is_active = true AND u.role = 'admin' AND rp.permission_id = ?", actor, permission).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrForbidden
	}
	return nil
}
func (r *GormRepository) UpdateSettings(ctx context.Context, actor string, input SettingsInput) (SchoolSettings, error) {
	var settings SchoolSettings
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "school_settings:update"); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("grading_schemes").Where("id = ?", input.GradingSchemeID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrInvalid
		}
		updates := map[string]any{"name_en": input.NameEN, "name_bn": input.NameBN, "logo_url": input.LogoURL, "address": input.Address, "phone": input.Phone, "email": input.Email, "preferred_language": input.PreferredLanguage, "primary_color": input.PrimaryColor, "secondary_color": input.SecondaryColor, "dashboard_theme": input.DashboardTheme, "grading_scheme_id": input.GradingSchemeID, "updated_at": time.Now().UTC()}
		result := tx.Model(&SchoolSettings{}).Where("id = ?", SingleSchoolID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		if err := tx.First(&settings, "id = ?", SingleSchoolID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.settings_updated", "school", "1", map[string]any{"fields": []string{"name_en", "name_bn", "logo_url", "address", "phone", "email", "preferred_language", "primary_color", "secondary_color", "dashboard_theme", "grading_scheme_id"}})
	})
	return settings, err
}
func gradingIn(tx *gorm.DB) ([]GradeBand, error) {
	var schemeID string
	if err := tx.Table("school_settings").Select("grading_scheme_id").Where("id = ?", SingleSchoolID).Scan(&schemeID).Error; err != nil {
		return nil, err
	}
	if schemeID == "" {
		return nil, ErrNotFound
	}
	bands := []GradeBand{}
	err := tx.Where("scheme_id = ?", schemeID).Order("min_marks DESC").Find(&bands).Error
	return bands, err
}
func (r *GormRepository) Grading(ctx context.Context) ([]GradeBand, error) {
	return gradingIn(r.db.WithContext(ctx))
}
func (r *GormRepository) ReplaceGrading(ctx context.Context, actor string, bands []GradeBand) ([]GradeBand, error) {
	var updated []GradeBand
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := schoolLock(tx); err != nil {
			return err
		}
		if err := schoolGuard(tx, actor, "grading:update"); err != nil {
			return err
		}
		var schemeID string
		if err := tx.Table("school_settings").Select("grading_scheme_id").Where("id = ?", SingleSchoolID).Scan(&schemeID).Error; err != nil {
			return err
		}
		if schemeID == "" {
			return ErrNotFound
		}
		if err := tx.Where("scheme_id = ?", schemeID).Delete(&GradeBand{}).Error; err != nil {
			return err
		}
		for _, b := range bands {
			b.SchemeID = schemeID
			if err := tx.Create(&b).Error; err != nil {
				return err
			}
		}
		var err error
		updated, err = gradingIn(tx)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "school.grading_updated", "grading_scheme", schemeID, map[string]any{"band_count": len(updated)})
	})
	return updated, err
}
