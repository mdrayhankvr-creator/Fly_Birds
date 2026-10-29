package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// query collects named arguments and AND-ed conditions for one statement.
type query struct {
	conds []string
	args  map[string]any
}

var namedParam = regexp.MustCompile(`@([A-Za-z_][A-Za-z0-9_]*)`)

// raw runs a statement written with @name placeholders. GORM's own named
// binding gives up on statements containing PostgreSQL :: casts, so names are
// rewritten to positional parameters here; an unknown name is an error.
func raw(db *gorm.DB, statement string, args map[string]any) *gorm.DB {
	values := []any{}
	var missing string
	sql := namedParam.ReplaceAllStringFunc(statement, func(m string) string {
		v, ok := args[m[1:]]
		if !ok {
			missing = m
		}
		values = append(values, v)
		return "?"
	})
	if missing != "" {
		_ = db.AddError(fmt.Errorf("analytics query: no value for %s", missing))
	}
	return db.Raw(sql, values...)
}

func newQuery(args map[string]any) *query { return &query{args: args} }

func (q *query) where(cond string) *query { q.conds = append(q.conds, cond); return q }
func (q *query) sql() string {
	if len(q.conds) == 0 {
		return "TRUE"
	}
	return strings.Join(q.conds, " AND ")
}

// assigned limits a class column (and optionally a subject column) to the
// teacher's own assignments in the selected session.
func assigned(classCol, subjectCol string) string {
	cond := "EXISTS (SELECT 1 FROM teacher_assignments ta JOIN employees em ON em.id = ta.employee_id WHERE em.user_id = @actor AND em.is_active AND ta.session_id = @session AND ta.class_id = " + classCol
	if subjectCol != "" {
		cond += " AND ta.subject_id = " + subjectCol
	}
	return cond + ")"
}

// contextConds applies the session, class, section and teacher filters to a
// table carrying session_id, class_id and section_id columns.
func contextConds(q *query, alias string, scope Scope, f Filter) *query {
	q.args["session"] = f.SessionID
	q.where(alias + ".session_id = @session")
	if f.ClassID != "" {
		q.args["class"] = f.ClassID
		q.where(alias + ".class_id = @class")
	}
	if f.SectionID != "" {
		q.args["section"] = f.SectionID
		q.where(alias + ".section_id = @section")
	}
	if scope.Teacher {
		q.args["actor"] = scope.UserID
		q.where(assigned(alias+".class_id", ""))
	}
	return q
}

func bucket(group, col string) string {
	switch group {
	case "week":
		return "to_char(date_trunc('week', " + col + ")::date, 'YYYY-MM-DD')"
	case "month":
		return "to_char(date_trunc('month', " + col + ")::date, 'YYYY-MM-DD')"
	}
	return "to_char(" + col + ", 'YYYY-MM-DD')"
}

// enrolledStudents is each student once per enrolment, placed where they
// were that session (class, section, roll and status), so a session's
// reports keep students who have since been promoted or moved.
const enrolledStudents = `(SELECT st.id, st.admission_no, st.name_en, st.name_bn, en.student_status AS status,
 en.session_id, en.class_id, en.section_id, en.roll_no FROM students st JOIN student_enrolments en ON en.student_id = st.id)`

const countCols = `count(*) AS marked, count(*) FILTER (WHERE a.status = 'present') AS present,
count(*) FILTER (WHERE a.status = 'late') AS late, count(*) FILTER (WHERE a.status = 'absent') AS absent`

const teacherCols = `count(*) AS marked, count(*) FILTER (WHERE t.status = 'present') AS present,
count(*) FILTER (WHERE t.status = 'absent') AS absent, count(*) FILTER (WHERE t.status = 'leave') AS leave`

func (r *Repository) Modules(ctx context.Context) (map[string]bool, error) {
	var rows []struct {
		ModuleKey string
		Enabled   bool
	}
	if err := r.db.WithContext(ctx).Raw("SELECT module_key, enabled FROM school_modules WHERE school_id = 1").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range rows {
		out[m.ModuleKey] = m.Enabled
	}
	return out, nil
}

func (r *Repository) Sessions(ctx context.Context) ([]Session, error) {
	out := []Session{}
	err := r.db.WithContext(ctx).Raw(`SELECT id, name, to_char(start_date, 'YYYY-MM-DD') AS start_date, to_char(end_date, 'YYYY-MM-DD') AS end_date, is_active
FROM academic_sessions ORDER BY is_active DESC, start_date DESC, name LIMIT 50`).Scan(&out).Error
	return out, err
}

// TeacherHasAssignment reports whether the viewer teaches the class (or, with
// an empty class, any class) in the session.
func (r *Repository) TeacherHasAssignment(ctx context.Context, actor, session, class string) (bool, error) {
	q := newQuery(map[string]any{"actor": actor, "session": session}).where(`em.user_id = @actor AND em.is_active AND ta.session_id = @session`)
	if class != "" {
		q.args["class"] = class
		q.where("ta.class_id = @class")
	}
	var ok bool
	err := raw(r.db.WithContext(ctx), "SELECT EXISTS (SELECT 1 FROM teacher_assignments ta JOIN employees em ON em.id = ta.employee_id WHERE "+q.sql()+")", q.args).Row().Scan(&ok)
	return ok, err
}

// SectionClass returns the class of a section in the session.
func (r *Repository) SectionClass(ctx context.Context, session, section string) (string, error) {
	var class string
	err := r.db.WithContext(ctx).Raw("SELECT class_id FROM academic_sections WHERE id = ? AND session_id = ?", section, session).Row().Scan(&class)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return class, err
}

