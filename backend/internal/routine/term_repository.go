package routine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine/planner"
	"gorm.io/gorm"
)

type GormTermRepository struct{ db *gorm.DB }

func NewTermRepository(db *gorm.DB) *GormTermRepository { return &GormTermRepository{db: db} }

var _ TermRepository = (*GormTermRepository)(nil)

// dbError turns constraint failures into ErrConflict.
func dbError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		switch p.Code {
		case "23505", "23503", "23514", "23P01":
			return ErrConflict
		case "22P02", "22007", "22008":
			return ErrInvalid
		}
	}
	return missing(err)
}

func ts(col string) string {
	return fmt.Sprintf(`to_char(%s AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`, col)
}
func hhmm(col string) string {
	return fmt.Sprintf("lpad((%[1]s/60)::text,2,'0')||':'||lpad((%[1]s%%60)::text,2,'0')", col)
}
func clockText(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// ---------------------------------------------------------------------------
// Rooms

func (r *GormTermRepository) ListRooms(ctx context.Context, all bool) ([]Room, error) {
	out := []Room{}
	q := r.db.WithContext(ctx).Table("exam_rooms").Select("id,name,name_bn,kind,capacity,is_active")
	if !all {
		q = q.Where("is_active")
	}
	err := q.Order("lower(name),id").Scan(&out).Error
	return out, err
}

func (r *GormTermRepository) SaveRoom(ctx context.Context, actor, id string, in RoomInput) (Room, error) {
	var out Room
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		active := true
		if in.IsActive != nil {
			active = *in.IsActive
		}
		var err error
		if id == "" {
			err = tx.Raw("INSERT INTO exam_rooms(name,name_bn,kind,capacity,is_active,created_by) VALUES (?,?,?,?,?,?) RETURNING id", in.Name, in.NameBN, in.Kind, in.Capacity, active, actor).Scan(&id).Error
		} else {
			res := tx.Exec("UPDATE exam_rooms SET name=?,name_bn=?,kind=?,capacity=?,is_active=?,updated_at=now() WHERE id=?", in.Name, in.NameBN, in.Kind, in.Capacity, active, id)
			err = res.Error
			if err == nil && res.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		if err != nil {
			return dbError(err)
		}
		if err := tx.Table("exam_rooms").Select("id,name,name_bn,kind,capacity,is_active").Where("id=?", id).Take(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.room_saved", "exam_room", id, map[string]any{"name": in.Name, "kind": in.Kind, "capacity": in.Capacity, "active": active})
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Terms and versions

const summaryColumns = `v.id, v.term_id, v.version, v.status, v.reason, v.returned_reason,
 to_char(v.starts_on,'YYYY-MM-DD') AS starts_on, to_char(v.ends_on,'YYYY-MM-DD') AS ends_on`

func summarySelect() string {
	return summaryColumns + "," + ts("v.created_at") + " AS created_at," + ts("v.submitted_at") + " AS submitted_at," + ts("v.approved_at") + " AS approved_at," +
		"au.display_name AS approved_by_name," + ts("v.published_at") + " AS published_at," + ts("v.superseded_at") + " AS superseded_at,v.content_sha256," +
		"(SELECT count(*) FROM routine_papers p WHERE p.version_id=v.id) AS papers,(SELECT count(*) FROM routine_papers p WHERE p.version_id=v.id AND p.exam_date IS NOT NULL) AS placed"
}

func (r *GormTermRepository) ListTerms(ctx context.Context, sessionID string) ([]TermSummary, error) {
	db := r.db.WithContext(ctx)
	terms := []TermSummary{}
	q := db.Table("exam_terms t").Joins("JOIN academic_sessions s ON s.id=t.session_id").
		Select("t.id AS term_id,t.name AS term_name,t.session_id,s.name AS session_name,to_char(t.starts_on,'YYYY-MM-DD') AS starts_on,to_char(t.ends_on,'YYYY-MM-DD') AS ends_on").
		Where("t.kind IN ('term','annual')")
	if sessionID != "" {
		q = q.Where("t.session_id=?", sessionID)
	}
	if err := q.Order("s.start_date DESC,t.sequence,t.name,t.id").Scan(&terms).Error; err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return terms, nil
	}
	termIDs := make([]string, len(terms))
	for i, t := range terms {
		termIDs[i] = t.TermID
		terms[i].Versions = []VersionSummary{}
	}
	versions := []VersionSummary{}
	if err := db.Table("routine_versions v").Joins("LEFT JOIN users au ON au.id=v.approved_by").Select(summarySelect()).Where("v.term_id IN ?", termIDs).Order("v.version DESC").Scan(&versions).Error; err != nil {
		return nil, err
	}
	for _, v := range versions {
		for i := range terms {
			if terms[i].TermID == v.TermID {
				terms[i].Versions = append(terms[i].Versions, v)
			}
		}
	}
	return terms, nil
}

type versionRow struct {
	VersionSummary
	BasedOn      *string
	Instructions string
	TermName     string
	SessionID    string
	SessionName  string
	TermStartsOn *string
	TermEndsOn   *string
	SessionStart string
	SessionEnd   string
}

func (r *GormTermRepository) versionRow(tx *gorm.DB, id string) (versionRow, error) {
	var row versionRow
	err := tx.Table("routine_versions v").Joins("JOIN exam_terms t ON t.id=v.term_id").Joins("JOIN academic_sessions s ON s.id=t.session_id").Joins("LEFT JOIN users au ON au.id=v.approved_by").
		Select(summarySelect()+",v.based_on,v.instructions,t.name AS term_name,t.session_id,s.name AS session_name,to_char(t.starts_on,'YYYY-MM-DD') AS term_starts_on,to_char(t.ends_on,'YYYY-MM-DD') AS term_ends_on,to_char(s.start_date,'YYYY-MM-DD') AS session_start,to_char(s.end_date,'YYYY-MM-DD') AS session_end").
		Where("v.id=?", id).Take(&row).Error
	return row, missing(err)
}

// lockDraft locks a version for a change and requires it to be a draft.
func lockVersion(tx *gorm.DB, id string) (string, error) {
	var status string
	err := tx.Raw("SELECT status FROM routine_versions WHERE id=? FOR UPDATE", id).Scan(&status).Error
	if err != nil {
		return "", err
	}
	if status == "" {
		return "", ErrNotFound
	}
	return status, nil
}
func lockDraft(tx *gorm.DB, id string) error {
	status, err := lockVersion(tx, id)
	if err != nil {
		return err
	}
	if status != "draft" {
		return ErrState
	}
	return nil
}

type paperRoomRow struct {
	PaperID string
	PaperRoom
}

func loadPapers(tx *gorm.DB, versionID string) ([]Paper, error) {
	papers := []Paper{}
	err := tx.Table("routine_papers p").Joins("JOIN academic_classes c ON c.id=p.class_id").Joins("JOIN academic_subjects sub ON sub.id=p.subject_id").
		Joins("JOIN academic_class_subjects acs ON acs.class_id=p.class_id AND acs.subject_id=p.subject_id").
		Select("p.id,p.class_id,c.name AS class_name,c.display_order AS class_order,p.subject_id,sub.code AS subject_code,sub.name_en AS subject_name,sub.name_bn AS subject_name_bn,p.paper_kind AS kind,p.paper_no,p.duration_minutes AS duration,p.needs_lab,acs.is_fourth_subject AS optional,p.pinned,to_char(p.exam_date,'YYYY-MM-DD') AS exam_date,p.sitting_id").
		Where("p.version_id=?", versionID).Order("c.display_order,c.name,c.id,sub.code,p.paper_no,array_position(ARRAY['theory','practical','viva','oral'],p.paper_kind),p.id").Scan(&papers).Error
	if err != nil {
		return nil, err
	}
	rooms := []paperRoomRow{}
	if err := tx.Table("routine_paper_rooms pr").Joins("JOIN routine_papers p ON p.id=pr.paper_id").Joins("JOIN academic_sections sec ON sec.id=pr.section_id").Joins("JOIN exam_rooms r ON r.id=pr.room_id").
		Select("pr.paper_id,pr.section_id,sec.name AS section_name,pr.room_id,r.name AS room_name,r.name_bn AS room_name_bn").Where("p.version_id=?", versionID).Order("sec.name,sec.id").Scan(&rooms).Error; err != nil {
		return nil, err
	}
	at := map[string]int{}
	for i := range papers {
		papers[i].Rooms = []PaperRoom{}
		at[papers[i].ID] = i
	}
	for _, x := range rooms {
		if i, ok := at[x.PaperID]; ok {
			papers[i].Rooms = append(papers[i].Rooms, x.PaperRoom)
		}
	}
	return papers, nil
}

func loadSittings(tx *gorm.DB, versionID string) ([]Sitting, error) {
	out := []Sitting{}
	err := tx.Table("routine_sittings s").Select("s.id,s.name,s.name_bn,s.start_minute AS start,s.end_minute AS \"end\","+hhmm("s.start_minute")+" AS start_time,"+hhmm("s.end_minute")+" AS end_time,s.end_minute-s.start_minute AS minutes").
		Where("s.version_id=?", versionID).Order("s.start_minute,s.id").Scan(&out).Error
	return out, err
}

func loadHolidays(tx *gorm.DB, from, to string) ([]Holiday, error) {
	rows := []Holiday{}
	err := tx.Raw(`SELECT DISTINCT ON (d) to_char(d,'YYYY-MM-DD') AS date, e.title_en, e.title_bn
 FROM school_events e, generate_series(greatest(e.starts_on, ?::date), least(e.ends_on, ?::date), interval '1 day') AS d
 WHERE e.kind = 'holiday' AND e.status = 'published' AND e.starts_on <= ?::date AND e.ends_on >= ?::date
 ORDER BY d, e.starts_on, e.created_at`, from, to, to, from).Scan(&rows).Error
	return rows, err
}

// load reads a version and the plan its rules check.
func (r *GormTermRepository) load(tx *gorm.DB, id string) (Version, *planner.Plan, error) {
	row, err := r.versionRow(tx, id)
	if err != nil {
		return Version{}, nil, err
	}
	v := Version{VersionSummary: row.VersionSummary, TermName: row.TermName, SessionID: row.SessionID, SessionName: row.SessionName, TermStartsOn: row.TermStartsOn, TermEndsOn: row.TermEndsOn, BasedOn: row.BasedOn, Instructions: row.Instructions}
	v.From, v.To = row.StartsOn, row.EndsOn
	if row.TermStartsOn != nil && *row.TermStartsOn > v.From {
		v.From = *row.TermStartsOn
	}
	if row.TermEndsOn != nil && *row.TermEndsOn < v.To {
		v.To = *row.TermEndsOn
	}
	if v.Sittings, err = loadSittings(tx, id); err != nil {
		return v, nil, err
	}
	if v.Papers, err = loadPapers(tx, id); err != nil {
		return v, nil, err
	}
	v.Duties = []Duty{}
	if err := tx.Table("routine_duties d").Joins("JOIN exam_rooms r ON r.id=d.room_id").Joins("JOIN employees e ON e.id=d.employee_id").Joins("JOIN routine_sittings s ON s.id=d.sitting_id").
		Select("d.id,to_char(d.exam_date,'YYYY-MM-DD') AS exam_date,d.sitting_id,d.room_id,r.name AS room_name,r.name_bn AS room_name_bn,d.employee_id,e.name_en AS employee_name,e.name_bn AS employee_name_bn").
		Where("d.version_id=?", id).Order("d.exam_date,s.start_minute,lower(r.name),e.name_en,d.id").Scan(&v.Duties).Error; err != nil {
		return v, nil, err
	}
	// Classes and their sections in the term's session.
	type secRow struct {
		ID, ClassID, Name string
		Students          int
	}
	secs := []secRow{}
	if err := tx.Raw(`SELECT sec.id, sec.class_id, sec.name,
 (SELECT count(*) FROM students s WHERE s.section_id = sec.id AND s.session_id = sec.session_id AND s.status = 'active') AS students
 FROM academic_sections sec
 WHERE sec.session_id = ? AND sec.class_id IN (SELECT DISTINCT class_id FROM routine_papers WHERE version_id = ?)
   AND (sec.is_active OR sec.id IN (SELECT pr.section_id FROM routine_paper_rooms pr JOIN routine_papers p ON p.id = pr.paper_id WHERE p.version_id = ?))
 ORDER BY sec.name, sec.id`, row.SessionID, id, id).Scan(&secs).Error; err != nil {
		return v, nil, err
	}
	v.Classes = []PlanClass{}
	classAt := map[string]int{}
	plan := &planner.Plan{From: v.From, To: v.To, OffDays: map[int]bool{}, Holidays: map[string]string{}, Takers: map[string]map[string]int{}, Both: map[string]map[string]int{}, Teaches: map[string]map[string]bool{}}
	for _, x := range v.Papers {
		if _, ok := classAt[x.ClassID]; !ok {
			classAt[x.ClassID] = len(v.Classes)
			v.Classes = append(v.Classes, PlanClass{ID: x.ClassID, Name: x.ClassName, Sections: []PlanSection{}})
			plan.Classes = append(plan.Classes, planner.Class{ID: x.ClassID, Name: x.ClassName, Order: x.ClassOrder})
		}
	}
	for _, s := range secs {
		if i, ok := classAt[s.ClassID]; ok {
			v.Classes[i].Sections = append(v.Classes[i].Sections, PlanSection{ID: s.ID, Name: s.Name, Students: s.Students})
			plan.Sections = append(plan.Sections, planner.Section{ID: s.ID, ClassID: s.ClassID, Name: s.Name})
		}
	}
	// Who sits what: every subject except an optional one a student did
	// not choose.
	type takerRow struct {
		SectionID, SubjectID string
		N                    int
	}
	takers := []takerRow{}
	if err := tx.Raw(`SELECT s.section_id, p.subject_id, count(*) AS n
 FROM students s JOIN (SELECT DISTINCT class_id, subject_id FROM routine_papers WHERE version_id = ?) p ON p.class_id = s.class_id
 WHERE s.session_id = ? AND s.status = 'active' AND student_takes_subject(s.id, s.session_id, s.class_id, p.subject_id)
 GROUP BY s.section_id, p.subject_id`, id, row.SessionID).Scan(&takers).Error; err != nil {
		return v, nil, err
	}
	for _, t := range takers {
		if plan.Takers[t.SectionID] == nil {
			plan.Takers[t.SectionID] = map[string]int{}
		}
		plan.Takers[t.SectionID][t.SubjectID] = t.N
	}
	type bothRow struct {
		ClassID, A, B string
		N             int
	}
	both := []bothRow{}
	if err := tx.Raw(`WITH o AS (SELECT DISTINCT p.class_id, p.subject_id FROM routine_papers p
   JOIN academic_class_subjects acs ON acs.class_id = p.class_id AND acs.subject_id = p.subject_id AND acs.is_fourth_subject
  WHERE p.version_id = ?)
 SELECT s.class_id, a.subject_id::text AS a, b.subject_id::text AS b, count(*) AS n
 FROM students s JOIN o a ON a.class_id = s.class_id JOIN o b ON b.class_id = s.class_id AND a.subject_id::text < b.subject_id::text
 WHERE s.session_id = ? AND s.status = 'active'
   AND student_takes_subject(s.id, s.session_id, s.class_id, a.subject_id) AND student_takes_subject(s.id, s.session_id, s.class_id, b.subject_id)
 GROUP BY s.class_id, a.subject_id, b.subject_id`, id, row.SessionID).Scan(&both).Error; err != nil {
		return v, nil, err
	}
	for _, b := range both {
		if plan.Both[b.ClassID] == nil {
			plan.Both[b.ClassID] = map[string]int{}
		}
		plan.Both[b.ClassID][b.A+"|"+b.B] = b.N
	}
	students := map[string]int{}
	for i := range v.Papers {
		x := &v.Papers[i]
		for j := range x.Rooms {
			x.Rooms[j].Students = plan.Takers[x.Rooms[j].SectionID][x.SubjectID]
		}
		for _, sec := range v.Classes[classAt[x.ClassID]].Sections {
			x.Students += plan.Takers[sec.ID][x.SubjectID]
		}
		students[x.ID] = x.Students
	}
	// Rooms in use, and every active room.
	v.Rooms = []Room{}
	if err := tx.Raw(`SELECT id, name, name_bn, kind, capacity, is_active FROM exam_rooms
 WHERE is_active OR id IN (SELECT pr.room_id FROM routine_paper_rooms pr JOIN routine_papers p ON p.id = pr.paper_id WHERE p.version_id = ?)
    OR id IN (SELECT room_id FROM routine_duties WHERE version_id = ?)
 ORDER BY lower(name), id`, id, id).Scan(&v.Rooms).Error; err != nil {
		return v, nil, err
	}
	v.Teachers = []Teacher{}
	if err := tx.Raw(`SELECT id, name_en AS name, name_bn FROM employees
 WHERE (kind = 'teacher' AND is_active) OR id IN (SELECT employee_id FROM routine_duties WHERE version_id = ?)
 ORDER BY name_en, id`, id).Scan(&v.Teachers).Error; err != nil {
		return v, nil, err
	}
	type teachRow struct{ EmployeeID, ClassID, SubjectID string }
	teaches := []teachRow{}
	if err := tx.Raw("SELECT employee_id, class_id, subject_id FROM teacher_assignments WHERE session_id = ?", row.SessionID).Scan(&teaches).Error; err != nil {
		return v, nil, err
	}
	for _, t := range teaches {
		if plan.Teaches[t.EmployeeID] == nil {
			plan.Teaches[t.EmployeeID] = map[string]bool{}
		}
		plan.Teaches[t.EmployeeID][t.ClassID+"|"+t.SubjectID] = true
	}
	v.OffDays = []int{}
	if err := tx.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY day_of_week").Scan(&v.OffDays).Error; err != nil {
		return v, nil, err
	}
	if v.Holidays, err = loadHolidays(tx, row.StartsOn, row.EndsOn); err != nil {
		return v, nil, err
	}
	for _, d := range v.OffDays {
		plan.OffDays[d] = true
	}
	for _, h := range v.Holidays {
		plan.Holidays[h.Date] = h.TitleEN
	}
	for _, s := range v.Sittings {
		plan.Sittings = append(plan.Sittings, planner.Sitting{ID: s.ID, Name: s.Name, Start: s.Start, End: s.End})
	}
	for _, rm := range v.Rooms {
		plan.Rooms = append(plan.Rooms, planner.Room{ID: rm.ID, Name: rm.Name, Kind: rm.Kind, Capacity: rm.Capacity, Active: rm.IsActive})
	}
	for _, t := range v.Teachers {
		plan.Teachers = append(plan.Teachers, planner.Teacher{ID: t.ID, Name: t.Name})
	}
	for _, x := range v.Papers {
		plan.Papers = append(plan.Papers, toPlanner(x))
	}
	for _, d := range v.Duties {
		plan.Duties = append(plan.Duties, planner.Duty{ID: d.ID, Date: d.ExamDate, SittingID: d.SittingID, RoomID: d.RoomID, EmployeeID: d.EmployeeID})
	}
	// Only active teachers are given new duties.
	plan.Teachers = plan.Teachers[:0]
	if err := tx.Raw("SELECT id, name_en AS name FROM employees WHERE kind = 'teacher' AND is_active ORDER BY name_en, id").Scan(&plan.Teachers).Error; err != nil {
		return v, nil, err
	}
	names := map[string]string{}
	for _, t := range v.Teachers {
		names[t.ID] = t.Name
	}
	plan.Teachers = append(plan.Teachers, inactiveOnDuty(v, plan.Teachers, names)...)
	v.Dates = plan.Dates()
	v.Issues = planner.Validate(plan, true)
	v.Changes = []Change{}
	if row.BasedOn != nil {
		prev, err := r.versionRow(tx, *row.BasedOn)
		if err != nil {
			return v, nil, err
		}
		n := prev.Version
		v.PreviousLabel = &n
		if v.Changes, err = diff(tx, *row.BasedOn, v); err != nil {
			return v, nil, err
		}
	}
	return v, plan, nil
}

// inactiveOnDuty keeps the names of staff already on duty who are no longer
// active, so messages can name them; they get no new duties.
func inactiveOnDuty(v Version, active []planner.Teacher, names map[string]string) []planner.Teacher {
	have := map[string]bool{}
	for _, t := range active {
		have[t.ID] = true
	}
	out := []planner.Teacher{}
	for _, d := range v.Duties {
		if !have[d.EmployeeID] {
			have[d.EmployeeID] = true
			out = append(out, planner.Teacher{ID: d.EmployeeID, Name: names[d.EmployeeID]})
		}
	}
	return out
}

func toPlanner(x Paper) planner.Paper {
	p := planner.Paper{ID: x.ID, ClassID: x.ClassID, SubjectID: x.SubjectID, SubjectCode: x.SubjectCode, SubjectName: x.SubjectName, Kind: x.Kind, PaperNo: x.PaperNo, Duration: x.Duration, NeedsLab: x.NeedsLab, Optional: x.Optional, Pinned: x.Pinned, Rooms: map[string]string{}}
	if x.ExamDate != nil && x.SittingID != nil {
		p.Date, p.SittingID = *x.ExamDate, *x.SittingID
	}
	for _, rm := range x.Rooms {
		p.Rooms[rm.SectionID] = rm.RoomID
	}
	return p
}

func paperKeyOf(x Paper) string {
	return fmt.Sprintf("%s|%s|%s|%d", x.ClassID, x.SubjectID, x.Kind, x.PaperNo)
}
func roomsText(x Paper) string {
	parts := []string{}
	for _, rm := range x.Rooms {
		parts = append(parts, rm.SectionName+": "+rm.RoomName)
	}
	return strings.Join(parts, ", ")
}

// diff lists what an amendment changes, paper by paper.
func diff(tx *gorm.DB, previousID string, v Version) ([]Change, error) {
	before, err := loadPapers(tx, previousID)
	if err != nil {
		return nil, err
	}
	prevSittings, err := loadSittings(tx, previousID)
	if err != nil {
		return nil, err
	}
	sitName := map[string]string{}
	for _, s := range prevSittings {
		sitName[s.ID] = s.Name + " " + s.StartTime
	}
	for _, s := range v.Sittings {
		sitName[s.ID] = s.Name + " " + s.StartTime
	}
	old := map[string]Paper{}
	for _, x := range before {
		old[paperKeyOf(x)] = x
	}
	out := []Change{}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	base := func(x Paper, kind string) Change {
		return Change{Change: kind, ClassName: x.ClassName, SubjectName: x.SubjectName, SubjectNameBN: x.SubjectNameBN, Kind: x.Kind, PaperNo: x.PaperNo, key: paperKeyOf(x)}
	}
	seen := map[string]bool{}
	for _, x := range v.Papers {
		k := paperKeyOf(x)
		seen[k] = true
		o, ok := old[k]
		if !ok {
			c := base(x, "added")
			c.ToDate, c.ToSitting, c.ToRooms = str(x.ExamDate), sitName[str(x.SittingID)], roomsText(x)
			out = append(out, c)
			continue
		}
		moved := str(o.ExamDate) != str(x.ExamDate) || sitName[str(o.SittingID)] != sitName[str(x.SittingID)]
		switch {
		case moved:
			c := base(x, "moved")
			c.FromDate, c.FromSitting, c.ToDate, c.ToSitting = str(o.ExamDate), sitName[str(o.SittingID)], str(x.ExamDate), sitName[str(x.SittingID)]
			if roomsText(o) != roomsText(x) {
				c.FromRooms, c.ToRooms = roomsText(o), roomsText(x)
			}
			out = append(out, c)
		case roomsText(o) != roomsText(x):
			c := base(x, "rooms")
			c.ToDate, c.ToSitting, c.FromRooms, c.ToRooms = str(x.ExamDate), sitName[str(x.SittingID)], roomsText(o), roomsText(x)
			out = append(out, c)
		case o.Duration != x.Duration:
			c := base(x, "duration")
			c.ToDate, c.ToSitting, c.FromDuration, c.ToDuration = str(x.ExamDate), sitName[str(x.SittingID)], o.Duration, x.Duration
			out = append(out, c)
		}
	}
	for _, o := range before {
		if !seen[paperKeyOf(o)] {
			c := base(o, "removed")
			c.FromDate, c.FromSitting = str(o.ExamDate), sitName[str(o.SittingID)]
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *GormTermRepository) GetVersion(ctx context.Context, id string) (Version, error) {
	v, _, err := r.load(r.db.WithContext(ctx), id)
	return v, err
}

// window checks a routine's dates against its term and session.
func window(row termRow, from, to string) bool {
	if from == "" || to == "" || to < from || from < row.SessionStart || to > row.SessionEnd {
		return false
	}
	if row.TermStartsOn != nil && from < *row.TermStartsOn {
		return false
	}
	if row.TermEndsOn != nil && to > *row.TermEndsOn {
		return false
	}
	a, _ := time.Parse(time.DateOnly, from)
	b, _ := time.Parse(time.DateOnly, to)
	return b.Sub(a) <= 120*24*time.Hour
}

type termRow struct {
	SessionID                string
	TermStartsOn, TermEndsOn *string
	SessionStart, SessionEnd string
}

func (r *GormTermRepository) CreateVersion(ctx context.Context, actor string, in VersionInput) (Version, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var term termRow
		if err := tx.Raw(`SELECT t.session_id, to_char(t.starts_on,'YYYY-MM-DD') AS term_starts_on, to_char(t.ends_on,'YYYY-MM-DD') AS term_ends_on,
 to_char(s.start_date,'YYYY-MM-DD') AS session_start, to_char(s.end_date,'YYYY-MM-DD') AS session_end
 FROM exam_terms t JOIN academic_sessions s ON s.id = t.session_id WHERE t.id = ? AND t.kind IN ('term','annual') FOR UPDATE OF t`, in.TermID).Scan(&term).Error; err != nil {
			return err
		}
		if term.SessionID == "" {
			return ErrNotFound
		}
		var n int64
		if err := tx.Table("routine_versions").Where("term_id=?", in.TermID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		if in.StartsOn == "" && term.TermStartsOn != nil {
			in.StartsOn = *term.TermStartsOn
		}
		if in.EndsOn == "" && term.TermEndsOn != nil {
			in.EndsOn = *term.TermEndsOn
		}
		if !window(term, in.StartsOn, in.EndsOn) {
			return ErrInvalid
		}
		if err := tx.Raw("INSERT INTO routine_versions(term_id,version,starts_on,ends_on,instructions,created_by) VALUES (?,1,?,?,?,?) RETURNING id", in.TermID, in.StartsOn, in.EndsOn, in.Instructions, actor).Scan(&id).Error; err != nil {
			return dbError(err)
		}
		if err := tx.Exec(`INSERT INTO routine_sittings(version_id,name,name_bn,start_minute,end_minute) VALUES (?,'Morning','সকাল',600,780),(?,'Afternoon','বিকাল',840,1020)`, id, id).Error; err != nil {
			return err
		}
		// The term's papers: every subject of each class that has an exam
		// in the term, or of every active class with sections.
		res := tx.Exec(`INSERT INTO routine_papers(version_id,class_id,subject_id,paper_kind,paper_no,duration_minutes)
 SELECT ?, acs.class_id, acs.subject_id, 'theory', 1, ?
 FROM academic_class_subjects acs JOIN academic_subjects sub ON sub.id = acs.subject_id AND sub.is_active
 JOIN academic_classes c ON c.id = acs.class_id AND c.is_active
 WHERE CASE WHEN EXISTS (SELECT 1 FROM exams e WHERE e.term_id = ?)
            THEN acs.class_id IN (SELECT e.class_id FROM exams e WHERE e.term_id = ?)
            ELSE EXISTS (SELECT 1 FROM academic_sections sec WHERE sec.class_id = acs.class_id AND sec.session_id = ? AND sec.is_active) END`, id, in.Duration, in.TermID, in.TermID, term.SessionID)
		if res.Error != nil {
			return res.Error
		}
		return audit.Record(tx, &actor, "routine.version_created", "routine_version", id, map[string]any{"term_id": in.TermID, "version": 1, "papers": res.RowsAffected, "starts_on": in.StartsOn, "ends_on": in.EndsOn})
	})
	if err != nil {
		return Version{}, err
	}
	return r.GetVersion(ctx, id)
}

func (r *GormTermRepository) UpdateVersion(ctx context.Context, actor, id string, in VersionInput) (Version, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDraft(tx, id); err != nil {
			return err
		}
		var t termRow
		if err := tx.Raw(`SELECT to_char(t.starts_on,'YYYY-MM-DD') AS term_starts_on, to_char(t.ends_on,'YYYY-MM-DD') AS term_ends_on,
 to_char(s.start_date,'YYYY-MM-DD') AS session_start, to_char(s.end_date,'YYYY-MM-DD') AS session_end
 FROM routine_versions v JOIN exam_terms t ON t.id = v.term_id JOIN academic_sessions s ON s.id = t.session_id WHERE v.id = ?`, id).Scan(&t).Error; err != nil {
			return err
		}
		if !window(t, in.StartsOn, in.EndsOn) {
			return ErrInvalid
		}
		if err := tx.Exec("UPDATE routine_versions SET starts_on=?,ends_on=?,instructions=? WHERE id=?", in.StartsOn, in.EndsOn, in.Instructions, id).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "routine.version_updated", "routine_version", id, map[string]any{"starts_on": in.StartsOn, "ends_on": in.EndsOn})
	})
	if err != nil {
		return Version{}, err
	}
	return r.GetVersion(ctx, id)
}

