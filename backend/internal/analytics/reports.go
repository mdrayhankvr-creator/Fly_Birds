package analytics

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
)

// Report describes one tabular report. Access is "academic" (analytics:read,
// or analytics:read_assigned limited to assigned classes), "school"
// (analytics:read only) or "finance" (analytics:finance).
type Report struct {
	Key     string   `json:"key"`
	Access  string   `json:"access"`
	Module  string   `json:"module"`
	Ranged  bool     `json:"ranged"`
	Columns []string `json:"columns"`
}

var reports = []Report{
	{Key: "student_attendance", Access: "academic", Module: "attendance", Ranged: true, Columns: []string{"admission_no", "name_en", "name_bn", "class", "section", "roll_no", "status", "marked", "present", "late", "absent", "rate"}},
	{Key: "teacher_attendance", Access: "school", Module: "attendance", Ranged: true, Columns: []string{"employee_code", "name_en", "name_bn", "designation", "marked", "present", "absent", "leave", "rate"}},
	{Key: "exam_results", Access: "academic", Module: "examinations", Columns: []string{"exam", "exam_type", "published_on", "class", "section", "admission_no", "name_en", "name_bn", "roll_no", "total_marks", "full_marks", "percent", "gpa", "grade", "passed", "section_position", "class_position"}},
	{Key: "fee_balances", Access: "finance", Module: "fees", Columns: []string{"admission_no", "name_en", "name_bn", "class", "section", "net_billed", "paid", "balance", "overdue", "oldest_overdue"}},
	{Key: "risks", Access: "academic", Module: "", Columns: []string{"level", "reasons", "admission_no", "name_en", "name_bn", "class", "section", "roll_no", "attendance_rate", "marked", "absent_streak", "last_present", "latest_exam", "latest_gpa"}},
}

func ReportByKey(key string) (Report, bool) {
	for _, r := range reports {
		if r.Key == key {
			return r, true
		}
	}
	return Report{}, false
}

func Reports() []Report { return append([]Report(nil), reports...) }

const utf8BOM = "\xEF\xBB\xBF"

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func optNum(v *float64) string {
	if v == nil {
		return ""
	}
	return num(*v)
}
func optInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
func optStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// Report returns one page of a report. limit 0 means MaxReportRows rows for
// a CSV export.
func (s *Service) Report(ctx context.Context, scope Scope, key string, f Filter, page, size int) (ReportPage, error) {
	rep, ok := ReportByKey(key)
	if !ok {
		return ReportPage{}, ErrNotFound
	}
	if page < 1 || page > 10000 || size < 1 || size > MaxReportRows {
		return ReportPage{}, ErrInvalid
	}
	if rep.Access == "school" && scope.Teacher {
		return ReportPage{}, ErrForbidden
	}
	days := 0
	if rep.Ranged {
		days = 30
	}
	if rep.Key != "exam_results" {
		f.ExamID, f.ExamType = "", ""
	}
	if rep.Key == "fee_balances" || rep.Key == "teacher_attendance" {
		f.SectionID = ""
		if rep.Key == "teacher_attendance" {
			f.ClassID = ""
		}
	}
	f, session, err := s.resolve(ctx, scope, f, days)
	out := ReportPage{Report: key, Filter: f, Columns: rep.Columns, Rows: [][]string{}, Page: page, PageSize: size}
	if err != nil || session == nil {
		return out, err
	}
	offset := (page - 1) * size
	switch key {
	case "risks":
		all, _, _, err := s.risks(ctx, scope, f, "")
		if err != nil {
			return out, err
		}
		out.Total = int64(len(all))
		for i := offset; i < len(all) && i < offset+size; i++ {
			r := all[i]
			out.Rows = append(out.Rows, []string{r.Level, strings.Join(r.Reasons, " "), r.AdmissionNo, r.NameEN, r.NameBN, r.ClassName, r.SectionName, optInt(r.RollNo),
				optNum(r.AttendanceRate), strconv.FormatInt(r.Marked, 10), strconv.FormatInt(r.AbsentStreak, 10), optStr(r.LastPresent), optStr(r.LatestExam), optNum(r.LatestGPA)})
		}
		return out, nil
	case "exam_results":
		bands, err := s.repo.Bands(ctx)
		if err != nil {
			return out, err
		}
		out.Total, out.Rows, err = s.repo.examReport(ctx, scope, f, bands, size, offset)
		return out, err
	case "student_attendance":
		out.Total, out.Rows, err = s.repo.studentAttendanceReport(ctx, scope, f, size, offset)
	case "teacher_attendance":
		out.Total, out.Rows, err = s.repo.teacherAttendanceReport(ctx, f, size, offset)
	case "fee_balances":
		out.Total, out.Rows, err = s.repo.feeReport(ctx, f, day(s.today()), size, offset)
	}
	return out, err
}

