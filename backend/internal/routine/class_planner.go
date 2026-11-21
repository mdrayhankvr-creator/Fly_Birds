package routine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ClassPlanner uses the existing class_routines table as the current timetable.
// Drafts and immutable publication snapshots live in class_routine_plan_versions.
type ClassPlannerService struct{ db *gorm.DB }

func NewClassPlannerService(db *gorm.DB) *ClassPlannerService { return &ClassPlannerService{db: db} }

type TeachingFrequency struct {
	ClassID        string `json:"class_id"`
	SubjectID      string `json:"subject_id"`
	PeriodsPerWeek int    `json:"periods_per_week"`
}
type TeacherUnavailable struct {
	TeacherID string `json:"teacher_id"`
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}
type ClassPlanConfig struct {
	StartTime             string               `json:"start_time"`
	EndTime               string               `json:"end_time"`
	LunchStart            string               `json:"lunch_start"`
	LunchMinutes          int                  `json:"lunch_minutes"`
	PeriodMinutes         int                  `json:"period_minutes"`
	TransitionMinutes     int                  `json:"transition_minutes"`
	Days                  []int                `json:"days"`
	AvailabilityConfirmed bool                 `json:"availability_confirmed"`
	Frequencies           []TeachingFrequency  `json:"frequencies"`
	Unavailable           []TeacherUnavailable `json:"unavailable"`
}
type ClassPlan struct {
	SessionID     string          `json:"session_id"`
	Config        ClassPlanConfig `json:"config"`
	Slots         []ClassInput    `json:"slots"`
	CombinedSlots []CombinedInput `json:"combined_slots,omitempty"`
}

// CombinedInput is one lesson with an explicit roster. The same lesson may
// include students from several sections without duplicating the teacher.
type CombinedInput struct {
	ClassID    string   `json:"class_id"`
	SubjectID  string   `json:"subject_id"`
	TeacherID  string   `json:"teacher_id"`
	DayOfWeek  int      `json:"day_of_week"`
	StartTime  string   `json:"start_time"`
	EndTime    string   `json:"end_time"`
	Room       string   `json:"room"`
	StudentIDs []string `json:"student_ids"`
}
type PlanPeriod struct {
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Kind      string `json:"kind"`
}
type PlanIssue struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	SectionID string `json:"section_id,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	Slot      int    `json:"slot,omitempty"`
}
type ClassPlanSection struct {
	ID        string `json:"id"`
	ClassID   string `json:"class_id"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
}
type ClassPlanSubject struct {
	ID      string `json:"id"`
	ClassID string `json:"class_id"`
	Name    string `json:"name"`
	NameBN  string `json:"name_bn"`
}
type ClassPlanAssignment struct {
	TeacherID   string `json:"teacher_id"`
	ClassID     string `json:"class_id"`
	SubjectID   string `json:"subject_id"`
	TeacherName string `json:"teacher_name"`
}
type PlanCatalogue struct {
	Sections    []ClassPlanSection    `json:"sections"`
	Subjects    []ClassPlanSubject    `json:"subjects"`
	Assignments []ClassPlanAssignment `json:"assignments"`
	Rooms       []string              `json:"rooms"`
	OffDays     []int                 `json:"off_days"`
}
type ClassPlanPreview struct {
	Plan             ClassPlan       `json:"plan"`
	Catalogue        PlanCatalogue   `json:"catalogue"`
	Periods          []PlanPeriod    `json:"periods"`
	Free             []PlanPeriod    `json:"free"`
	Issues           []PlanIssue     `json:"issues"`
	Existing         []ClassInput    `json:"existing"`
	ExistingCombined []CombinedInput `json:"existing_combined"`
	PublishedVersion int             `json:"published_version"`
	CanPublish       bool            `json:"can_publish"`
	Digest           string          `json:"digest"`
}
type ClassPlanVersion struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Version   int       `json:"version"`
	State     string    `json:"state"`
	Plan      ClassPlan `json:"plan" gorm:"-"`
	UpdatedAt string    `json:"updated_at"`
}
type plannerVersionRow struct {
	ID        string
	SessionID string
	Version   int
	State     string
	PlanJSON  []byte `gorm:"column:plan"`
	UpdatedAt string
}