func (r *GormTermRepository) DeleteVersion(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDraft(tx, id); err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM routine_versions WHERE id=?", id).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "routine.version_deleted", "routine_version", id, nil)
	})
}

// change runs one edit of a draft and returns the version.
func (r *GormTermRepository) change(ctx context.Context, versionID string, edit func(tx *gorm.DB) error) (Version, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDraft(tx, versionID); err != nil {
			return err
		}
		return edit(tx)
	})
	if err != nil {
		return Version{}, err
	}
	return r.GetVersion(ctx, versionID)
}

func (r *GormTermRepository) SaveSitting(ctx context.Context, actor, versionID, id string, in SittingInput, start, end int) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		var overlap int64
		if err := tx.Table("routine_sittings").Where("version_id=? AND start_minute<? AND end_minute>? AND id::text IS DISTINCT FROM ?", versionID, end, start, id).Count(&overlap).Error; err != nil {
			return err
		}
		if overlap > 0 {
			return ErrConflict
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO routine_sittings(version_id,name,name_bn,start_minute,end_minute) VALUES (?,?,?,?,?) RETURNING id", versionID, in.Name, in.NameBN, start, end).Scan(&id).Error; err != nil {
				return dbError(err)
			}
		} else {
			res := tx.Exec("UPDATE routine_sittings SET name=?,name_bn=?,start_minute=?,end_minute=? WHERE id=? AND version_id=?", in.Name, in.NameBN, start, end, id, versionID)
			if res.Error != nil {
				return dbError(res.Error)
			}
			if res.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		return audit.Record(tx, &actor, "routine.sitting_saved", "routine_version", versionID, map[string]any{"sitting_id": id, "name": in.Name, "start": in.StartTime, "end": in.EndTime})
	})
}

