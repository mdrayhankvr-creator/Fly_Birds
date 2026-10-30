package promotion

import (
	"context"

	"gorm.io/gorm"
)

// LeaverLogin is a student who already left (withdrawn or transferred)
// while their login is still active: left over from before withdrawal
// suspended logins (QA-03).
type LeaverLogin struct {
	StudentID   string `json:"student_id"`
	AdmissionNo string `json:"admission_no"`
	StudentName string `json:"student_name"`
	Status      string `json:"status"`
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Sessions    int    `json:"sessions"`
}

const leaverLogins = `SELECT s.id AS student_id, s.admission_no, s.name_en AS student_name, s.status, u.id AS user_id, u.email,
       (SELECT count(*) FROM auth_sessions a WHERE a.user_id = u.id AND a.revoked_at IS NULL AND a.expires_at > now()) AS sessions
  FROM students s JOIN users u ON u.id = s.user_id
 WHERE s.status IN ('inactive', 'transferred') AND u.is_active
 ORDER BY s.admission_no, s.id`

// ListLeaverLogins only reads.
func ListLeaverLogins(ctx context.Context, db *gorm.DB) ([]LeaverLogin, error) {
	out := []LeaverLogin{}
	err := db.WithContext(ctx).Raw(leaverLogins).Scan(&out).Error
	return out, err
}

// SuspendLeaverLogins disables every such login in one transaction: the
// account is deactivated, its sessions are revoked, the student is marked
// as suspended by leaving (so readmission restores the login, as for a new
// withdrawal) and one audit event is written per student. It returns the
// students it changed.
func SuspendLeaverLogins(ctx context.Context, db *gorm.DB, command string) ([]LeaverLogin, error) {
	var out []LeaverLogin
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		out = []LeaverLogin{}
		if err := tx.Raw(leaverLogins + " FOR UPDATE OF s, u").Scan(&out).Error; err != nil {
			return err
		}
		for _, l := range out {
			if err := tx.Exec("UPDATE users SET is_active = false, updated_at = now() WHERE id = ? AND is_active", l.UserID).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE auth_sessions SET revoked_at = now(), updated_at = now() WHERE user_id = ? AND revoked_at IS NULL", l.UserID).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE students SET login_suspended_at = now() WHERE id = ?", l.StudentID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO audit_events (actor_user_id, action, target_type, target_id, outcome, metadata)
VALUES (NULL, 'student.login_suspended', 'student', ?, 'success', jsonb_build_object('user_id', ?::text, 'status', ?::text, 'source', ?::text, 'sessions_revoked', ?::int))`,
				l.StudentID, l.UserID, l.Status, command, l.Sessions).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
