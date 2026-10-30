package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	FindUserByEmail(context.Context, string) (User, error)
	RecordLoginFailure(context.Context, string, time.Time) error
	ClearLoginFailures(context.Context, string, time.Time) error
	CreateSession(context.Context, *Session) error
	RotateSession(context.Context, []byte, []byte, []byte, []byte, time.Time, time.Time) (Session, User, error)
	FindActiveUser(context.Context, string) (User, error)
	ChangePassword(context.Context, string, string, string, string, time.Time) (int64, error)
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
			"failed_login_attempts": gorm.Expr("failed_login_attempts + 1"),
			"locked_until":          gorm.Expr("CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END", MaxLoginFailure, now.Add(LockoutDuration)),
			"updated_at":            now,
		}).Error; err != nil {
			return err
		}
		return audit.Record(tx, &userID, "auth.login_failed", "user", userID, nil)
	})
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		return audit.Record(tx, &session.UserID, "auth.login", "user", session.UserID, nil)
	})
}

func (r *GormRepository) RotateSession(ctx context.Context, oldHash, csrfHash, newHash, newCSRFHash []byte, expiresAt, now time.Time) (Session, User, error) {
	var session Session
	var user User
	reused := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("refresh_hash = ?", oldHash).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var revoke bool
				if revoke, err = revokeReplayedSession(tx, oldHash, now); err != nil {
					return err
				}
				// Commit the revocation; the caller still receives ErrInvalidSession.
				reused = revoke
				return nil
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
		// Remember the outgoing token so a later replay can be recognised. Entries
		// older than a refresh lifetime belong to tokens that have expired anyway.
		if err := tx.Where("session_id = ? AND rotated_at < ?", session.ID, now.Add(-RefreshLifetime)).Delete(&RefreshHistory{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&RefreshHistory{RefreshHash: oldHash, SessionID: session.ID, RotatedAt: now}).Error; err != nil {
			return err
		}
		session.RefreshHash, session.CSRFHash, session.ExpiresAt = newHash, newCSRFHash, expiresAt
		return nil
	})
	if err == nil && (reused || session.ID == "" || user.ID == "") {
		return Session{}, User{}, ErrInvalidSession
	}
	return session, user, err
}

// revokeReplayedSession handles a refresh token that matches no current
// session. If the session previously rotated away from it, the token was
// reused: the whole session is revoked unless the replay arrived within the
// grace period, which covers two browser tabs refreshing at the same moment.
func revokeReplayedSession(tx *gorm.DB, refreshHash []byte, now time.Time) (bool, error) {
	var previous RefreshHistory
	if err := tx.Where("refresh_hash = ?", refreshHash).First(&previous).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if now.Sub(previous.RotatedAt) <= RefreshReuseGrace {
		return false, nil
	}
	var session Session
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", previous.SessionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if session.RevokedAt != nil {
		return false, nil
	}
	if err := tx.Model(&Session{}).Where("id = ?", session.ID).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
		return false, err
	}
	return true, audit.Record(tx, &session.UserID, "auth.refresh_reuse_detected", "user", session.UserID, map[string]any{"session_id": session.ID})
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
		if err := tx.Model(&Session{}).Where("id = ?", session.ID).Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return audit.Record(tx, &session.UserID, "auth.logout", "user", session.UserID, nil)
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

func (r *GormRepository) FindActiveUser(ctx context.Context, id string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("id = ? AND is_active = true", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

// ChangePassword replaces the hash only if it still equals the one that was
// verified, then revokes every other session of the user, in one audited
// transaction. It returns the number of sessions revoked.
func (r *GormRepository) ChangePassword(ctx context.Context, userID, keepSessionID, verifiedHash, newHash string, now time.Time) (int64, error) {
	var revoked int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&User{}).Where("id = ? AND is_active = true AND password_hash = ?", userID, verifiedHash).Updates(map[string]any{
			"password_hash": newHash, "failed_login_attempts": 0, "locked_until": nil, "must_change_password": false, "updated_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrInvalidCredentials
		}
		result := tx.Model(&Session{}).Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, keepSessionID).Updates(map[string]any{"revoked_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		revoked = result.RowsAffected
		return audit.Record(tx, &userID, "user.password_changed", "user", userID, map[string]any{"source": "self_service", "sessions_revoked": revoked})
	})
	return revoked, err
}