func (r *GormTermRepository) DeleteSitting(ctx context.Context, actor, versionID, id string) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM routine_sittings WHERE id=? AND version_id=?", id, versionID)
		if res.Error != nil {
			return dbError(res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "routine.sitting_deleted", "routine_version", versionID, map[string]any{"sitting_id": id})
	})
}

func (r *GormTermRepository) AddPaper(ctx context.Context, actor, versionID string, in PaperInput) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		var ok int64
		if err := tx.Table("academic_class_subjects acs").Joins("JOIN academic_subjects sub ON sub.id=acs.subject_id AND sub.is_active").Joins("JOIN academic_classes c ON c.id=acs.class_id AND c.is_active").
			Where("acs.class_id=? AND acs.subject_id=?", in.ClassID, in.SubjectID).Count(&ok).Error; err != nil {
			return err
		}
		if ok == 0 {
			return ErrInvalid
		}
		var id string
		if err := tx.Raw("INSERT INTO routine_papers(version_id,class_id,subject_id,paper_kind,paper_no,duration_minutes,needs_lab) VALUES (?,?,?,?,?,?,?) RETURNING id", versionID, in.ClassID, in.SubjectID, in.Kind, in.PaperNo, in.Duration, in.NeedsLab).Scan(&id).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "routine.paper_added", "routine_version", versionID, map[string]any{"paper_id": id, "class_id": in.ClassID, "subject_id": in.SubjectID, "kind": in.Kind, "paper_no": in.PaperNo})
	})
}