func clock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }
func plannedPeriod(day, a, b int, kind string) PlanPeriod {
	return PlanPeriod{DayOfWeek: day, StartTime: clock(a), EndTime: clock(b), Kind: kind}
}
func issue(code, message string) PlanIssue { return PlanIssue{Code: code, Message: message} }
func normalizedRoom(name string) string    { return strings.ToLower(strings.TrimSpace(name)) }

// buildPeriods never invents hours or period lengths. A partial config yields
// a clear issue and no candidate periods.
func buildPeriods(c ClassPlanConfig, off map[int]bool) ([]PlanPeriod, []PlanIssue) {
	issues := []PlanIssue{}
	start, okStart := minute(c.StartTime)
	end, okEnd := minute(c.EndTime)
	lunch, okLunch := minute(c.LunchStart)
	if !okStart || !okEnd || !okLunch || start >= lunch || lunch+45 >= end || c.LunchMinutes != 45 ||
		c.PeriodMinutes < 20 || c.PeriodMinutes > 120 || c.TransitionMinutes < 0 || c.TransitionMinutes > 30 {
		return []PlanPeriod{}, []PlanIssue{issue("invalid_day_config", "Set valid school hours, period length, transition gap and an exact 45-minute lunch within the day")}
	}
	if len(c.Days) == 0 {
		issues = append(issues, issue("missing_teaching_days", "Choose the teaching days"))
	}
	seen := map[int]bool{}
	periods := []PlanPeriod{}
	for _, day := range c.Days {
		if day < 0 || day > 6 || seen[day] || off[day] {
			issues = append(issues, issue("invalid_teaching_day", "Teaching days must be distinct and cannot include a weekly off day"))
			continue
		}
		seen[day] = true
		for _, segment := range [][2]int{{start, lunch}, {lunch + 45, end}} {
			for at := segment[0]; at+c.PeriodMinutes <= segment[1]; at += c.PeriodMinutes + c.TransitionMinutes {
				periods = append(periods, plannedPeriod(day, at, at+c.PeriodMinutes, "lesson"))
			}
		}
		periods = append(periods, plannedPeriod(day, lunch, lunch+45, "lunch"))
	}
	if len(periods) == len(seen) {
		issues = append(issues, issue("no_periods", "The configured school day has no complete teaching period"))
	}
	sort.Slice(periods, func(i, j int) bool {
		if periods[i].DayOfWeek != periods[j].DayOfWeek {
			return periods[i].DayOfWeek < periods[j].DayOfWeek
		}
		return periods[i].StartTime < periods[j].StartTime
	})
	return periods, issues
}

func (s *ClassPlannerService) catalogue(tx *gorm.DB, session string) (PlanCatalogue, []ClassInput, []ClassInput, int, error) {
	c := PlanCatalogue{Sections: []ClassPlanSection{}, Subjects: []ClassPlanSubject{}, Assignments: []ClassPlanAssignment{}, Rooms: []string{}, OffDays: []int{}}
	if err := tx.Raw(`SELECT sec.id,c.id AS class_id,sec.name,c.name AS class_name FROM academic_sections sec JOIN academic_classes c ON c.id=sec.class_id
WHERE sec.session_id=? AND sec.is_active AND c.is_active ORDER BY c.display_order,sec.display_order,sec.name`, session).Scan(&c.Sections).Error; err != nil {
		return c, nil, nil, 0, err
	}
	if err := tx.Raw(`SELECT DISTINCT sub.id,acs.class_id,sub.name_en AS name,sub.name_bn FROM academic_class_subjects acs JOIN academic_subjects sub ON sub.id=acs.subject_id
JOIN academic_sections sec ON sec.class_id=acs.class_id AND sec.session_id=? AND sec.is_active WHERE sub.is_active ORDER BY acs.class_id,sub.name_en`, session).Scan(&c.Subjects).Error; err != nil {
		return c, nil, nil, 0, err
	}
	if err := tx.Raw(`SELECT a.employee_id AS teacher_id,a.class_id,a.subject_id,e.name_en AS teacher_name FROM teacher_assignments a JOIN employees e ON e.id=a.employee_id
WHERE a.session_id=? AND e.kind='teacher' AND e.is_active ORDER BY e.name_en,a.class_id,a.subject_id`, session).Scan(&c.Assignments).Error; err != nil {
		return c, nil, nil, 0, err
	}
	if err := tx.Raw("SELECT name FROM exam_rooms WHERE is_active ORDER BY name").Scan(&c.Rooms).Error; err != nil {
		return c, nil, nil, 0, err
	}
	if err := tx.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY day_of_week").Scan(&c.OffDays).Error; err != nil {
		return c, nil, nil, 0, err
	}
	selectSlots := `SELECT r.session_id,r.class_id,r.section_id,r.subject_id,r.teacher_id,r.day_of_week,
lpad((r.start_minute/60)::text,2,'0')||':'||lpad((r.start_minute%60)::text,2,'0') AS start_time,
lpad((r.end_minute/60)::text,2,'0')||':'||lpad((r.end_minute%60)::text,2,'0') AS end_time,r.room FROM class_routines r`
	current := []ClassInput{}
	if err := tx.Raw(selectSlots+" WHERE r.session_id=? ORDER BY r.day_of_week,r.start_minute,r.section_id", session).Scan(&current).Error; err != nil {
		return c, nil, nil, 0, err
	}
	other := []ClassInput{}
	if err := tx.Raw(selectSlots+` JOIN academic_sessions a ON a.id=r.session_id JOIN academic_sessions selected ON selected.id=?
WHERE r.session_id<>? AND a.start_date<=selected.end_date AND a.end_date>=selected.start_date`, session, session).Scan(&other).Error; err != nil {
		return c, nil, nil, 0, err
	}
	var published int
	if err := tx.Raw("SELECT coalesce(max(version),0) FROM class_routine_plan_versions WHERE session_id=? AND state='published'", session).Row().Scan(&published); err != nil {
		return c, nil, nil, 0, err
	}
	return c, current, other, published, nil
}