func (r *Repository) Options(ctx context.Context, scope Scope, session string) ([]Class, []Section, []Exam, error) {
	db := r.db.WithContext(ctx)
	args := map[string]any{"session": session, "actor": scope.UserID}
	classCond := "EXISTS (SELECT 1 FROM academic_sections s WHERE s.class_id = c.id AND s.session_id = @session)"
	if scope.Teacher {
		classCond = assigned("c.id", "")
	}
	classes, sections, exams := []Class{}, []Section{}, []Exam{}
	if err := raw(db, "SELECT c.id, c.name, c.display_order AS \"order\" FROM academic_classes c WHERE "+classCond+" ORDER BY c.display_order, c.name", args).Scan(&classes).Error; err != nil {
		return nil, nil, nil, err
	}
	sectionCond := "s.session_id = @session"
	if scope.Teacher {
		sectionCond += " AND " + assigned("s.class_id", "")
	}
	if err := raw(db, "SELECT s.id, s.class_id, s.name FROM academic_sections s JOIN academic_classes c ON c.id = s.class_id WHERE "+sectionCond+" ORDER BY c.display_order, c.name, s.display_order, s.name", args).Scan(&sections).Error; err != nil {
		return nil, nil, nil, err
	}
	examCond := "e.session_id = @session AND e.status IN ('published', 'locked')"
	if scope.Teacher {
		examCond += " AND " + assigned("e.class_id", "")
	}
	if err := raw(db, "SELECT e.id, e.name, e.exam_type, e.class_id, e.section_id FROM exams e WHERE "+examCond+" ORDER BY e.published_at DESC NULLS LAST, e.name LIMIT 200", args).Scan(&exams).Error; err != nil {
		return nil, nil, nil, err
	}
	return classes, sections, exams, nil
}

func (r *Repository) Settings(ctx context.Context) (Settings, error) {
	var s Settings
	err := r.db.WithContext(ctx).Raw(`SELECT attendance_threshold::float8 AS attendance_threshold, attendance_window_days AS attendance_window, min_marked_days,
gpa_threshold::float8 AS gpa_threshold, consecutive_absences, inactivity_days, to_char(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
FROM analytics_settings WHERE id = 1`).Scan(&s).Error
	if err == nil && s.AttendanceWindow == 0 {
		return DefaultSettings(), nil
	}
	return s, err
}

func (r *Repository) SaveSettings(ctx context.Context, actor string, s Settings) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`INSERT INTO analytics_settings (id, attendance_threshold, attendance_window_days, min_marked_days, gpa_threshold, consecutive_absences, inactivity_days, updated_by, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, now())
ON CONFLICT (id) DO UPDATE SET attendance_threshold = EXCLUDED.attendance_threshold, attendance_window_days = EXCLUDED.attendance_window_days,
  min_marked_days = EXCLUDED.min_marked_days, gpa_threshold = EXCLUDED.gpa_threshold, consecutive_absences = EXCLUDED.consecutive_absences,
  inactivity_days = EXCLUDED.inactivity_days, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			s.AttendanceThreshold, s.AttendanceWindow, s.MinMarkedDays, s.GPAThreshold, s.ConsecutiveAbsences, s.InactivityDays, actor)
		if res.Error != nil {
			return res.Error
		}
		return recordAudit(tx, actor, "analytics.settings_updated", "analytics_settings", "1", map[string]any{
			"attendance_threshold": s.AttendanceThreshold, "attendance_window_days": s.AttendanceWindow, "min_marked_days": s.MinMarkedDays,
			"gpa_threshold": s.GPAThreshold, "consecutive_absences": s.ConsecutiveAbsences, "inactivity_days": s.InactivityDays})
	})
}

type countRow struct {
	Period  string
	ID      string
	Name    string
	Marked  int64
	Present int64
	Late    int64
	Absent  int64
	Leave   int64
}

func (c countRow) counts() Counts {
	out := Counts{Marked: c.Marked, Present: c.Present, Late: c.Late, Absent: c.Absent}
	out.finish()
	return out
}
func (c countRow) teacher() TeacherCounts {
	out := TeacherCounts{Marked: c.Marked, Present: c.Present, Absent: c.Absent, Leave: c.Leave}
	out.finish()
	return out
}

// Attendance aggregates student attendance (and, for school scope, teacher
// attendance) over from..to.
func (r *Repository) Attendance(ctx context.Context, scope Scope, f Filter, group string) (Attendance, error) {
	db := r.db.WithContext(ctx)
	out := Attendance{Filter: f, Trend: []TrendPoint{}, Groups: []GroupCounts{}}
	q := contextConds(newQuery(map[string]any{"from": f.From, "to": f.To}), "a", scope, f).where("a.attendance_date BETWEEN @from::date AND @to::date")
	var total struct {
		C        countRow `gorm:"embedded"`
		Days     int64
		Students int64
	}
	if err := raw(db, "SELECT "+countCols+", count(DISTINCT a.attendance_date) AS days, count(DISTINCT a.student_id) AS students FROM student_attendance a WHERE "+q.sql(), q.args).Scan(&total).Error; err != nil {
		return out, err
	}
	out.Summary, out.Days, out.Students = total.C.counts(), total.Days, total.Students
	var trend []countRow
	if err := raw(db, "SELECT "+bucket(group, "a.attendance_date")+" AS period, "+countCols+" FROM student_attendance a WHERE "+q.sql()+" GROUP BY 1 ORDER BY 1", q.args).Scan(&trend).Error; err != nil {
		return out, err
	}
	for _, p := range trend {
		out.Trend = append(out.Trend, TrendPoint{Period: p.Period, Counts: p.counts()})
	}
	var groups []countRow
	var err error
	if f.ClassID == "" {
		out.GroupBy = "class"
		err = raw(db, "SELECT c.id, c.name, "+countCols+" FROM student_attendance a JOIN academic_classes c ON c.id = a.class_id WHERE "+q.sql()+" GROUP BY c.id, c.name, c.display_order ORDER BY c.display_order, c.name", q.args).Scan(&groups).Error
	} else {
		out.GroupBy = "section"
		err = raw(db, "SELECT sec.id, sec.name, "+countCols+" FROM student_attendance a JOIN academic_sections sec ON sec.id = a.section_id WHERE "+q.sql()+" GROUP BY sec.id, sec.name, sec.display_order ORDER BY sec.display_order, sec.name", q.args).Scan(&groups).Error
	}
	if err != nil {
		return out, err
	}
	for _, g := range groups {
		out.Groups = append(out.Groups, GroupCounts{ID: g.ID, Name: g.Name, Counts: g.counts()})
	}
	if scope.Teacher {
		return out, nil
	}
	tq := newQuery(map[string]any{"session": f.SessionID, "from": f.From, "to": f.To}).where("t.session_id = @session AND t.attendance_date BETWEEN @from::date AND @to::date")
	var teachers countRow
	if err := raw(db, "SELECT "+teacherCols+" FROM teacher_attendance t WHERE "+tq.sql(), tq.args).Scan(&teachers).Error; err != nil {
		return out, err
	}
	tc := teachers.teacher()
	out.Teachers = &tc
	var tt []countRow
	if err := raw(db, "SELECT "+bucket(group, "t.attendance_date")+" AS period, "+teacherCols+" FROM teacher_attendance t WHERE "+tq.sql()+" GROUP BY 1 ORDER BY 1", tq.args).Scan(&tt).Error; err != nil {
		return out, err
	}
	out.TeacherTrend = []TeacherTrendPoint{}
	for _, p := range tt {
		out.TeacherTrend = append(out.TeacherTrend, TeacherTrendPoint{Period: p.Period, TeacherCounts: p.teacher()})
	}
	return out, nil
}

func (r *Repository) Bands(ctx context.Context) ([]Band, error) {
	var bands []Band
	err := r.db.WithContext(ctx).Raw(`SELECT b.grade, b.gpa::float8 AS gpa FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1
