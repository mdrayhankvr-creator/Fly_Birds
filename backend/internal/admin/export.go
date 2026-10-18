// Package admin implements the administrator control center: data export,
// database backups and the system overview.
package admin

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

var (
	ErrInvalid  = errors.New("invalid administration request")
	ErrNotFound = errors.New("administration resource not found")
	ErrBusy     = errors.New("another backup is already running")
)

// Dataset is one exportable table. Ranged datasets filter by a date column
// and need from and to, limited to MaxExportDays.
type Dataset struct {
	Key    string `json:"key"`
	Ranged bool   `json:"ranged"`
	Rows   int64  `json:"rows"`
	query  string
	count  string
}

const MaxExportDays = 400

// utf8BOM lets spreadsheet programs detect UTF-8, so Bangla text opens correctly.
const utf8BOM = "\xEF\xBB\xBF"

// Exports never include password hashes, sessions, tokens or file contents.
var datasets = []Dataset{
	{Key: "users", query: `SELECT u.email, u.display_name, u.role, u.is_active, to_char(u.created_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS created_at FROM users u ORDER BY u.role, u.email`, count: `SELECT count(*) FROM users`},
	{Key: "students", query: `SELECT s.admission_no, s.name_en, s.name_bn, s.date_of_birth, s.status, se.name AS session, c.name AS class, sec.name AS section, s.roll_no, s.user_id IS NOT NULL AS has_account
FROM students s JOIN academic_sessions se ON se.id = s.session_id JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id
ORDER BY se.start_date DESC, c.display_order, sec.name, s.roll_no NULLS LAST, s.admission_no`, count: `SELECT count(*) FROM students`},
	{Key: "guardians", query: `SELECT g.name_en, g.name_bn, g.phone, g.email, g.address, string_agg(s.admission_no || ' (' || sg.relationship || ')', '; ' ORDER BY s.admission_no) AS children
FROM guardians g LEFT JOIN student_guardians sg ON sg.guardian_id = g.id LEFT JOIN students s ON s.id = sg.student_id
GROUP BY g.id ORDER BY g.name_en`, count: `SELECT count(*) FROM guardians`},
	{Key: "staff", query: `SELECT e.employee_code, e.name_en, e.name_bn, e.kind, e.designation, e.phone, e.joined_on, e.left_on, e.is_active
FROM employees e ORDER BY e.is_active DESC, e.employee_code`, count: `SELECT count(*) FROM employees`},
	{Key: "student_attendance", Ranged: true, query: `SELECT a.attendance_date, s.admission_no, s.name_en, c.name AS class, sec.name AS section, a.status, a.source, a.note
FROM student_attendance a JOIN students s ON s.id = a.student_id JOIN academic_classes c ON c.id = a.class_id JOIN academic_sections sec ON sec.id = a.section_id
WHERE a.attendance_date BETWEEN @from AND @to ORDER BY a.attendance_date, c.display_order, sec.name, s.roll_no NULLS LAST`, count: `SELECT count(*) FROM student_attendance WHERE attendance_date BETWEEN @from AND @to`},
	{Key: "exam_results", query: `SELECT x.name AS exam, x.exam_type, x.status, se.name AS session, c.name AS class, sec.name AS section, s.admission_no, s.name_en, r.total_marks, r.full_marks, r.gpa, r.passed, r.class_position, r.section_position
FROM exam_results r JOIN exams x ON x.id = r.exam_id JOIN students s ON s.id = r.student_id JOIN academic_sessions se ON se.id = r.session_id JOIN academic_classes c ON c.id = r.class_id JOIN academic_sections sec ON sec.id = r.section_id
ORDER BY se.start_date DESC, x.name, c.display_order, sec.name, r.section_position`, count: `SELECT count(*) FROM exam_results`},
	{Key: "fee_payments", Ranged: true, query: `SELECT p.receipt_no, p.paid_on, s.admission_no, s.name_en, p.amount, p.method, p.source, p.reference, p.note
FROM fee_payments p JOIN students s ON s.id = p.student_id WHERE p.paid_on BETWEEN @from AND @to ORDER BY p.paid_on, p.receipt_no`, count: `SELECT count(*) FROM fee_payments WHERE paid_on BETWEEN @from AND @to`},
	{Key: "ledger", Ranged: true, query: `SELECT l.entry_date, l.entry_type, l.category, l.amount, l.description, l.reference
FROM ledger_entries l WHERE l.entry_date BETWEEN @from AND @to ORDER BY l.entry_date, l.created_at`, count: `SELECT count(*) FROM ledger_entries WHERE entry_date BETWEEN @from AND @to`},
	{Key: "audit_log", Ranged: true, query: `SELECT to_char(e.created_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI:SS') AS created_at, COALESCE(u.email, '') AS actor, e.action, e.target_type, e.target_id, e.outcome, e.metadata::text AS metadata
FROM audit_events e LEFT JOIN users u ON u.id = e.actor_user_id
WHERE (e.created_at AT TIME ZONE 'Asia/Dhaka')::date BETWEEN @from AND @to ORDER BY e.created_at, e.id`, count: `SELECT count(*) FROM audit_events WHERE (created_at AT TIME ZONE 'Asia/Dhaka')::date BETWEEN @from AND @to`},
}

