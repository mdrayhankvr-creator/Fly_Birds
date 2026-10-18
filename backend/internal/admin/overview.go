package admin

import (
	"context"

	"gorm.io/gorm"
)

type RoleCount struct {
	Role   string `json:"role"`
	Total  int64  `json:"total"`
	Active int64  `json:"active"`
}
type RecentEvent struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action"`
	Outcome   string `json:"outcome"`
}

// Overview is the administrator control center summary. Every figure comes
// from the database; nothing is estimated.
type Overview struct {
	SchemaVersion   int           `json:"schema_version"`
	DatabaseBytes   int64         `json:"database_bytes"`
	Users           []RoleCount   `json:"users"`
	LockedUsers     int64         `json:"locked_users"`
	ActiveStudents  int64         `json:"active_students"`
	Guardians       int64         `json:"guardians"`
	ActiveStaff     int64         `json:"active_staff"`
	ModulesEnabled  int64         `json:"modules_enabled"`
	ModulesTotal    int64         `json:"modules_total"`
	FeesOutstanding float64       `json:"fees_outstanding"`
	FeesOverdue     float64       `json:"fees_overdue"`
	ExamsPublished  int64         `json:"exams_published"`
	ExamsDraft      int64         `json:"exams_draft"`
	SMSQueued       int64         `json:"sms_queued"`
	SMSFailed       int64         `json:"sms_failed"`
	Attachments     int64         `json:"attachments"`
	AttachmentBytes int64         `json:"attachment_bytes"`
	AuditLastDay    int64         `json:"audit_last_day"`
	DeniedLastDay   int64         `json:"denied_last_day"`
	LastBackup      *Run          `json:"last_backup"`
	RecentEvents    []RecentEvent `json:"recent_events"`
}

type OverviewReader struct {
	db      *gorm.DB
	backups *Backups
}

func NewOverviewReader(db *gorm.DB, backups *Backups) *OverviewReader {
	return &OverviewReader{db: db, backups: backups}
}

func (o *OverviewReader) Overview(ctx context.Context) (Overview, error) {
	db := o.db.WithContext(ctx)
	out := Overview{Users: []RoleCount{}, RecentEvents: []RecentEvent{}}
	scalars := []struct {
		query  string
		target any
	}{
		{"SELECT COALESCE(max(version), 0) FROM schema_migrations", &out.SchemaVersion},
		{"SELECT pg_database_size(current_database())", &out.DatabaseBytes},
		{"SELECT count(*) FROM users WHERE locked_until > now()", &out.LockedUsers},
		{"SELECT count(*) FROM students WHERE status = 'active'", &out.ActiveStudents},
		{"SELECT count(*) FROM guardians", &out.Guardians},
		{"SELECT count(*) FROM employees WHERE is_active", &out.ActiveStaff},
		{"SELECT count(*) FILTER (WHERE enabled) FROM school_modules WHERE school_id = 1", &out.ModulesEnabled},
		{"SELECT count(*) FROM school_modules WHERE school_id = 1", &out.ModulesTotal},
		{"SELECT COALESCE(sum(balance), 0) FROM student_fees", &out.FeesOutstanding},
		{"SELECT COALESCE(sum(balance) FILTER (WHERE due_date < (now() AT TIME ZONE 'Asia/Dhaka')::date), 0) FROM student_fees", &out.FeesOverdue},
		{"SELECT count(*) FROM exams WHERE status IN ('published', 'locked')", &out.ExamsPublished},
		{"SELECT count(*) FROM exams WHERE status = 'draft'", &out.ExamsDraft},
		{"SELECT count(*) FROM sms_messages WHERE status = 'queued'", &out.SMSQueued},
		{"SELECT count(*) FROM sms_messages WHERE status = 'failed'", &out.SMSFailed},
		{"SELECT count(*) FROM file_objects WHERE linked_at IS NOT NULL", &out.Attachments},
		{"SELECT COALESCE(sum(size_bytes), 0) FROM file_objects WHERE linked_at IS NOT NULL", &out.AttachmentBytes},
		{"SELECT count(*) FROM audit_events WHERE created_at > now() - interval '1 day'", &out.AuditLastDay},
		{"SELECT count(*) FROM audit_events WHERE outcome = 'denied' AND created_at > now() - interval '1 day'", &out.DeniedLastDay},
	}
	for _, s := range scalars {
		if err := db.Raw(s.query).Row().Scan(s.target); err != nil {
			return out, err
		}
	}
	if err := db.Raw(`SELECT r.id AS role, count(u.id) AS total, count(u.id) FILTER (WHERE u.is_active) AS active
FROM roles r LEFT JOIN users u ON u.role = r.id GROUP BY r.id
ORDER BY array_position(ARRAY['admin','head_teacher','teacher','student','guardian','accountant'], r.id), r.id`).Scan(&out.Users).Error; err != nil {
		return out, err
	}
	if err := db.Raw(`SELECT e.id, to_char(e.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
  COALESCE(NULLIF(u.display_name, ''), u.email, '') AS actor_name, e.action, e.outcome
FROM audit_events e LEFT JOIN users u ON u.id = e.actor_user_id ORDER BY e.created_at DESC, e.id DESC LIMIT 8`).Scan(&out.RecentEvents).Error; err != nil {
		return out, err
	}
	var lastID string
	if err := db.Raw("SELECT id FROM backup_runs WHERE status = 'completed' AND deleted_at IS NULL ORDER BY started_at DESC LIMIT 1").Row().Scan(&lastID); err == nil && o.backups != nil {
		if r, err := o.backups.run(db, lastID); err == nil {
			out.LastBackup = &r
		}
	}
	return out, nil
}