ORDER BY b.gpa DESC, b.min_marks DESC`).Scan(&bands).Error
	return bands, err
}

func examConds(q *query, scope Scope, f Filter) *query {
	contextConds(q, "e", scope, f).where("e.status IN ('published', 'locked')")
	if f.ExamID != "" {
		q.args["exam"] = f.ExamID
		q.where("e.id = @exam")
	}
	if f.ExamType != "" {
		q.args["exam_type"] = f.ExamType
		q.where("e.exam_type = @exam_type")
	}
	return q
}

type resultRow struct {
	ID      string
	Name    string
	Results int64
	Passed  int64
	AvgGPA  *float64
	GPA5    int64
}

// Exams summarises published and locked examination results.
func (r *Repository) Exams(ctx context.Context, scope Scope, f Filter, bands []Band) (Exams, error) {
	db := r.db.WithContext(ctx)
	out := Exams{Filter: f, Exams: []ExamRow{}, Subjects: []SubjectRow{}, Groups: []GroupResult{}}
	q := examConds(newQuery(map[string]any{}), scope, f)
	var sum resultRow
	if err := raw(db, "SELECT count(*) AS results, count(*) FILTER (WHERE r.passed) AS passed, avg(r.gpa)::float8 AS avg_gpa, count(*) FILTER (WHERE r.passed AND r.gpa >= 5) AS gpa5 FROM exam_results r JOIN exams e ON e.id = r.exam_id WHERE "+q.sql(), q.args).Scan(&sum).Error; err != nil {
		return out, err
	}
	out.Summary = ResultSummary{Results: sum.Results, Passed: sum.Passed, Failed: sum.Results - sum.Passed, PassRate: percent(float64(sum.Passed), float64(sum.Results)), AvgGPA: ptr2(sum.AvgGPA), GoldenA: sum.GPA5}
	var exams []struct {
		ExamID, Name, ExamType, ClassName, SectionName, PublishedOn string
		Results, Passed                                             int64
		AvgGPA                                                      *float64
	}
	if err := raw(db, `SELECT e.id AS exam_id, e.name, e.exam_type, c.name AS class_name, sec.name AS section_name,