// WriteCSV streams a whole report (up to MaxReportRows rows) as UTF-8 CSV with
// a byte order mark and records the export in the audit log.
func (s *Service) WriteCSV(ctx context.Context, w io.Writer, actor string, page ReportPage) error {
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return err
	}
	out := csv.NewWriter(w)
	if err := out.Write(page.Columns); err != nil {
		return err
	}
	for _, row := range page.Rows {
		cells := make([]string, len(row))
		for i, v := range row {
			if !utf8.ValidString(v) {
				v = strings.ToValidUTF8(v, "�")
			}
			cells[i] = admin.SafeCell(v)
		}
		if err := out.Write(cells); err != nil {
			return err
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return err
	}
	meta := map[string]any{"report": page.Report, "rows": len(page.Rows), "session_id": page.Filter.SessionID}
	for k, v := range map[string]string{"class_id": page.Filter.ClassID, "section_id": page.Filter.SectionID, "exam_id": page.Filter.ExamID, "from": page.Filter.From, "to": page.Filter.To} {
		if v != "" {
			meta[k] = v
		}
	}
	return audit.Record(s.repo.db.WithContext(ctx), &actor, "analytics.exported", "report", page.Report, meta)
}

// FileName is the download name of a report export.
func FileName(page ReportPage) string {
	if page.Filter.From != "" {
		return fmt.Sprintf("flybirds-%s-%s-to-%s.csv", page.Report, page.Filter.From, page.Filter.To)
	}
	return fmt.Sprintf("flybirds-%s.csv", page.Report)
}

func (r *Repository) count(ctx context.Context, sql string, args map[string]any) (int64, error) {
	var n int64
	err := raw(r.db.WithContext(ctx), sql, args).Row().Scan(&n)
	return n, err
}

func (r *Repository) studentAttendanceReport(ctx context.Context, scope Scope, f Filter, limit, offset int) (int64, [][]string, error) {
	q := contextConds(newQuery(map[string]any{"from": f.From, "to": f.To, "limit": limit, "offset": offset}), "s", scope, f)
	total, err := r.count(ctx, "SELECT count(*) FROM students s WHERE "+q.sql(), q.args)
	if err != nil {
		return 0, nil, err
	}
	var rows []struct {
		AdmissionNo, NameEN, NameBN, ClassName, SectionName, Status string
		RollNo                                                      *int
		C                                                           countRow `gorm:"embedded"`
	}
	err = raw(r.db.WithContext(ctx), `SELECT s.admission_no, s.name_en, s.name_bn, c.name AS class_name, sec.name AS section_name, s.roll_no, s.status, `+countCols+`
FROM students s JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id
LEFT JOIN student_attendance a ON a.student_id = s.id AND a.session_id = s.session_id AND a.attendance_date BETWEEN @from::date AND @to::date
WHERE `+q.sql()+` GROUP BY s.id, c.name, c.display_order, sec.name, sec.display_order
ORDER BY c.display_order, c.name, sec.display_order, sec.name, s.roll_no NULLS LAST, s.admission_no LIMIT @limit OFFSET @offset`, q.args).Scan(&rows).Error
	out := [][]string{}
	for _, x := range rows {
		// count(*) counts the outer-joined NULL row; marked is the sum of statuses.
		x.C.Marked = x.C.Present + x.C.Late + x.C.Absent
		c := x.C.counts()
		out = append(out, []string{x.AdmissionNo, x.NameEN, x.NameBN, x.ClassName, x.SectionName, optInt(x.RollNo), x.Status,
			strconv.FormatInt(c.Marked, 10), strconv.FormatInt(c.Present, 10), strconv.FormatInt(c.Late, 10), strconv.FormatInt(c.Absent, 10), optNum(c.Rate)})
	}
	return total, out, err
}

func (r *Repository) teacherAttendanceReport(ctx context.Context, f Filter, limit, offset int) (int64, [][]string, error) {
	args := map[string]any{"session": f.SessionID, "from": f.From, "to": f.To, "limit": limit, "offset": offset}
	total, err := r.count(ctx, "SELECT count(*) FROM employees WHERE kind = 'teacher' AND is_active", args)
	if err != nil {
		return 0, nil, err
	}
	var rows []struct {
		EmployeeCode, NameEN, NameBN, Designation string
		C                                         countRow `gorm:"embedded"`
	}
	err = raw(r.db.WithContext(ctx), `SELECT e.employee_code, e.name_en, e.name_bn, e.designation, count(t.id) AS marked,
count(*) FILTER (WHERE t.status = 'present') AS present, count(*) FILTER (WHERE t.status = 'absent') AS absent, count(*) FILTER (WHERE t.status = 'leave') AS leave
FROM employees e LEFT JOIN teacher_attendance t ON t.employee_id = e.id AND t.session_id = @session AND t.attendance_date BETWEEN @from::date AND @to::date
WHERE e.kind = 'teacher' AND e.is_active GROUP BY e.id ORDER BY e.name_en, e.employee_code LIMIT @limit OFFSET @offset`, args).Scan(&rows).Error
	out := [][]string{}
	for _, x := range rows {
		c := x.C.teacher()
		out = append(out, []string{x.EmployeeCode, x.NameEN, x.NameBN, x.Designation, strconv.FormatInt(c.Marked, 10), strconv.FormatInt(c.Present, 10),
			strconv.FormatInt(c.Absent, 10), strconv.FormatInt(c.Leave, 10), optNum(c.Rate)})
	}
	return total, out, err
}

