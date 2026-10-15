package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	FindUserByEmail(context.Context, string) (User, error)
	RecordLoginFailure(context.Context, string, time.Time) error
	ClearLoginFailures(context.Context, string, time.Time) error
	CreateSession(context.Context, *Session) error
	RotateSession(context.Context, []byte, []byte, []byte, []byte, time.Time, time.Time) (Session, User, error)
	RevokeSession(context.Context, []byte, []byte, time.Time) error
	ActiveUserBySession(context.Context, string, time.Time) (User, error)
	CreateUser(context.Context, *User) error
	HasPermission(context.Context, string, string) (bool, error)
	PermissionsForUser(context.Context, string) ([]string, error)
}

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) FindUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (r *GormRepository) RecordLoginFailure(ctx context.Context, userID string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"failed_login_attempts": gorm.Expr("failed_login_attempts + 1"),
		"locked_until":          gorm.Expr("CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END", MaxLoginFailure, now.Add(LockoutDuration)),
		"updated_at":            now,
	}).Error
}

func (r *GormRepository) ClearLoginFailures(ctx context.Context, userID string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&User{}).Where("id = ? AND (locked_until IS NULL OR locked_until <= ?)", userID, now).Updates(map[string]any{
		"failed_login_attempts": 0,
		"locked_until":          nil,
		"updated_at":            now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInvalidCredentials
	}
	return nil
}

func (r *GormRepository) CreateSession(ctx context.Context, session *Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *GormRepository) RotateSession(ctx context.Context, oldHash, csrfHash, newHash, newCSRFHash []byte, expiresAt, now time.Time) (Session, User, error) {
	var session Session
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("refresh_hash = ?", oldHash).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return err
		}
		if session.RevokedAt != nil || !session.ExpiresAt.After(now) || subtle.ConstantTimeCompare(session.CSRFHash, csrfHash) != 1 {
			return ErrInvalidSession
		}
		if err := tx.Where("id = ? AND is_active = true", session.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return err
		}
		if err := tx.Model(&Session{}).Where("id = ?", session.ID).Updates(map[string]any{
			"refresh_hash": newHash, "csrf_hash": newCSRFHash, "expires_at": expiresAt, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		session.RefreshHash, session.CSRFHash, session.ExpiresAt = newHash, newCSRFHash, expiresAt
		return nil
	})
	return session, user, err
}

func (r *GormRepository) RevokeSession(ctx context.Context, refreshHash, csrfHash []byte, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("refresh_hash = ?", refreshHash).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidSession
			}
			return err
		}
		if session.RevokedAt != nil || !session.ExpiresAt.After(now) || subtle.ConstantTimeCompare(session.CSRFHash, csrfHash) != 1 {
			return ErrInvalidSession
		}
		return tx.Model(&Session{}).Where("id = ?", session.ID).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error
	})
}

func (r *GormRepository) ActiveUserBySession(ctx context.Context, sessionID string, now time.Time) (User, error) {
	var session Session
	if err := r.db.WithContext(ctx).Where("id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, now).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrInvalidSession
		}
		return User{}, err
	}
	var user User
	if err := r.db.WithContext(ctx).Where("id = ? AND is_active = true", session.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrInvalidSession
		}
		return User{}, err
	}
	return user, nil
}

func (r *GormRepository) CreateUser(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *GormRepository) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id = u.role").
		Where("u.id = ? AND u.is_active = true AND rp.permission_id = ?", userID, permission).Count(&count).Error
	return count > 0, err
}

func (r *GormRepository) PermissionsForUser(ctx context.Context, userID string) ([]string, error) {
	ids := []string{}
	err := r.db.WithContext(ctx).Table("users AS u").Joins("JOIN role_permissions AS rp ON rp.role_id = u.role").
		Where("u.id = ? AND u.is_active = true", userID).Order("rp.permission_id").Pluck("rp.permission_id", &ids).Error
	return ids, err
}
