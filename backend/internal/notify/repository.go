package notify

import (
	"context"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notices"
	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

const utcFormat = `'YYYY-MM-DD"T"HH24:MI:SS"Z"'`

func (r *GormRepository) Inbox(ctx context.Context, user string, unreadOnly bool, page, size int) (Inbox, error) {
	db := r.db.WithContext(ctx)
	out := Inbox{Items: []Notification{}, Page: page, PageSize: size}
	base := db.Table("notifications").Where("user_id = ? AND withdrawn_at IS NULL", user)
	if err := base.Session(&gorm.Session{}).Where("read_at IS NULL").Count(&out.Unread).Error; err != nil {
		return out, err
	}
	if unreadOnly {
		base = base.Where("read_at IS NULL")
	}
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := base.Select("id,category,title_en,title_bn,body_en,body_bn,link,urgent,read_at IS NOT NULL AS read,to_char(created_at AT TIME ZONE 'UTC'," + utcFormat + ") AS created_at").
		Order("read_at IS NOT NULL, created_at DESC, id").Limit(size).Offset((page - 1) * size).Find(&out.Items).Error
	return out, err
}

func (r *GormRepository) MarkRead(ctx context.Context, user, id string) error {
	res := r.db.WithContext(ctx).Exec("UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = ? AND user_id = ? AND withdrawn_at IS NULL", id, user)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) MarkAllRead(ctx context.Context, user string) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE notifications SET read_at = now() WHERE user_id = ? AND read_at IS NULL AND withdrawn_at IS NULL", user)
	return res.RowsAffected, res.Error
}

func (r *GormRepository) Preferences(ctx context.Context, user string) ([]Preference, error) {
	out := []Preference{}
	err := r.db.WithContext(ctx).Raw(`SELECT c.category, COALESCE(p.in_app, true) AS in_app, COALESCE(p.sms, true) AS sms
FROM unnest(ARRAY['notice','absence','result','fee','diary']) WITH ORDINALITY AS c(category, ord)
LEFT JOIN notification_preferences p ON p.user_id = ? AND p.category = c.category ORDER BY c.ord`, user).Scan(&out).Error
	return out, err
}

func (r *GormRepository) SavePreferences(ctx context.Context, user string, prefs []Preference) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, p := range prefs {
			if err := tx.Exec(`INSERT INTO notification_preferences(user_id,category,in_app,sms) VALUES (?,?,?,?)
ON CONFLICT (user_id, category) DO UPDATE SET in_app = EXCLUDED.in_app, sms = EXCLUDED.sms, updated_at = now()`, user, p.Category, p.InApp, p.SMS).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormRepository) SMSLog(ctx context.Context, status string, page, size int) (SMSPage, error) {
	db := r.db.WithContext(ctx)
	out := SMSPage{Items: []SMS{}, Page: page, PageSize: size, Counts: map[string]int64{}}
	var counts []struct {
		Status string
		N      int64
	}
	if err := db.Table("sms_messages").Select("status, count(*) AS n").Group("status").Scan(&counts).Error; err != nil {
		return out, err
	}
	for _, c := range counts {
		out.Counts[c.Status] = c.N
	}
	base := db.Table("sms_messages m").Joins("JOIN users u ON u.id = m.user_id")
	if status != "" {
		base = base.Where("m.status = ?", status)
	}
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	// Phone numbers are masked in the log; only the last three digits remain.
	err := base.Select("m.id,COALESCE(NULLIF(u.display_name,''),u.email) AS recipient_name,repeat('•', greatest(length(m.phone) - 3, 0)) || right(m.phone, 3) AS phone,m.category,m.body,m.status,m.provider,m.provider_ref,m.error,m.attempts," +
		"to_char(m.created_at AT TIME ZONE 'UTC'," + utcFormat + ") AS created_at,to_char(m.processed_at AT TIME ZONE 'UTC'," + utcFormat + ") AS processed_at").
		Order("m.created_at DESC, m.id").Limit(size).Offset((page - 1) * size).Find(&out.Items).Error
	return out, err
}