func (r *Repository) examReport(ctx context.Context, scope Scope, f Filter, bands []Band, limit, offset int) (int64, [][]string, error) {
	q := examConds(newQuery(map[string]any{"limit": limit, "offset": offset}), scope, f)
	total, err := r.count(ctx, "SELECT count(*) FROM exam_results r JOIN exams e ON e.id = r.exam_id WHERE "+q.sql(), q.args)
	if err != nil {
		return 0, nil, err
	}
	var rows []struct {
		Exam, ExamType, PublishedOn, ClassName, SectionName, AdmissionNo, NameEN, NameBN string
		RollNo                                                                           *int
		TotalMarks, FullMarks, GPA                                                       float64
		Passed                                                                           bool
		SectionPosition, ClassPosition                                                   int
	}
	err = raw(r.db.WithContext(ctx), `SELECT e.name AS exam, e.exam_type, COALESCE(to_char(e.published_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD'), '') AS published_on,
c.name AS class_name, sec.name AS section_name, s.admission_no, s.name_en, s.name_bn, s.roll_no, r.total_marks::float8 AS total_marks, r.full_marks::float8 AS full_marks,
r.gpa::float8 AS gpa, r.passed, r.section_position, r.class_position
FROM exam_results r JOIN exams e ON e.id = r.exam_id JOIN students s ON s.id = r.student_id JOIN academic_classes c ON c.id = e.class_id JOIN academic_sections sec ON sec.id = e.section_id
WHERE `+q.sql()+` ORDER BY e.published_at DESC NULLS LAST, e.name, c.display_order, sec.display_order, r.section_position, s.admission_no LIMIT @limit OFFSET @offset`, q.args).Scan(&rows).Error
	out := [][]string{}
	for _, x := range rows {
		passed := "no"
		if x.Passed {
			passed = "yes"
		}
		out = append(out, []string{x.Exam, x.ExamType, x.PublishedOn, x.ClassName, x.SectionName, x.AdmissionNo, x.NameEN, x.NameBN, optInt(x.RollNo),
			num(x.TotalMarks), num(x.FullMarks), optNum(percent(x.TotalMarks, x.FullMarks)), num(round2(x.GPA)), GradeFor(bands, x.GPA, x.Passed), passed,
			strconv.Itoa(x.SectionPosition), strconv.Itoa(x.ClassPosition)})
	}
	return total, out, err
}

func (r *Repository) feeReport(ctx context.Context, f Filter, today string, limit, offset int) (int64, [][]string, error) {
	args := map[string]any{"session": f.SessionID, "today": today, "limit": limit, "offset": offset}
	cond := "f.session_id = @session"
	if f.ClassID != "" {
		args["class"] = f.ClassID
		cond += " AND f.class_id = @class"
	}
	total, err := r.count(ctx, "SELECT count(DISTINCT f.student_id) FROM student_fees f WHERE "+cond, args)
	if err != nil {
		return 0, nil, err
	}
	var rows []struct {
		AdmissionNo, NameEN, NameBN, ClassName, SectionName string
		NetBilled, Paid, Balance, Overdue                   float64
		OldestOverdue                                       *string
	}
	err = raw(r.db.WithContext(ctx), `SELECT s.admission_no, s.name_en, s.name_bn, string_agg(DISTINCT c.name, ', ') AS class_name, COALESCE(sec.name, '') AS section_name,
sum(f.amount - f.discount_amount)::float8 AS net_billed, sum(f.paid_amount)::float8 AS paid, sum(f.balance)::float8 AS balance,
COALESCE(sum(f.balance) FILTER (WHERE f.due_date < @today::date), 0)::float8 AS overdue,
to_char(min(f.due_date) FILTER (WHERE f.balance > 0 AND f.due_date < @today::date), 'YYYY-MM-DD') AS oldest_overdue
FROM student_fees f JOIN students s ON s.id = f.student_id JOIN academic_classes c ON c.id = f.class_id LEFT JOIN academic_sections sec ON sec.id = s.section_id
WHERE `+cond+` GROUP BY s.id, sec.name ORDER BY overdue DESC, balance DESC, s.admission_no LIMIT @limit OFFSET @offset`, args).Scan(&rows).Error
	out := [][]string{}
	for _, x := range rows {
		out = append(out, []string{x.AdmissionNo, x.NameEN, x.NameBN, x.ClassName, x.SectionName, num(round2(x.NetBilled)), num(round2(x.Paid)), num(round2(x.Balance)), num(round2(x.Overdue)), optStr(x.OldestOverdue)})
	}
	return total, out, err
}