COALESCE(to_char(e.published_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD'), '') AS published_on,
count(*) AS results, count(*) FILTER (WHERE r.passed) AS passed, avg(r.gpa)::float8 AS avg_gpa
FROM exams e JOIN exam_results r ON r.exam_id = e.id JOIN academic_classes c ON c.id = e.class_id JOIN academic_sections sec ON sec.id = e.section_id
WHERE `+q.sql()+` GROUP BY e.id, c.name, sec.name ORDER BY e.published_at DESC NULLS LAST, e.name LIMIT 60`, q.args).Scan(&exams).Error; err != nil {
		return out, err
	}
	for _, e := range exams {
		out.Exams = append(out.Exams, ExamRow{ExamID: e.ExamID, Name: e.Name, ExamType: e.ExamType, ClassName: e.ClassName, SectionName: e.SectionName,
			PublishedOn: e.PublishedOn, Results: e.Results, Passed: e.Passed, PassRate: percent(float64(e.Passed), float64(e.Results)), AvgGPA: ptr2(e.AvgGPA)})
	}
	var gpas []gpaCount
	if err := raw(db, "SELECT r.gpa::float8 AS gpa, r.passed, count(*) AS count FROM exam_results r JOIN exams e ON e.id = r.exam_id WHERE "+q.sql()+" GROUP BY r.gpa, r.passed", q.args).Scan(&gpas).Error; err != nil {
		return out, err
	}
	out.Distribution = Distribution(bands, gpas)
	sq := examConds(newQuery(map[string]any{}), scope, f).where("m.passed IS NOT NULL")
	if scope.Teacher {
		sq.where(assigned("e.class_id", "es.subject_id"))
	}
	var subjects []struct {
		SubjectID, NameEN, NameBN, Code string
		Entries, Passed                 int64
		AvgPercent                      *float64
	}
	if err := raw(db, `SELECT sub.id AS subject_id, sub.name_en, sub.name_bn, sub.code, count(*) AS entries, count(*) FILTER (WHERE m.passed) AS passed,
avg(coalesce(m.marks, 0) * 100 / es.full_marks)::float8 AS avg_percent
FROM exam_marks m JOIN exams e ON e.id = m.exam_id JOIN exam_subjects es ON es.id = m.exam_subject_id JOIN academic_subjects sub ON sub.id = es.subject_id
WHERE `+sq.sql()+` AND m.status NOT IN ('exempt', 'withheld') GROUP BY sub.id ORDER BY sub.code, sub.name_en`, sq.args).Scan(&subjects).Error; err != nil {
		return out, err
	}
	for _, s := range subjects {
		var avg *float64
		if s.AvgPercent != nil {
			v := float64(int(*s.AvgPercent*10+0.5)) / 10
			avg = &v
		}
		out.Subjects = append(out.Subjects, SubjectRow{SubjectID: s.SubjectID, NameEN: s.NameEN, NameBN: s.NameBN, Code: s.Code, Entries: s.Entries, Passed: s.Passed,
			PassRate: percent(float64(s.Passed), float64(s.Entries)), AvgPercent: avg})
	}
	var groups []resultRow
	var err error
	if f.ClassID == "" {
		out.GroupBy = "class"
		err = raw(db, "SELECT c.id, c.name, count(*) AS results, count(*) FILTER (WHERE r.passed) AS passed, avg(r.gpa)::float8 AS avg_gpa FROM exam_results r JOIN exams e ON e.id = r.exam_id JOIN academic_classes c ON c.id = e.class_id WHERE "+q.sql()+" GROUP BY c.id, c.name, c.display_order ORDER BY c.display_order, c.name", q.args).Scan(&groups).Error
	} else {
		out.GroupBy = "section"
		err = raw(db, "SELECT sec.id, sec.name, count(*) AS results, count(*) FILTER (WHERE r.passed) AS passed, avg(r.gpa)::float8 AS avg_gpa FROM exam_results r JOIN exams e ON e.id = r.exam_id JOIN academic_sections sec ON sec.id = e.section_id WHERE "+q.sql()+" GROUP BY sec.id, sec.name, sec.display_order ORDER BY sec.display_order, sec.name", q.args).Scan(&groups).Error
	}
	if err != nil {
		return out, err
	}
	for _, g := range groups {
		out.Groups = append(out.Groups, GroupResult{ID: g.ID, Name: g.Name, Results: g.Results, Passed: g.Passed, PassRate: percent(float64(g.Passed), float64(g.Results)), AvgGPA: ptr2(g.AvgGPA)})
	}
	return out, nil
}

// Fees aggregates dues by due date and collections by payment date within
// from..to, plus balances as of today. Ledger income and expense are
// school-wide (the ledger has no class).
func (r *Repository) Fees(ctx context.Context, f Filter, today string) (Fees, error) {
	db := r.db.WithContext(ctx)
	out := Fees{Filter: f, Months: []FeeMonth{}, Methods: []MethodRow{}, Classes: []ClassFees{}, Aging: []AgingBucket{}, TopOverdue: []Overdue{}, Ledger: []CategoryAmount{}}
	args := map[string]any{"session": f.SessionID, "from": f.From, "to": f.To, "today": today}
	dues := newQuery(args).where("f.session_id = @session")
	if f.ClassID != "" {
		args["class"] = f.ClassID
		dues.where("f.class_id = @class")
	}
	inRange := dues.sql() + " AND f.due_date BETWEEN @from::date AND @to::date"
	var billed struct{ Billed, Discounts, Paid float64 }
	if err := raw(db, "SELECT COALESCE(sum(f.amount), 0)::float8 AS billed, COALESCE(sum(f.discount_amount), 0)::float8 AS discounts, COALESCE(sum(f.paid_amount), 0)::float8 AS paid FROM student_fees f WHERE "+inRange, args).Scan(&billed).Error; err != nil {
		return out, err
	}
	var bal struct {
		Outstanding, Overdue float64
		Students             int64
	}
	if err := raw(db, "SELECT COALESCE(sum(f.balance), 0)::float8 AS outstanding, COALESCE(sum(f.balance) FILTER (WHERE f.due_date < @today::date), 0)::float8 AS overdue, count(DISTINCT f.student_id) FILTER (WHERE f.balance > 0 AND f.due_date < @today::date) AS students FROM student_fees f WHERE "+dues.sql(), args).Scan(&bal).Error; err != nil {
		return out, err
	}
	paidIn := "FROM fee_payment_allocations al JOIN fee_payments p ON p.id = al.payment_id JOIN student_fees f ON f.id = al.student_fee_id WHERE " + dues.sql() + " AND p.paid_on BETWEEN @from::date AND @to::date AND NOT p.simulated"
	var collected float64
	if err := raw(db, "SELECT COALESCE(sum(al.amount), 0)::float8 "+paidIn, args).Row().Scan(&collected); err != nil {
		return out, err
	}
	var ledger []CategoryAmount
	if err := raw(db, "SELECT entry_type, category, sum(amount)::float8 AS amount FROM ledger_entries WHERE entry_date BETWEEN @from::date AND @to::date AND NOT EXISTS (SELECT 1 FROM fee_payments sp WHERE sp.id = ledger_entries.payment_id AND sp.simulated) GROUP BY entry_type, category ORDER BY entry_type DESC, amount DESC, category", args).Scan(&ledger).Error; err != nil {
		return out, err
	}
	var income, expense float64
	for _, l := range ledger {
		l.Amount = round2(l.Amount)
		if l.EntryType == "income" {
			income += l.Amount
		} else {
			expense += l.Amount
		}
		out.Ledger = append(out.Ledger, l)
	}
	net := billed.Billed - billed.Discounts
	out.Summary = FeeSummary{Billed: round2(billed.Billed), Discounts: round2(billed.Discounts), NetBilled: round2(net), PaidAgainst: round2(billed.Paid),
		CollectionRate: percent(billed.Paid, net), Collected: round2(collected), Outstanding: round2(bal.Outstanding), Overdue: round2(bal.Overdue),
		OverdueStudents: bal.Students, Income: round2(income), Expense: round2(expense), Net: round2(income - expense)}

	months := map[string]*FeeMonth{}
	var order []string
	var series []string
	if err := raw(db, "SELECT to_char(m, 'YYYY-MM') FROM generate_series(date_trunc('month', @from::date), date_trunc('month', @to::date), interval '1 month') AS m", args).Scan(&series).Error; err != nil {
		return out, err
	}
	for _, m := range series {
		months[m] = &FeeMonth{Month: m}
		order = append(order, m)
	}
	type monthAmount struct {
		Month  string
		Kind   string
		Amount float64
	}
	var amounts []monthAmount
	if err := raw(db, `SELECT to_char(f.due_date, 'YYYY-MM') AS month, 'billed' AS kind, sum(f.amount - f.discount_amount)::float8 AS amount FROM student_fees f WHERE `+inRange+` GROUP BY 1
UNION ALL SELECT to_char(p.paid_on, 'YYYY-MM'), 'collected', sum(al.amount)::float8 `+paidIn+` GROUP BY 1
UNION ALL SELECT to_char(entry_date, 'YYYY-MM'), entry_type, sum(amount)::float8 FROM ledger_entries WHERE entry_date BETWEEN @from::date AND @to::date AND NOT EXISTS (SELECT 1 FROM fee_payments sp WHERE sp.id = ledger_entries.payment_id AND sp.simulated) GROUP BY 1, 2`, args).Scan(&amounts).Error; err != nil {
		return out, err
	}
	for _, a := range amounts {
		m := months[a.Month]
		if m == nil {
			continue
		}
		switch a.Kind {
		case "billed":
			m.Billed = round2(a.Amount)
		case "collected":
			m.Collected = round2(a.Amount)
		case "income":
			m.Income = round2(a.Amount)
		case "expense":
			m.Expense = round2(a.Amount)
		}
	}
	for _, m := range order {
		out.Months = append(out.Months, *months[m])
	}
	if err := raw(db, "SELECT p.method, sum(al.amount)::float8 AS amount, count(DISTINCT p.id) AS receipts "+paidIn+" GROUP BY p.method ORDER BY amount DESC, p.method", args).Scan(&out.Methods).Error; err != nil {
		return out, err
	}
	for i := range out.Methods {
		out.Methods[i].Amount = round2(out.Methods[i].Amount)
	}
	var classes []struct {
		ID, Name                     string
		NetBilled, Paid, Outstanding float64
	}
	if err := raw(db, `SELECT c.id, c.name, sum(f.amount - f.discount_amount)::float8 AS net_billed, sum(f.paid_amount)::float8 AS paid, sum(f.balance)::float8 AS outstanding
FROM student_fees f JOIN academic_classes c ON c.id = f.class_id WHERE `+inRange+` GROUP BY c.id, c.name, c.display_order ORDER BY c.display_order, c.name`, args).Scan(&classes).Error; err != nil {
		return out, err
	}
	for _, c := range classes {
		out.Classes = append(out.Classes, ClassFees{ID: c.ID, Name: c.Name, NetBilled: round2(c.NetBilled), Paid: round2(c.Paid), Outstanding: round2(c.Outstanding), Rate: percent(c.Paid, c.NetBilled)})
	}
	var aging []AgingBucket
	if err := raw(db, `SELECT b.bucket, COALESCE(sum(f.balance), 0)::float8 AS amount, count(DISTINCT f.student_id) AS students
FROM (VALUES ('0_30', 0, 30, 1), ('31_60', 31, 60, 2), ('61_90', 61, 90, 3), ('90_plus', 91, 100000, 4)) AS b(bucket, lo, hi, ord)
LEFT JOIN student_fees f ON `+dues.sql()+` AND f.balance > 0 AND f.due_date < @today::date AND (@today::date - f.due_date) BETWEEN b.lo AND b.hi
GROUP BY b.bucket, b.ord ORDER BY b.ord`, args).Scan(&aging).Error; err != nil {
		return out, err
	}
	for i := range aging {
		aging[i].Amount = round2(aging[i].Amount)
	}
	out.Aging = aging
	if err := raw(db, `SELECT s.id AS student_id, s.name_en, s.name_bn, s.admission_no, c.name AS class_name, COALESCE(sec.name, '') AS section_name,
sum(f.balance)::float8 AS overdue, to_char(min(f.due_date), 'YYYY-MM-DD') AS oldest_due
FROM student_fees f JOIN students s ON s.id = f.student_id JOIN academic_classes c ON c.id = f.class_id LEFT JOIN student_enrolments en ON en.student_id = s.id AND en.session_id = f.session_id LEFT JOIN academic_sections sec ON sec.id = COALESCE(en.section_id, s.section_id)
WHERE `+dues.sql()+` AND f.balance > 0 AND f.due_date < @today::date
GROUP BY s.id, c.name, sec.name ORDER BY overdue DESC, oldest_due, s.admission_no LIMIT 10`, args).Scan(&out.TopOverdue).Error; err != nil {
		return out, err
	}
	for i := range out.TopOverdue {
		out.TopOverdue[i].Overdue = round2(out.TopOverdue[i].Overdue)
	}
	return out, nil
}

// SessionComparison compares the eight most recent sessions.
func (r *Repository) SessionComparison(ctx context.Context, finance bool) ([]SessionRow, error) {
	var rows []struct {
		Session
		ActiveStudents, Marked, Attended, Exams, Results, Passed int64
		AvgGPA                                                   *float64
		NetBilled, Paid                                          float64
	}
	err := r.db.WithContext(ctx).Raw(`SELECT s.id, s.name, to_char(s.start_date, 'YYYY-MM-DD') AS start_date, to_char(s.end_date, 'YYYY-MM-DD') AS end_date, s.is_active,
(SELECT count(*) FROM student_rolls st WHERE st.session_id = s.id) AS active_students,
a.marked, a.attended, x.exams, x.results, x.passed, x.avg_gpa, f.net_billed, f.paid
FROM (SELECT * FROM academic_sessions ORDER BY start_date DESC, name LIMIT 8) s
CROSS JOIN LATERAL (SELECT count(*) AS marked, count(*) FILTER (WHERE status IN ('present', 'late')) AS attended FROM student_attendance WHERE session_id = s.id) a
CROSS JOIN LATERAL (SELECT count(DISTINCT e.id) AS exams, count(r.student_id) AS results, count(*) FILTER (WHERE r.passed) AS passed, avg(r.gpa)::float8 AS avg_gpa
  FROM exams e JOIN exam_results r ON r.exam_id = e.id WHERE e.session_id = s.id AND e.status IN ('published', 'locked')) x
CROSS JOIN LATERAL (SELECT COALESCE(sum(amount - discount_amount), 0)::float8 AS net_billed, COALESCE(sum(paid_amount), 0)::float8 AS paid FROM student_fees WHERE session_id = s.id) f
ORDER BY s.start_date, s.name`).Scan(&rows).Error
	out := []SessionRow{}
	for _, row := range rows {
		item := SessionRow{Session: row.Session, ActiveStudents: row.ActiveStudents, Marked: row.Marked, AttendanceRate: percent(float64(row.Attended), float64(row.Marked)),
			Exams: row.Exams, Results: row.Results, PassRate: percent(float64(row.Passed), float64(row.Results)), AvgGPA: ptr2(row.AvgGPA)}
		if finance {
			nb, paid := round2(row.NetBilled), round2(row.Paid)
			item.NetBilled, item.Collected, item.CollectionRate = &nb, &paid, percent(row.Paid, row.NetBilled)
		}
		out = append(out, item)
	}
	return out, err
}

// Population counts students, staff and guardians.
func (r *Repository) Population(ctx context.Context, session string) (Overview, error) {
	db := r.db.WithContext(ctx)
	out := Overview{StudentsByState: map[string]int64{}, ByClass: []ClassCount{}}
	var states []struct {
		Status string
		Count  int64
	}
	if err := db.Raw("SELECT status, count(*) AS count FROM students WHERE session_id = ? GROUP BY status", session).Scan(&states).Error; err != nil {
		return out, err
	}
	for _, s := range states {
		out.StudentsByState[s.Status] = s.Count
	}
	out.ActiveStudents = out.StudentsByState["active"]
	if err := db.Raw(`SELECT c.id, c.name, count(s.student_id) AS students FROM academic_classes c JOIN student_rolls s ON s.class_id = c.id AND s.session_id = ?
GROUP BY c.id, c.name, c.display_order ORDER BY c.display_order, c.name`, session).Scan(&out.ByClass).Error; err != nil {
		return out, err
	}
	scalars := []struct {
		q string
		v *int64
	}{
		{"SELECT count(*) FROM employees WHERE is_active AND kind = 'teacher'", &out.Teachers},
		{"SELECT count(*) FROM employees WHERE is_active AND kind = 'non_teaching'", &out.NonTeaching},
		{"SELECT count(*) FROM guardians", &out.Guardians},
	}
	for _, s := range scalars {
		if err := db.Raw(s.q).Row().Scan(s.v); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Signals measures every active student of the session in scope for the
// alert rules. One row per student; the attendance look-back is bounded to
// 120 days, the longest configurable window.
func (r *Repository) Signals(ctx context.Context, scope Scope, f Filter, studentID, today, windowStart, inactivityStart, lookback string) ([]studentSignals, error) {
	args := map[string]any{"today": today, "wstart": windowStart, "istart": inactivityStart, "lookback": lookback}
	q := newQuery(args).where("s.status = 'active'")
	contextConds(q, "s", scope, f)
	if studentID != "" {
		args["student"] = studentID
		q.where("s.id = @student")
	}
	var rows []studentSignals
	err := raw(r.db.WithContext(ctx), `WITH st AS (
  SELECT s.id, s.name_en, s.name_bn, s.admission_no, s.roll_no, s.section_id, s.created_at, c.name AS class_name, c.display_order AS class_order, sec.name AS section_name, sec.display_order AS section_order
  FROM students s JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id WHERE `+q.sql()+`
), att AS (
  SELECT a.student_id,
    count(*) FILTER (WHERE a.attendance_date >= @wstart::date) AS marked,
    count(*) FILTER (WHERE a.attendance_date >= @wstart::date AND a.status IN ('present', 'late')) AS attended,
    max(a.attendance_date) FILTER (WHERE a.status IN ('present', 'late')) AS last_present
  FROM student_attendance a JOIN st ON st.id = a.student_id
  WHERE a.session_id = @session AND a.attendance_date BETWEEN @lookback::date AND @today::date GROUP BY a.student_id
), streak AS (
  SELECT a.student_id, count(*) AS n FROM student_attendance a JOIN st ON st.id = a.student_id LEFT JOIN att ON att.student_id = a.student_id
  WHERE a.session_id = @session AND a.attendance_date BETWEEN @lookback::date AND @today::date AND a.status = 'absent'
    AND (att.last_present IS NULL OR a.attendance_date > att.last_present) GROUP BY a.student_id
), secdays AS (
  SELECT a.section_id, count(DISTINCT a.attendance_date) AS days FROM student_attendance a
  WHERE a.session_id = @session AND a.attendance_date BETWEEN @istart::date AND @today::date AND a.section_id IN (SELECT section_id FROM st) GROUP BY a.section_id
), latest AS (
  SELECT DISTINCT ON (r.student_id) r.student_id, e.name, r.gpa::float8 AS gpa, r.passed FROM exam_results r JOIN exams e ON e.id = r.exam_id JOIN st ON st.id = r.student_id
  WHERE r.session_id = @session AND e.status IN ('published', 'locked') ORDER BY r.student_id, e.published_at DESC NULLS LAST, e.id DESC
)
SELECT st.id AS student_id, st.name_en, st.name_bn, st.admission_no, st.roll_no, st.class_name, st.class_order, st.section_name,
  COALESCE(att.marked, 0) AS marked, COALESCE(att.attended, 0) AS attended, COALESCE(streak.n, 0) AS absent_streak,
  to_char(att.last_present, 'YYYY-MM-DD') AS last_present, COALESCE(secdays.days, 0) AS section_days,
  (st.created_at AT TIME ZONE 'Asia/Dhaka')::date < @istart::date AS enrolled_early,
  latest.name AS latest_exam, latest.gpa AS latest_gpa, latest.passed AS latest_passed
FROM st LEFT JOIN att ON att.student_id = st.id LEFT JOIN streak ON streak.student_id = st.id
LEFT JOIN secdays ON secdays.section_id = st.section_id LEFT JOIN latest ON latest.student_id = st.id
ORDER BY st.class_order, st.class_name, st.section_order, st.section_name, st.roll_no NULLS LAST, st.admission_no`, args).Scan(&rows).Error
	return rows, err
}

// Student reads one student's identity and current placement.
func (r *Repository) Student(ctx context.Context, id string) (StudentInfo, error) {
	var s StudentInfo
	res := r.db.WithContext(ctx).Raw(`SELECT s.id, s.name_en, s.name_bn, s.admission_no, s.roll_no, s.status, s.session_id, ses.name AS session_name,
s.class_id, c.name AS class_name, s.section_id, sec.name AS section_name
FROM students s JOIN academic_sessions ses ON ses.id = s.session_id JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id
WHERE s.id = ?`, id).Scan(&s)
	if res.Error != nil {
		return s, res.Error
	}
	if res.RowsAffected == 0 {
		return s, ErrNotFound
	}
	return s, nil
}

// StudentForUser resolves the student record linked to a student account.
func (r *Repository) StudentForUser(ctx context.Context, user string) (string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Raw("SELECT id FROM students WHERE user_id = ?", user).Scan(&ids).Error; err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", ErrNotFound
	}
	return ids[0], nil
}

// Children lists students linked to a guardian account.
func (r *Repository) Children(ctx context.Context, user string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Raw(`SELECT s.id FROM students s JOIN student_guardians sg ON sg.student_id = s.id JOIN guardians g ON g.id = sg.guardian_id
WHERE g.user_id = ? ORDER BY s.name_en, s.id LIMIT 20`, user).Scan(&ids).Error
	return ids, err
}

// TeacherTeachesStudent reports whether the student's current class is
// assigned to the teacher in the student's session.
func (r *Repository) TeacherTeachesStudent(ctx context.Context, actor, student string) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM students s JOIN teacher_assignments ta ON ta.session_id = s.session_id AND ta.class_id = s.class_id
JOIN employees em ON em.id = ta.employee_id WHERE s.id = ? AND em.user_id = ? AND em.is_active)`, student, actor).Row().Scan(&ok)
	return ok, err
}