// source is a SELECT yielding one row per recipient with the columns
// user_id, source_key, title_en, title_bn, body_en, body_bn, link, urgent,
// phone, wants_sms and sms_body.
type source struct {
	category string
	module   string
	sql      string
	args     func(today string, at time.Time) []any
}

const school = `(SELECT preferred_language FROM school_settings WHERE id = 1)`
const schoolName = `(SELECT CASE WHEN preferred_language = 'bn' AND name_bn <> '' THEN name_bn ELSE name_en END FROM school_settings WHERE id = 1)`

// recipients expands a student to the student's own account and the
// accounts of linked guardians, with each guardian's phone for SMS.
const recipients = `JOIN LATERAL (
  SELECT s.user_id AS user_id, false AS guardian, '' AS phone_raw WHERE s.user_id IS NOT NULL
  UNION ALL
  SELECT g.user_id, true, g.phone FROM student_guardians sg JOIN guardians g ON g.id = sg.guardian_id WHERE sg.student_id = s.id AND g.user_id IS NOT NULL
) rcp ON true
JOIN users u ON u.id = rcp.user_id AND u.is_active`

var sources = []source{
	{category: "notice", module: "notices", sql: `SELECT u.id AS user_id, 'notice:' || n.id AS source_key,
  left(n.title_en, 70) AS title_en, left(COALESCE(NULLIF(n.title_bn, ''), n.title_en), 220) AS title_bn,
  left(n.body_en, 240) AS body_en, left(COALESCE(NULLIF(n.body_bn, ''), n.body_en), 700) AS body_bn,
  '/notices' AS link, n.priority = 'urgent' AS urgent,
  regexp_replace(COALESCE(g.phone, ''), '[^0-9+]', '', 'g') AS phone,
  (n.priority = 'urgent' AND u.role = 'guardian') AS wants_sms,
  ` + schoolName + ` || ': ' || CASE WHEN ` + school + ` = 'bn' THEN COALESCE(NULLIF(n.title_bn, ''), n.title_en) ELSE n.title_en END AS sms_body
FROM notices n
JOIN users u ON u.is_active AND u.role <> 'admin' AND ` + notices.AudiencePredicate("n", "notice_targets", "notice_id") + `
LEFT JOIN guardians g ON g.user_id = u.id
WHERE n.status = 'published' AND n.publish_at <= now() AND (n.expires_at IS NULL OR n.expires_at > now()) AND n.publish_at >= now() - interval '30 days'`},
	{category: "absence", module: "attendance", sql: `SELECT u.id AS user_id, 'absence:' || e.id AS source_key,
  left('Absence: ' || s.name_en, 70) AS title_en, left(COALESCE(NULLIF(s.name_bn, ''), s.name_en) || ' অনুপস্থিত', 220) AS title_bn,
  s.name_en || ' was marked absent on ' || to_char(e.attendance_date, 'DD Mon YYYY') || '.' AS body_en,
  COALESCE(NULLIF(s.name_bn, ''), s.name_en) || ' ' || to_char(e.attendance_date, 'DD-MM-YYYY') || ' তারিখে অনুপস্থিত ছিল।' AS body_bn,
  '/guardian/children/' || s.id || '/attendance' AS link, false AS urgent,
  regexp_replace(g.phone, '[^0-9+]', '', 'g') AS phone, true AS wants_sms,
  ` + schoolName + ` || ': ' || CASE WHEN ` + school + ` = 'bn' THEN COALESCE(NULLIF(s.name_bn, ''), s.name_en) || ' ' || to_char(e.attendance_date, 'DD-MM-YYYY') || ' তারিখে অনুপস্থিত।' ELSE s.name_en || ' was absent on ' || to_char(e.attendance_date, 'DD Mon YYYY') || '.' END AS sms_body
FROM attendance_absence_events e
JOIN students s ON s.id = e.student_id
JOIN student_guardians sg ON sg.student_id = s.id
JOIN guardians g ON g.id = sg.guardian_id
JOIN users u ON u.id = g.user_id AND u.is_active
WHERE e.state = 'pending' AND e.attendance_date >= ?::date - 7`, args: func(today string, _ time.Time) []any { return []any{today} }},
	{category: "result", module: "examinations", sql: `SELECT u.id AS user_id, 'result:' || x.id || ':' || s.id AS source_key,
  left('Results published: ' || x.name, 70) AS title_en, left('ফলাফল প্রকাশিত: ' || x.name, 220) AS title_bn,
  left(s.name_en || ' — ' || x.name || '. Open the report card for marks and GPA.', 240) AS body_en,
  left(COALESCE(NULLIF(s.name_bn, ''), s.name_en) || ' — ' || x.name || '। নম্বর ও জিপিএ দেখতে রিপোর্ট কার্ড খুলুন।', 700) AS body_bn,
  CASE WHEN rcp.guardian THEN '/guardian/children/' || s.id || '/results' ELSE '/student/results' END AS link, false AS urgent,
  regexp_replace(rcp.phone_raw, '[^0-9+]', '', 'g') AS phone, rcp.guardian AS wants_sms,
  ` + schoolName + ` || ': ' || CASE WHEN ` + school + ` = 'bn' THEN COALESCE(NULLIF(s.name_bn, ''), s.name_en) || '-এর ' || x.name || ' ফলাফল প্রকাশিত হয়েছে।' ELSE x.name || ' results are published for ' || s.name_en || '.' END AS sms_body
FROM exams x
JOIN exam_results er ON er.exam_id = x.id
JOIN students s ON s.id = er.student_id
` + recipients + `
WHERE x.status IN ('published', 'locked') AND x.published_at >= now() - interval '60 days'`},
	{category: "fee", module: "fees", sql: `SELECT u.id AS user_id, 'fee:' || f.id AS source_key,
  left('Fee overdue: ' || c.name_en, 70) AS title_en, left('বকেয়া ফি: ' || COALESCE(NULLIF(c.name_bn, ''), c.name_en), 220) AS title_bn,
  left(s.name_en || ' has Tk ' || f.balance || ' outstanding, due ' || to_char(f.due_date, 'DD Mon YYYY') || '.', 240) AS body_en,
  left(COALESCE(NULLIF(s.name_bn, ''), s.name_en) || '-এর ৳' || f.balance || ' বকেয়া, শেষ তারিখ ' || to_char(f.due_date, 'DD-MM-YYYY') || '।', 700) AS body_bn,
  CASE WHEN rcp.guardian THEN '/guardian/children/' || s.id || '/fees' ELSE '/student/fees' END AS link, false AS urgent,
  regexp_replace(rcp.phone_raw, '[^0-9+]', '', 'g') AS phone, rcp.guardian AS wants_sms,
  ` + schoolName + ` || ': ' || CASE WHEN ` + school + ` = 'bn' THEN COALESCE(NULLIF(s.name_bn, ''), s.name_en) || '-এর ' || COALESCE(NULLIF(c.name_bn, ''), c.name_en) || ' ৳' || f.balance || ' বকেয়া।' ELSE s.name_en || ' owes Tk ' || f.balance || ' for ' || c.name_en || '.' END AS sms_body
FROM student_fees f
JOIN fee_structures fs ON fs.id = f.structure_id
JOIN fee_categories c ON c.id = fs.category_id
JOIN students s ON s.id = f.student_id
` + recipients + `
WHERE f.balance > 0 AND f.due_date < ?::date AND f.due_date >= ?::date - 90`, args: func(today string, _ time.Time) []any { return []any{today, today} }},
	{category: "diary", module: "diary", sql: `SELECT u.id AS user_id, 'diary:' || d.id || ':' || s.id AS source_key,
  left('Class diary: ' || sub.name_en || ' (' || to_char(d.diary_date, 'DD Mon') || ')', 70) AS title_en,
  left('ক্লাস ডায়েরি: ' || COALESCE(NULLIF(sub.name_bn, ''), sub.name_en) || ' (' || to_char(d.diary_date, 'DD-MM') || ')', 220) AS title_bn,
  left(CASE WHEN d.homework <> '' THEN 'Homework: ' || d.homework ELSE d.lesson_topics END, 240) AS body_en,
  left(CASE WHEN d.homework <> '' THEN 'বাড়ির কাজ: ' || d.homework ELSE d.lesson_topics END, 700) AS body_bn,
  CASE WHEN rcp.guardian THEN '/guardian/children/' || s.id || '/diary' ELSE '/student/diary' END AS link, false AS urgent,
  '' AS phone, false AS wants_sms, '' AS sms_body
FROM class_diaries d
JOIN academic_subjects sub ON sub.id = d.subject_id
JOIN students s ON s.section_id = d.section_id AND s.session_id = d.session_id AND s.status = 'active'
` + recipients + `
WHERE d.status = 'published' AND d.published_at >= now() - interval '3 days'`},
}