func (s *ClassPlannerService) preview(tx *gorm.DB, in ClassPlan) (ClassPlanPreview, error) {
	p := ClassPlanPreview{Plan: in, Periods: []PlanPeriod{}, Free: []PlanPeriod{}, Issues: []PlanIssue{}, Existing: []ClassInput{}, ExistingCombined: []CombinedInput{}}
	if !uuid.MatchString(in.SessionID) || len(in.Slots) > 3000 || len(in.CombinedSlots) > 1000 || len(in.Config.Frequencies) > 500 || len(in.Config.Unavailable) > 500 {
		return p, ErrInvalid
	}
	var exists int64
	if err := tx.Raw("SELECT count(*) FROM academic_sessions WHERE id=? AND is_active", in.SessionID).Row().Scan(&exists); err != nil {
		return p, err
	}
	if exists == 0 {
		return p, ErrInvalid
	}
	c, current, other, published, err := s.catalogue(tx, in.SessionID)
	if err != nil {
		return p, err
	}
	p.Catalogue, p.Existing, p.PublishedVersion = c, current, published
	if p.ExistingCombined, err = s.currentCombined(tx, in.SessionID); err != nil {
		return p, err
	}
	off := map[int]bool{}
	for _, d := range c.OffDays {
		off[d] = true
	}
	p.Periods, p.Issues = buildPeriods(in.Config, off)
	if !in.Config.AvailabilityConfirmed {
		p.Issues = append(p.Issues, issue("availability_unconfirmed", "Confirm teacher availability and enter any unavailable intervals"))
	}
	if len(c.Sections) == 0 {
		p.Issues = append(p.Issues, issue("no_sections", "No active sections exist in this session"))
	}
	if len(c.Rooms) == 0 {
		p.Issues = append(p.Issues, issue("no_rooms", "Add active rooms before publishing"))
	}
	sections := map[string]ClassPlanSection{}
	for _, v := range c.Sections {
		sections[v.ID] = v
	}
	subjects := map[string]bool{}
	for _, v := range c.Subjects {
		subjects[v.ClassID+"|"+v.ID] = true
	}
	assignments := map[string]bool{}
	teachers := map[string]bool{}
	for _, v := range c.Assignments {
		assignments[v.ClassID+"|"+v.SubjectID+"|"+v.TeacherID] = true
		teachers[v.TeacherID] = true
	}
	rooms := map[string]bool{}
	for _, v := range c.Rooms {
		rooms[normalizedRoom(v)] = true
	}
	frequencies := map[string]int{}
	for _, v := range in.Config.Frequencies {
		key := v.ClassID + "|" + v.SubjectID
		if !uuid.MatchString(v.ClassID) || !uuid.MatchString(v.SubjectID) || !subjects[key] || v.PeriodsPerWeek < 0 || v.PeriodsPerWeek > 30 {
			return p, ErrInvalid
		}
		if _, ok := frequencies[key]; ok {
			return p, ErrInvalid
		}
		frequencies[key] = v.PeriodsPerWeek
	}
	for _, sub := range c.Subjects {
		key := sub.ClassID + "|" + sub.ID
		n, ok := frequencies[key]
		if !ok {
			p.Issues = append(p.Issues, PlanIssue{Code: "frequency_missing", Message: "Set weekly frequency for " + sub.Name, SubjectID: sub.ID})
			continue
		}
		if n > 0 {
			hasTeacher := false
			for _, a := range c.Assignments {
				if a.ClassID == sub.ClassID && a.SubjectID == sub.ID {
					hasTeacher = true
					break
				}
			}
			if !hasTeacher {
				p.Issues = append(p.Issues, PlanIssue{Code: "teacher_missing", Message: "No assigned teacher for " + sub.Name, SubjectID: sub.ID})
			}
		}
	}
	unavailable := map[string][][2]int{}
	for _, v := range in.Config.Unavailable {
		a, okA := minute(v.StartTime)
		b, okB := minute(v.EndTime)
		if !uuid.MatchString(v.TeacherID) || !teachers[v.TeacherID] || v.DayOfWeek < 0 || v.DayOfWeek > 6 || !okA || !okB || a >= b {
			return p, ErrInvalid
		}
		unavailable[fmt.Sprintf("%s|%d", v.TeacherID, v.DayOfWeek)] = append(unavailable[fmt.Sprintf("%s|%d", v.TeacherID, v.DayOfWeek)], [2]int{a, b})
	}
	allowed := map[string]bool{}
	used := map[string]bool{}
	for _, period := range p.Periods {
		if period.Kind == "lesson" {
			allowed[fmt.Sprintf("%d|%s|%s", period.DayOfWeek, period.StartTime, period.EndTime)] = true
		}
	}
	counts := map[string]int{}
	for i, v := range in.Slots {
		sec, ok := sections[v.SectionID]
		a, okA := minute(v.StartTime)
		b, okB := minute(v.EndTime)
		valid := uuid.MatchString(v.TeacherID) && uuid.MatchString(v.SubjectID) && ok && sec.ClassID == v.ClassID && v.SessionID == in.SessionID && okA && okB && a < b && v.DayOfWeek >= 0 && v.DayOfWeek <= 6
		if !valid {
			p.Issues = append(p.Issues, PlanIssue{Code: "invalid_slot", Message: "Choose an active section, subject, teacher and valid time", Slot: i + 1})
			continue
		}
		if !allowed[fmt.Sprintf("%d|%s|%s", v.DayOfWeek, v.StartTime, v.EndTime)] {
			p.Issues = append(p.Issues, PlanIssue{Code: "outside_period", Message: "Lesson must match a teaching period and cannot cross lunch or a weekly off day", Slot: i + 1})
		}
		if !subjects[v.ClassID+"|"+v.SubjectID] || !assignments[v.ClassID+"|"+v.SubjectID+"|"+v.TeacherID] {
			p.Issues = append(p.Issues, PlanIssue{Code: "assignment_missing", Message: "Subject and teacher assignment must match the class", Slot: i + 1})
		}
		if !rooms[normalizedRoom(v.Room)] {
			p.Issues = append(p.Issues, PlanIssue{Code: "room_missing", Message: "Choose an active registered room", Slot: i + 1})
		}
		for _, block := range unavailable[fmt.Sprintf("%s|%d", v.TeacherID, v.DayOfWeek)] {
			if a < block[1] && b > block[0] {
				p.Issues = append(p.Issues, PlanIssue{Code: "teacher_unavailable", Message: "Teacher is unavailable during this period", Slot: i + 1})
			}
		}
		for j := 0; j < i; j++ {
			otherSlot := in.Slots[j]
			x, xok := minute(otherSlot.StartTime)
			y, yok := minute(otherSlot.EndTime)
			if xok && yok && v.DayOfWeek == otherSlot.DayOfWeek && a < y && b > x &&
				(v.SectionID == otherSlot.SectionID || v.TeacherID == otherSlot.TeacherID || normalizedRoom(v.Room) == normalizedRoom(otherSlot.Room)) {
				p.Issues = append(p.Issues, PlanIssue{Code: "overlap", Message: "Teacher, section or room overlaps another proposed lesson", Slot: i + 1})
			}
		}
		for _, otherSlot := range other {
			x, xok := minute(otherSlot.StartTime)
			y, yok := minute(otherSlot.EndTime)
			if xok && yok && v.DayOfWeek == otherSlot.DayOfWeek && a < y && b > x &&
				(v.TeacherID == otherSlot.TeacherID || normalizedRoom(v.Room) == normalizedRoom(otherSlot.Room)) {
				p.Issues = append(p.Issues, PlanIssue{Code: "overlap_existing_session", Message: "Teacher or room overlaps a concurrent academic session", Slot: i + 1})
			}
		}
		var cohortOverlap int64
		if err := tx.Raw(`SELECT count(*) FROM class_cohort_lessons l
JOIN academic_sessions prior ON prior.id=l.session_id
JOIN academic_sessions selected ON selected.id=?
WHERE l.is_current AND l.session_id<>? AND prior.start_date<=selected.end_date
AND prior.end_date>=selected.start_date AND l.day_of_week=?
AND l.start_minute<? AND l.end_minute>?
AND (l.teacher_id=? OR lower(l.room)=lower(?) OR EXISTS (
 SELECT 1 FROM class_cohort_members m JOIN student_enrolments en
 ON en.student_id=m.student_id AND en.session_id=?
 WHERE m.lesson_id=l.id AND en.section_id=? AND en.state='current'))`,
			in.SessionID, in.SessionID, v.DayOfWeek, b, a, v.TeacherID, v.Room, in.SessionID, v.SectionID).Row().Scan(&cohortOverlap); err != nil {
			return p, err
		}
		if cohortOverlap > 0 {
			p.Issues = append(p.Issues, PlanIssue{Code: "overlap_existing_cohort", Message: "Section lesson overlaps an active cohort in another academic session", Slot: i + 1})
		}
		counts[v.SectionID+"|"+v.SubjectID]++
		used[fmt.Sprintf("%s|%d|%s|%s", v.SectionID, v.DayOfWeek, v.StartTime, v.EndTime)] = true
	}
	if err := s.validateCombined(tx, &p, in, c, sections, subjects, assignments, rooms, unavailable, allowed, counts, used, other); err != nil {
		return p, err
	}
	// Religion Education and fourth-subject lessons are student-specific.
	// Their class-wide frequency applies to a section only when at least one
	// current student there actually takes that subject.
	specialRows := []struct{ ClassID, SubjectID string }{}
	if err := tx.Raw(`SELECT acs.class_id,acs.subject_id FROM academic_class_subjects acs
LEFT JOIN class_religion_subjects cr ON cr.class_id=acs.class_id AND cr.subject_id=acs.subject_id
WHERE acs.is_fourth_subject OR cr.subject_id IS NOT NULL`).Scan(&specialRows).Error; err != nil {
		return p, err
	}
	special := map[string]bool{}
	for _, row := range specialRows {
		special[row.ClassID+"|"+row.SubjectID] = true
	}
	applicableRows := []struct{ SectionID, SubjectID string }{}
	if err := tx.Raw(`SELECT DISTINCT en.section_id,acs.subject_id FROM student_enrolments en
JOIN academic_class_subjects acs ON acs.class_id=en.class_id
LEFT JOIN class_religion_subjects cr ON cr.class_id=acs.class_id AND cr.subject_id=acs.subject_id
WHERE en.session_id=? AND en.state='current' AND en.student_status='active'
AND (acs.is_fourth_subject OR cr.subject_id IS NOT NULL)
AND student_takes_subject(en.student_id,en.session_id,en.class_id,acs.subject_id)`, in.SessionID).Scan(&applicableRows).Error; err != nil {
		return p, err
	}
	applicable := map[string]bool{}
	for _, row := range applicableRows {
		applicable[row.SectionID+"|"+row.SubjectID] = true
	}
	for _, sec := range c.Sections {
		for _, sub := range c.Subjects {
			if sub.ClassID != sec.ClassID {
				continue
			}
			want, ok := frequencies[sec.ClassID+"|"+sub.ID]
			if special[sec.ClassID+"|"+sub.ID] && !applicable[sec.ID+"|"+sub.ID] {
				want = 0
			}
			if ok && counts[sec.ID+"|"+sub.ID] != want {
				p.Issues = append(p.Issues, PlanIssue{Code: "frequency_mismatch", Message: fmt.Sprintf("%s %s needs %d periods; proposed %d", sec.ClassName+" "+sec.Name, sub.Name, want, counts[sec.ID+"|"+sub.ID]), SectionID: sec.ID, SubjectID: sub.ID})
			}
		}
	}
	for _, sec := range c.Sections {
		for _, period := range p.Periods {
			if period.Kind == "lesson" && !used[fmt.Sprintf("%s|%d|%s|%s", sec.ID, period.DayOfWeek, period.StartTime, period.EndTime)] {
				p.Free = append(p.Free, PlanPeriod{DayOfWeek: period.DayOfWeek, StartTime: period.StartTime, EndTime: period.EndTime, Kind: sec.ID})
			}
		}
	}
	if len(in.Slots) == 0 && len(in.CombinedSlots) == 0 {
		p.Issues = append(p.Issues, issue("empty_plan", "Add lessons manually before publishing"))
	}
	p.CanPublish = len(p.Issues) == 0
	bytes, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	sum := sha256.Sum256(bytes)
	p.Digest = hex.EncodeToString(sum[:])
	return p, nil
}

