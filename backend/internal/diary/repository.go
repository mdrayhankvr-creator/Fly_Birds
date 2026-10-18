package diary

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, files.ErrInvalid) {
		return ErrInvalid
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22P02", "22007", "22008":
			return ErrInvalid
		}
	}
	return err
}

const utcFormat = `'YYYY-MM-DD"T"HH24:MI:SS"Z"'`
const teacherName = "COALESCE((SELECT e.name_en FROM employees e WHERE e.user_id = d.teacher_user_id ORDER BY e.is_active DESC LIMIT 1), NULLIF(tu.display_name, ''), tu.email)"
const diarySelect = "d.id,d.session_id,d.class_id,d.section_id,d.subject_id,c.name AS class_name,sec.name AS section_name,sub.name_en AS subject_name_en,sub.name_bn AS subject_name_bn," +
	teacherName + " AS teacher_name,to_char(d.diary_date,'YYYY-MM-DD') AS diary_date,d.lesson_topics,d.class_notes,d.homework,d.instructions,d.status," +
	"to_char(d.published_at AT TIME ZONE 'UTC'," + utcFormat + ") AS published_at,d.submission_enabled,to_char(d.submission_due_at AT TIME ZONE 'UTC'," + utcFormat + ") AS submission_due_at," +
	"(d.status = 'published' AND d.submission_enabled AND (d.submission_due_at IS NULL OR d.submission_due_at > now())) AS submission_open," +
	"(SELECT count(*) FROM homework_submissions hs WHERE hs.diary_id = d.id) AS submission_count"

func diaryBase(db *gorm.DB) *gorm.DB {
	return db.Table("class_diaries d").Joins("JOIN academic_classes c ON c.id = d.class_id").Joins("JOIN academic_sections sec ON sec.id = d.section_id").
		Joins("JOIN academic_subjects sub ON sub.id = d.subject_id").Joins("JOIN users tu ON tu.id = d.teacher_user_id")
}

func filter(base *gorm.DB, q Query) *gorm.DB {
	if q.From != "" {
		base = base.Where("d.diary_date >= ?::date", q.From)
	}
	if q.To != "" {
		base = base.Where("d.diary_date <= ?::date", q.To)
	}
	if q.SubjectID != "" {
		base = base.Where("d.subject_id = ?", q.SubjectID)
	}
	if q.SectionID != "" {
		base = base.Where("d.section_id = ?", q.SectionID)
	}
	if q.Status != "" {
		base = base.Where("d.status = ?", q.Status)
	}
	if q.Search != "" {
		p := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(q.Search)) + "%"
		base = base.Where("(lower(d.lesson_topics) LIKE ? OR lower(d.class_notes) LIKE ? OR lower(d.homework) LIKE ? OR lower(d.instructions) LIKE ? OR lower(sub.name_en) LIKE ? OR sub.name_bn LIKE ?)", p, p, p, p, p, "%"+q.Search+"%")
	}
	return base
}

func attachments(db *gorm.DB, table, fk string, ids []string) (map[string][]Attachment, error) {
	out := map[string][]Attachment{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		Owner string
		Attachment
	}
	err := db.Table(table+" a").Joins("JOIN file_objects f ON f.id = a.file_id").Select("a."+fk+" AS owner,f.id,f.original_name AS name,f.content_type,f.size_bytes AS size").
		Where("a."+fk+" IN ?", ids).Order("a.position").Find(&rows).Error
	for _, r := range rows {
		out[r.Owner] = append(out[r.Owner], r.Attachment)
	}
	return out, err
}

func hydrate(db *gorm.DB, items []Diary) error {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	files, err := attachments(db, "diary_attachments", "diary_id", ids)
	for i := range items {
		items[i].Attachments = files[items[i].ID]
		if items[i].Attachments == nil {
			items[i].Attachments = []Attachment{}
		}
	}
	return err
}

