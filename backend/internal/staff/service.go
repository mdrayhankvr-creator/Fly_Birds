package staff

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid staff input")
	ErrNotFound  = errors.New("staff record not found")
	ErrDuplicate = errors.New("staff record already exists")
	ErrForbidden = errors.New("staff operation forbidden")
	ErrConflict  = errors.New("staff relationship or state conflict")
)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var code = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{1,31}$`)
var phone = regexp.MustCompile(`^[+0-9() -]{0,32}$`)

type Employee struct {
	ID           string  `json:"id"`
	EmployeeCode string  `json:"employee_code"`
	UserID       *string `json:"user_id"`
	Kind         string  `json:"kind"`
	NameEN       string  `json:"name_en"`
	NameBN       string  `json:"name_bn"`
	Designation  string  `json:"designation"`
	Phone        string  `json:"phone"`
	JoinedOn     string  `json:"joined_on"`
	LeftOn       *string `json:"left_on"`
	IsActive     bool    `json:"is_active"`
}
type EmployeeInput struct {
	EmployeeCode string  `json:"employee_code"`
	UserID       *string `json:"user_id"`
	Kind         string  `json:"kind"`
	NameEN       string  `json:"name_en"`
	NameBN       string  `json:"name_bn"`
	Designation  string  `json:"designation"`
	Phone        string  `json:"phone"`
	JoinedOn     string  `json:"joined_on"`
	LeftOn       *string `json:"left_on"`
	IsActive     *bool   `json:"is_active"`
}
type Assignment struct {
	ID            string `json:"id"`
	EmployeeID    string `json:"employee_id"`
	SessionID     string `json:"session_id"`
	ClassID       string `json:"class_id"`
	SubjectID     string `json:"subject_id"`
	SessionName   string `json:"session_name"`
	ClassName     string `json:"class_name"`
	SubjectNameEN string `json:"subject_name_en"`
	SubjectNameBN string `json:"subject_name_bn"`
}
type AssignmentInput struct {
	SessionID string `json:"session_id"`
	ClassID   string `json:"class_id"`
	SubjectID string `json:"subject_id"`
}
type Leave struct {
	ID         string  `json:"id"`
	EmployeeID string  `json:"employee_id"`
	StartsOn   string  `json:"starts_on"`
	EndsOn     string  `json:"ends_on"`
	LeaveType  string  `json:"leave_type"`
	Reason     string  `json:"reason"`
	Status     string  `json:"status"`
	ReviewedBy *string `json:"reviewed_by"`
}
type LeaveInput struct {
	StartsOn  string `json:"starts_on"`
	EndsOn    string `json:"ends_on"`
	LeaveType string `json:"leave_type"`
	Reason    string `json:"reason"`
}
type Salary struct {
	ID           string `json:"id"`
	EmployeeID   string `json:"employee_id"`
	EffectiveOn  string `json:"effective_on"`
	BasicBDT     string `json:"basic_bdt"`
	AllowanceBDT string `json:"allowance_bdt"`
	DeductionBDT string `json:"deduction_bdt"`
	Note         string `json:"note"`
}
type SalaryInput struct {
	EffectiveOn  string `json:"effective_on"`
	BasicBDT     string `json:"basic_bdt"`
	AllowanceBDT string `json:"allowance_bdt"`
	DeductionBDT string `json:"deduction_bdt"`
	Note         string `json:"note"`
}
type Query struct {
	Search   string
	Kind     string
	Status   string
	Page     int
	PageSize int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type Repository interface {
	ListEmployees(context.Context, Query) (Page[Employee], error)
	GetEmployee(context.Context, string) (Employee, error)
	OwnEmployee(context.Context, string) (Employee, error)
	SaveEmployee(context.Context, string, string, Employee) (Employee, error)
	ListAssignments(context.Context, string, Query) (Page[Assignment], error)
	AddAssignment(context.Context, string, Assignment) (Assignment, error)
	DeleteAssignment(context.Context, string, string, string) error
	ListLeaves(context.Context, string, string, bool, Query) (Page[Leave], error)
	RequestLeave(context.Context, string, string, Leave) (Leave, error)
	DecideLeave(context.Context, string, string, string) (Leave, error)
	ListSalaries(context.Context, string, Query) (Page[Salary], error)
	AddSalary(context.Context, string, Salary) (Salary, error)
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }
func validText(s string, max int, required bool) bool {
	return utf8.ValidString(s) && len(s) <= max && (!required || strings.TrimSpace(s) != "")
}
func validDate(s string) bool {
	v, e := time.Parse("2006-01-02", s)
	return e == nil && v.Format("2006-01-02") == s
}
func validID(s string) bool { return uuid.MatchString(s) }
func validQuery(q Query) bool {
	return q.Page > 0 && q.Page <= 100000 && q.PageSize > 0 && q.PageSize <= 100 && validText(q.Search, 100, false) && (q.Kind == "" || q.Kind == "teacher" || q.Kind == "non_teaching") && (q.Status == "" || q.Status == "active" || q.Status == "inactive")
}
func (s *Service) ListEmployees(ctx context.Context, q Query) (Page[Employee], error) {
	if !validQuery(q) {
		return Page[Employee]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListEmployees(ctx, q)
}
func (s *Service) GetEmployee(ctx context.Context, id string) (Employee, error) {
	if !validID(id) {
		return Employee{}, ErrInvalid
	}
	return s.repo.GetEmployee(ctx, id)
}
func (s *Service) OwnEmployee(ctx context.Context, user string) (Employee, error) {
	if !validID(user) {
		return Employee{}, ErrInvalid
	}
	return s.repo.OwnEmployee(ctx, user)
}
func (s *Service) SaveEmployee(ctx context.Context, actor, id string, input EmployeeInput) (Employee, error) {
	if !validID(actor) || (id != "" && !validID(id)) {
		return Employee{}, ErrInvalid
	}
	input.EmployeeCode = strings.ToUpper(strings.TrimSpace(input.EmployeeCode))
	input.NameEN = strings.TrimSpace(input.NameEN)
	input.NameBN = strings.TrimSpace(input.NameBN)
	input.Designation = strings.TrimSpace(input.Designation)
	input.Phone = strings.TrimSpace(input.Phone)
	if !code.MatchString(input.EmployeeCode) || (input.Kind != "teacher" && input.Kind != "non_teaching") || !validText(input.NameEN, 160, true) || !validText(input.NameBN, 160, false) || !validText(input.Designation, 120, true) || !phone.MatchString(input.Phone) || !validDate(input.JoinedOn) {
		return Employee{}, ErrInvalid
	}
	if input.UserID != nil && !validID(*input.UserID) {
		return Employee{}, ErrInvalid
	}
	if input.LeftOn != nil && (!validDate(*input.LeftOn) || *input.LeftOn < input.JoinedOn) {
		return Employee{}, ErrInvalid
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	if active && input.LeftOn != nil {
		return Employee{}, ErrInvalid
	}
	return s.repo.SaveEmployee(ctx, actor, id, Employee{EmployeeCode: input.EmployeeCode, UserID: input.UserID, Kind: input.Kind, NameEN: input.NameEN, NameBN: input.NameBN, Designation: input.Designation, Phone: input.Phone, JoinedOn: input.JoinedOn, LeftOn: input.LeftOn, IsActive: active})
}
func (s *Service) ListAssignments(ctx context.Context, id string, q Query) (Page[Assignment], error) {
	if !validID(id) || !validQuery(q) || q.Kind != "" || q.Status != "" {
		return Page[Assignment]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListAssignments(ctx, id, q)
}
func (s *Service) AddAssignment(ctx context.Context, actor, employee string, input AssignmentInput) (Assignment, error) {
	if !validID(actor) || !validID(employee) || !validID(input.SessionID) || !validID(input.ClassID) || !validID(input.SubjectID) {
		return Assignment{}, ErrInvalid
	}
	return s.repo.AddAssignment(ctx, actor, Assignment{EmployeeID: employee, SessionID: input.SessionID, ClassID: input.ClassID, SubjectID: input.SubjectID})
}
func (s *Service) DeleteAssignment(ctx context.Context, actor, employee, id string) error {
	if !validID(actor) || !validID(employee) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteAssignment(ctx, actor, employee, id)
}
func (s *Service) ListLeaves(ctx context.Context, actor, employee string, self bool, q Query) (Page[Leave], error) {
	if !validID(actor) || (employee != "" && !validID(employee)) || !validQuery(q) || q.Kind != "" || q.Status != "" {
		return Page[Leave]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListLeaves(ctx, actor, employee, self, q)
}
func (s *Service) RequestLeave(ctx context.Context, actor, employee string, self bool, input LeaveInput) (Leave, error) {
	if !validID(actor) || (employee != "" && !validID(employee)) || !validDate(input.StartsOn) || !validDate(input.EndsOn) || input.EndsOn < input.StartsOn || !validText(input.Reason, 1000, false) {
		return Leave{}, ErrInvalid
	}
	switch input.LeaveType {
	case "casual", "sick", "annual", "other":
	default:
		return Leave{}, ErrInvalid
	}
	if self {
		employee = ""
	}
	return s.repo.RequestLeave(ctx, actor, employee, Leave{StartsOn: input.StartsOn, EndsOn: input.EndsOn, LeaveType: input.LeaveType, Reason: strings.TrimSpace(input.Reason), Status: "pending"})
}
func (s *Service) DecideLeave(ctx context.Context, actor, id, status string) (Leave, error) {
	if !validID(actor) || !validID(id) || (status != "approved" && status != "rejected") {
		return Leave{}, ErrInvalid
	}
	return s.repo.DecideLeave(ctx, actor, id, status)
}
func (s *Service) ListSalaries(ctx context.Context, id string, q Query) (Page[Salary], error) {
	if !validID(id) || !validQuery(q) || q.Kind != "" || q.Status != "" {
		return Page[Salary]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListSalaries(ctx, id, q)
}
func (s *Service) AddSalary(ctx context.Context, actor, employee string, input SalaryInput) (Salary, error) {
	if !validID(actor) || !validID(employee) || !validDate(input.EffectiveOn) || !validText(input.Note, 500, false) {
		return Salary{}, ErrInvalid
	}
	if input.AllowanceBDT == "" {
		input.AllowanceBDT = "0"
	}
	if input.DeductionBDT == "" {
		input.DeductionBDT = "0"
	}
	for _, v := range []string{input.BasicBDT, input.AllowanceBDT, input.DeductionBDT} {
		if !regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,2})?$`).MatchString(v) {
			return Salary{}, ErrInvalid
		}
	}
	return s.repo.AddSalary(ctx, actor, Salary{EmployeeID: employee, EffectiveOn: input.EffectiveOn, BasicBDT: input.BasicBDT, AllowanceBDT: input.AllowanceBDT, DeductionBDT: input.DeductionBDT, Note: strings.TrimSpace(input.Note)})
}
