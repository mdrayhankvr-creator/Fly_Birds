package routine

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid routine input")
	ErrNotFound  = errors.New("routine not found")
	ErrConflict  = errors.New("routine schedule conflict")
	ErrForbidden = errors.New("routine access denied")
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ClassSlot struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	ClassID       string `json:"class_id"`
	SectionID     string `json:"section_id"`
	SubjectID     string `json:"subject_id"`
	TeacherID     string `json:"teacher_id"`
	DayOfWeek     int    `json:"day_of_week"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	Room          string `json:"room"`
	ClassName     string `json:"class_name"`
	SectionName   string `json:"section_name"`
	SubjectName   string `json:"subject_name"`
	SubjectNameBN string `json:"subject_name_bn"`
	TeacherName   string `json:"teacher_name"`
}
type ClassInput struct {
	SessionID string `json:"session_id"`
	ClassID   string `json:"class_id"`
	SectionID string `json:"section_id"`
	SubjectID string `json:"subject_id"`
	TeacherID string `json:"teacher_id"`
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Room      string `json:"room"`
}
type Exam struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	SessionID   string     `json:"session_id"`
	ClassID     string     `json:"class_id"`
	SectionID   string     `json:"section_id"`
	ClassName   string     `json:"class_name"`
	SectionName string     `json:"section_name"`
	IsPublished bool       `json:"is_published"`
	Slots       []ExamSlot `json:"slots" gorm:"-"`
}
type ExamInput struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	ClassID   string `json:"class_id"`
	SectionID string `json:"section_id"`
}
type ExamSlot struct {
	ID            string `json:"id"`
	RoutineID     string `json:"routine_id"`
	SubjectID     string `json:"subject_id"`
	SubjectName   string `json:"subject_name"`
	SubjectNameBN string `json:"subject_name_bn"`
	ExamDate      string `json:"exam_date"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	Room          string `json:"room"`
}
type ExamSlotInput struct {
	SubjectID string `json:"subject_id"`
	ExamDate  string `json:"exam_date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Room      string `json:"room"`
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

type Repository interface {
	SaveClass(context.Context, string, string, ClassInput, int, int) (ClassSlot, error)
	DeleteClass(context.Context, string, string) error
	ListClasses(context.Context, Query, string, string, string) (Page[ClassSlot], error)
	SaveExam(context.Context, string, string, ExamInput) (Exam, error)
	PublishExam(context.Context, string, string, bool) (Exam, error)
	DeleteExam(context.Context, string, string) error
	ListExams(context.Context, Query, string, string, string) (Page[Exam], error)
	GetExam(context.Context, string, string, string, string) (Exam, error)
	SaveExamSlot(context.Context, string, string, string, ExamSlotInput, int, int) (Exam, error)
	DeleteExamSlot(context.Context, string, string, string) error
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func minute(v string) (int, bool) {
	t, err := time.Parse("15:04", v)
	if err != nil || t.Format("15:04") != v {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
func times(start, end string) (int, int, bool) {
	a, okA := minute(start)
	b, okB := minute(end)
	return a, b, okA && okB && b > a
}
func room(v string) (string, bool) {
	v = strings.TrimSpace(v)
	return v, v != "" && utf8.ValidString(v) && len(v) <= 80
}
func validQuery(q Query) bool {
	return q.Page >= 1 && q.Page <= 100000 && q.PageSize >= 1 && q.PageSize <= 100 && len(q.Search) <= 100 && utf8.ValidString(q.Search) && (q.SessionID == "" || uuid.MatchString(q.SessionID)) && (q.ClassID == "" || uuid.MatchString(q.ClassID)) && (q.SectionID == "" || uuid.MatchString(q.SectionID))
}
func (s *Service) SaveClass(ctx context.Context, actor, id string, in ClassInput) (ClassSlot, error) {
	a, b, ok := times(in.StartTime, in.EndTime)
	var okRoom bool
	in.Room, okRoom = room(in.Room)
	if !uuid.MatchString(actor) || (id != "" && !uuid.MatchString(id)) || !uuid.MatchString(in.SessionID) || !uuid.MatchString(in.ClassID) || !uuid.MatchString(in.SectionID) || !uuid.MatchString(in.SubjectID) || !uuid.MatchString(in.TeacherID) || in.DayOfWeek < 0 || in.DayOfWeek > 6 || !ok || !okRoom {
		return ClassSlot{}, ErrInvalid
	}
	return s.repo.SaveClass(ctx, actor, id, in, a, b)
}
func (s *Service) DeleteClass(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteClass(ctx, actor, id)
}
func (s *Service) ListClasses(ctx context.Context, q Query, scope, actor, studentID string) (Page[ClassSlot], error) {
	q.Search = strings.TrimSpace(q.Search)
	if !validQuery(q) || (scope != "admin" && scope != "teacher" && scope != "student" && scope != "guardian") || (scope != "admin" && !uuid.MatchString(actor)) || (scope == "guardian" && !uuid.MatchString(studentID)) {
		return Page[ClassSlot]{}, ErrInvalid
	}
	return s.repo.ListClasses(ctx, q, scope, actor, studentID)
}
func (s *Service) SaveExam(ctx context.Context, actor, id string, in ExamInput) (Exam, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !uuid.MatchString(actor) || (id != "" && !uuid.MatchString(id)) || !uuid.MatchString(in.SessionID) || !uuid.MatchString(in.ClassID) || !uuid.MatchString(in.SectionID) || in.Name == "" || len(in.Name) > 120 || !utf8.ValidString(in.Name) {
		return Exam{}, ErrInvalid
	}
	return s.repo.SaveExam(ctx, actor, id, in)
}
func (s *Service) PublishExam(ctx context.Context, actor, id string, published bool) (Exam, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return Exam{}, ErrInvalid
	}
	return s.repo.PublishExam(ctx, actor, id, published)
}
func (s *Service) DeleteExam(ctx context.Context, actor, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteExam(ctx, actor, id)
}
func (s *Service) ListExams(ctx context.Context, q Query, scope, actor, studentID string) (Page[Exam], error) {
	q.Search = strings.TrimSpace(q.Search)
	if !validQuery(q) || (scope != "admin" && scope != "student" && scope != "guardian") || (scope != "admin" && !uuid.MatchString(actor)) || (scope == "guardian" && !uuid.MatchString(studentID)) {
		return Page[Exam]{}, ErrInvalid
	}
	return s.repo.ListExams(ctx, q, scope, actor, studentID)
}
func (s *Service) GetExam(ctx context.Context, id, scope, actor, studentID string) (Exam, error) {
	if !uuid.MatchString(id) || (scope != "admin" && scope != "student" && scope != "guardian") || (scope != "admin" && !uuid.MatchString(actor)) || (scope == "guardian" && !uuid.MatchString(studentID)) {
		return Exam{}, ErrInvalid
	}
	return s.repo.GetExam(ctx, id, scope, actor, studentID)
}
func (s *Service) SaveExamSlot(ctx context.Context, actor, routineID, id string, in ExamSlotInput) (Exam, error) {
	a, b, ok := times(in.StartTime, in.EndTime)
	v, okRoom := room(in.Room)
	in.Room = v
	d, err := time.Parse("2006-01-02", in.ExamDate)
	if !uuid.MatchString(actor) || !uuid.MatchString(routineID) || (id != "" && !uuid.MatchString(id)) || !uuid.MatchString(in.SubjectID) || err != nil || d.Format("2006-01-02") != in.ExamDate || !ok || !okRoom {
		return Exam{}, ErrInvalid
	}
	return s.repo.SaveExamSlot(ctx, actor, routineID, id, in, a, b)
}
func (s *Service) DeleteExamSlot(ctx context.Context, actor, routineID, id string) error {
	if !uuid.MatchString(actor) || !uuid.MatchString(routineID) || !uuid.MatchString(id) {
		return ErrInvalid
	}
	return s.repo.DeleteExamSlot(ctx, actor, routineID, id)
}
