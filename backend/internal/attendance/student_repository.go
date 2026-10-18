package attendance

import (
	"context"
	"errors"
	"strings"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type StudentGormRepository struct{ db *gorm.DB }

func NewStudentRepository(db *gorm.DB) *StudentGormRepository { return &StudentGormRepository{db: db} }

func activeAttendanceRole(tx *gorm.DB, actor string) (string, error) {
	var role string
	err := tx.Table("users").Select("role").Where("id=? AND is_active=true", actor).Take(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrForbidden
	}
	return role, err
}
func authorizeClass(tx *gorm.DB, actor, sessionID, classID string) error {
	role, err := activeAttendanceRole(tx, actor)
	if err != nil {
		return err
	}
	if role == "admin" {
		return nil
	}
	if role != "teacher" && role != "head_teacher" {
		return ErrForbidden
	}
	var n int64
	err = tx.Table("teacher_assignments AS a").Joins("JOIN employees AS e ON e.id=a.employee_id").Where("e.user_id=? AND e.is_active=true AND e.kind='teacher' AND a.session_id=? AND a.class_id=?", actor, sessionID, classID).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrForbidden
	}
	return nil
}
func studentContext(tx *gorm.DB, id string) (StudentRecord, string, error) {
	var v StudentRecord
	var status string
	row := struct {
		StudentRecord
		StatusValue string
	}{}
	err := tx.Table("students AS s").Joins("JOIN academic_sections AS sec ON sec.id=s.section_id").Select("s.id AS student_id,s.session_id,s.class_id,s.section_id,s.admission_no,s.name_en,s.name_bn,s.status AS status_value").Where("s.id=?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, "", ErrNotFound
	}
	if err != nil {
		return v, "", err
	}
	v = row.StudentRecord
	status = row.StatusValue
	return v, status, nil
}
func syncAbsence(tx *gorm.DB, v StudentRecord) error {
	if v.Status == "absent" {
		return tx.Exec("INSERT INTO attendance_absence_events(attendance_id,student_id,session_id,attendance_date,state) VALUES (?,?,?,?,'pending') ON CONFLICT (student_id,session_id,attendance_date) DO UPDATE SET state='pending',updated_at=now()", v.ID, v.StudentID, v.SessionID, v.AttendanceDate).Error
	}
	return tx.Exec("UPDATE attendance_absence_events SET state='cancelled',updated_at=now() WHERE attendance_id=?", v.ID).Error
}

// versionExpr renders updated_at with microseconds as an optimistic lock token.
const versionExpr = `to_char(a.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US')`

// lockSectionDay serializes every write to one section's attendance day, so a
// roster save, a QR scan and a single correction cannot interleave.
func lockSectionDay(tx *gorm.DB, sectionID, date string) error {
	var unused any
	return tx.Raw("SELECT pg_advisory_xact_lock(502, hashtext(lower(?) || ?))", sectionID, date).Row().Scan(&unused)
}

const studentRecordSelect = "a.id,a.student_id,a.session_id,a.class_id,a.section_id,to_char(a.attendance_date,'YYYY-MM-DD') AS attendance_date,a.status,a.source,a.note,s.admission_no,s.name_en,s.name_bn"