// reminder notifies teachers who still have unpublished diaries for today's
// assigned sections when the school requires daily publication.
const reminder = `SELECT e.user_id AS user_id, 'diary_reminder:' || ?::text AS source_key,
  'Class diary reminder' AS title_en, 'ক্লাস ডায়েরির অনুস্মারক' AS title_bn,
  count(*) || ' class diaries for today are not published yet.' AS body_en,
  'আজকের ' || count(*) || 'টি ক্লাস ডায়েরি এখনো প্রকাশিত হয়নি।' AS body_bn,
  '/teacher/diary' AS link, false AS urgent, '' AS phone, false AS wants_sms, '' AS sms_body
FROM teacher_assignments a
JOIN employees e ON e.id = a.employee_id AND e.is_active AND e.user_id IS NOT NULL
JOIN users u ON u.id = e.user_id AND u.is_active
JOIN academic_sessions se ON se.id = a.session_id AND se.is_active AND ?::date BETWEEN se.start_date AND se.end_date
JOIN academic_sections sec ON sec.session_id = a.session_id AND sec.class_id = a.class_id AND sec.is_active
WHERE NOT EXISTS (SELECT 1 FROM class_diaries d WHERE d.section_id = sec.id AND d.subject_id = a.subject_id AND d.diary_date = ?::date AND d.status = 'published')
GROUP BY e.user_id`

