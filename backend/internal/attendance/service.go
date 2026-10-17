package attendance

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid attendance input")
	ErrNotFound  = errors.New("attendance record not found")
	ErrDuplicate = errors.New("attendance already recorded")
	ErrForbidden = errors.New("attendance operation forbidden")
	ErrConflict  = errors.New("attendance relationship conflict")
)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

type Record struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	EmployeeID     string    `json:"employee_id"`
	AttendanceDate string    `json:"attendance_date"`
	Status         string    `json:"status"`
	Note           string    `json:"note"`
	RecordedBy     string    `json:"recorded_by"`
	UpdatedBy      string    `json:"updated_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	SessionName    string    `json:"session_name"`
	EmployeeCode   string    `json:"employee_code"`
	EmployeeNameEN string    `json:"employee_name_en"`
	EmployeeNameBN string    `json:"employee_name_bn"`
}
type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	IsActive  bool   `json:"is_active"`
}
type RecordInput struct {
	SessionID      string `json:"session_id"`
	EmployeeID     string `json:"employee_id"`
	AttendanceDate string `json:"attendance_date"`
	Status         string `json:"status"`
	Note           string `json:"note"`
}
type CorrectionInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}
type RosterItem struct {
	EmployeeID   string  `json:"employee_id"`
	EmployeeCode string  `json:"employee_code"`
	NameEN       string  `json:"name_en"`
	NameBN       string  `json:"name_bn"`
	RecordID     *string `json:"record_id"`
	Status       *string `json:"status"`
	Note         *string `json:"note"`
}
type Summary struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeCode string `json:"employee_code"`
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	Present      int64  `json:"present"`
	Absent       int64  `json:"absent"`
	Leave        int64  `json:"leave"`
	Total        int64  `json:"total"`
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
type Query struct {
	SessionID string
	Date      string
	From      string
	To        string
	Month     string
	Search    string
	Page      int
	PageSize  int
}
type Repository interface {
	ListSessions(context.Context, Query) (Page[Session], error)
	Roster(context.Context, Query) (Page[RosterItem], error)
	Record(context.Context, string, Record) (Record, error)
	Correct(context.Context, string, string, CorrectionInput) (Record, error)
	OwnHistory(context.Context, string, Query) (Page[Record], error)
	Monthly(context.Context, Query) (Page[Summary], error)
	OwnMonthly(context.Context, string, Query) (Summary, error)
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }
func (s *Service) ListSessions(ctx context.Context, q Query) (Page[Session], error) {
	if !validPaging(q) {
		return Page[Session]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListSessions(ctx, q)
}
func validDate(s string) bool {
	v, e := time.Parse("2006-01-02", s)
	return e == nil && v.Format("2006-01-02") == s
}
func validStatus(s string) bool { return s == "present" || s == "absent" || s == "leave" }
func validNote(s string) bool   { return utf8.ValidString(s) && len(s) <= 500 }
func validPaging(q Query) bool {
	return q.Page >= 1 && q.Page <= 100000 && q.PageSize >= 1 && q.PageSize <= 100 && utf8.ValidString(q.Search) && len(q.Search) <= 100
}
func (s *Service) Roster(ctx context.Context, q Query) (Page[RosterItem], error) {
	if !validPaging(q) || !uuid.MatchString(q.SessionID) || !validDate(q.Date) {
		return Page[RosterItem]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.Roster(ctx, q)
}
func (s *Service) Record(ctx context.Context, actor string, input RecordInput) (Record, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(input.SessionID) || !uuid.MatchString(input.EmployeeID) || !validDate(input.AttendanceDate) || !validStatus(input.Status) || !validNote(input.Note) {
		return Record{}, ErrInvalid
	}
	return s.repo.Record(ctx, actor, Record{SessionID: input.SessionID, EmployeeID: input.EmployeeID, AttendanceDate: input.AttendanceDate, Status: input.Status, Note: strings.TrimSpace(input.Note)})
}
func (s *Service) Correct(ctx context.Context, actor, id string, input CorrectionInput) (Record, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || !validStatus(input.Status) || !validNote(input.Note) {
		return Record{}, ErrInvalid
	}
	input.Note = strings.TrimSpace(input.Note)
	return s.repo.Correct(ctx, actor, id, input)
}
func (s *Service) OwnHistory(ctx context.Context, actor string, q Query) (Page[Record], error) {
	if !uuid.MatchString(actor) || !validPaging(q) || (q.SessionID != "" && !uuid.MatchString(q.SessionID)) || (q.From != "" && !validDate(q.From)) || (q.To != "" && !validDate(q.To)) || (q.From != "" && q.To != "" && q.To < q.From) {
		return Page[Record]{}, ErrInvalid
	}
	return s.repo.OwnHistory(ctx, actor, q)
}
func (s *Service) Monthly(ctx context.Context, q Query) (Page[Summary], error) {
	if !validPaging(q) || !uuid.MatchString(q.SessionID) || !monthPattern.MatchString(q.Month) {
		return Page[Summary]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.Monthly(ctx, q)
}
func (s *Service) OwnMonthly(ctx context.Context, actor string, q Query) (Summary, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(q.SessionID) || !monthPattern.MatchString(q.Month) {
		return Summary{}, ErrInvalid
	}
	return s.repo.OwnMonthly(ctx, actor, q)
}