func (r *GormTermRepository) DeletePaper(ctx context.Context, actor, versionID, id string) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM routine_papers WHERE id=? AND version_id=?", id, versionID)
		if res.Error != nil {
			return dbError(res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "routine.paper_deleted", "routine_version", versionID, map[string]any{"paper_id": id})
	})
}

// PlacePaper sets a paper's date, sitting and rooms by hand. A placement
// that breaks a hard rule (a clash, a closed day, a full room …) is
// refused with the rules it breaks.
func (r *GormTermRepository) PlacePaper(ctx context.Context, actor, versionID, id string, in Placement) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		var paper struct{ ClassID, SessionID string }
		if err := tx.Raw("SELECT p.class_id, t.session_id FROM routine_papers p JOIN routine_versions v ON v.id = p.version_id JOIN exam_terms t ON t.id = v.term_id WHERE p.id = ? AND p.version_id = ?", id, versionID).Scan(&paper).Error; err != nil {
			return err
		}
		if paper.ClassID == "" {
			return ErrNotFound
		}
		pinned := in.ExamDate != ""
		if in.Pinned != nil {
			pinned = *in.Pinned && in.ExamDate != ""
		}
		var date, sitting any
		if in.ExamDate != "" {
			var n int64
			if err := tx.Table("routine_sittings").Where("id=? AND version_id=?", in.SittingID, versionID).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return ErrInvalid
			}
			date, sitting = in.ExamDate, in.SittingID
		}
		for _, a := range in.Rooms {
			var n int64
			if err := tx.Table("academic_sections").Where("id=? AND class_id=? AND session_id=?", a.SectionID, paper.ClassID, paper.SessionID).Count(&n).Error; err != nil {
				return err
			}
			var m int64
			if err := tx.Table("exam_rooms").Where("id=? AND is_active", a.RoomID).Count(&m).Error; err != nil {
				return err
			}
			if n == 0 || m == 0 {
				return ErrInvalid
			}
		}
		if err := tx.Exec("UPDATE routine_papers SET duration_minutes=?,needs_lab=?,exam_date=?,sitting_id=?,pinned=? WHERE id=?", in.Duration, in.NeedsLab, date, sitting, pinned, id).Error; err != nil {
			return dbError(err)
		}
		if err := tx.Exec("DELETE FROM routine_paper_rooms WHERE paper_id=?", id).Error; err != nil {
			return err
		}
		for _, a := range in.Rooms {
			if err := tx.Exec("INSERT INTO routine_paper_rooms(paper_id,section_id,room_id) VALUES (?,?,?)", id, a.SectionID, a.RoomID).Error; err != nil {
				return dbError(err)
			}
		}
		_, plan, err := r.load(tx, versionID)
		if err != nil {
			return err
		}
		if issues := planner.Involving(planner.Validate(plan, false), id); len(issues) > 0 {
			return &IssuesError{Code: "routine_conflict", Issues: issues}
		}
		return audit.Record(tx, &actor, "routine.paper_placed", "routine_version", versionID, map[string]any{"paper_id": id, "date": in.ExamDate, "sitting_id": in.SittingID, "rooms": len(in.Rooms), "pinned": pinned})
	})
}

