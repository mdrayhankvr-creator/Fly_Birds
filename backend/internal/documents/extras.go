package documents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

// Marksheet v2 prints more than the published result: the student's
// parents and date of birth, term attendance, the class teacher's comment
// and signature, and the head teacher's name. These are read live (they are
// not part of a result) and folded into the document's cache key, so a
// changed comment, signature or attendance makes a new document. None of it
// changes a published result.

type studentExtra struct {
	// Code is the school's student ID (also the sign-in ID and on the ID
	// card); empty when the school gave none.
	Code                                   string
	FatherEN, FatherBN, MotherEN, MotherBN string
	DOB                                    *string
	Present, Working                       int
	HasAttendance                          bool
	PresetEN, PresetBN, Custom             string
	TeacherEN, TeacherBN                   string
	SignatureVersion                       string
	Signature                              []byte
}

type extras struct {
	students map[string]*studentExtra
	headEN   string
	headBN   string
	window   [2]string // attendance window (YYYY-MM-DD), "" when unknown
	subjects map[string]subjectName
}

type subjectName struct{ Code, EN, BN string }

// key is what the extras add to a document's cache key.
func (x extras) key() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s\n", x.headEN, x.headBN, x.window[0], x.window[1])
	ids := make([]string, 0, len(x.students))
	for id := range x.students {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := x.students[id]
		dob := ""
		if e.DOB != nil {
			dob = *e.DOB
		}
		fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%d|%d|%v|%s|%s|%s|%s|%s|%s\n", id, e.FatherEN, e.FatherBN, e.MotherEN, e.MotherBN, dob, e.Present, e.Working, e.HasAttendance,
			e.PresetEN, e.PresetBN, e.Custom, e.TeacherEN, e.TeacherBN, e.SignatureVersion)
	}
	codes := make([]string, 0, len(x.subjects))
	for id, s := range x.subjects {
		codes = append(codes, id+"|"+s.Code+"|"+s.EN+"|"+s.BN)
	}
	sort.Strings(codes)
	h.Write([]byte(strings.Join(codes, "\n")))
	return hex.EncodeToString(h.Sum(nil))
}

// loadExtras reads what marksheets print besides the results.
func (s *Service) loadExtras(ctx context.Context, set exams.DocSet, results []exams.DocResult) (extras, error) {
	db := s.db.WithContext(ctx)
	x := extras{students: map[string]*studentExtra{}, subjects: map[string]subjectName{}}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Student.ID)
		x.students[r.Student.ID] = &studentExtra{}
	}
	if len(ids) == 0 {
		return x, nil
	}
	if err := db.Raw("SELECT head_teacher_name_en, head_teacher_name_bn FROM school_settings WHERE id = 1").Row().Scan(&x.headEN, &x.headBN); err != nil {
		return x, err
	}

	// Parents and date of birth.
	var people []struct {
		ID, Code                               string
		FatherEN, FatherBN, MotherEN, MotherBN string
		DOB                                    *string
	}
	if err := db.Raw(`SELECT id, coalesce(student_code, '') AS code, father_name_en AS father_en, father_name_bn AS father_bn, mother_name_en AS mother_en, mother_name_bn AS mother_bn,
 to_char(date_of_birth, 'YYYY-MM-DD') AS dob FROM students WHERE id IN ?`, ids).Scan(&people).Error; err != nil {
		return x, err
	}
	for _, p := range people {
		e := x.students[p.ID]
		e.Code, e.FatherEN, e.FatherBN, e.MotherEN, e.MotherBN, e.DOB = strings.TrimSpace(p.Code), p.FatherEN, p.FatherBN, p.MotherEN, p.MotherBN, p.DOB
	}

	// The class teacher of each student's section (the section the result
	// was published for) and their signature.
	sections := []string{}
	for _, r := range results {
		sections = append(sections, r.Student.SectionID)
	}
	var teachers []struct {
		SectionID, NameEN, NameBN, Version string
		Content                            []byte
	}
	if err := db.Raw(`SELECT ct.section_id, e.name_en, e.name_bn, coalesce(sig.version, '') AS version, sig.content
 FROM section_class_teachers ct JOIN employees e ON e.id = ct.employee_id LEFT JOIN employee_signatures sig ON sig.employee_id = e.id
 WHERE ct.section_id IN ?`, sections).Scan(&teachers).Error; err != nil {
		return x, err
	}
	for _, r := range results {
		for _, t := range teachers {
			if t.SectionID == r.Student.SectionID {
				e := x.students[r.Student.ID]
				e.TeacherEN, e.TeacherBN, e.SignatureVersion, e.Signature = t.NameEN, t.NameBN, t.Version, t.Content
			}
		}
	}

	// Comments.
	var comments []struct {
		StudentID, TextEN, TextBN, Custom string
	}
	if err := db.Raw(`SELECT rc.student_id, coalesce(p.text_en, '') AS text_en, coalesce(p.text_bn, '') AS text_bn, rc.custom_text AS custom
 FROM result_comments rc LEFT JOIN result_comment_presets p ON p.id = rc.preset_id
 WHERE rc.scope = ? AND rc.scope_id = ? AND rc.student_id IN ?`, set.Scope, set.ScopeID, ids).Scan(&comments).Error; err != nil {
		return x, err
	}
	for _, c := range comments {
		e := x.students[c.StudentID]
		e.PresetEN, e.PresetBN, e.Custom = c.TextEN, c.TextBN, c.Custom
	}

	// Subject codes and names of combined papers.
	subjectIDs := map[string]bool{}
	for _, r := range results {
		for _, line := range r.Outputs.Subjects {
			if len(line.SubjectIDs) > 1 {
				for _, id := range line.SubjectIDs {
					subjectIDs[id] = true
				}
			}
		}
	}
	if len(subjectIDs) > 0 {
		list := make([]string, 0, len(subjectIDs))
		for id := range subjectIDs {
			list = append(list, id)
		}
		var subs []struct{ ID, Code, NameEN, NameBN string }
		if err := db.Raw("SELECT id, code, name_en, name_bn FROM academic_subjects WHERE id IN ?", list).Scan(&subs).Error; err != nil {
			return x, err
		}
		for _, sb := range subs {
			x.subjects[sb.ID] = subjectName{sb.Code, sb.NameEN, sb.NameBN}
		}
	}

	// Attendance over the result's period.
	if err := s.attendance(ctx, set, ids, &x); err != nil {
		return x, err
	}
	return x, nil
}

