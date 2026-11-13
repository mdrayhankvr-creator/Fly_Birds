package diary

import (
	"context"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type commentDiary struct {
	SectionID     string
	SessionID     string
	TeacherUserID string
	Status        string
	Open          bool
}

func isClassTeacher(tx *gorm.DB, actor, sectionID string) (bool, error) {
	var n int64
	err := tx.Table("section_class_teachers ct").Joins("JOIN employees e ON e.id = ct.employee_id").
		Where("ct.section_id = ? AND e.user_id = ? AND e.is_active", sectionID, actor).Count(&n).Error
	return n > 0, err
}

// The dispatcher uses the same source key. One row per student and entry is
// kept; an edit can reopen it only after ten minutes and only if enabled.
func notifyDiary(tx *gorm.DB, diaryID string, edit bool) error {
	conflict := "DO NOTHING"
	if edit {
		conflict = `DO UPDATE SET read_at=NULL,created_at=now(),
  body_en=EXCLUDED.body_en,body_bn=EXCLUDED.body_bn
WHERE notifications.created_at <= now() - interval '10 minutes'`
	}
	return tx.Exec(`INSERT INTO notifications(user_id,category,source_key,title_en,title_bn,body_en,body_bn,link)
SELECT u.id,'diary','diary:' || d.id || ':' || s.id,
  left('Class diary: ' || sub.name_en,70),left('ক্লাস ডায়েরি: ' || COALESCE(NULLIF(sub.name_bn,''),sub.name_en),220),
  left(CASE WHEN d.homework <> '' THEN 'Homework: ' || d.homework ELSE d.lesson_topics END,240),
  left(CASE WHEN d.homework <> '' THEN 'বাড়ির কাজ: ' || d.homework ELSE d.lesson_topics END,700),'/student/diary'
FROM class_diaries d JOIN academic_subjects sub ON sub.id = d.subject_id
JOIN students s ON s.section_id = d.section_id AND s.session_id = d.session_id AND s.status = 'active'
JOIN users u ON u.id = s.user_id AND u.is_active
LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.category = 'diary'
WHERE d.id = ?::uuid AND d.status = 'published' AND COALESCE(p.in_app,true)
ON CONFLICT (user_id,source_key) `+conflict, diaryID).Error
}

// commentAccess is intentionally narrower than diary oversight: discussion
// belongs only to current-section students, its author, and class teacher.
func commentAccess(tx *gorm.DB, v Viewer, diaryID string) (commentDiary, bool, error) {
	var d commentDiary
	err := tx.Table("class_diaries d").Select(`d.section_id,d.session_id,d.teacher_user_id,d.status,
  d.diary_date >= (now() AT TIME ZONE 'Asia/Dhaka')::date - 30 AS open`).Where("d.id = ?", diaryID).Take(&d).Error
	if err != nil {
		return d, false, dbErr(err)
	}
	if d.Status != "published" {
		return d, false, ErrNotFound
	}
	if v.Role == "student" {
		m, err := student(tx, v)
		if err != nil || m.SectionID != d.SectionID || m.SessionID != d.SessionID {
			return d, false, ErrNotFound
		}
		return d, false, nil
	}
	if v.Role != "teacher" && v.Role != "head_teacher" {
		return d, false, ErrNotFound
	}
	if strings.EqualFold(v.UserID, d.TeacherUserID) {
		return d, true, nil
	}
	allowed, err := isClassTeacher(tx, v.UserID, d.SectionID)
	if err != nil {
		return d, false, err
	}
	if !allowed {
		return d, false, ErrNotFound
	}
	return d, true, nil
}

const commentSelect = `c.id,c.diary_id,c.parent_id,c.author_user_id,c.author_role,
  COALESCE(NULLIF(u.display_name,''),u.email) AS author_name,
  CASE WHEN c.hidden_at IS NULL AND c.deleted_at IS NULL THEN c.body ELSE '' END AS body,
  to_char(c.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
  c.hidden_at IS NOT NULL AS hidden,c.hidden_reason,c.deleted_at IS NOT NULL AS deleted,
  (c.author_user_id = ? AND c.author_role = 'student' AND c.hidden_at IS NULL AND c.deleted_at IS NULL AND now() < c.created_at + interval '10 minutes') AS can_delete`

func commentRow(tx *gorm.DB, actor, id string) (Comment, error) {
	var out Comment
	err := tx.Table("diary_comments c").Joins("JOIN users u ON u.id = c.author_user_id").
		Select(commentSelect, actor).Where("c.id = ?", id).Take(&out).Error
	return out, dbErr(err)
}

func (r *GormRepository) Comments(ctx context.Context, v Viewer, diaryID string) ([]Comment, error) {
	out := []Comment{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, teacher, err := commentAccess(tx, v, diaryID)
		if err != nil {
			return err
		}
		if err := tx.Table("diary_comments c").Joins("JOIN users u ON u.id = c.author_user_id").
			Select(commentSelect, v.UserID).Where("c.diary_id = ?", diaryID).Order("c.created_at,c.id").Find(&out).Error; err != nil {
			return err
		}
		if teacher {
			return tx.Exec(`INSERT INTO diary_comment_reads(diary_id,user_id,last_read_at) VALUES (?,?,now())
ON CONFLICT (diary_id,user_id) DO UPDATE SET last_read_at = EXCLUDED.last_read_at`, diaryID, v.UserID).Error
		}
		return nil
	})
	return out, dbErr(err)
}