func (r *Repository) StudentAttendance(ctx context.Context, student StudentInfo, today, last30 string) (StudentAttendance, error) {
	db := r.db.WithContext(ctx)
	out := StudentAttendance{Months: []TrendPoint{}, Recent: []DayStatus{}}
	args := map[string]any{"student": student.ID, "session": student.SessionID, "today": today, "last30": last30}
	var all, recent countRow
	base := "FROM student_attendance a WHERE a.student_id = @student AND a.session_id = @session AND a.attendance_date <= @today::date"
	if err := raw(db, "SELECT "+countCols+" "+base, args).Scan(&all).Error; err != nil {
		return out, err
	}
	if err := raw(db, "SELECT "+countCols+" "+base+" AND a.attendance_date >= @last30::date", args).Scan(&recent).Error; err != nil {
		return out, err
	}
	out.Session, out.Last30 = all.counts(), recent.counts()
	var months []countRow
	if err := raw(db, "SELECT "+bucket("month", "a.attendance_date")+" AS period, "+countCols+" "+base+" GROUP BY 1 ORDER BY 1 DESC LIMIT 13", args).Scan(&months).Error; err != nil {
		return out, err
	}
	for i := len(months) - 1; i >= 0; i-- {
		out.Months = append(out.Months, TrendPoint{Period: months[i].Period, Counts: months[i].counts()})
	}
	err := raw(db, "SELECT to_char(a.attendance_date, 'YYYY-MM-DD') AS date, a.status "+base+" ORDER BY a.attendance_date DESC LIMIT 14", args).Scan(&out.Recent).Error
	return out, err
}