// AddDuty puts a member of staff in a room for a sitting; nobody is in two
// rooms at once.
func (r *GormTermRepository) AddDuty(ctx context.Context, actor, versionID string, in DutyInput) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		var n int64
		if err := tx.Table("routine_sittings").Where("id=? AND version_id=?", in.SittingID, versionID).Count(&n).Error; err != nil || n == 0 {
			if err != nil {
				return err
			}
			return ErrInvalid
		}
		if err := tx.Table("exam_rooms").Where("id=? AND is_active", in.RoomID).Count(&n).Error; err != nil || n == 0 {
			if err != nil {
				return err
			}
			return ErrInvalid
		}
		if err := tx.Table("employees").Where("id=? AND kind='teacher' AND is_active", in.EmployeeID).Count(&n).Error; err != nil || n == 0 {
			if err != nil {
				return err
			}
			return ErrInvalid
		}
		_, plan, err := r.load(tx, versionID)
		if err != nil {
			return err
		}
		plan.Duties = append(plan.Duties, planner.Duty{Date: in.ExamDate, SittingID: in.SittingID, RoomID: in.RoomID, EmployeeID: in.EmployeeID})
		for _, i := range planner.Validate(plan, false) {
			if i.Code == "invigilator_clash" && i.TeacherID == in.EmployeeID && i.Date == in.ExamDate && i.SittingID == in.SittingID {
				return &IssuesError{Code: "routine_conflict", Issues: []planner.Issue{i}}
			}
		}
		var id string
		if err := tx.Raw("INSERT INTO routine_duties(version_id,exam_date,sitting_id,room_id,employee_id) VALUES (?,?,?,?,?) RETURNING id", versionID, in.ExamDate, in.SittingID, in.RoomID, in.EmployeeID).Scan(&id).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &actor, "routine.duty_added", "routine_version", versionID, map[string]any{"duty_id": id, "date": in.ExamDate, "room_id": in.RoomID, "employee_id": in.EmployeeID})
	})
}

func (r *GormTermRepository) DeleteDuty(ctx context.Context, actor, versionID, id string) (Version, error) {
	return r.change(ctx, versionID, func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM routine_duties WHERE id=? AND version_id=?", id, versionID)
		if res.Error != nil {
			return dbError(res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "routine.duty_deleted", "routine_version", versionID, map[string]any{"duty_id": id})
	})
}

// Generate drafts the routine: every paper that is not pinned is placed
// again by the deterministic generator, and rooms in use get invigilators.
func (r *GormTermRepository) Generate(ctx context.Context, actor, versionID string) (GenerateResult, error) {
	var report planner.Report
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDraft(tx, versionID); err != nil {
			return err
		}
		_, plan, err := r.load(tx, versionID)
		if err != nil {
			return err
		}
		out, rep := planner.Generate(plan)
		report = rep
		if err := tx.Exec("DELETE FROM routine_paper_rooms WHERE paper_id IN (SELECT id FROM routine_papers WHERE version_id=?)", versionID).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM routine_duties WHERE version_id=?", versionID).Error; err != nil {
			return err
		}
		for _, x := range out.Papers {
			var date, sitting any
			if x.Placed() {
				date, sitting = x.Date, x.SittingID
			}
			if err := tx.Exec("UPDATE routine_papers SET exam_date=?,sitting_id=?,pinned=? WHERE id=?", date, sitting, x.Pinned, x.ID).Error; err != nil {
				return dbError(err)
			}
			sections := make([]string, 0, len(x.Rooms))
			for s := range x.Rooms {
				sections = append(sections, s)
			}
			sort.Strings(sections)
			for _, s := range sections {
				if err := tx.Exec("INSERT INTO routine_paper_rooms(paper_id,section_id,room_id) VALUES (?,?,?)", x.ID, s, x.Rooms[s]).Error; err != nil {
					return dbError(err)
				}
			}
		}
		for _, d := range out.Duties {
			if err := tx.Exec("INSERT INTO routine_duties(version_id,exam_date,sitting_id,room_id,employee_id) VALUES (?,?,?,?,?)", versionID, d.Date, d.SittingID, d.RoomID, d.EmployeeID).Error; err != nil {
				return dbError(err)
			}
		}
		return audit.Record(tx, &actor, "routine.generated", "routine_version", versionID, map[string]any{"placed": rep.Placed, "unplaced": len(rep.Unplaced), "kept": rep.Kept, "duties": rep.Duties})
	})
	if err != nil {
		return GenerateResult{}, err
	}
	v, err := r.GetVersion(ctx, versionID)
	return GenerateResult{Report: report, Version: v}, err
}