// args binds the date window; unranged queries take no parameters.
func (d Dataset) args(from, to string) []any {
	if !d.Ranged {
		return nil
	}
	return []any{sql.Named("from", from), sql.Named("to", to)}
}

func datasetByKey(key string) (Dataset, bool) {
	for _, d := range datasets {
		if d.Key == key {
			return d, true
		}
	}
	return Dataset{}, false
}

// SafeCell neutralizes spreadsheet formulas: a cell starting with = + - @ tab
// or carriage return is prefixed with an apostrophe so it stays plain text.
func SafeCell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// ExportRange validates a ranged export window.
func ExportRange(from, to string) (time.Time, time.Time, bool) {
	f, err1 := time.Parse(time.DateOnly, from)
	t, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > MaxExportDays*24*time.Hour {
		return time.Time{}, time.Time{}, false
	}
	return f, t, true
}

type Exporter struct{ db *gorm.DB }

func NewExporter(db *gorm.DB) *Exporter { return &Exporter{db: db} }

// Datasets lists exports with current row counts (ranged ones for the window).
func (e *Exporter) Datasets(ctx context.Context, from, to string) ([]Dataset, error) {
	out := make([]Dataset, 0, len(datasets))
	_, _, ranged := ExportRange(from, to)
	for _, d := range datasets {
		if d.Ranged && !ranged {
			d.Rows = -1
		} else if err := e.db.WithContext(ctx).Raw(d.count, d.args(from, to)...).Row().Scan(&d.Rows); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// Export streams one dataset as CSV with a UTF-8 BOM (so spreadsheet
// programs read Bangla correctly) and records the export in the audit log.
func (e *Exporter) Export(ctx context.Context, w io.Writer, actor, key, from, to string) (int64, error) {
	d, ok := datasetByKey(key)
	if !ok {
		return 0, ErrNotFound
	}
	if d.Ranged {
		if _, _, ok := ExportRange(from, to); !ok {
			return 0, ErrInvalid
		}
	}
	rows, err := e.db.WithContext(ctx).Raw(d.query, d.args(from, to)...).Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return 0, err
	}
	out := csv.NewWriter(w)
	if err := out.Write(columns); err != nil {
		return 0, err
	}
	values := make([]sql.NullString, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	record := make([]string, len(columns))
	var count int64
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return count, err
		}
		for i, v := range values {
			cell := v.String
			if !utf8.ValidString(cell) {
				cell = strings.ToValidUTF8(cell, "�")
			}
			record[i] = SafeCell(cell)
		}
		if err := out.Write(record); err != nil {
			return count, err
		}
		count++
		if count%500 == 0 {
			out.Flush()
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return count, err
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	meta := map[string]any{"dataset": key, "rows": count}
	if d.Ranged {
		meta["from"], meta["to"] = from, to
	}
	return count, audit.Record(e.db.WithContext(ctx), &actor, "data.exported", "dataset", key, meta)
}

// FileName returns a predictable download name for an export.
func FileName(key, from, to string, now time.Time) string {
	if _, _, ok := ExportRange(from, to); ok {
		return fmt.Sprintf("flybirds-%s-%s-to-%s.csv", key, from, to)
	}
	return fmt.Sprintf("flybirds-%s-%s.csv", key, now.Format("20060102"))
}