func (r *Repository) StudentResults(ctx context.Context, student StudentInfo, bands []Band) ([]StudentResult, []StudentSubject, error) {
	db := r.db.WithContext(ctx)
	results, subjects := []StudentResult{}, []StudentSubject{}
	var rows []struct {
		StudentResult
		LatestOrder int
	}
	if err := db.Raw(`SELECT e.id AS exam_id, e.name, e.exam_type, COALESCE(to_char(e.published_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD'), '') AS published_on,
r.total_marks::float8 AS total_marks, r.full_marks::float8 AS full_marks, r.gpa::float8 AS gpa, r.passed, r.class_position, r.section_position
FROM exam_results r JOIN exams e ON e.id = r.exam_id
WHERE r.student_id = ? AND r.session_id = ? AND e.status IN ('published', 'locked')
ORDER BY e.published_at DESC NULLS LAST, e.id DESC LIMIT 20`, student.ID, student.SessionID).Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	for i := len(rows) - 1; i >= 0; i-- {
		res := rows[i].StudentResult
		res.Percent = percent(res.TotalMarks, res.FullMarks)
		res.GPA = round2(res.GPA)
		res.Grade = GradeFor(bands, res.GPA, res.Passed)
		results = append(results, res)
	}
	if len(rows) == 0 {
		return results, subjects, nil
	}
	var subs []struct {
		NameEN, NameBN   string
		Marks, FullMarks float64
		Grade            *string
		GradePoint       *float64
		Passed           *bool
	}
	if err := db.Raw(`SELECT sub.name_en, sub.name_bn, coalesce(m.marks, 0)::float8 AS marks, es.full_marks::float8 AS full_marks, m.grade, m.grade_point::float8 AS grade_point, m.passed
FROM exam_marks m JOIN exam_subjects es ON es.id = m.exam_subject_id JOIN academic_subjects sub ON sub.id = es.subject_id
WHERE m.exam_id = ? AND m.student_id = ? AND m.status NOT IN ('exempt', 'withheld') ORDER BY sub.code, sub.name_en`, rows[0].ExamID, student.ID).Scan(&subs).Error; err != nil {
		return nil, nil, err
	}
	for _, s := range subs {
		subjects = append(subjects, StudentSubject{NameEN: s.NameEN, NameBN: s.NameBN, Marks: s.Marks, FullMarks: s.FullMarks, Percent: percent(s.Marks, s.FullMarks), Grade: s.Grade, GradePoint: s.GradePoint, Passed: s.Passed})
	}
	return results, subjects, nil
}

