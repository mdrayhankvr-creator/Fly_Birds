package analytics

import (
	"context"
	"sort"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

type Service struct {
	repo *Repository
	now  func() time.Time
}

func NewService(repo *Repository) *Service { return &Service{repo: repo, now: time.Now} }

// SetClock replaces the clock; tests use it to pin "today".
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func recordAudit(tx *gorm.DB, actor, action, targetType, targetID string, meta map[string]any) error {
	return audit.Record(tx, &actor, action, targetType, targetID, meta)
}

func (s *Service) today() time.Time {
	n := s.now().In(Dhaka)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func day(t time.Time) string { return t.Format(time.DateOnly) }

// resolve validates a filter and fills the default session (the active one,
// else the most recent) and, when days > 0, the date range.
func (s *Service) resolve(ctx context.Context, scope Scope, f Filter, days int) (Filter, *Session, error) {
	if !validID(f.SessionID) || !validID(f.ClassID) || !validID(f.SectionID) || !validID(f.ExamID) || !examTypes[f.ExamType] {
		return f, nil, ErrInvalid
	}
	sessions, err := s.repo.Sessions(ctx)
	if err != nil {
		return f, nil, err
	}
	var session *Session
	for i := range sessions {
		if (f.SessionID == "" && i == 0) || sessions[i].ID == f.SessionID {
			session = &sessions[i]
			break
		}
	}
	if session == nil {
		if f.SessionID != "" {
			return f, nil, ErrNotFound
		}
		return f, nil, nil
	}
	f.SessionID = session.ID
	if f.SectionID != "" {
		class, err := s.repo.SectionClass(ctx, f.SessionID, f.SectionID)
		if err != nil {
			return f, nil, err
		}
		if f.ClassID != "" && f.ClassID != class {
			return f, nil, ErrInvalid
		}
		f.ClassID = class
	}
	if scope.Teacher && f.ClassID != "" {
		ok, err := s.repo.TeacherHasAssignment(ctx, scope.UserID, f.SessionID, f.ClassID)
		if err != nil {
			return f, nil, err
		}
		if !ok {
			return f, nil, ErrForbidden
		}
	}
	if days > 0 {
		from, to, err := Range(f.From, f.To, s.today(), days)
		if err != nil {
			return f, nil, err
		}
		// A default window never reaches back before the session began.
		if start, perr := time.Parse(time.DateOnly, session.StartDate); f.From == "" && perr == nil && start.After(from) && !start.After(to) {
			from = start
		}
		if to.After(s.today()) {
			return f, nil, ErrInvalid
		}
		f.From, f.To = day(from), day(to)
		g, err := Grouping(f.Group, from, to)
		if err != nil {
			return f, nil, err
		}
		f.Group = g
	} else {
		f.From, f.To, f.Group = "", "", ""
	}
	return f, session, nil
}

func (s *Service) Options(ctx context.Context, scope Scope, f Filter, academic bool) (Options, error) {
	f, session, err := s.resolve(ctx, scope, Filter{SessionID: f.SessionID}, 0)
	if err != nil {
		return Options{}, err
	}
	out := Options{Classes: []Class{}, Sections: []Section{}, Exams: []Exam{}, Session: session, Today: day(s.today())}
	if out.Sessions, err = s.repo.Sessions(ctx); err != nil {
		return out, err
	}
	if out.Modules, err = s.repo.Modules(ctx); err != nil {
		return out, err
	}
	if out.Settings, err = s.repo.Settings(ctx); err != nil {
		return out, err
	}
	if session == nil {
		return out, nil
	}
	classes, sections, exams, err := s.repo.Options(ctx, scope, session.ID)
	if err != nil {
		return out, err
	}
	out.Classes, out.Sections = classes, sections
	if academic {
		out.Exams = exams
	}
	return out, nil
}

func (s *Service) Attendance(ctx context.Context, scope Scope, f Filter) (Attendance, error) {
	f, session, err := s.resolve(ctx, scope, f, 30)
	if err != nil || session == nil {
		return Attendance{Filter: f, Trend: []TrendPoint{}, Groups: []GroupCounts{}}, err
	}
	return s.repo.Attendance(ctx, scope, f, f.Group)
}

func (s *Service) Exams(ctx context.Context, scope Scope, f Filter) (Exams, error) {
	f, session, err := s.resolve(ctx, scope, f, 0)
	empty := Exams{Filter: f, Exams: []ExamRow{}, Distribution: []GradeCount{}, Subjects: []SubjectRow{}, Groups: []GroupResult{}}
	if err != nil || session == nil {
		return empty, err
	}
	bands, err := s.repo.Bands(ctx)
	if err != nil {
		return empty, err
	}
	return s.repo.Exams(ctx, scope, f, bands)
}

func (s *Service) Fees(ctx context.Context, f Filter) (Fees, error) {
	// By default: the current month and the eleven before it.
	t := s.today()
	days := int(t.Sub(time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)).Hours()/24) + 1
	f.SectionID, f.ExamID, f.ExamType = "", "", ""
	f, session, err := s.resolve(ctx, Scope{}, f, days)
	if err != nil || session == nil {
		return Fees{Filter: f, Months: []FeeMonth{}, Methods: []MethodRow{}, Classes: []ClassFees{}, Aging: []AgingBucket{}, TopOverdue: []Overdue{}, Ledger: []CategoryAmount{}}, err
	}
	return s.repo.Fees(ctx, f, day(s.today()))
}

func (s *Service) Sessions(ctx context.Context, finance bool) ([]SessionRow, error) {
	return s.repo.SessionComparison(ctx, finance)
}

func (s *Service) Settings(ctx context.Context) (Settings, error) { return s.repo.Settings(ctx) }

func (s *Service) SaveSettings(ctx context.Context, actor string, in Settings) (Settings, error) {
	if err := in.Validate(); err != nil {
		return Settings{}, err
	}
	if err := s.repo.SaveSettings(ctx, actor, in); err != nil {
		return Settings{}, err
	}
	return s.repo.Settings(ctx)
}

// risks classifies every active student in scope, dropout alerts first and
// then the lowest attendance.
func (s *Service) risks(ctx context.Context, scope Scope, f Filter, student string) ([]Risk, RiskSummary, Settings, error) {
	summary := RiskSummary{Reasons: map[string]int64{}}
	set, err := s.repo.Settings(ctx)
	if err != nil {
		return nil, summary, set, err
	}
	modules, err := s.repo.Modules(ctx)
	if err != nil {
		return nil, summary, set, err
	}
	t := s.today()
	istart := day(t.AddDate(0, 0, -(set.InactivityDays - 1)))
	signals, err := s.repo.Signals(ctx, scope, f, student, day(t), day(t.AddDate(0, 0, -(set.AttendanceWindow-1))), istart, day(t.AddDate(0, 0, -119)))
	if err != nil {
		return nil, summary, set, err
	}
	summary.Students = int64(len(signals))
	type ranked struct {
		Risk
		order int
		index int
	}
	list := []ranked{}
	for i, sig := range signals {
		r := Classify(sig, set, modules["attendance"], modules["examinations"], istart)
		if r == nil {
			continue
		}
		if r.Level == "dropout" {
			summary.Dropout++
		} else {
			summary.AtRisk++
		}
		for _, reason := range r.Reasons {
			summary.Reasons[reason]++
		}
		list = append(list, ranked{*r, sig.ClassOrder, i})
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.Level == "dropout") != (b.Level == "dropout") {
			return a.Level == "dropout"
		}
		if (a.AttendanceRate == nil) != (b.AttendanceRate == nil) {
			return b.AttendanceRate == nil
		}
		if a.AttendanceRate != nil && *a.AttendanceRate != *b.AttendanceRate {
			return *a.AttendanceRate < *b.AttendanceRate
		}
		return a.index < b.index
	})
	out := make([]Risk, len(list))
	for i, r := range list {
		out[i] = r.Risk
	}
	return out, summary, set, nil
}