func insertFrom(tx *gorm.DB, category, src string, args []any) (int64, int64, error) {
	all := append(append([]any{}, args...), category, category)
	res := tx.Exec(`WITH src AS (`+src+`)
INSERT INTO notifications(user_id, category, source_key, title_en, title_bn, body_en, body_bn, link, urgent)
SELECT src.user_id, ?, src.source_key, src.title_en, src.title_bn, src.body_en, src.body_bn, src.link, src.urgent FROM src
LEFT JOIN notification_preferences p ON p.user_id = src.user_id AND p.category = ?
WHERE COALESCE(p.in_app, true) OR src.urgent
ON CONFLICT (user_id, source_key) DO UPDATE SET withdrawn_at = NULL, read_at = NULL, created_at = now() WHERE notifications.withdrawn_at IS NOT NULL`, all...)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	sms := tx.Exec(`WITH src AS (`+src+`)
INSERT INTO sms_messages(user_id, phone, category, source_key, body)
SELECT src.user_id, src.phone, ?, src.source_key, left(src.sms_body, 300) FROM src
LEFT JOIN notification_preferences p ON p.user_id = src.user_id AND p.category = ?
WHERE src.wants_sms AND src.phone ~ '^[+]{0,1}[0-9]{8,15}$' AND COALESCE(p.sms, true)
ON CONFLICT (user_id, source_key) DO NOTHING`, all...)
	return res.RowsAffected, sms.RowsAffected, sms.Error
}

