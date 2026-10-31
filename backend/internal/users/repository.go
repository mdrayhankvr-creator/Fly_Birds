package users

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
func mapError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrDuplicate
	}
	return err
}
func require(tx *gorm.DB, actor, permission string) error {
	var count int64
	if err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.id=? AND u.is_active=true AND u.role='admin' AND rp.permission_id=?", actor, permission).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrForbidden
	}
	return nil
}
func (r *GormRepository) Create(ctx context.Context, actor string, user User, hash string) (User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := require(tx, actor, "users:create"); err != nil {
			return err
		}
		if err := require(tx, actor, "permissions:manage"); err != nil {
			return err
		}
		if err := tx.Table("users").Create(map[string]any{"email": user.Email, "display_name": user.DisplayName, "role": user.Role, "is_active": true, "password_hash": hash}).Error; err != nil {
			return err
		}
		if err := tx.Table("users").Select("id,email,display_name,role,is_active").Where("email=?", user.Email).Take(&user).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "user.created", "user", user.ID, map[string]any{"role": user.Role})
	})
	return user, mapError(err)
}
func (r *GormRepository) List(ctx context.Context, q Query) (Page, error) {
	base := r.db.WithContext(ctx).Table("users")
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("email ILIKE ? OR display_name ILIKE ?", term, term)
	}
	if q.Role != "" {
		base = base.Where("role=?", q.Role)
	}
	if q.Status != "" {
		base = base.Where("is_active=?", q.Status == "active")
	}
	page := Page{Items: []User{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&page.Total).Error; err != nil {
		return Page{}, err
	}
	err := base.Select("id,email,display_name,role,is_active").Order("email ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&page.Items).Error
	return page, err
}
func (r *GormRepository) Get(ctx context.Context, id string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Table("users").Select("id,email,display_name,role,is_active").Where("id=?", id).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}
func (r *GormRepository) Update(ctx context.Context, actor, id string, input UpdateInput) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := require(tx, actor, "users:update"); err != nil {
			return err
		}
		var before User
		if err := tx.Table("users").Select("id,email,display_name,role,is_active").Where("id=?", id).Take(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if before.Email == input.Email && before.DisplayName == input.DisplayName {
			user = before
			return nil
		}
		if err := tx.Table("users").Where("id=?", id).Updates(map[string]any{"email": input.Email, "display_name": input.DisplayName, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := audit.Record(tx, &actor, "user.updated", "user", id, map[string]any{"fields": []string{"email", "display_name"}}); err != nil {
			return err
		}
		user = before
		user.Email = input.Email
		user.DisplayName = input.DisplayName
		return nil
	})
	return user, mapError(err)
}

// ResetPassword stores the new hash, requires a password change at the next
// sign-in, clears any lockout and revokes every session of the account, in
// one transaction. The audit entry never holds the password.
func (r *GormRepository) ResetPassword(ctx context.Context, actor, id, hash string) (User, int64, error) {
	var user User
	var revoked int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := require(tx, actor, "users:update"); err != nil {
			return err
		}
		if err := tx.Table("users").Select("id,email,display_name,role,is_active").Where("id=?", id).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if err := tx.Table("users").Where("id=?", id).Updates(map[string]any{"password_hash": hash, "must_change_password": true, "failed_login_attempts": 0, "locked_until": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		result := tx.Table("auth_sessions").Where("user_id=? AND revoked_at IS NULL", id).Updates(map[string]any{"revoked_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		revoked = result.RowsAffected
		return audit.Record(tx, &actor, "user.password_reset", "user", id, map[string]any{"source": "admin", "sessions_revoked": revoked})
	})
	return user, revoked, mapError(err)
}
func (r *GormRepository) SetStatus(ctx context.Context, actor, id string, active bool) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", 517002).Error; err != nil {
			return err
		}
		if err := require(tx, actor, "users:update"); err != nil {
			return err
		}
		if err := tx.Table("users").Select("id,email,display_name,role,is_active").Where("id=?", id).Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if user.IsActive == active {
			return nil
		}
		if user.Role == "admin" && user.IsActive && !active {
			var count int64
			if err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id=u.role").Where("u.role='admin' AND u.is_active=true AND rp.permission_id='permissions:manage'").Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdmin
			}
		}
		now := time.Now().UTC()
		if err := tx.Table("users").Where("id=?", id).Updates(map[string]any{"is_active": active, "updated_at": now}).Error; err != nil {
			return err
		}
		if !active {
			if err := tx.Table("auth_sessions").Where("user_id=? AND revoked_at IS NULL", id).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if err := audit.Record(tx, &actor, "user.status_changed", "user", id, map[string]any{"before_active": user.IsActive, "after_active": active}); err != nil {
			return err
		}
		user.IsActive = active
		return nil
	})
	return user, err
}