// contentHash is the SHA-256 of a version's canonical content, recorded at
// publication.
func contentHash(v Version) string {
	type room struct{ Section, Room string }
	type paper struct {
		Class, Subject, Kind string
		No, Duration         int
		Lab                  bool
		Date, Sitting        string
		Rooms                []room
	}
	type duty struct{ Date, Sitting, Room, Employee string }
	sit := map[string]string{}
	content := struct {
		From, To, Instructions string
		Sittings               []string
		Papers                 []paper
		Duties                 []duty
	}{From: v.StartsOn, To: v.EndsOn, Instructions: v.Instructions}
	for _, s := range v.Sittings {
		sit[s.ID] = fmt.Sprintf("%s|%s|%d|%d", s.Name, s.NameBN, s.Start, s.End)
		content.Sittings = append(content.Sittings, sit[s.ID])
	}
	for _, x := range v.Papers {
		p := paper{Class: x.ClassID, Subject: x.SubjectID, Kind: x.Kind, No: x.PaperNo, Duration: x.Duration, Lab: x.NeedsLab}
		if x.ExamDate != nil && x.SittingID != nil {
			p.Date, p.Sitting = *x.ExamDate, sit[*x.SittingID]
		}
		for _, rm := range x.Rooms {
			p.Rooms = append(p.Rooms, room{rm.SectionID, rm.RoomID})
		}
		sort.Slice(p.Rooms, func(i, j int) bool { return p.Rooms[i].Section < p.Rooms[j].Section })
		content.Papers = append(content.Papers, p)
	}
	sort.Slice(content.Papers, func(i, j int) bool {
		a, b := content.Papers[i], content.Papers[j]
		return fmt.Sprintf("%s|%s|%s|%d", a.Class, a.Subject, a.Kind, a.No) < fmt.Sprintf("%s|%s|%s|%d", b.Class, b.Subject, b.Kind, b.No)
	})
	for _, d := range v.Duties {
		content.Duties = append(content.Duties, duty{d.ExamDate, sit[d.SittingID], d.RoomID, d.EmployeeID})
	}
	sort.Slice(content.Duties, func(i, j int) bool {
		return fmt.Sprint(content.Duties[i]) < fmt.Sprint(content.Duties[j])
	})
	raw, _ := json.Marshal(content)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Move takes a version through its workflow. Submission, approval and
// publication run every rule; a hard issue blocks them.
func (r *GormTermRepository) Move(ctx context.Context, actor, versionID, to, reason string) (Version, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockVersion(tx, versionID)
		if err != nil {
			return err
		}
		allowed := map[string][]string{"submitted": {"draft"}, "approved": {"submitted"}, "published": {"approved"}, "draft": {"submitted", "approved"}}
		ok := false
		for _, from := range allowed[to] {
			ok = ok || status == from
		}
		if !ok {
			return ErrState
		}
		v, plan, err := r.load(tx, versionID)
		if err != nil {
			return err
		}
		if to != "draft" {
			if hard := planner.Hard(planner.Validate(plan, true)); len(hard) > 0 {
				return &IssuesError{Code: "routine_not_ready", Issues: hard}
			}
		}
		meta := map[string]any{"term_id": v.TermID, "version": v.Version, "from": status}
		switch to {
		case "submitted":
			err = tx.Exec("UPDATE routine_versions SET status='submitted',submitted_by=?,submitted_at=now(),returned_reason=NULL WHERE id=?", actor, versionID).Error
			if err == nil {
				err = audit.Record(tx, &actor, "routine.version_submitted", "routine_version", versionID, meta)
			}
		case "approved":
			err = tx.Exec("UPDATE routine_versions SET status='approved',approved_by=?,approved_at=now() WHERE id=?", actor, versionID).Error
			if err == nil {
				err = audit.Record(tx, &actor, "routine.version_approved", "routine_version", versionID, meta)
			}
		case "draft":
			err = tx.Exec("UPDATE routine_versions SET status='draft',returned_reason=?,submitted_by=NULL,submitted_at=NULL,approved_by=NULL,approved_at=NULL WHERE id=?", reason, versionID).Error
			if err == nil {
				meta["reason"] = reason
				err = audit.Record(tx, &actor, "routine.version_returned", "routine_version", versionID, meta)
			}
		case "published":
			var superseded []string
			if err := tx.Raw("UPDATE routine_versions SET status='superseded',superseded_at=now() WHERE term_id=? AND status='published' RETURNING id", v.TermID).Scan(&superseded).Error; err != nil {
				return dbError(err)
			}
			err = tx.Exec("UPDATE routine_versions SET status='published',published_by=?,published_at=now(),content_sha256=? WHERE id=?", actor, contentHash(v), versionID).Error
			if err == nil {
				meta["superseded"] = superseded
				meta["changes"] = len(v.Changes)
				err = audit.Record(tx, &actor, "routine.version_published", "routine_version", versionID, meta)
			}
		}
		return dbError(err)
	})
	if err != nil {
		return Version{}, err
	}
	return r.GetVersion(ctx, versionID)
}

// Amend starts the next version of a published routine, a copy of it, with
// the reason for the change. The published version stays in force until
// the amendment is published.
func (r *GormTermRepository) Amend(ctx context.Context, actor, versionID, reason string) (Version, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status, err := lockVersion(tx, versionID)
		if err != nil {
			return err
		}
		if status != "published" {
			return ErrState
		}
		var old struct {
			TermID       string
			Version      int
			StartsOn     string
			EndsOn       string
			Instructions string
		}
		if err := tx.Raw("SELECT term_id, version, to_char(starts_on,'YYYY-MM-DD') AS starts_on, to_char(ends_on,'YYYY-MM-DD') AS ends_on, instructions FROM routine_versions WHERE id=?", versionID).Scan(&old).Error; err != nil {
			return err
		}
		if err := tx.Raw(`INSERT INTO routine_versions(term_id,version,based_on,reason,starts_on,ends_on,instructions,created_by)
 SELECT ?, max(version)+1, ?, ?, ?, ?, ?, ? FROM routine_versions WHERE term_id = ? RETURNING id`, old.TermID, versionID, reason, old.StartsOn, old.EndsOn, old.Instructions, actor, old.TermID).Scan(&id).Error; err != nil {
			return dbError(err)
		}
		sittings := map[string]string{}
		rows, err := loadSittings(tx, versionID)
		if err != nil {
			return err
		}
		for _, s := range rows {
			var nid string
			if err := tx.Raw("INSERT INTO routine_sittings(version_id,name,name_bn,start_minute,end_minute) VALUES (?,?,?,?,?) RETURNING id", id, s.Name, s.NameBN, s.Start, s.End).Scan(&nid).Error; err != nil {
				return err
			}
			sittings[s.ID] = nid
		}
		papers, err := loadPapers(tx, versionID)
		if err != nil {
			return err
		}
		for _, x := range papers {
			var date, sitting any
			if x.ExamDate != nil && x.SittingID != nil {
				date, sitting = *x.ExamDate, sittings[*x.SittingID]
			}
			var nid string
			// Every paper of a published routine was placed and kept.
			if err := tx.Raw("INSERT INTO routine_papers(version_id,class_id,subject_id,paper_kind,paper_no,duration_minutes,needs_lab,exam_date,sitting_id,pinned) VALUES (?,?,?,?,?,?,?,?,?,?) RETURNING id",
				id, x.ClassID, x.SubjectID, x.Kind, x.PaperNo, x.Duration, x.NeedsLab, date, sitting, date != nil).Scan(&nid).Error; err != nil {
				return err
			}
			for _, rm := range x.Rooms {
				if err := tx.Exec("INSERT INTO routine_paper_rooms(paper_id,section_id,room_id) VALUES (?,?,?)", nid, rm.SectionID, rm.RoomID).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Exec("INSERT INTO routine_duties(version_id,exam_date,sitting_id,room_id,employee_id) SELECT ?, d.exam_date, ns.id, d.room_id, d.employee_id FROM routine_duties d JOIN routine_sittings os ON os.id = d.sitting_id JOIN routine_sittings ns ON ns.version_id = ? AND ns.name = os.name AND ns.start_minute = os.start_minute WHERE d.version_id = ?", id, id, versionID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "routine.version_amended", "routine_version", id, map[string]any{"term_id": old.TermID, "amends": versionID, "from_version": old.Version, "version": old.Version + 1, "reason": reason})
	})
	if err != nil {
		return Version{}, err
	}
	return r.GetVersion(ctx, id)
}

// ---------------------------------------------------------------------------
// Students and teachers

type studentRow struct {
	ID, SessionID, ClassID, SectionID, ClassName, SectionName string
}