func notifyComment(tx *gorm.DB, diaryID, commentID string, reply bool) error {
	if reply {
		return tx.Exec(`INSERT INTO notifications(user_id,category,source_key,title_en,title_bn,body_en,body_bn,link)
SELECT u.id,'diary','diary_reply:' || ?::text,'Teacher replied to your diary question','শিক্ষক আপনার প্রশ্নের উত্তর দিয়েছেন',
  'Open the class diary to read the reply.','উত্তরটি পড়তে ক্লাস ডায়েরি খুলুন।','/student/diary'
FROM diary_comments parent JOIN users u ON u.id = parent.author_user_id AND u.is_active
LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.category = 'diary'
WHERE parent.id = (SELECT parent_id FROM diary_comments WHERE id = ?::uuid) AND COALESCE(p.in_app,true)
ON CONFLICT (user_id,source_key) DO NOTHING`, commentID, commentID).Error
	}
	return tx.Exec(`INSERT INTO notifications(user_id,category,source_key,title_en,title_bn,body_en,body_bn,link)
SELECT u.id,'diary','diary_comment:' || ?::text,'New class diary question','ক্লাস ডায়েরিতে নতুন প্রশ্ন',
  'A student commented on your class diary.','একজন শিক্ষার্থী আপনার ক্লাস ডায়েরিতে মন্তব্য করেছে।','/teacher/diary'
FROM class_diaries d
JOIN LATERAL (
  SELECT d.teacher_user_id AS user_id
  UNION SELECT e.user_id FROM section_class_teachers ct JOIN employees e ON e.id = ct.employee_id AND e.is_active WHERE ct.section_id = d.section_id
) recipient ON true
JOIN users u ON u.id = recipient.user_id AND u.is_active
LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.category = 'diary'
WHERE d.id = ?::uuid AND COALESCE(p.in_app,true)
ON CONFLICT (user_id,source_key) DO NOTHING`, commentID, diaryID).Error
}

func (r *GormRepository) PostComment(ctx context.Context, v Viewer, diaryID, parentID, body string) (Comment, error) {
	var out Comment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		d, teacher, err := commentAccess(tx, v, diaryID)
		if err != nil {
			return err
		}
		if !d.Open {
			return ErrCommentsClosed
		}
		if (parentID == "") == teacher {
			return ErrForbidden
		}
		role := "student"
		if teacher {
			role = "teacher"
			var parent struct {
				DiaryID    string
				AuthorRole string
				Hidden     bool
				Deleted    bool
			}
			if err := tx.Table("diary_comments").Select("diary_id,author_role,hidden_at IS NOT NULL AS hidden,deleted_at IS NOT NULL AS deleted").Where("id = ?", parentID).Take(&parent).Error; err != nil {
				return ErrNotFound
			}
			if parent.DiaryID != diaryID || parent.AuthorRole != "student" || parent.Hidden || parent.Deleted {
				return ErrNotFound
			}
		}
		var id string
		if err := tx.Raw(`INSERT INTO diary_comments(diary_id,parent_id,author_user_id,author_role,body) VALUES (?,NULLIF(?,'')::uuid,?,?,?) RETURNING id`, diaryID, parentID, v.UserID, role, body).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		if err := notifyComment(tx, diaryID, id, teacher); err != nil {
			return err
		}
		out, err = commentRow(tx, v.UserID, id)
		return err
	})
	return out, dbErr(err)
}

func (r *GormRepository) HideComment(ctx context.Context, v Viewer, diaryID, commentID, reason string) (Comment, error) {
	var out Comment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, teacher, err := commentAccess(tx, v, diaryID)
		if err != nil {
			return err
		}
		if !teacher {
			return ErrForbidden
		}
		var c struct {
			DiaryID    string
			AuthorRole string
			Hidden     bool
			Deleted    bool
		}
		if err := tx.Table("diary_comments").Select("diary_id,author_role,hidden_at IS NOT NULL AS hidden,deleted_at IS NOT NULL AS deleted").Where("id = ?", commentID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&c).Error; err != nil {
			return ErrNotFound
		}
		if c.DiaryID != diaryID || c.AuthorRole != "student" || c.Hidden || c.Deleted {
			return ErrNotFound
		}
		if err := tx.Exec("UPDATE diary_comments SET hidden_at=now(),hidden_by=?,hidden_reason=? WHERE id=?", v.UserID, reason, commentID).Error; err != nil {
			return err
		}
		if err := audit.Record(tx, &v.UserID, "diary.comment_hidden", "diary_comment", commentID, map[string]any{"reason": reason}); err != nil {
			return err
		}
		out, err = commentRow(tx, v.UserID, commentID)
		return err
	})
	return out, dbErr(err)
}

func (r *GormRepository) DeleteComment(ctx context.Context, v Viewer, diaryID, commentID string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, teacher, err := commentAccess(tx, v, diaryID)
		if err != nil {
			return err
		}
		if teacher {
			return ErrForbidden
		}
		var c struct {
			DiaryID      string
			AuthorUserID string
			AuthorRole   string
			CanDelete    bool
		}
		if err := tx.Table("diary_comments").Select(`diary_id,author_user_id,author_role,
  hidden_at IS NULL AND deleted_at IS NULL AND now() < created_at + interval '10 minutes' AS can_delete`).
			Where("id = ?", commentID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&c).Error; err != nil {
			return ErrNotFound
		}
		if c.DiaryID != diaryID || c.AuthorUserID != v.UserID || c.AuthorRole != "student" {
			return ErrNotFound
		}
		if !c.CanDelete {
			return ErrConflict
		}
		if err := tx.Exec("UPDATE diary_comments SET deleted_at=now() WHERE id=?", commentID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &v.UserID, "diary.comment_deleted", "diary_comment", commentID, nil)
	}))
}