// withdrawals hide notifications whose cause no longer holds and cancel any
// still-queued SMS for them. Already-sent mock SMS stay in the log.
var withdrawals = []string{
	`category = 'absence' AND source_key IN (SELECT 'absence:' || id FROM attendance_absence_events WHERE state = 'cancelled')`,
	`category = 'result' AND split_part(source_key, ':', 2) IN (SELECT id::text FROM exams WHERE status = 'draft')`,
	`category = 'notice' AND split_part(source_key, ':', 2) IN (SELECT id::text FROM notices WHERE status = 'archived')`,
}

func (r *GormRepository) Dispatch(ctx context.Context, at time.Time) (Result, error) {
	res := Result{Created: map[string]int64{}}
	today := at.In(Dhaka).Format(time.DateOnly)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(903)").Row().Scan(&locked); err != nil {
			return err
		}
		if !locked {
			res.Skipped = true
			return nil
		}
		var enabled []string
		if err := tx.Table("school_modules").Where("school_id = 1 AND enabled").Pluck("module_key", &enabled).Error; err != nil {
			return err
		}
		on := map[string]bool{}
		for _, k := range enabled {
			on[k] = true
		}
		if !on["notices"] {
			res.Disabled = true
			return nil
		}
		for _, cond := range withdrawals {
			w := tx.Exec("UPDATE notifications SET withdrawn_at = now() WHERE withdrawn_at IS NULL AND " + cond)
			if w.Error != nil {
				return w.Error
			}
			res.Withdrawn += w.RowsAffected
			if err := tx.Exec("UPDATE sms_messages SET status = 'cancelled', processed_at = now(), error = 'withdrawn before sending' WHERE status = 'queued' AND " + cond).Error; err != nil {
				return err
			}
		}
		for _, s := range sources {
			if !on[s.module] {
				continue
			}
			var args []any
			if s.args != nil {
				args = s.args(today, at)
			}
			created, sms, err := insertFrom(tx, s.category, s.sql, args)
			if err != nil {
				return err
			}
			res.Created[s.category] += created
			res.SMSQueued += sms
		}
		if on["diary"] && at.In(Dhaka).Weekday() != time.Friday {
			var settings struct {
				DailyRequired bool
				Due           bool
			}
			if err := tx.Raw("SELECT daily_required, ?::time >= reminder_time AS due FROM diary_settings WHERE id = 1", at.In(Dhaka).Format("15:04")).Scan(&settings).Error; err != nil {
				return err
			}
			if settings.DailyRequired && settings.Due {
				created, _, err := insertFrom(tx, "diary", reminder, []any{today, today, today})
				if err != nil {
					return err
				}
				res.Created["diary_reminder"] = created
			}
		}
		return nil
	})
	return res, err
}

// ClaimSMS moves due messages forward one attempt; SKIP LOCKED lets parallel
// workers share the queue without sending a message twice.
func (r *GormRepository) ClaimSMS(ctx context.Context, limit int) ([]QueuedSMS, error) {
	out := []QueuedSMS{}
	err := r.db.WithContext(ctx).Raw(`UPDATE sms_messages SET attempts = attempts + 1 WHERE id IN (
  SELECT id FROM sms_messages WHERE status = 'queued' AND attempts < ? ORDER BY created_at LIMIT ? FOR UPDATE SKIP LOCKED
) RETURNING id, phone, body, attempts - 1 AS attempts`, MaxSMSAttempts, limit).Scan(&out).Error
	return out, err
}

func (r *GormRepository) FinishSMS(ctx context.Context, id, status, ref, failure string) error {
	if len(failure) > 500 {
		failure = failure[:500]
	}
	return r.db.WithContext(ctx).Exec("UPDATE sms_messages SET status = ?, provider_ref = ?, error = ?, processed_at = CASE WHEN ? = 'queued' THEN NULL ELSE now() END WHERE id = ? AND status = 'queued'", status, ref, failure, status, id).Error
}