func (r *GormTermRepository) student(tx *gorm.DB, actor string) (studentRow, error) {
	var s studentRow
	err := tx.Raw(`SELECT s.id, s.session_id, s.class_id, s.section_id, c.name AS class_name, sec.name AS section_name
 FROM students s JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id
 WHERE s.user_id = ? AND s.status = 'active' LIMIT 1`, actor).Scan(&s).Error
	if err == nil && s.ID == "" {
		err = ErrNotFound
	}
	return s, err
}

func studentPapers(tx *gorm.DB, s studentRow, versionID string) ([]StudentPaper, error) {
	out := []StudentPaper{}
	err := tx.Raw(`SELECT p.id AS paper_id, to_char(p.exam_date,'YYYY-MM-DD') AS exam_date, `+hhmm("st.start_minute")+` AS start_time, `+hhmm("st.end_minute")+` AS end_time,
 st.name AS sitting_name, st.name_bn AS sitting_name_bn, sub.code AS subject_code, sub.name_en AS subject_name, sub.name_bn AS subject_name_bn,
 p.paper_kind AS kind, p.paper_no, p.duration_minutes AS duration, acs.is_fourth_subject AS optional,
 coalesce(r.name,'') AS room_name, coalesce(r.name_bn,'') AS room_name_bn, p.class_id, p.subject_id, p.sitting_id, coalesce(r.id::text,'') AS room_id
 FROM routine_papers p JOIN routine_sittings st ON st.id = p.sitting_id
 JOIN academic_subjects sub ON sub.id = p.subject_id
 JOIN academic_class_subjects acs ON acs.class_id = p.class_id AND acs.subject_id = p.subject_id
 LEFT JOIN routine_paper_rooms pr ON pr.paper_id = p.id AND pr.section_id = ?
 LEFT JOIN exam_rooms r ON r.id = pr.room_id
 WHERE p.version_id = ? AND p.class_id = ? AND p.exam_date IS NOT NULL
   AND student_takes_subject(?, ?, p.class_id, p.subject_id)
 ORDER BY p.exam_date, st.start_minute, sub.code, p.paper_no`, s.SectionID, versionID, s.ClassID, s.ID, s.SessionID).Scan(&out).Error
	return out, err
}

// StudentRoutines lists the published routines of the student's session
// with the papers the student sits and their own room. Papers an
// amendment changed in the last 7 days are marked.
func (r *GormTermRepository) StudentRoutines(ctx context.Context, actor string) ([]StudentRoutine, string, string, error) {
	db := r.db.WithContext(ctx)
	s, err := r.student(db, actor)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return []StudentRoutine{}, "", "", nil
		}
		return nil, "", "", err
	}
	out := []StudentRoutine{}
	if err := db.Raw(`SELECT v.id AS version_id, v.term_id, t.name AS term_name, v.version, `+ts("v.published_at")+` AS published_at,
 CASE WHEN v.version > 1 THEN `+ts("v.published_at")+` END AS amended_on, v.reason
 FROM routine_versions v JOIN exam_terms t ON t.id = v.term_id
 WHERE v.status = 'published' AND t.session_id = ? ORDER BY t.sequence, t.name, v.id`, s.SessionID).Scan(&out).Error; err != nil {
		return nil, "", "", err
	}
	for i := range out {
		out[i].ClassName, out[i].SectionName = s.ClassName, s.SectionName
		if out[i].Papers, err = studentPapers(db, s, out[i].VersionID); err != nil {
			return nil, "", "", err
		}
		if out[i].AmendedOn == nil {
			continue
		}
		var recent struct {
			BasedOn *string
			Recent  bool
		}
		if err := db.Raw("SELECT based_on, published_at > now() - interval '7 days' AS recent FROM routine_versions WHERE id = ?", out[i].VersionID).Scan(&recent).Error; err != nil {
			return nil, "", "", err
		}
		if !recent.Recent || recent.BasedOn == nil {
			continue
		}
		before, err := studentPapers(db, s, *recent.BasedOn)
		if err != nil {
			return nil, "", "", err
		}
		key := func(p StudentPaper) string { return fmt.Sprintf("%s|%s|%d", p.SubjectID, p.Kind, p.PaperNo) }
		old := map[string]StudentPaper{}
		for _, p := range before {
			old[key(p)] = p
		}
		for j, p := range out[i].Papers {
			o, ok := old[key(p)]
			out[i].Papers[j].Changed = !ok || o.ExamDate != p.ExamDate || o.StartTime != p.StartTime || o.RoomName != p.RoomName
		}
	}
	return out, s.ClassID, s.SectionID, nil
}

