package exams

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid examination input")
	ErrNotFound  = errors.New("examination not found")
	ErrConflict  = errors.New("examination conflict")
	ErrForbidden = errors.New("examination access denied")
)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Exam struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ExamType    string    `json:"exam_type"`
	SessionID   string    `json:"session_id"`
	ClassID     string    `json:"class_id"`
	SectionID   string    `json:"section_id"`
	RoutineID   *string   `json:"routine_id"`
	Status      string    `json:"status"`
	SessionName string    `json:"session_name"`
	ClassName   string    `json:"class_name"`
	SectionName string    `json:"section_name"`
	Subjects    []Subject `json:"subjects" gorm:"-"`
}
type ExamInput struct {
	Name      string  `json:"name"`
	ExamType  string  `json:"exam_type"`
	SessionID string  `json:"session_id"`
	ClassID   string  `json:"class_id"`
	SectionID string  `json:"section_id"`
	RoutineID *string `json:"routine_id"`
}
type Subject struct {
	ID        string  `json:"id"`
	ExamID    string  `json:"exam_id"`
	SubjectID string  `json:"subject_id"`
	NameEN    string  `json:"name_en"`
	NameBN    string  `json:"name_bn"`
	FullMarks float64 `json:"full_marks"`
	PassMarks float64 `json:"pass_marks"`
	IsFourth  bool    `json:"is_fourth_subject"`
}
type SubjectInput struct {
	SubjectID string  `json:"subject_id"`
	FullMarks float64 `json:"full_marks"`
	PassMarks float64 `json:"pass_marks"`
}
type MarkInput struct {
	ExamSubjectID string  `json:"exam_subject_id"`
	StudentID     string  `json:"student_id"`
	Marks         float64 `json:"marks"`
}
type MarksInput struct {
	Entries []MarkInput `json:"entries"`
}
type MarkRow struct {
	ExamSubjectID string   `json:"exam_subject_id"`
	StudentID     string   `json:"student_id"`
	StudentName   string   `json:"student_name"`
	AdmissionNo   string   `json:"admission_no"`
	RollNo        *int     `json:"roll_no"`
	SubjectName   string   `json:"subject_name"`
	Marks         *float64 `json:"marks"`
	Grade         *string  `json:"grade"`
	GradePoint    *float64 `json:"grade_point"`
	Passed        *bool    `json:"passed"`
}
type Result struct {
	ExamID          string  `json:"exam_id"`
	StudentID       string  `json:"student_id"`
	StudentName     string  `json:"student_name"`
	StudentNameBN   string  `json:"student_name_bn"`
	AdmissionNo     string  `json:"admission_no"`
	RollNo          *int    `json:"roll_no"`
	TotalMarks      float64 `json:"total_marks"`
	FullMarks       float64 `json:"full_marks"`
	GPA             float64 `json:"gpa"`
	Passed          bool    `json:"passed"`
	ClassPosition   int     `json:"class_position"`
	SectionPosition int     `json:"section_position"`
}
type Card struct {
	Exam         Exam      `json:"exam"`
	Result       Result    `json:"result"`
	Subjects     []MarkRow `json:"subjects"`
	SchoolNameEN string    `json:"school_name_en"`
	SchoolNameBN string    `json:"school_name_bn"`
	LogoURL      string    `json:"logo_url"`
}
type Query struct {
	SessionID string
	ClassID   string
	SectionID string
	Search    string
	Page      int
	PageSize  int
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
type Band struct {
	Grade    string
	MinMarks int
	MaxMarks int
	GPA      float64
}
type ScoredSubject struct {
	Marks     float64
	FullMarks float64
	PassMarks float64
	Fourth    bool
}
type Grade struct {
	Name   string
	Point  float64
	Passed bool
}

func GradeFor(marks, full, pass float64, bands []Band) (Grade, error) {
	if !finiteMoney(marks) || !finiteMoney(full) || !finiteMoney(pass) || full <= 0 || marks > full || pass > full {
		return Grade{}, ErrInvalid
	}
	pct := marks * 100 / full
	for _, b := range bands {
		if pct >= float64(b.MinMarks) && (pct < float64(b.MaxMarks+1) || b.MaxMarks == 100 && pct <= 100) {
			return Grade{Name: b.Grade, Point: b.GPA, Passed: marks >= pass && b.GPA > 0}, nil
		}
	}
	return Grade{}, ErrInvalid
}
func GPA(subjects []ScoredSubject, bands []Band) (float64, bool, []Grade, error) {
	if len(subjects) == 0 {
		return 0, false, nil, ErrInvalid
	}
	required, optional := 0, 0
	sum, bonus, passed := 0.0, 0.0, true
	grades := make([]Grade, 0, len(subjects))
	for _, s := range subjects {
		g, err := GradeFor(s.Marks, s.FullMarks, s.PassMarks, bands)
		if err != nil {
			return 0, false, nil, err
		}
		grades = append(grades, g)
		if s.Fourth {
			optional++
			bonus = math.Max(0, g.Point-2)
			continue
		}
		required++
		sum += g.Point
		if !g.Passed {
			passed = false
		}
	}
	if required == 0 || optional > 1 {
		return 0, false, nil, ErrInvalid
	}
	if !passed {
		return 0, false, grades, nil
	}
	return math.Round(math.Min(5, (sum+bonus)/float64(required))*100) / 100, true, grades, nil
}
func finiteMoney(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && math.Abs(v*100-math.Round(v*100)) < 0.000001
}
func validQuery(q Query) bool {
	return q.Page >= 1 && q.Page <= 100000 && q.PageSize >= 1 && q.PageSize <= 100 && len(q.Search) <= 100 && utf8.ValidString(q.Search) && (q.SessionID == "" || uuid.MatchString(q.SessionID)) && (q.ClassID == "" || uuid.MatchString(q.ClassID)) && (q.SectionID == "" || uuid.MatchString(q.SectionID))
}

type Repository interface {
	SaveExam(context.Context, string, string, ExamInput) (Exam, error)
	DeleteExam(context.Context, string, string) error
	ListExams(context.Context, Query, string, string) (Page[Exam], error)
	GetExam(context.Context, string) (Exam, error)
	SaveSubject(context.Context, string, string, SubjectInput) (Exam, error)
	DeleteSubject(context.Context, string, string, string) error
	Roster(context.Context, string, string, string) ([]MarkRow, error)
	SaveMarks(context.Context, string, string, string, []MarkInput) error
	SetState(context.Context, string, string, string) (Exam, error)
	Tabulation(context.Context, string) ([]Result, error)
	Cards(context.Context, string, string, string, string) ([]Card, error)
	Card(context.Context, string, string, string, string) (Card, error)
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }
func (s *Service) SaveExam(ctx context.Context, actor, id string, in ExamInput) (Exam, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !uuid.MatchString(actor) || (id != "" && !uuid.MatchString(id)) || !uuid.MatchString(in.SessionID) || !uuid.MatchString(in.ClassID) || !uuid.MatchString(in.SectionID) || in.Name == "" || len(in.Name) > 120 || !utf8.ValidString(in.Name) {
		return Exam{}, ErrInvalid
	}
	if in.RoutineID != nil && *in.RoutineID != "" && !uuid.MatchString(*in.RoutineID) {
		return Exam{}, ErrInvalid
	}
	switch in.ExamType {
	case "class_test", "half_yearly", "annual", "other":
	default:
		return Exam{}, ErrInvalid
	}
	return s.repo.SaveExam(ctx, actor, id, in)
}
func (s *Service) DeleteExam(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteExam(ctx, actor, id)
}
func (s *Service) ListExams(ctx context.Context, q Query, scope, actor string) (Page[Exam], error) {
	if !validQuery(q) || !uuid.MatchString(actor) {
		return Page[Exam]{}, ErrInvalid
	}
	return s.repo.ListExams(ctx, q, scope, actor)
}
func (s *Service) GetExam(ctx context.Context, id string) (Exam, error) {
	if !uuid.MatchString(id) {
		return Exam{}, ErrInvalid
	}
	return s.repo.GetExam(ctx, id)
}
func (s *Service) SaveSubject(ctx context.Context, actor, examID string, in SubjectInput) (Exam, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !uuid.MatchString(in.SubjectID) || !finiteMoney(in.FullMarks) || in.FullMarks <= 0 || in.FullMarks > 1000 || !finiteMoney(in.PassMarks) || in.PassMarks > in.FullMarks {
		return Exam{}, ErrInvalid
	}
	return s.repo.SaveSubject(ctx, actor, examID, in)
}
func (s *Service) DeleteSubject(ctx context.Context, actor, examID, subjectID string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !uuid.MatchString(subjectID) {
		return ErrInvalid
	}
	return s.repo.DeleteSubject(ctx, actor, examID, subjectID)
}
func (s *Service) Roster(ctx context.Context, examID, actor, scope string) ([]MarkRow, error) {
	if !uuid.MatchString(examID) || !uuid.MatchString(actor) {
		return nil, ErrInvalid
	}
	return s.repo.Roster(ctx, examID, actor, scope)
}
func (s *Service) SaveMarks(ctx context.Context, actor, role, examID string, entries []MarkInput) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || len(entries) == 0 || len(entries) > 500 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, e := range entries {
		key := e.ExamSubjectID + ":" + e.StudentID
		if !uuid.MatchString(e.ExamSubjectID) || !uuid.MatchString(e.StudentID) || !finiteMoney(e.Marks) || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
	}
	return s.repo.SaveMarks(ctx, actor, role, examID, entries)
}
func (s *Service) SetState(ctx context.Context, actor, id, state string) (Exam, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || (state != "draft" && state != "published" && state != "locked") {
		return Exam{}, ErrInvalid
	}
	return s.repo.SetState(ctx, actor, id, state)
}
func (s *Service) Tabulation(ctx context.Context, id string) ([]Result, error) {
	if !uuid.MatchString(id) {
		return nil, ErrInvalid
	}
	return s.repo.Tabulation(ctx, id)
}
func (s *Service) Cards(ctx context.Context, scope, actor, studentID, examID string) ([]Card, error) {
	if !uuid.MatchString(actor) || (studentID != "" && !uuid.MatchString(studentID)) || (examID != "" && !uuid.MatchString(examID)) {
		return nil, ErrInvalid
	}
	return s.repo.Cards(ctx, scope, actor, studentID, examID)
}
func (s *Service) Card(ctx context.Context, scope, actor, studentID, examID string) (Card, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || (studentID != "" && !uuid.MatchString(studentID)) {
		return Card{}, ErrInvalid
	}
	return s.repo.Card(ctx, scope, actor, studentID, examID)
}