func studentRecord(tx *gorm.DB, id string) (StudentRecord, error) {
	var v StudentRecord
	err := tx.Table("student_attendance AS a").Joins("JOIN students AS s ON s.id=a.student_id").Select(studentRecordSelect).Where("a.id=?", id).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, ErrNotFound
	}
	return v, err
}
func (r *StudentGormRepository) StudentRoster(ctx context.Context, actor string, q StudentQuery) (Page[StudentRosterItem], error) {
	db := r.db.WithContext(ctx)
	var contextRow struct {
		SessionID string
		ClassID   string
	}
	err := db.Table("academic_sections").Select("session_id,class_id").Where("id=?", q.SectionID).Take(&contextRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Page[StudentRosterItem]{}, ErrNotFound
	}
	if err != nil {
		return Page[StudentRosterItem]{}, err
	}
	if contextRow.SessionID != q.SessionID {
		return Page[StudentRosterItem]{}, ErrConflict
	}
	if err = validateDate(db, q.SessionID, q.Date); err != nil {
		return Page[StudentRosterItem]{}, err
	}
	if err = authorizeClass(db, actor, q.SessionID, contextRow.ClassID); err != nil {
		return Page[StudentRosterItem]{}, err
	}
	base := db.Table("students AS s").Joins("LEFT JOIN student_attendance AS a ON a.student_id=s.id AND a.session_id=? AND a.attendance_date=?", q.SessionID, q.Date).Where("s.session_id=? AND s.section_id=? AND (s.status='active' OR a.id IS NOT NULL)", q.SessionID, q.SectionID)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.admission_no ILIKE ? OR s.name_en ILIKE ? OR s.name_bn ILIKE ?", term, term, term)
	}
	p := Page[StudentRosterItem]{Items: []StudentRosterItem{}, Page: q.Page, PageSize: q.PageSize}
	if err = base.Count(&p.Total).Error; err != nil {
		return p, err
	}
	err = base.Select("s.id AS student_id,s.admission_no,s.name_en,s.name_bn,s.roll_no,a.id AS record_id,a.status,a.source,a.note,"+versionExpr+" AS version").Order("s.roll_no NULLS LAST,s.name_en,s.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&p.Items).Error
	return p, err
}
func (r *StudentGormRepository) RecordStudent(ctx context.Context, actor string, in StudentInput, source string) (StudentRecord, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v, status, err := studentContext(tx, in.StudentID)
		if err != nil {
			return err
		}
		if status != "active" {
			return ErrConflict
		}
		if err = authorizeClass(tx, actor, v.SessionID, v.ClassID); err != nil {
			return err
		}
		if err = validateDate(tx, v.SessionID, in.AttendanceDate); err != nil {
			return err
		}
		if err = lockSectionDay(tx, v.SectionID, in.AttendanceDate); err != nil {
			return err
		}
		var active bool
		if err = tx.Raw("SELECT sec.is_active AND c.is_active FROM academic_sections sec JOIN academic_classes c ON c.id=sec.class_id WHERE sec.id=?", v.SectionID).Scan(&active).Error; err != nil {
			return err
		}
		if !active {
			return ErrConflict
		}
		err = tx.Raw("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,note,recorded_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?,?) RETURNING id", v.StudentID, v.SessionID, v.ClassID, v.SectionID, in.AttendanceDate, in.Status, source, in.Note, actor, actor).Scan(&id).Error
		if err != nil {
			return err
		}
		rec := StudentRecord{ID: id, StudentID: v.StudentID, SessionID: v.SessionID, AttendanceDate: in.AttendanceDate, Status: in.Status}
		if err = syncAbsence(tx, rec); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "attendance.student_recorded", "student_attendance", id, map[string]any{"student_id": v.StudentID, "session_id": v.SessionID, "date": in.AttendanceDate, "status": in.Status, "source": source})
	})
	if err != nil {
		return StudentRecord{}, dbError(err)
	}
	return studentRecord(r.db.WithContext(ctx), id)
}
func (r *StudentGormRepository) CorrectStudent(ctx context.Context, actor, id string, in StudentCorrection) (StudentRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before StudentRecord
		query := "id,student_id,session_id,class_id,section_id,to_char(attendance_date,'YYYY-MM-DD') AS attendance_date,status"
		err := tx.Table("student_attendance").Select(query).Where("id=?", id).Take(&before).Error
		if err == nil {
			if err = lockSectionDay(tx, before.SectionID, before.AttendanceDate); err == nil {
				err = tx.Table("student_attendance").Clauses(clause.Locking{Strength: "UPDATE"}).Select(query).Where("id=?", id).Take(&before).Error
			}
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err = authorizeClass(tx, actor, before.SessionID, before.ClassID); err != nil {
			return err
		}
		if err = tx.Table("student_attendance").Where("id=?", id).Updates(map[string]any{"status": in.Status, "note": in.Note, "updated_by": actor, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		oldStatus := before.Status
		before.Status = in.Status
		if err = syncAbsence(tx, before); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "attendance.student_corrected", "student_attendance", id, map[string]any{"old_status": oldStatus, "new_status": in.Status})
	})
	if err != nil {
		return StudentRecord{}, dbError(err)
	}
	return studentRecord(r.db.WithContext(ctx), id)
}
func (r *StudentGormRepository) StudentMonthly(ctx context.Context, actor string, q StudentQuery) (Page[StudentSummary], error) {
	db := r.db.WithContext(ctx)
	var row struct {
		SessionID string
		ClassID   string
	}
	err := db.Table("academic_sections").Select("session_id,class_id").Where("id=?", q.SectionID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Page[StudentSummary]{}, ErrNotFound
	}
	if err != nil {
		return Page[StudentSummary]{}, err
	}
	if row.SessionID != q.SessionID {
		return Page[StudentSummary]{}, ErrConflict
	}
	if err = authorizeClass(db, actor, q.SessionID, row.ClassID); err != nil {
		return Page[StudentSummary]{}, err
	}
	start, next := monthBounds(q.Month)
	sessionStart, sessionEnd, err := sessionDates(db, q.SessionID)
	if err != nil {
		return Page[StudentSummary]{}, err
	}
	if next <= sessionStart || start > sessionEnd {
		return Page[StudentSummary]{}, ErrConflict
	}
	base := db.Table("students AS s").Joins("LEFT JOIN student_attendance AS a ON a.student_id=s.id AND a.section_id=? AND a.attendance_date>=? AND a.attendance_date<?", q.SectionID, start, next).Where("s.session_id=? AND s.section_id=?", q.SessionID, q.SectionID)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.admission_no ILIKE ? OR s.name_en ILIKE ? OR s.name_bn ILIKE ?", term, term, term)
	}
	p := Page[StudentSummary]{Items: []StudentSummary{}, Page: q.Page, PageSize: q.PageSize}
	if err = base.Distinct("s.id").Count(&p.Total).Error; err != nil {
		return p, err
	}
	err = base.Select("s.id AS student_id,s.admission_no,s.name_en,s.name_bn,count(a.id) FILTER (WHERE a.status='present') AS present,count(a.id) FILTER (WHERE a.status='absent') AS absent,count(a.id) FILTER (WHERE a.status='late') AS late,count(a.id) AS total").Group("s.id,s.admission_no,s.name_en,s.name_bn").Order("s.name_en,s.id").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&p.Items).Error
	return p, err
}
func (r *StudentGormRepository) StudentReport(ctx context.Context, actor, studentID, month, scope string) (StudentReport, error) {
	db := r.db.WithContext(ctx)
	var id string
	var err error
	if scope == "self" {
		err = db.Table("students AS s").Joins("JOIN users AS u ON u.id=s.user_id").Select("s.id").Where("s.user_id=? AND u.is_active=true AND u.role='student'", actor).Take(&id).Error
	} else {
		err = db.Table("student_guardians AS sg").Joins("JOIN guardians AS g ON g.id=sg.guardian_id").Joins("JOIN users AS u ON u.id=g.user_id").Select("sg.student_id").Where("sg.student_id=? AND g.user_id=? AND u.is_active=true AND u.role='guardian'", studentID, actor).Take(&id).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StudentReport{}, ErrNotFound
	}
	if err != nil {
		return StudentReport{}, err
	}
	start, next := monthBounds(month)
	out := StudentReport{Items: []StudentRecord{}}
	err = db.Table("students").Select("id AS student_id,admission_no,name_en,name_bn").Where("id=?", id).Take(&out.Student).Error
	if err != nil {
		return out, err
	}
	err = db.Table("student_attendance AS a").Joins("JOIN students AS s ON s.id=a.student_id").Select(studentRecordSelect).Where("a.student_id=? AND a.attendance_date>=? AND a.attendance_date<?", id, start, next).Order("a.attendance_date DESC").Scan(&out.Items).Error
	if err != nil {
		return out, err
	}
	for _, item := range out.Items {
		switch item.Status {
		case "present":
			out.Student.Present++
		case "absent":
			out.Student.Absent++
		case "late":
			out.Student.Late++
		}
		out.Student.Total++
	}
	return out, nil
}
func (r *StudentGormRepository) AbsenceEvents(ctx context.Context, actor string, q StudentQuery) (Page[AbsenceEvent], error) {
	db := r.db.WithContext(ctx)
	role, err := activeAttendanceRole(db, actor)
	if err != nil {
		return Page[AbsenceEvent]{}, err
	}
	if role != "admin" {
		return Page[AbsenceEvent]{}, ErrForbidden
	}
	base := db.Table("attendance_absence_events AS e").Joins("JOIN students AS s ON s.id=e.student_id").Where("e.state='pending'")
	if q.Date != "" {
		base = base.Where("e.attendance_date=?", q.Date)
	}
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("s.admission_no ILIKE ? OR s.name_en ILIKE ?", term, term)
	}
	p := Page[AbsenceEvent]{Items: []AbsenceEvent{}, Page: q.Page, PageSize: q.PageSize}
	if err = base.Count(&p.Total).Error; err != nil {
		return p, err
	}
	err = base.Select("e.id,e.student_id,to_char(e.attendance_date,'YYYY-MM-DD') AS attendance_date,e.state,s.name_en,(SELECT count(*) FROM student_guardians sg WHERE sg.student_id=e.student_id) AS guardian_count").Order("e.attendance_date DESC,e.id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&p.Items).Error
	return p, err
}