func (s *ClassPlannerService) Preview(ctx context.Context, in ClassPlan) (ClassPlanPreview, error) {
	return s.preview(s.db.WithContext(ctx), in)
}

func (s *ClassPlannerService) Draft(ctx context.Context, session string) (ClassPlanVersion, error) {
	var out ClassPlanVersion
	if !uuid.MatchString(session) {
		return out, ErrInvalid
	}
	var row plannerVersionRow
	err := s.db.WithContext(ctx).Raw(`SELECT id,session_id,version,state,plan,to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') AS updated_at
FROM class_routine_plan_versions WHERE session_id=? AND state='draft'`, session).Scan(&row).Error
	if err != nil {
		return out, err
	}
	if row.ID == "" {
		return out, ErrNotFound
	}
	out = ClassPlanVersion{ID: row.ID, SessionID: row.SessionID, Version: row.Version, State: row.State, UpdatedAt: row.UpdatedAt}
	return out, json.Unmarshal(row.PlanJSON, &out.Plan)
}

func (s *ClassPlannerService) SaveDraft(ctx context.Context, actor string, in ClassPlan, previewDigest string) (ClassPlanVersion, error) {
	if !uuid.MatchString(actor) || len(previewDigest) != 64 {
		return ClassPlanVersion{}, ErrInvalid
	}
	preview, err := s.Preview(ctx, in)
	if err != nil {
		return ClassPlanVersion{}, err
	}
	if preview.Digest != previewDigest {
		return ClassPlanVersion{}, ErrConflict
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		body, err := json.Marshal(in)
		if err != nil {
			return err
		}
		var id string
		if err := tx.Raw("SELECT id FROM class_routine_plan_versions WHERE session_id=? AND state='draft'", in.SessionID).Row().Scan(&id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if id == "" {
			var version int
			if err := tx.Raw("SELECT coalesce(max(version),0)+1 FROM class_routine_plan_versions WHERE session_id=?", in.SessionID).Row().Scan(&version); err != nil {
				return err
			}
			if err := tx.Raw(`INSERT INTO class_routine_plan_versions(session_id,version,state,plan,created_by,updated_by) VALUES (?,?,'draft',?::jsonb,?,?) RETURNING id`, in.SessionID, version, string(body), actor, actor).Row().Scan(&id); err != nil {
				return err
			}
		} else {
			if err := tx.Exec("UPDATE class_routine_plan_versions SET plan=?::jsonb,updated_by=?,updated_at=now() WHERE id=?", string(body), actor, id).Error; err != nil {
				return err
			}
		}
		return audit.Record(tx, &actor, "routine.class_plan_draft_saved", "class_routine_plan", id, map[string]any{"session_id": in.SessionID, "slots": len(in.Slots)})
	})
	if err != nil {
		return ClassPlanVersion{}, err
	}
	return s.Draft(ctx, in.SessionID)
}

