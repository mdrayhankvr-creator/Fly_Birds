package rbac

import (
	"context"
	"errors"
	"sort"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) ListRoles(ctx context.Context, search string, page, size int) ([]Role, int64, error) {
	q := r.db.WithContext(ctx).Table("roles")
	if search != "" {
		q = q.Where("id ILIKE ? OR name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []Role{}
	err := q.Order("id").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}
func (r *GormRepository) ListPermissions(ctx context.Context, search string, page, size int) ([]Permission, int64, error) {
	q := r.db.WithContext(ctx).Table("permissions")
	if search != "" {
		q = q.Where("id ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []Permission{}
	err := q.Order("id").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}
func (r *GormRepository) RolePermissions(ctx context.Context, role string) ([]string, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("roles").Where("id = ?", role).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNotFound
	}
	ids := []string{}
	err := r.db.WithContext(ctx).Table("role_permissions").Where("role_id = ?", role).Order("permission_id").Pluck("permission_id", &ids).Error
	return ids, err
}
func (r *GormRepository) UserRoles(ctx context.Context, user string) ([]string, error) {
	var role string
	err := r.db.WithContext(ctx).Table("users").Select("role").Where("id = ?", user).Scan(&role).Error
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrNotFound
	}
	return []string{role}, nil
}
func guard(tx *gorm.DB, actor string) error {
	var count int64
	err := tx.Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id = u.role").
		Where("u.id = ? AND u.is_active = true AND u.role = 'admin' AND rp.permission_id = 'permissions:manage'", actor).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrForbidden
	}
	return nil
}
func lockUpdates(tx *gorm.DB) error { return tx.Exec("SELECT pg_advisory_xact_lock(?)", 517002).Error }

func (r *GormRepository) ReplaceRolePermissions(ctx context.Context, actor, role string, ids []string) (Change, error) {
	change := Change{ActorID: actor, TargetID: role}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUpdates(tx); err != nil {
			return err
		}
		if err := guard(tx, actor); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("roles").Where("id = ?", role).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		if len(ids) > 0 {
			if err := tx.Table("permissions").Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return ErrInvalid
			}
		}
		before := []string{}
		if err := tx.Table("role_permissions").Where("role_id = ?", role).Order("permission_id").Pluck("permission_id", &before).Error; err != nil {
			return err
		}
		change.Before = before
		// The management gateway requires the admin role. Removing its grant would
		// lock out every administrator, including currently active ones.
		if role == "admin" {
			hasManage, hasSelf := false, false
			for _, id := range ids {
				if id == "permissions:manage" {
					hasManage = true
				}
				if id == "auth:read_self" {
					hasSelf = true
				}
			}
			if !hasManage || !hasSelf {
				return ErrLastAdmin
			}
		}
		if err := tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", role).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES (?,?)", role, id).Error; err != nil {
				return err
			}
		}
		change.After = append([]string{}, ids...)
		sort.Strings(change.After)
		return audit.Record(tx, &actor, "permissions.replace", "role", role, map[string]any{"before": before, "after": change.After})
	})
	return change, err
}
func (r *GormRepository) ReplaceUserRole(ctx context.Context, actor, user, role string) (Change, error) {
	change := Change{ActorID: actor, TargetID: user}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUpdates(tx); err != nil {
			return err
		}
		if err := guard(tx, actor); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("roles").Where("id = ?", role).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrInvalid
		}
		var existing struct {
			Role     string
			IsActive bool
		}
		result := tx.Table("users").Select("role,is_active").Where("id = ?", user).Take(&existing)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		change.Before = []string{existing.Role}
		change.After = []string{role}
		if existing.Role == "admin" && role != "admin" && existing.IsActive {
			if err := tx.Table("users").Where("role = 'admin' AND is_active = true").Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdmin
			}
		}
		if existing.Role == role {
			return nil
		}
		if err := tx.Table("users").Where("id = ?", user).Updates(map[string]any{"role": role, "updated_at": gorm.Expr("now()")}).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "user.role_changed", "user", user, map[string]any{"before_role": existing.Role, "after_role": role})
	})
	return change, err
}