func list(db, base *gorm.DB, q Query, sel string, args ...any) (Page[Diary], error) {
	out := Page[Diary]{Items: []Diary{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := base.Select(sel, args...).Order("d.diary_date DESC, sub.code, sec.name, d.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&out.Items).Error; err != nil {
		return out, err
	}
	return out, hydrate(db, out.Items)
}

func assigned(tx *gorm.DB, actor, sessionID, classID, subjectID string) (bool, error) {
	var n int64
	err := tx.Table("teacher_assignments a").Joins("JOIN employees e ON e.id = a.employee_id").
		Where("e.user_id = ? AND e.is_active AND a.session_id = ? AND a.class_id = ? AND a.subject_id = ?", actor, sessionID, classID, subjectID).Count(&n).Error
	return n > 0, err
}

func (r *GormRepository) Assignments(ctx context.Context, actor, date string) ([]Assignment, error) {
	out := []Assignment{}
	err := r.db.WithContext(ctx).Raw(`SELECT se.id AS session_id, se.name AS session_name, c.id AS class_id, c.name AS class_name, sec.id AS section_id, sec.name AS section_name,
  sub.id AS subject_id, sub.name_en AS subject_name_en, sub.name_bn AS subject_name_bn, d.id AS diary_id, d.status AS diary_status
FROM teacher_assignments a
JOIN employees e ON e.id = a.employee_id AND e.is_active AND e.user_id = ?
JOIN academic_sessions se ON se.id = a.session_id AND ?::date BETWEEN se.start_date AND se.end_date
JOIN academic_classes c ON c.id = a.class_id
JOIN academic_sections sec ON sec.session_id = a.session_id AND sec.class_id = a.class_id AND sec.is_active
JOIN academic_subjects sub ON sub.id = a.subject_id
LEFT JOIN class_diaries d ON d.section_id = sec.id AND d.subject_id = a.subject_id AND d.diary_date = ?::date
ORDER BY c.display_order, c.name, sec.display_order, sec.name, sub.code`, actor, date, date).Scan(&out).Error
	return out, err
}

func (r *GormRepository) Teaching(ctx context.Context, actor string, q Query) (Page[Diary], error) {
	db := r.db.WithContext(ctx)
	return list(db, filter(diaryBase(db).Where("d.teacher_user_id = ?", actor), q), q, diarySelect)
}

type member struct {
	ID        string
	SessionID string
	SectionID string
}

// student resolves the student a student or guardian may act for; unknown and
// unlinked students both read as not found.
func student(tx *gorm.DB, v Viewer) (member, error) {
	var m member
	var err error
	switch v.Role {
	case "student":
		err = tx.Table("students").Select("id,session_id,section_id").Where("user_id = ?", v.UserID).Take(&m).Error
	case "guardian":
		err = tx.Table("students s").Joins("JOIN student_guardians sg ON sg.student_id = s.id").Joins("JOIN guardians g ON g.id = sg.guardian_id").
			Select("s.id,s.session_id,s.section_id").Where("s.id = ? AND g.user_id = ?", v.StudentID, v.UserID).Take(&m).Error
	default:
		return m, ErrNotFound
	}
	if err != nil {
		return m, ErrNotFound
	}
	return m, nil
}

const mySubmission = ",(SELECT hs.status FROM homework_submissions hs WHERE hs.diary_id = d.id AND hs.student_id = ?) AS my_submission"

func (r *GormRepository) Feed(ctx context.Context, v Viewer, q Query) (Page[Diary], error) {
	db := r.db.WithContext(ctx)
	m, err := student(db, v)
	if err != nil {
		return Page[Diary]{}, err
	}
	q.Status = ""
	base := filter(diaryBase(db).Where("d.status = 'published' AND d.section_id = ? AND d.session_id = ?", m.SectionID, m.SessionID), q)
	return list(db, base, q, diarySelect+mySubmission, m.ID)
}

func (r *GormRepository) Oversight(ctx context.Context, q Query) (Page[Diary], error) {
	db := r.db.WithContext(ctx)
	return list(db, filter(diaryBase(db), q), q, diarySelect)
}

// canView applies the single diary visibility rule: the author and oversight
// roles see any diary; students and guardians see published diaries of the
// student's current section. It returns the student id for personal fields.
func canView(tx *gorm.DB, v Viewer, id string) (string, error) {
	var d struct {
		TeacherUserID string
		Status        string
		SectionID     string
		SessionID     string
	}
	if err := tx.Table("class_diaries").Select("teacher_user_id,status,section_id,session_id").Where("id = ?", id).Take(&d).Error; err != nil {
		return "", dbErr(err)
	}
	if strings.EqualFold(d.TeacherUserID, v.UserID) || v.Oversight {
		return "", nil
	}
	if d.Status != "published" {
		return "", ErrNotFound
	}
	if v.Role == "guardian" && v.StudentID == "" {
		var sid string
		err := tx.Raw(`SELECT s.id FROM students s JOIN student_guardians sg ON sg.student_id = s.id JOIN guardians g ON g.id = sg.guardian_id
WHERE g.user_id = ? AND s.section_id = ? AND s.session_id = ? ORDER BY s.id LIMIT 1`, v.UserID, d.SectionID, d.SessionID).Row().Scan(&sid)
		if err != nil {
			return "", ErrNotFound
		}
		return sid, nil
	}
	m, err := student(tx, v)
	if err != nil || m.SectionID != d.SectionID || m.SessionID != d.SessionID {
		return "", ErrNotFound
	}
	return m.ID, nil
}

func (r *GormRepository) get(db *gorm.DB, v Viewer, id string) (Diary, error) {
	sid, err := canView(db, v, id)
	if err != nil {
		return Diary{}, err
	}
	var d Diary
	sel, args := diarySelect, []any{}
	if sid != "" {
		sel, args = diarySelect+mySubmission, []any{sid}
	}
	if err := diaryBase(db).Select(sel, args...).Where("d.id = ?", id).Take(&d).Error; err != nil {
		return Diary{}, dbErr(err)
	}
	items := []Diary{d}
	err = hydrate(db, items)
	return items[0], err
}

func (r *GormRepository) Get(ctx context.Context, v Viewer, id string) (Diary, error) {
	return r.get(r.db.WithContext(ctx), v, id)
}

// replaceFiles makes table rows for owner equal ids, releasing removed files
// and claiming new ones from the actor's own uploads of the purpose.
func replaceFiles(tx *gorm.DB, table, fk, owner, actor, purpose string, ids []string) error {
	var current []string
	if err := tx.Table(table).Where(fk+" = ?", owner).Pluck("file_id", &current).Error; err != nil {
		return err
	}
	removed, added := []string{}, []string{}
	for _, f := range current {
		if !slices.ContainsFunc(ids, func(v string) bool { return strings.EqualFold(v, f) }) {
			removed = append(removed, f)
		}
	}
	for _, f := range ids {
		if !slices.ContainsFunc(current, func(v string) bool { return strings.EqualFold(v, f) }) {
			added = append(added, f)
		}
	}
	if err := tx.Exec("DELETE FROM "+table+" WHERE "+fk+" = ?", owner).Error; err != nil {
		return err
	}
	if err := files.UnlinkTx(tx, removed); err != nil {
		return err
	}
	if err := files.LinkTx(tx, actor, purpose, added); err != nil {
		return ErrInvalid
	}
	for i, f := range ids {
		if err := tx.Exec("INSERT INTO "+table+"("+fk+",file_id,position) VALUES (?,?,?)", owner, f, i).Error; err != nil {
			return dbErr(err)
		}
	}
	return nil
}

type owned struct {
	TeacherUserID string
	Status        string
	SessionID     string
	ClassID       string
	SubjectID     string
}

// lockOwn locks a diary the actor wrote and still teaches.
func lockOwn(tx *gorm.DB, actor, id string) (owned, error) {
	var d owned
	if err := tx.Table("class_diaries").Select("teacher_user_id,status,session_id,class_id,subject_id").Where("id = ?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error; err != nil {
		return d, dbErr(err)
	}
	if !strings.EqualFold(d.TeacherUserID, actor) {
		return d, ErrNotFound
	}
	ok, err := assigned(tx, actor, d.SessionID, d.ClassID, d.SubjectID)
	if err != nil {
		return d, err
	}
	if !ok {
		return d, ErrForbidden
	}
	return d, nil
}

func (r *GormRepository) Save(ctx context.Context, actor, id string, in DiaryInput) (Diary, error) {
	var out Diary
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == "" {
			var sec struct {
				SessionID string
				ClassID   string
				Active    bool
				InSession bool
			}
			if err := tx.Table("academic_sections s").Joins("JOIN academic_sessions se ON se.id = s.session_id").
				Select("s.session_id,s.class_id,s.is_active AS active,(?::date BETWEEN se.start_date AND se.end_date) AS in_session", in.DiaryDate).Where("s.id = ?", in.SectionID).Take(&sec).Error; err != nil {
				return ErrInvalid
			}
			if !sec.Active || !sec.InSession {
				return ErrInvalid
			}
			ok, err := assigned(tx, actor, sec.SessionID, sec.ClassID, in.SubjectID)
			if err != nil {
				return err
			}
			if !ok {
				return ErrForbidden
			}
			if err := tx.Raw(`INSERT INTO class_diaries(session_id,class_id,section_id,subject_id,teacher_user_id,diary_date,lesson_topics,class_notes,homework,instructions,submission_enabled,submission_due_at)
VALUES (?,?,?,?,?,?::date,?,?,?,?,?,?) RETURNING id`, sec.SessionID, sec.ClassID, in.SectionID, in.SubjectID, actor, in.DiaryDate, in.LessonTopics, in.ClassNotes, in.Homework, in.Instructions, in.SubmissionEnabled, in.SubmissionDueAt).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			if _, err := lockOwn(tx, actor, id); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE class_diaries SET lesson_topics=?,class_notes=?,homework=?,instructions=?,submission_enabled=?,submission_due_at=?,updated_at=now() WHERE id=?`,
				in.LessonTopics, in.ClassNotes, in.Homework, in.Instructions, in.SubmissionEnabled, in.SubmissionDueAt, id).Error; err != nil {
				return dbErr(err)
			}
		}
		if err := replaceFiles(tx, "diary_attachments", "diary_id", id, actor, "diary", in.FileIDs); err != nil {
			return err
		}
		var err error
		if out, err = r.get(tx, Viewer{UserID: actor, Role: "teacher"}, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "diary.saved", "class_diary", id, map[string]any{"status": out.Status, "attachments": len(in.FileIDs), "submission_enabled": in.SubmissionEnabled})
	})
	return out, dbErr(err)
}

func (r *GormRepository) Publish(ctx context.Context, actor, id string) (Diary, error) {
	var out Diary
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		d, err := lockOwn(tx, actor, id)
		if err != nil {
			return err
		}
		if d.Status == "published" {
			return ErrConflict
		}
		if err := tx.Exec("UPDATE class_diaries SET status='published',published_at=now(),updated_at=now() WHERE id=?", id).Error; err != nil {
			return dbErr(err)
		}
		if out, err = r.get(tx, Viewer{UserID: actor, Role: "teacher"}, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "diary.published", "class_diary", id, nil)
	})
	return out, dbErr(err)
}

func (r *GormRepository) Delete(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		d, err := lockOwn(tx, actor, id)
		if err != nil {
			return err
		}
		if d.Status != "draft" {
			return ErrConflict
		}
		var attached []string
		if err := tx.Table("diary_attachments").Where("diary_id = ?", id).Pluck("file_id", &attached).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM class_diaries WHERE id = ?", id).Error; err != nil {
			return err
		}
		if err := files.UnlinkTx(tx, attached); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "diary.deleted", "class_diary", id, nil)
	}))
}

func (r *GormRepository) FileAccess(ctx context.Context, v Viewer, diaryID, fileID string) error {
	db := r.db.WithContext(ctx)
	if _, err := canView(db, v, diaryID); err != nil {
		return err
	}
	var n int64
	if err := db.Table("diary_attachments").Where("diary_id = ? AND file_id = ?", diaryID, fileID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const submissionSelect = "hs.id,hs.diary_id,s.id AS student_id,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,s.roll_no,COALESCE(hs.answer_text,'') AS answer_text," +
	"COALESCE(hs.status,'missing') AS status,COALESCE(hs.review_note,'') AS review_note,to_char(hs.submitted_at AT TIME ZONE 'UTC'," + utcFormat + ") AS submitted_at,to_char(hs.reviewed_at AT TIME ZONE 'UTC'," + utcFormat + ") AS reviewed_at"

func hydrateSubmissions(db *gorm.DB, items []Submission) error {
	ids := []string{}
	for _, s := range items {
		if s.ID != "" {
			ids = append(ids, s.ID)
		}
	}
	found, err := attachments(db, "homework_submission_files", "submission_id", ids)
	for i := range items {
		items[i].Files = found[items[i].ID]
		if items[i].Files == nil {
			items[i].Files = []Attachment{}
		}
	}
	return err
}

func submissionByID(db *gorm.DB, id string) (Submission, error) {
	var out Submission
	if err := db.Table("homework_submissions hs").Joins("JOIN students s ON s.id = hs.student_id").Select(submissionSelect).Where("hs.id = ?", id).Take(&out).Error; err != nil {
		return out, dbErr(err)
	}
	items := []Submission{out}
	err := hydrateSubmissions(db, items)
	return items[0], err
}

func (r *GormRepository) MySubmission(ctx context.Context, actor, diaryID string) (Submission, error) {
	db := r.db.WithContext(ctx)
	m, err := student(db, Viewer{UserID: actor, Role: "student"})
	if err != nil {
		return Submission{}, err
	}
	if _, err := canView(db, Viewer{UserID: actor, Role: "student"}, diaryID); err != nil {
		return Submission{}, err
	}
	var id string
	if err := db.Table("homework_submissions").Select("id").Where("diary_id = ? AND student_id = ?", diaryID, m.ID).Take(&id).Error; err != nil {
		return Submission{}, dbErr(err)
	}
	return submissionByID(db, id)
}

func (r *GormRepository) Submit(ctx context.Context, actor, diaryID string, in SubmissionInput) (Submission, error) {
	var out Submission
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v := Viewer{UserID: actor, Role: "student"}
		m, err := student(tx, v)
		if err != nil {
			return err
		}
		var d struct {
			Status            string
			SectionID         string
			SessionID         string
			Open              bool
			SubmissionEnabled bool
		}
		if err := tx.Table("class_diaries").Select("status,section_id,session_id,submission_enabled,(submission_due_at IS NULL OR submission_due_at > now()) AS open").Where("id = ?", diaryID).
			Clauses(clause.Locking{Strength: "SHARE"}).Take(&d).Error; err != nil {
			return dbErr(err)
		}
		if d.Status != "published" || d.SectionID != m.SectionID || d.SessionID != m.SessionID {
			return ErrNotFound
		}
		var active bool
		if err := tx.Raw("SELECT status = 'active' FROM students WHERE id = ?", m.ID).Row().Scan(&active); err != nil {
			return err
		}
		if !d.SubmissionEnabled || !d.Open || !active {
			return ErrClosed
		}
		var existing struct {
			ID     string
			Status string
		}
		err = tx.Table("homework_submissions").Select("id,status").Where("diary_id = ? AND student_id = ?", diaryID, m.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Raw("INSERT INTO homework_submissions(diary_id,student_id,answer_text) VALUES (?,?,?) ON CONFLICT (diary_id, student_id) DO NOTHING RETURNING id", diaryID, m.ID, in.AnswerText).Row().Scan(&existing.ID); err != nil {
				return ErrConflict
			}
		case err != nil:
			return err
		case existing.Status == "reviewed":
			return ErrConflict
		default:
			if err := tx.Exec("UPDATE homework_submissions SET answer_text=?,submitted_at=now(),updated_at=now() WHERE id=?", in.AnswerText, existing.ID).Error; err != nil {
				return err
			}
		}
		if err := replaceFiles(tx, "homework_submission_files", "submission_id", existing.ID, actor, "homework", in.FileIDs); err != nil {
			return err
		}
		if out, err = submissionByID(tx, existing.ID); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "homework.submitted", "homework_submission", existing.ID, map[string]any{"diary_id": diaryID, "files": len(in.FileIDs)})
	})
	return out, dbErr(err)
}

// Submissions lists every student of the diary's section with their
// submission, so the teacher sees who has not submitted.
func (r *GormRepository) Submissions(ctx context.Context, v Viewer, diaryID string) ([]Submission, error) {
	db := r.db.WithContext(ctx)
	var d struct {
		TeacherUserID string
		SectionID     string
		SessionID     string
	}
	if err := db.Table("class_diaries").Select("teacher_user_id,section_id,session_id").Where("id = ?", diaryID).Take(&d).Error; err != nil {
		return nil, dbErr(err)
	}
	if !v.Oversight && !strings.EqualFold(d.TeacherUserID, v.UserID) {
		return nil, ErrNotFound
	}
	out := []Submission{}
	err := db.Table("students s").Joins("LEFT JOIN homework_submissions hs ON hs.student_id = s.id AND hs.diary_id = ?", diaryID).
		Where("s.section_id = ? AND s.session_id = ? AND (s.status = 'active' OR hs.id IS NOT NULL)", d.SectionID, d.SessionID).
		Select("COALESCE(hs.id::text,'') AS id,'"+diaryID+"' AS diary_id,"+strings.SplitN(submissionSelect, ",", 3)[2]).Order("s.roll_no NULLS LAST, s.name_en").Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, hydrateSubmissions(db, out)
}

func (r *GormRepository) Review(ctx context.Context, actor, submissionID, note string) (Submission, error) {
	var out Submission
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var teacher string
		if err := tx.Table("homework_submissions hs").Joins("JOIN class_diaries d ON d.id = hs.diary_id").Select("d.teacher_user_id").Where("hs.id = ?", submissionID).
			Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "hs"}}).Take(&teacher).Error; err != nil {
			return dbErr(err)
		}
		if !strings.EqualFold(teacher, actor) {
			return ErrNotFound
		}
		if err := tx.Exec("UPDATE homework_submissions SET status='reviewed',review_note=?,reviewed_by=?,reviewed_at=now(),updated_at=now() WHERE id=?", note, actor, submissionID).Error; err != nil {
			return dbErr(err)
		}
		var err error
		if out, err = submissionByID(tx, submissionID); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "homework.reviewed", "homework_submission", submissionID, nil)
	})
	return out, dbErr(err)
}

// SubmissionFileAccess allows the student, the student's guardians, the diary
// author and oversight roles.
func (r *GormRepository) SubmissionFileAccess(ctx context.Context, v Viewer, submissionID, fileID string) error {
	db := r.db.WithContext(ctx)
	var row struct {
		TeacherUserID string
		StudentUserID *string
		StudentID     string
	}
	if err := db.Table("homework_submissions hs").Joins("JOIN class_diaries d ON d.id = hs.diary_id").Joins("JOIN students s ON s.id = hs.student_id").
		Select("d.teacher_user_id,s.user_id AS student_user_id,s.id AS student_id").Where("hs.id = ?", submissionID).Take(&row).Error; err != nil {
		return dbErr(err)
	}
	allowed := v.Oversight || strings.EqualFold(row.TeacherUserID, v.UserID) || (row.StudentUserID != nil && strings.EqualFold(*row.StudentUserID, v.UserID))
	if !allowed && v.Role == "guardian" {
		if _, err := student(db, Viewer{UserID: v.UserID, Role: "guardian", StudentID: row.StudentID}); err == nil {
			allowed = true
		}
	}
	if !allowed {
		return ErrNotFound
	}
	var n int64
	if err := db.Table("homework_submission_files").Where("submission_id = ? AND file_id = ?", submissionID, fileID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) Settings(ctx context.Context) (Settings, error) {
	var s Settings
	err := r.db.WithContext(ctx).Table("diary_settings").Select("daily_required,to_char(reminder_time,'HH24:MI') AS reminder_time").Where("id = 1").Take(&s).Error
	return s, dbErr(err)
}

func (r *GormRepository) SaveSettings(ctx context.Context, actor string, in Settings) (Settings, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE diary_settings SET daily_required=?,reminder_time=?::time,updated_by=?,updated_at=now() WHERE id=1", in.DailyRequired, in.ReminderTime, actor).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "diary.settings_saved", "diary_settings", "1", map[string]any{"daily_required": in.DailyRequired, "reminder_time": in.ReminderTime})
	})
	if err != nil {
		return Settings{}, err
	}
	return r.Settings(ctx)
}

func (r *GormRepository) Compliance(ctx context.Context, date string) ([]ComplianceRow, error) {
	out := []ComplianceRow{}
	err := r.db.WithContext(ctx).Raw(`SELECT c.name AS class_name, sec.name AS section_name, sub.name_en AS subject_name_en, sub.name_bn AS subject_name_bn,
  string_agg(DISTINCT e.name_en, ', ') AS teachers, max(d.status) AS diary_status
FROM teacher_assignments a
JOIN employees e ON e.id = a.employee_id AND e.is_active
JOIN academic_sessions se ON se.id = a.session_id AND ?::date BETWEEN se.start_date AND se.end_date
JOIN academic_classes c ON c.id = a.class_id
JOIN academic_sections sec ON sec.session_id = a.session_id AND sec.class_id = a.class_id AND sec.is_active
JOIN academic_subjects sub ON sub.id = a.subject_id
LEFT JOIN class_diaries d ON d.section_id = sec.id AND d.subject_id = a.subject_id AND d.diary_date = ?::date
GROUP BY c.display_order, c.name, sec.display_order, sec.name, sub.code, sub.name_en, sub.name_bn
ORDER BY c.display_order, c.name, sec.display_order, sec.name, sub.code`, date, date).Scan(&out).Error
	return out, err
}
