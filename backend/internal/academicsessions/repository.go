package academicsessions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

const columns = "id,name,to_char(start_date,'YYYY-MM-DD') AS start_date,to_char(end_date,'YYYY-MM-DD') AS end_date,is_active,created_at,updated_at"

func mapError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "academic_sessions_name_key_key" {
		return ErrDuplicate
	}
	return err
}
func requireWrite(tx *gorm.DB, actor, permission string) error {
	var count int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").
		Joins("JOIN school_modules AS sm ON sm.module_key='academics' AND sm.school_id=1").
		Where("u.id=? AND u.is_active=true AND u.role='admin' AND rp.permission_id=? AND sm.enabled=true", actor, permission).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}
func byID(tx *gorm.DB, id string) (Session, error) {
	var result Session
	err := tx.Table("academic_sessions").Select(columns).Where("id=?", id).Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, ErrNotFound
	}
	return result, err
}
func (r *GormRepository) Create(ctx context.Context, actor string, input Input, key string) (Session, error) {
	var result Session
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireWrite(tx, actor, "academic_sessions:create"); err != nil {
			return err
		}
		if err := tx.Table("academic_sessions").Create(map[string]any{"name": input.Name, "start_date": input.StartDate, "end_date": input.EndDate}).Error; err != nil {
			return err
		}
		if err := tx.Table("academic_sessions").Select(columns).Where("name_key=?", key).Take(&result).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic_session.created", "academic_session", result.ID, map[string]any{"name": result.Name})
	})
	return result, mapError(err)
}
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
func (r *GormRepository) List(ctx context.Context, q Query) (Page, error) {
	base := r.db.WithContext(ctx).Table("academic_sessions")
	if q.Search != "" {
		base = base.Where("name ILIKE ? ESCAPE '\\'", "%"+escapeLike(q.Search)+"%")
	}
	result := Page{Items: []Session{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page{}, err
	}
	err := base.Select(columns).Order("start_date DESC,id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}
func (r *GormRepository) Get(ctx context.Context, id string) (Session, error) {
	return byID(r.db.WithContext(ctx), id)
}
func (r *GormRepository) Update(ctx context.Context, actor, id string, input Input) (Session, error) {
	var result Session
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireWrite(tx, actor, "academic_sessions:update"); err != nil {
			return err
		}
		var before Session
		err := tx.Table("academic_sessions").Clauses(clause.Locking{Strength: "UPDATE"}).Select(columns).Where("id=?", id).Take(&before).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if before.Name == input.Name && before.StartDate == input.StartDate && before.EndDate == input.EndDate {
			result = before
			return nil
		}
		if err := tx.Table("academic_sessions").Where("id=?", id).Updates(map[string]any{"name": input.Name, "start_date": input.StartDate, "end_date": input.EndDate, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		result, err = byID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic_session.updated", "academic_session", id, map[string]any{"name": result.Name})
	})
	return result, mapError(err)
}
func (r *GormRepository) SetActive(ctx context.Context, actor, id string, active bool) (Session, error) {
	var result Session
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// All activation and deactivation requests serialize before reading state.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", 517101).Error; err != nil {
			return err
		}
		if err := requireWrite(tx, actor, "academic_sessions:activate"); err != nil {
			return err
		}
		var err error
		result, err = byID(tx, id)
		if err != nil {
			return err
		}
		if result.IsActive == active {
			return nil
		}
		now := time.Now().UTC()
		if active {
			if err := tx.Table("academic_sessions").Where("is_active=true").Updates(map[string]any{"is_active": false, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Table("academic_sessions").Where("id=?", id).Updates(map[string]any{"is_active": active, "updated_at": now}).Error; err != nil {
			return err
		}
		result, err = byID(tx, id)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic_session.activation_changed", "academic_session", id, map[string]any{"is_active": active})
	})
	return result, err
}
func (r *GormRepository) Current(ctx context.Context) (Session, error) {
	var result Session
	err := r.db.WithContext(ctx).Table("academic_sessions").Select(columns).Where("is_active=true").Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, ErrNotFound
	}
	return result, err
}
