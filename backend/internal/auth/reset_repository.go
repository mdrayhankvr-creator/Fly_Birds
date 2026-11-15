package auth

import (
	"context"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ResetRequest is one row of password_reset_requests (migration 1813): an
// issued link, or an attempt for an address with no account, which is kept
// only so the rate limit counts it.
type ResetRequest struct {
	ID          string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	EmailSHA256 []byte     `gorm:"type:bytea;not null;column:email_sha256"`
	RequesterIP string     `gorm:"not null;column:requester_ip"`
	UserID      *string    `gorm:"type:uuid;column:user_id"`
	TokenSHA256 []byte     `gorm:"type:bytea;column:token_sha256"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
	UsedAt      *time.Time `gorm:"column:used_at"`
}

func (ResetRequest) TableName() string { return "password_reset_requests" }

// resetIPColumn keeps the stored client address short and printable; an
// unknown or malformed address is stored as the empty string, which the
// per-client limit then skips.
func resetIPColumn(ip string) string {
	if len(ip) > 64 {
		return ""
	}
	for _, r := range ip {
		if r < 0x20 || r > 0x7e {
			return ""
		}
	}
	return ip
}

// FindResetCandidate returns the single active account that signs in with
// this address. A student login with the placeholder address has no mailbox
// to send to, and a suspended account is not recovered by email, so neither
// is a candidate; both answer ErrNotFound, which the service turns into the
// same reply as a success.
func (r *GormRepository) FindResetCandidate(ctx context.Context, email string) (User, error) {
	if IsPlaceholderEmail(email) {
		return User{}, ErrNotFound
	}
	var user User
	err := r.db.WithContext(ctx).Where("email = ? AND is_active", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

// RecentResetAttempts counts the rows logged for this address and from this
// client since the given time. Issued links and logged misses both count, so
// an address with no account cannot be used to probe the limit.
func (r *GormRepository) RecentResetAttempts(ctx context.Context, emailDigest []byte, ip string, since time.Time) (ResetAttempts, error) {
	var counts ResetAttempts
	db := r.db.WithContext(ctx).Model(&ResetRequest{}).Where("created_at >= ?", since)
	if err := db.Session(&gorm.Session{}).Where("email_sha256 = ?", emailDigest).Count(&counts.ByEmail).Error; err != nil {
		return ResetAttempts{}, err
	}
	if column := resetIPColumn(ip); column != "" {
		if err := db.Session(&gorm.Session{}).Where("requester_ip = ?", column).Count(&counts.ByIP).Error; err != nil {
			return ResetAttempts{}, err
		}
	}
	return counts, nil
}

// LogResetAttempt records a request that issued no link.
func (r *GormRepository) LogResetAttempt(ctx context.Context, emailDigest []byte, ip string, now time.Time) error {
	return r.db.WithContext(ctx).Create(&ResetRequest{
		EmailSHA256: emailDigest, RequesterIP: resetIPColumn(ip), CreatedAt: now,
	}).Error
}

// CreateResetToken records an issued link and audits it in the same
// transaction. Only the SHA-256 of the token is stored, so the table cannot
// be read back into a working link, and the audit record names the account
// without the address: audit records never carry credentials.
func (r *GormRepository) CreateResetToken(ctx context.Context, emailDigest []byte, ip, userID string, tokenDigest []byte, now, expiresAt time.Time) error {
	expires := expiresAt
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ResetRequest{
			EmailSHA256: emailDigest, RequesterIP: resetIPColumn(ip), UserID: &userID,
			TokenSHA256: tokenDigest, CreatedAt: now, ExpiresAt: &expires,
		}).Error; err != nil {
			return err
		}
		return audit.Record(tx, nil, "user.password_reset_requested", "user", userID,
			map[string]any{"source": "email_link", "expires_at": expires.UTC().Format(time.RFC3339)})
	})
}

// liveToken matches an unused link that has not expired and whose account is
// still active.
func liveToken(db *gorm.DB, tokenDigest []byte, now time.Time) *gorm.DB {
	return db.Where("token_sha256 = ? AND used_at IS NULL AND expires_at > ?", tokenDigest, now)
}

// ResetTokenUser returns the account a live link belongs to, leaving the link
// usable.
func (r *GormRepository) ResetTokenUser(ctx context.Context, tokenDigest []byte, now time.Time) (User, error) {
	var request ResetRequest
	err := liveToken(r.db.WithContext(ctx), tokenDigest, now).First(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && request.UserID == nil) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	var user User
	err = r.db.WithContext(ctx).Where("id = ? AND is_active", *request.UserID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

// ConsumeResetToken sets the password through a live link. Everything happens
// in one transaction: the link is marked used, so a second attempt with the
// same link finds nothing; the account's other outstanding links are marked
// used too, so an older email in the same inbox stops working; the lockout
// and the forced password change are cleared, because the account has now
// chosen its own password; and every session is revoked, so a signed-in
// thief is signed out. It returns the account and the number of sessions
// revoked.
func (r *GormRepository) ConsumeResetToken(ctx context.Context, tokenDigest []byte, newHash string, now time.Time) (User, int64, error) {
	var user User
	var revoked int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Claiming the link first makes a second, concurrent use find no row.
		claim := liveToken(tx.Model(&ResetRequest{}), tokenDigest, now).Update("used_at", now)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return ErrNotFound
		}
		var request ResetRequest
		if err := tx.Where("token_sha256 = ?", tokenDigest).First(&request).Error; err != nil {
			return err
		}
		if request.UserID == nil {
			return ErrNotFound
		}
		if err := tx.Where("id = ? AND is_active", *request.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"password_hash": newHash, "failed_login_attempts": 0, "locked_until": nil,
			"must_change_password": false, "temporary_password_expires_at": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		// Any other link the account has outstanding dies with this one.
		if err := tx.Model(&ResetRequest{}).
			Where("user_id = ? AND used_at IS NULL AND token_sha256 IS NOT NULL", user.ID).
			Update("used_at", now).Error; err != nil {
			return err
		}
		sessions := tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Updates(map[string]any{"revoked_at": now, "updated_at": now})
		if sessions.Error != nil {
			return sessions.Error
		}
		revoked = sessions.RowsAffected
		actor := user.ID
		return audit.Record(tx, &actor, "user.password_reset", "user", user.ID, map[string]any{
			"source": "email_link", "sessions_revoked": revoked,
		})
	})
	if err != nil {
		return User{}, 0, err
	}
	user.PasswordHash = newHash
	user.MustChangePassword = false
	return user, revoked, nil
}