func (r *Repository) StudentFees(ctx context.Context, student StudentInfo, today string) (StudentFees, error) {
	var out StudentFees
	err := raw(r.db.WithContext(ctx), `SELECT COALESCE(sum(balance), 0)::float8 AS outstanding, COALESCE(sum(balance) FILTER (WHERE due_date < @today::date), 0)::float8 AS overdue,
COALESCE(sum(paid_amount) FILTER (WHERE session_id = @session), 0)::float8 AS paid_session,
to_char(min(due_date) FILTER (WHERE balance > 0 AND due_date >= @today::date), 'YYYY-MM-DD') AS next_due
FROM student_fees WHERE student_id = @student`, map[string]any{"student": student.ID, "session": student.SessionID, "today": today}).Scan(&out).Error
	out.Outstanding, out.Overdue, out.PaidSession = round2(out.Outstanding), round2(out.Overdue), round2(out.PaidSession)
	return out, err
}

// LastDayAttendance returns student and teacher attendance on one day.
func (r *Repository) Day(ctx context.Context, session, day string) (Counts, TeacherCounts, error) {
	db := r.db.WithContext(ctx)
	var s, t countRow
	if err := db.Raw("SELECT "+countCols+" FROM student_attendance a WHERE a.session_id = ? AND a.attendance_date = ?::date", session, day).Scan(&s).Error; err != nil {
		return Counts{}, TeacherCounts{}, err
	}
	if err := db.Raw("SELECT "+teacherCols+" FROM teacher_attendance t WHERE t.session_id = ? AND t.attendance_date = ?::date", session, day).Scan(&t).Error; err != nil {
		return Counts{}, TeacherCounts{}, err
	}
	return s.counts(), t.teacher(), nil
}
