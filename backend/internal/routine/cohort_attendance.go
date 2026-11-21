package routine

import (
	"context"
	"fmt"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type CohortMember struct {
	StudentID   string `json:"student_id"`
	Name        string `json:"name"`
	AdmissionNo string `json:"admission_no"`
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
	Status      string `json:"status"`
}

type CohortRoster struct {
	LessonID  string         `json:"lesson_id"`
	SessionID string         `json:"session_id"`
	ClassID   string         `json:"class_id"`
	SubjectID string         `json:"subject_id"`
	TeacherID string         `json:"teacher_id"`
	Date      string         `json:"date"`
	Members   []CohortMember `json:"members"`
}

type CohortAttendanceEntry struct {
	StudentID string `json:"student_id"`
	Status    string `json:"status"`
}

type CohortLessonSummary struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	ClassID       string `json:"class_id"`
	SubjectID     string `json:"subject_id"`
	SubjectName   string `json:"subject_name"`
	SubjectNameBN string `json:"subject_name_bn"`
	TeacherName   string `json:"teacher_name"`
	DayOfWeek     int    `json:"day_of_week"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	Room          string `json:"room"`
	MemberCount   int    `json:"member_count"`
}

func (s *ClassPlannerService) CohortLessons(ctx context.Context, actorID, role, sessionID string) ([]CohortLessonSummary, error) {
	if !uuid.MatchString(actorID) || role != "admin" && role != "teacher" && role != "student" {
		return nil, ErrInvalid
	}
	if sessionID == "" && role != "admin" {
		if err := s.db.WithContext(ctx).Raw("SELECT id FROM academic_sessions WHERE is_active ORDER BY start_date DESC LIMIT 1").Row().Scan(&sessionID); err != nil {
			return nil, ErrNotFound
		}
	}
	if !uuid.MatchString(sessionID) {
		return nil, ErrInvalid
	}
	out := []CohortLessonSummary{}
	query := `SELECT l.id,l.session_id,l.class_id,l.subject_id,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,e.name_en AS teacher_name,
l.day_of_week,lpad((l.start_minute/60)::text,2,'0')||':'||lpad((l.start_minute%60)::text,2,'0') AS start_time,
lpad((l.end_minute/60)::text,2,'0')||':'||lpad((l.end_minute%60)::text,2,'0') AS end_time,l.room,
(SELECT count(*) FROM class_cohort_members m WHERE m.lesson_id=l.id) AS member_count
FROM class_cohort_lessons l JOIN academic_subjects sub ON sub.id=l.subject_id
JOIN employees e ON e.id=l.teacher_id WHERE l.session_id=? AND l.is_current`
	args := []any{sessionID}
	switch role {
	case "teacher":
		query += ` AND EXISTS (SELECT 1 FROM employees own WHERE own.id=l.teacher_id AND own.user_id=?)`
		args = append(args, actorID)
	case "student":
		query += ` AND EXISTS (SELECT 1 FROM class_cohort_members m JOIN students st ON st.id=m.student_id
WHERE m.lesson_id=l.id AND st.user_id=?)`
		args = append(args, actorID)
	}
	query += ` ORDER BY l.day_of_week,l.start_minute,l.id`
	return out, s.db.WithContext(ctx).Raw(query, args...).Scan(&out).Error
}

func (s *ClassPlannerService) cohortRoster(tx *gorm.DB, actorID, role, lessonID, date string, requireCurrent bool) (CohortRoster, error) {
	out := CohortRoster{LessonID: lessonID, Date: date, Members: []CohortMember{}}
	if !uuid.MatchString(actorID) || !uuid.MatchString(lessonID) || role != "admin" && role != "teacher" {
		return out, ErrForbidden
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return out, ErrInvalid
	}
	var row struct {
		SessionID, ClassID, SubjectID, TeacherID string
		DayOfWeek                                int
		Current                                  bool
		StartDate, EndDate                       time.Time
	}
	err = tx.Raw(`SELECT l.session_id,l.class_id,l.subject_id,l.teacher_id,l.day_of_week,l.is_current AS current,
s.start_date,s.end_date FROM class_cohort_lessons l JOIN academic_sessions s ON s.id=l.session_id WHERE l.id=?`, lessonID).Scan(&row).Error
	if err != nil {
		return out, err
	}
	if row.SessionID == "" {
		return out, ErrNotFound
	}
	if requireCurrent && !row.Current {
		return out, ErrConflict
	}
	if day.Weekday() != time.Weekday(row.DayOfWeek) || day.Before(row.StartDate) || day.After(row.EndDate) {
		return out, ErrInvalid
	}
	if role == "teacher" {
		var n int64
		if err := tx.Raw("SELECT count(*) FROM employees WHERE id=? AND user_id=? AND kind='teacher' AND is_active", row.TeacherID, actorID).Row().Scan(&n); err != nil {
			return out, err
		}
		if n != 1 {
			return out, ErrForbidden
		}
	}
	out.SessionID, out.ClassID, out.SubjectID, out.TeacherID = row.SessionID, row.ClassID, row.SubjectID, row.TeacherID
	err = tx.Raw(`SELECT m.student_id,st.name_en AS name,st.admission_no,m.section_id,sec.name AS section_name,
coalesce(a.status,'') AS status FROM class_cohort_members m JOIN students st ON st.id=m.student_id
JOIN academic_sections sec ON sec.id=m.section_id
LEFT JOIN class_cohort_attendance a ON a.lesson_id=m.lesson_id AND a.student_id=m.student_id AND a.attendance_date=?
WHERE m.lesson_id=? ORDER BY sec.display_order,st.admission_no,st.id`, date, lessonID).Scan(&out.Members).Error
	return out, err
}

func (s *ClassPlannerService) CohortRoster(ctx context.Context, actorID, role, lessonID, date string) (CohortRoster, error) {
	return s.cohortRoster(s.db.WithContext(ctx), actorID, role, lessonID, date, false)
}

func (s *ClassPlannerService) SaveCohortAttendance(ctx context.Context, actorID, role, lessonID, date string, entries []CohortAttendanceEntry) (CohortRoster, error) {
	if len(entries) == 0 || len(entries) > 500 {
		return CohortRoster{}, ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", lessonID+date).Error; err != nil {
			return err
		}
		roster, err := s.cohortRoster(tx, actorID, role, lessonID, date, true)
		if err != nil {
			return err
		}
		if len(entries) != len(roster.Members) {
			return ErrInvalid
		}
		members := map[string]bool{}
		for _, m := range roster.Members {
			members[m.StudentID] = true
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			if !members[entry.StudentID] || seen[entry.StudentID] || entry.Status != "present" && entry.Status != "absent" && entry.Status != "late" {
				return ErrInvalid
			}
			seen[entry.StudentID] = true
			if err := tx.Exec(`INSERT INTO class_cohort_attendance(lesson_id,student_id,attendance_date,status,recorded_by)
VALUES (?,?,?,?,?) ON CONFLICT (lesson_id,student_id,attendance_date)
DO UPDATE SET status=EXCLUDED.status,recorded_by=EXCLUDED.recorded_by,updated_at=now()`, lessonID, entry.StudentID, date, entry.Status, actorID).Error; err != nil {
				return err
			}
		}
		if err := audit.Record(tx, &actorID, "attendance.cohort_roster_saved", "class_cohort_lesson", lessonID,
			map[string]any{"date": date, "count": len(entries)}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return CohortRoster{}, fmt.Errorf("save cohort attendance: %w", err)
	}
	return s.CohortRoster(ctx, actorID, role, lessonID, date)
}
