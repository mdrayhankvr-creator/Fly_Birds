package auth

import (
	"context"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ErrUserNotFound is returned when no account has the given email.
var ErrUserNotFound = errors.New("no account with that email")

// ResetPassword is the server operator's recovery path (cmd/reset-password):
// it sets a new password, clears any login lockout and revokes every session
// of the account, in one audited transaction.
func (r *GormRepository) ResetPassword(ctx context.Context, rawEmail, password string, now time.Time) (User, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, ErrUserNotFound
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var user User
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email = ?", email).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"password_hash": hash, "failed_login_attempts": 0, "locked_until": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		revoked := tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).Updates(map[string]any{"revoked_at": now, "updated_at": now})
		if revoked.Error != nil {
			return revoked.Error
		}
		return audit.Record(tx, nil, "user.password_reset", "user", user.ID, map[string]any{"source": "operator_cli", "sessions_revoked": revoked.RowsAffected})
	})
	return user, err
}