// attendance counts each student's days present (present or late) and the
// working days of the period: every day from its start to its end (or
// today, if earlier), except the weekly off days and published holidays.
//
// The period is the term's dates; a term without dates starts the day after
// the previous term's end (or on the session's first day) and ends on the
// day the results were published. The annual result covers the session.
func (s *Service) attendance(ctx context.Context, set exams.DocSet, ids []string, x *extras) error {
	db := s.db.WithContext(ctx)
	var w struct{ From, To *string }
	if set.Scope == exams.ScopeAnnual {
		if err := db.Raw("SELECT to_char(start_date, 'YYYY-MM-DD') AS \"from\", to_char(end_date, 'YYYY-MM-DD') AS \"to\" FROM academic_sessions WHERE id = ?", set.ScopeID).Scan(&w).Error; err != nil {
			return err
		}
	} else {
		if err := db.Raw(`SELECT to_char(coalesce(t.starts_on,
   (SELECT p.ends_on + 1 FROM exam_terms p WHERE p.session_id = t.session_id AND p.sequence < t.sequence AND p.ends_on IS NOT NULL ORDER BY p.sequence DESC LIMIT 1),
   ses.start_date), 'YYYY-MM-DD') AS "from",
 to_char(t.ends_on, 'YYYY-MM-DD') AS "to"
 FROM exam_terms t JOIN academic_sessions ses ON ses.id = t.session_id WHERE t.id = ?`, set.ScopeID).Scan(&w).Error; err != nil {
			return err
		}
		if w.To == nil && set.PublishedAt != "" {
			if t, err := time.Parse(time.RFC3339, set.PublishedAt); err == nil {
				d := t.In(dhaka).Format("2006-01-02")
				w.To = &d
			}
		}
	}
	if w.From == nil || w.To == nil {
		return nil
	}
	today := s.now().In(dhaka).Format("2006-01-02")
	to := *w.To
	if to > today {
		to = today
	}
	if *w.From > to {
		return nil
	}
	x.window = [2]string{*w.From, to}
	var working int
	if err := db.Raw(`SELECT count(*) FROM generate_series(?::date, ?::date, interval '1 day') AS d(day)
 WHERE NOT EXISTS (SELECT 1 FROM school_weekly_off_days o WHERE o.day_of_week = EXTRACT(DOW FROM d.day))
   AND NOT EXISTS (SELECT 1 FROM school_events e WHERE e.kind = 'holiday' AND e.status = 'published' AND d.day::date BETWEEN e.starts_on AND e.ends_on)`, *w.From, to).Scan(&working).Error; err != nil {
		return err
	}
	var rows []struct {
		StudentID string
		Present   int
		Any       int
	}
	if err := db.Raw(`SELECT a.student_id, count(*) FILTER (WHERE a.status IN ('present', 'late')) AS present, count(*) AS any
 FROM student_attendance a
 WHERE a.student_id IN ? AND a.attendance_date BETWEEN ?::date AND ?::date
   AND NOT EXISTS (SELECT 1 FROM school_weekly_off_days o WHERE o.day_of_week = EXTRACT(DOW FROM a.attendance_date))
   AND NOT EXISTS (SELECT 1 FROM school_events e WHERE e.kind = 'holiday' AND e.status = 'published' AND a.attendance_date BETWEEN e.starts_on AND e.ends_on)
 GROUP BY a.student_id`, ids, *w.From, to).Scan(&rows).Error; err != nil {
		return err
	}
	for _, id := range ids {
		x.students[id].Working = working
	}
	for _, r := range rows {
		e := x.students[r.StudentID]
		e.Present, e.HasAttendance = r.Present, r.Any > 0
	}
	return nil
}

// now is the service's clock (tests set Now).
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