// TeacherDuties lists a teacher's invigilation duties in published
// routines.
func (r *GormTermRepository) TeacherDuties(ctx context.Context, actor string) ([]TeacherDuty, error) {
	db := r.db.WithContext(ctx)
	out := []TeacherDuty{}
	if err := db.Raw(`SELECT v.id AS version_id, t.name AS term_name, to_char(d.exam_date,'YYYY-MM-DD') AS exam_date, `+hhmm("st.start_minute")+` AS start_time, `+hhmm("st.end_minute")+` AS end_time,
 st.name AS sitting_name, st.name_bn AS sitting_name_bn, r.name AS room_name, r.name_bn AS room_name_bn, d.room_id, d.sitting_id
 FROM routine_duties d JOIN routine_versions v ON v.id = d.version_id AND v.status = 'published'
 JOIN exam_terms t ON t.id = v.term_id JOIN routine_sittings st ON st.id = d.sitting_id
 JOIN exam_rooms r ON r.id = d.room_id JOIN employees e ON e.id = d.employee_id
 WHERE e.user_id = ? ORDER BY d.exam_date, st.start_minute, r.name`, actor).Scan(&out).Error; err != nil {
		return nil, err
	}
	for i := range out {
		type row struct {
			ClassName, SectionName, SubjectName, SubjectNameBN, Kind string
			PaperNo                                                  int
			Students                                                 int
		}
		rows := []row{}
		if err := db.Raw(`SELECT c.name AS class_name, sec.name AS section_name, sub.name_en AS subject_name, sub.name_bn AS subject_name_bn, p.paper_kind AS kind, p.paper_no,
 (SELECT count(*) FROM students s WHERE s.section_id = sec.id AND s.session_id = sec.session_id AND s.status = 'active' AND student_takes_subject(s.id, s.session_id, s.class_id, p.subject_id)) AS students
 FROM routine_paper_rooms pr JOIN routine_papers p ON p.id = pr.paper_id
 JOIN academic_classes c ON c.id = p.class_id JOIN academic_sections sec ON sec.id = pr.section_id JOIN academic_subjects sub ON sub.id = p.subject_id
 WHERE p.version_id = ? AND p.exam_date = ?::date AND p.sitting_id = ? AND pr.room_id = ?
 ORDER BY c.display_order, c.name, sec.name`, out[i].VersionID, out[i].ExamDate, out[i].SittingID, out[i].RoomID).Scan(&rows).Error; err != nil {
			return nil, err
		}
		out[i].Papers, out[i].PapersBN = []string{}, []string{}
		for _, x := range rows {
			out[i].Papers = append(out[i].Papers, fmt.Sprintf("%s %s · %s", x.ClassName, x.SectionName, documents.RoutinePaperName("en", documents.Name{EN: x.SubjectName, BN: x.SubjectNameBN}, x.Kind, x.PaperNo, false)))
			out[i].PapersBN = append(out[i].PapersBN, fmt.Sprintf("%s %s · %s", x.ClassName, x.SectionName, documents.RoutinePaperName("bn", documents.Name{EN: x.SubjectName, BN: x.SubjectNameBN}, x.Kind, x.PaperNo, false)))
			out[i].Students += x.Students
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Documents

func (r *GormTermRepository) Document(ctx context.Context, versionID string, in DocumentRequest) (documents.RoutineDoc, error) {
	v, _, err := r.load(r.db.WithContext(ctx), versionID)
	if err != nil {
		return documents.RoutineDoc{}, err
	}
	return routineDoc(v, in), nil
}

// StudentDocument is a student's class routine with only their section.
func (r *GormTermRepository) StudentDocument(ctx context.Context, actor, versionID, language string) (documents.RoutineDoc, error) {
	db := r.db.WithContext(ctx)
	s, err := r.student(db, actor)
	if err != nil {
		return documents.RoutineDoc{}, err
	}
	var ok int64
	if err := db.Table("routine_versions v").Joins("JOIN exam_terms t ON t.id=v.term_id").Where("v.id=? AND v.status='published' AND t.session_id=?", versionID, s.SessionID).Count(&ok).Error; err != nil {
		return documents.RoutineDoc{}, err
	}
	if ok == 0 {
		return documents.RoutineDoc{}, ErrNotFound
	}
	v, _, err := r.load(db, versionID)
	if err != nil {
		return documents.RoutineDoc{}, err
	}
	// Only the papers the student sits.
	mine, err := studentPapers(db, s, versionID)
	if err != nil {
		return documents.RoutineDoc{}, err
	}
	keep := map[string]bool{}
	for _, p := range mine {
		keep[p.PaperID] = true
	}
	papers := v.Papers[:0:0]
	for _, x := range v.Papers {
		if keep[x.ID] {
			papers = append(papers, x)
		}
	}
	v.Papers = papers
	return routineDoc(v, DocumentRequest{Kind: "class", Language: language, ClassID: s.ClassID, SectionID: s.SectionID}), nil
}

func routineDoc(v Version, in DocumentRequest) documents.RoutineDoc {
	doc := documents.RoutineDoc{Kind: in.Kind, Language: in.Language, Term: v.TermName, Session: v.SessionName, Version: v.Version, Status: v.Status, Instructions: v.Instructions, From: v.From, To: v.To}
	if v.PublishedAt != nil {
		doc.PublishedAt = *v.PublishedAt
	}
	if v.Reason != nil {
		doc.Reason = *v.Reason
	}
	for _, h := range v.Holidays {
		doc.Holidays = append(doc.Holidays, documents.RoutineHoliday{Date: h.Date, Title: documents.Name{EN: h.TitleEN, BN: h.TitleBN}})
	}
	sit := map[string]Sitting{}
	sitOrder := map[string]int{}
	for i, s := range v.Sittings {
		sit[s.ID], sitOrder[s.ID] = s, i
	}
	rooms := map[string]Room{}
	for _, rm := range v.Rooms {
		rooms[rm.ID] = rm
	}
	roomName := func(id string) documents.Name {
		rm := rooms[id]
		return documents.Name{EN: rm.Name, BN: rm.NameBN}
	}
	placed := []Paper{}
	for _, x := range v.Papers {
		if x.ExamDate != nil && x.SittingID != nil {
			placed = append(placed, x)
		}
	}
	sort.SliceStable(placed, func(i, j int) bool {
		a, b := placed[i], placed[j]
		if *a.ExamDate != *b.ExamDate {
			return *a.ExamDate < *b.ExamDate
		}
		return sitOrder[*a.SittingID] < sitOrder[*b.SittingID]
	})
	paperOf := func(x Paper, sections []PaperRoom) documents.RoutinePaper {
		p := documents.RoutinePaper{Class: x.ClassName, Subject: documents.Name{EN: x.SubjectName, BN: x.SubjectNameBN}, Code: x.SubjectCode, Kind: x.Kind, PaperNo: x.PaperNo, Duration: x.Duration, Optional: x.Optional}
		names := []string{}
		for _, rm := range sections {
			p.Rooms = append(p.Rooms, documents.RoutineRoomUse{Section: rm.SectionName, Room: roomName(rm.RoomID)})
			names = append(names, rm.SectionName)
		}
		p.Sections = strings.Join(names, ", ")
		return p
	}
	row := func(date, sittingID string) documents.RoutineRow {
		s := sit[sittingID]
		return documents.RoutineRow{Date: date, Sitting: documents.Name{EN: s.Name, BN: s.NameBN}, Start: s.Start, End: s.End}
	}
	duties := map[string][]documents.Name{}
	for _, d := range v.Duties {
		k := d.ExamDate + "|" + d.SittingID + "|" + d.RoomID
		duties[k] = append(duties[k], documents.Name{EN: d.EmployeeName, BN: d.EmployeeNameBN})
	}
	switch in.Kind {
	case "class":
		for _, c := range v.Classes {
			if in.ClassID != "" && c.ID != in.ClassID {
				continue
			}
			page := documents.RoutinePage{Title: documents.Name{EN: c.Name, BN: c.Name}}
			if in.SectionID != "" {
				for _, sec := range c.Sections {
					if sec.ID == in.SectionID {
						page.Title.EN += " " + sec.Name
						page.Title.BN += " " + sec.Name
					}
				}
			}
			for _, x := range placed {
				if x.ClassID != c.ID {
					continue
				}
				secs := []PaperRoom{}
				for _, rm := range x.Rooms {
					if in.SectionID == "" || rm.SectionID == in.SectionID {
						secs = append(secs, rm)
					}
				}
				rw := row(*x.ExamDate, *x.SittingID)
				rw.Papers = []documents.RoutinePaper{paperOf(x, secs)}
				page.Rows = append(page.Rows, rw)
			}
			doc.Pages = append(doc.Pages, page)
		}
	case "room", "invigilation":
		type use struct {
			date, sitting, room string
			papers              []documents.RoutinePaper
			students            int
		}
		uses := []*use{}
		at := map[string]*use{}
		for _, x := range placed {
			byRoom := map[string][]PaperRoom{}
			order := []string{}
			for _, rm := range x.Rooms {
				if _, ok := byRoom[rm.RoomID]; !ok {
					order = append(order, rm.RoomID)
				}
				byRoom[rm.RoomID] = append(byRoom[rm.RoomID], rm)
			}
			for _, roomID := range order {
				k := *x.ExamDate + "|" + *x.SittingID + "|" + roomID
				u := at[k]
				if u == nil {
					u = &use{date: *x.ExamDate, sitting: *x.SittingID, room: roomID}
					at[k] = u
					uses = append(uses, u)
				}
				u.papers = append(u.papers, paperOf(x, byRoom[roomID]))
				for _, rm := range byRoom[roomID] {
					u.students += rm.Students
				}
			}
		}
		// Duties in a room that holds no paper still appear on the roster.
		for _, d := range v.Duties {
			k := d.ExamDate + "|" + d.SittingID + "|" + d.RoomID
			if at[k] == nil && in.Kind == "invigilation" {
				u := &use{date: d.ExamDate, sitting: d.SittingID, room: d.RoomID}
				at[k] = u
				uses = append(uses, u)
			}
		}
		sort.SliceStable(uses, func(i, j int) bool {
			a, b := uses[i], uses[j]
			if a.date != b.date {
				return a.date < b.date
			}
			if sitOrder[a.sitting] != sitOrder[b.sitting] {
				return sitOrder[a.sitting] < sitOrder[b.sitting]
			}
			return strings.ToLower(rooms[a.room].Name) < strings.ToLower(rooms[b.room].Name)
		})
		toRow := func(u *use) documents.RoutineRow {
			rw := row(u.date, u.sitting)
			rw.Room, rw.Papers, rw.Students, rw.Invigilators = roomName(u.room), u.papers, u.students, duties[u.date+"|"+u.sitting+"|"+u.room]
			return rw
		}
		if in.Kind == "invigilation" {
			page := documents.RoutinePage{Title: documents.Name{EN: "All rooms", BN: "সব কক্ষ"}}
			for _, u := range uses {
				page.Rows = append(page.Rows, toRow(u))
			}
			doc.Pages = append(doc.Pages, page)
			break
		}
		for _, rm := range v.Rooms {
			if in.RoomID != "" && rm.ID != in.RoomID {
				continue
			}
			page := documents.RoutinePage{Title: documents.Name{EN: rm.Name, BN: firstNonEmpty(rm.NameBN, rm.Name)}, Note: documents.RoomNote(rm.Kind, rm.Capacity)}
			for _, u := range uses {
				if u.room == rm.ID {
					page.Rows = append(page.Rows, toRow(u))
				}
			}
			if len(page.Rows) == 0 && in.RoomID == "" {
				continue
			}
			doc.Pages = append(doc.Pages, page)
		}
	}
	if len(doc.Pages) == 0 {
		doc.Pages = []documents.RoutinePage{{Title: documents.Name{EN: v.TermName, BN: v.TermName}}}
	}
	return doc
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