func (r *StudentGormRepository) Sections(ctx context.Context, actor, sessionID string) ([]StudentSection, error) {
	db := r.db.WithContext(ctx)
	role, err := activeAttendanceRole(db, actor)
	if err != nil {
		return nil, err
	}
	out := []StudentSection{}
	base := db.Table("academic_sections AS sec").Joins("JOIN academic_classes AS c ON c.id=sec.class_id").Where("sec.session_id=? AND sec.is_active=true AND c.is_active=true", sessionID)
	if role != "admin" {
		if role != "teacher" && role != "head_teacher" {
			return nil, ErrForbidden
		}
		base = base.Where("EXISTS (SELECT 1 FROM teacher_assignments a JOIN employees e ON e.id=a.employee_id WHERE a.session_id=sec.session_id AND a.class_id=sec.class_id AND e.user_id=? AND e.is_active=true AND e.kind='teacher')", actor)
	}
	err = base.Select("sec.id,sec.name,c.name AS class_name,sec.session_id").Order("c.display_order,sec.display_order,sec.id").Scan(&out).Error
	return out, err
}

// SaveRoster applies one day's marks for a section atomically. New marks need
// an active student in the section; corrections must name the record and
// version the client loaded. A mark equal to the stored one is a no-op, so a
// repeated submission changes nothing. Any stale entry rejects the whole save.
func (r *StudentGormRepository) SaveRoster(ctx context.Context, actor string, in RosterInput) (RosterResult, error) {
	var out RosterResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var section struct {
			SessionID string
			ClassID   string
			Active    bool
		}
		err := tx.Table("academic_sections AS sec").Joins("JOIN academic_classes AS c ON c.id=sec.class_id").Select("sec.session_id,sec.class_id,(sec.is_active AND c.is_active) AS active").Where("sec.id=?", in.SectionID).Take(&section).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if section.SessionID != in.SessionID || !section.Active {
			return ErrConflict
		}
		if err = validateDate(tx, in.SessionID, in.AttendanceDate); err != nil {
			return err
		}
		if err = authorizeClass(tx, actor, in.SessionID, section.ClassID); err != nil {
			return err
		}
		if err = lockSectionDay(tx, in.SectionID, in.AttendanceDate); err != nil {
			return err
		}
		ids := make([]string, 0, len(in.Entries))
		for _, e := range in.Entries {
			ids = append(ids, e.StudentID)
		}
		var students []struct {
			ID     string
			Status string
		}
		if err = tx.Table("students").Select("id,status").Where("id IN ? AND session_id=? AND section_id=?", ids, in.SessionID, in.SectionID).Find(&students).Error; err != nil {
			return err
		}
		if len(students) != len(ids) {
			return ErrConflict
		}
		studentStatus := map[string]string{}
		for _, st := range students {
			studentStatus[strings.ToLower(st.ID)] = st.Status
		}
		var existing []struct {
			ID        string
			StudentID string
			Status    string
			Note      string
			Version   string
		}
		if err = tx.Table("student_attendance AS a").Clauses(clause.Locking{Strength: "UPDATE"}).Select("a.id,a.student_id,a.status,a.note,"+versionExpr+" AS version").Where("a.session_id=? AND a.attendance_date=? AND a.student_id IN ?", in.SessionID, in.AttendanceDate, ids).Find(&existing).Error; err != nil {
			return err
		}
		current := map[string]int{}
		for i, row := range existing {
			current[strings.ToLower(row.StudentID)] = i
		}
		stale := []string{}
		for _, e := range in.Entries {
			i, found := current[strings.ToLower(e.StudentID)]
			switch {
			case found && existing[i].Status == e.Status && existing[i].Note == e.Note:
				out.Unchanged++
			case found && (e.RecordID == nil || !strings.EqualFold(*e.RecordID, existing[i].ID) || e.Version != existing[i].Version):
				stale = append(stale, e.StudentID)
			case !found && e.RecordID != nil:
				stale = append(stale, e.StudentID)
			case found:
				row := existing[i]
				if err = tx.Exec("UPDATE student_attendance SET status=?,note=?,updated_by=?,updated_at=clock_timestamp() WHERE id=?", e.Status, e.Note, actor, row.ID).Error; err != nil {
					return err
				}
				if err = syncAbsence(tx, StudentRecord{ID: row.ID, StudentID: row.StudentID, SessionID: in.SessionID, AttendanceDate: in.AttendanceDate, Status: e.Status}); err != nil {
					return err
				}
				if err = audit.Record(tx, &actor, "attendance.student_corrected", "student_attendance", row.ID, map[string]any{"old_status": row.Status, "new_status": e.Status, "via": "roster"}); err != nil {
					return err
				}
				out.Updated++
			default:
				if studentStatus[strings.ToLower(e.StudentID)] != "active" {
					return ErrConflict
				}
				var id string
				if err = tx.Raw("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,note,recorded_by,updated_by) VALUES (?,?,?,?,?,?,'manual',?,?,?) RETURNING id", e.StudentID, in.SessionID, section.ClassID, in.SectionID, in.AttendanceDate, e.Status, e.Note, actor, actor).Scan(&id).Error; err != nil {
					return err
				}
				if err = syncAbsence(tx, StudentRecord{ID: id, StudentID: e.StudentID, SessionID: in.SessionID, AttendanceDate: in.AttendanceDate, Status: e.Status}); err != nil {
					return err
				}
				if err = audit.Record(tx, &actor, "attendance.student_recorded", "student_attendance", id, map[string]any{"student_id": e.StudentID, "session_id": in.SessionID, "date": in.AttendanceDate, "status": e.Status, "source": "manual", "via": "roster"}); err != nil {
					return err
				}
				out.Created++
			}
		}
		if len(stale) > 0 {
			return &StaleError{StudentIDs: stale}
		}
		return audit.Record(tx, &actor, "attendance.student_roster_saved", "academic_section", in.SectionID, map[string]any{"date": in.AttendanceDate, "created": out.Created, "updated": out.Updated, "unchanged": out.Unchanged})
	})
	if err != nil {
		var stale *StaleError
		if errors.As(err, &stale) {
			return RosterResult{}, stale
		}
		return RosterResult{}, dbError(err)
	}
	return out, nil
}