var riskReasons = map[string]bool{"": true, "low_attendance": true, "low_gpa": true, "failed_exam": true, "consecutive_absences": true, "no_recent_presence": true}

// Risks returns one page of alerts, optionally narrowed to a level or reason.
func (s *Service) Risks(ctx context.Context, scope Scope, f Filter, level, reason string, page, size int) (RiskPage, error) {
	out := RiskPage{Items: []Risk{}, Summary: RiskSummary{Reasons: map[string]int64{}}, Page: page, PageSize: size}
	if (level != "" && level != "at_risk" && level != "dropout") || !riskReasons[reason] || page < 1 || page > 10000 || size < 1 || size > MaxPageSize {
		return out, ErrInvalid
	}
	f.ExamID, f.ExamType = "", ""
	f, session, err := s.resolve(ctx, scope, f, 0)
	out.Filter = f
	if err != nil {
		return out, err
	}
	if out.Settings, err = s.repo.Settings(ctx); err != nil || session == nil {
		return out, err
	}
	all, summary, set, err := s.risks(ctx, scope, f, "")
	if err != nil {
		return out, err
	}
	out.Summary, out.Settings = summary, set
	filtered := []Risk{}
	for _, r := range all {
		if level != "" && r.Level != level {
			continue
		}
		if reason != "" && !contains(r.Reasons, reason) {
			continue
		}
		filtered = append(filtered, r)
	}
	out.Total = int64(len(filtered))
	start := (page - 1) * size
	if start < len(filtered) {
		end := min(start+size, len(filtered))
		out.Items = filtered[start:end]
	}
	return out, nil
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// Overview is the school-wide headline: population, today's attendance,
// the last 30 days, results, alerts and (for finance viewers) fees.
func (s *Service) Overview(ctx context.Context, scope Scope, sessionID string) (Overview, error) {
	f, session, err := s.resolve(ctx, scope, Filter{SessionID: sessionID}, 0)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{StudentsByState: map[string]int64{}, ByClass: []ClassCount{}, Today: day(s.today())}
	if session == nil {
		return out, nil
	}
	if out, err = s.repo.Population(ctx, session.ID); err != nil {
		return out, err
	}
	out.Session, out.Today = session, day(s.today())
	modules, err := s.repo.Modules(ctx)
	if err != nil {
		return out, err
	}
	if modules["attendance"] {
		st, tc, err := s.repo.Day(ctx, session.ID, out.Today)
		if err != nil {
			return out, err
		}
		out.TodayStudents, out.TodayTeachers = &st, &tc
		att, err := s.Attendance(ctx, scope, Filter{SessionID: session.ID})
		if err != nil {
			return out, err
		}
		out.Last30 = &att.Summary
	}
	if modules["examinations"] {
		ex, err := s.Exams(ctx, scope, Filter{SessionID: session.ID})
		if err != nil {
			return out, err
		}
		out.Results = &ex.Summary
	}
	_, summary, _, err := s.risks(ctx, scope, f, "")
	if err != nil {
		return out, err
	}
	out.Risks = &summary
	if scope.Finance && modules["fees"] {
		fees, err := s.Fees(ctx, Filter{SessionID: session.ID})
		if err != nil {
			return out, err
		}
		out.Fees = &fees.Summary
	}
	return out, nil
}

// StudentAccess says how a viewer reaches a student summary.
type StudentAccess struct {
	Kind   string // self, guardian, school, teacher
	UserID string
	Fees   bool // viewer may see the student's fee balance
}

// StudentSummary returns one student's attendance, results, fees and alert.
// Access is checked here: a student sees only their own record, a guardian
// only linked children, a teacher only students of assigned classes.
func (s *Service) StudentSummary(ctx context.Context, access StudentAccess, studentID string) (StudentSummary, error) {
	var err error
	switch access.Kind {
	case "self":
		if studentID, err = s.repo.StudentForUser(ctx, access.UserID); err != nil {
			return StudentSummary{}, err
		}
	case "guardian":
		if !validID(studentID) || studentID == "" {
			return StudentSummary{}, ErrInvalid
		}
		ids, err := s.repo.Children(ctx, access.UserID)
		if err != nil {
			return StudentSummary{}, err
		}
		if !contains(ids, studentID) {
			return StudentSummary{}, ErrNotFound
		}
	case "teacher":
		if !validID(studentID) || studentID == "" {
			return StudentSummary{}, ErrInvalid
		}
		ok, err := s.repo.TeacherTeachesStudent(ctx, access.UserID, studentID)
		if err != nil {
			return StudentSummary{}, err
		}
		if !ok {
			return StudentSummary{}, ErrNotFound
		}
	case "school":
		if !validID(studentID) || studentID == "" {
			return StudentSummary{}, ErrInvalid
		}
	default:
		return StudentSummary{}, ErrForbidden
	}
	student, err := s.repo.Student(ctx, studentID)
	if err != nil {
		return StudentSummary{}, err
	}
	t := s.today()
	out := StudentSummary{Student: student, Today: day(t), Results: []StudentResult{}, LatestSubjects: []StudentSubject{}}
	modules, err := s.repo.Modules(ctx)
	if err != nil {
		return out, err
	}
	if modules["attendance"] {
		att, err := s.repo.StudentAttendance(ctx, student, day(t), day(t.AddDate(0, 0, -29)))
		if err != nil {
			return out, err
		}
		out.Attendance = &att
	}
	if modules["examinations"] {
		bands, err := s.repo.Bands(ctx)
		if err != nil {
			return out, err
		}
		if out.Results, out.LatestSubjects, err = s.repo.StudentResults(ctx, student, bands); err != nil {
			return out, err
		}
	}
	if access.Fees && modules["fees"] {
		fees, err := s.repo.StudentFees(ctx, student, day(t))
		if err != nil {
			return out, err
		}
		out.Fees = &fees
	}
	if student.Status == "active" {
		list, _, set, err := s.risks(ctx, Scope{}, Filter{SessionID: student.SessionID}, student.ID)
		if err != nil {
			return out, err
		}
		out.Settings = set
		if len(list) > 0 {
			out.Alert = &list[0]
		}
	} else if out.Settings, err = s.repo.Settings(ctx); err != nil {
		return out, err
	}
	return out, nil
}

// Children summarises every child linked to a guardian account.
func (s *Service) Children(ctx context.Context, user string, fees bool) ([]StudentSummary, error) {
	ids, err := s.repo.Children(ctx, user)
	if err != nil {
		return nil, err
	}
	out := []StudentSummary{}
	for _, id := range ids {
		summary, err := s.StudentSummary(ctx, StudentAccess{Kind: "guardian", UserID: user, Fees: fees}, id)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}