func (s *ClassPlannerService) Publish(ctx context.Context, actor, session, draftID, digest string, confirmed bool) (ClassPlanVersion, error) {
	if !confirmed || !uuid.MatchString(actor) || !uuid.MatchString(session) || !uuid.MatchString(draftID) || len(digest) != 64 {
		return ClassPlanVersion{}, ErrInvalid
	}
	var out ClassPlanVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		var row plannerVersionRow
		if err := tx.Raw("SELECT id,session_id,version,state,plan FROM class_routine_plan_versions WHERE id=? AND session_id=? AND state='draft' FOR UPDATE", draftID, session).Scan(&row).Error; err != nil {
			return err
		}
		if row.ID == "" {
			return ErrNotFound
		}
		var plan ClassPlan
		if err := json.Unmarshal(row.PlanJSON, &plan); err != nil {
			return err
		}
		// Hold the rows that determine cohort eligibility and conflicts while
		// rechecking the preview and publishing. A competing writer makes the
		// publication fail rather than racing a stale roster.
		if err := tx.Exec(`LOCK TABLE student_enrolments, student_subject_choices, class_religion_subjects,
class_group_subjects, class_routines, class_cohort_lessons, class_cohort_members,
academic_class_subjects, teacher_assignments, exam_rooms, school_weekly_off_days
IN SHARE ROW EXCLUSIVE MODE NOWAIT`).Error; err != nil {
			return err
		}
		preview, err := s.preview(tx, plan)
		if err != nil {
			return err
		}
		if !preview.CanPublish || preview.Digest != digest {
			return ErrConflict
		}
		if row.Version == 1 && len(preview.Existing) > 0 {
			old := ClassPlan{SessionID: session, Slots: preview.Existing}
			body, err := json.Marshal(old)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO class_routine_plan_versions(session_id,version,state,plan) VALUES (?,0,'legacy',?::jsonb) ON CONFLICT (session_id,version) DO NOTHING`, session, string(body)).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("SELECT set_config('app.class_planner_publish','on',true)").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM class_routines WHERE session_id=?", session).Error; err != nil {
			return err
		}
		for _, slot := range plan.Slots {
			a, _ := minute(slot.StartTime)
			b, _ := minute(slot.EndTime)
			if err := tx.Exec(`INSERT INTO class_routines(session_id,class_id,section_id,subject_id,teacher_id,day_of_week,start_minute,end_minute,room,created_by,updated_by)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`, session, slot.ClassID, slot.SectionID, slot.SubjectID, slot.TeacherID, slot.DayOfWeek, a, b, strings.TrimSpace(slot.Room), actor, actor).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE class_cohort_lessons SET is_current=false WHERE session_id=? AND is_current", session).Error; err != nil {
			return err
		}
		for _, slot := range plan.CombinedSlots {
			a, _ := minute(slot.StartTime)
			b, _ := minute(slot.EndTime)
			var lessonID string
			if err := tx.Raw(`INSERT INTO class_cohort_lessons(plan_version_id,session_id,class_id,subject_id,teacher_id,day_of_week,start_minute,end_minute,room)
VALUES (?,?,?,?,?,?,?,?,?) RETURNING id`, draftID, session, slot.ClassID, slot.SubjectID, slot.TeacherID, slot.DayOfWeek, a, b, strings.TrimSpace(slot.Room)).Row().Scan(&lessonID); err != nil {
				return err
			}
			for _, studentID := range slot.StudentIDs {
				var sectionID string
				if err := tx.Raw(`SELECT section_id FROM student_enrolments WHERE student_id=? AND session_id=? AND class_id=? AND state='current' AND student_status='active'`, studentID, session, slot.ClassID).Row().Scan(&sectionID); err != nil {
					return err
				}
				if err := tx.Exec(`INSERT INTO class_cohort_members(lesson_id,student_id,session_id,class_id,section_id) VALUES (?,?,?,?,?)`, lessonID, studentID, session, slot.ClassID, sectionID).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Exec("UPDATE class_routine_plan_versions SET state='published',published_by=?,published_at=now(),updated_at=now() WHERE id=?", actor, draftID).Error; err != nil {
			return err
		}
		if err := audit.Record(tx, &actor, "routine.class_plan_published", "class_routine_plan", draftID, map[string]any{"session_id": session, "version": row.Version, "slots": len(plan.Slots), "combined_slots": len(plan.CombinedSlots)}); err != nil {
			return err
		}
		out = ClassPlanVersion{ID: draftID, SessionID: session, Version: row.Version, State: "published", Plan: plan}
		return nil
	})
	return out, err
}
