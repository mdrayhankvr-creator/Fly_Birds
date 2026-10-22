package exams

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

var (
	ErrInvalid   = errors.New("invalid examination input")
	ErrNotFound  = errors.New("examination not found")
	ErrConflict  = errors.New("examination conflict")
	ErrForbidden = errors.New("examination access denied")
)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Exam struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ExamType    string  `json:"exam_type"`
	SessionID   string  `json:"session_id"`
	ClassID     string  `json:"class_id"`
	SectionID   string  `json:"section_id"`
	RoutineID   *string `json:"routine_id"`
	Status      string  `json:"status"`
	SessionName string  `json:"session_name"`
	ClassName   string  `json:"class_name"`
	SectionName string  `json:"section_name"`
	// E5: the term and assessment the exam belongs to, and the blueprint
	// version it was created from (all empty for exams made outside a term).
	TermID             *string   `json:"term_id"`
	TermName           *string   `json:"term_name"`
	AssessmentKey      *string   `json:"assessment_key"`
	BlueprintVersionID *string   `json:"blueprint_version_id"`
	MarksDueOn         *string   `json:"marks_due_on"`
	Subjects           []Subject `json:"subjects" gorm:"-"`
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
	SubjectNameBN string   `json:"subject_name_bn"`
	Marks         *float64 `json:"marks"`
	Grade         *string  `json:"grade"`
	GradePoint    *float64 `json:"grade_point"`
	Passed        *bool    `json:"passed"`
	// Status is empty when no mark has been entered (E3).
	Status       string `json:"status"`
	StatusReason string `json:"status_reason"`
	RowVersion   int    `json:"row_version"`
	// Components of a component sheet's mark (E5).
	Components map[string]float64 `json:"components,omitempty" gorm:"-"`
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
	// Withheld: the school holds this result back; no marks are included.
	Withheld     bool      `json:"withheld"`
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

// GradeFor and GPA keep their float signatures for callers, but grade
// through the exact engine in internal/results/calc: band edges are compared
// in integer hundredths, so a mark exactly on an edge (16.83 of 51 is 33 %)
// gets that band.
func GradeFor(marks, full, pass float64, bands []Band) (Grade, error) {
	scheme, err := exactScheme(bands)
	if err != nil {
		return Grade{}, err
	}
	m, f, p, err := exactMarks(marks, full, pass)
	if err != nil {
		return Grade{}, err
	}
	g, err := calc.GradeSubject(m, f, p, scheme)
	if err != nil {
		return Grade{}, ErrInvalid
	}
	return Grade{Name: g.Grade, Point: g.Point.Float(), Passed: g.Passed}, nil
}
func GPA(subjects []ScoredSubject, bands []Band) (float64, bool, []Grade, error) {
	r, err := Score(subjects, bands)
	if err != nil {
		return 0, false, nil, err
	}
	return r.GPA.Float(), r.Passed, gradesOf(r), nil
}

// Score is GPA with exact totals, for the publish path.
func Score(subjects []ScoredSubject, bands []Band) (calc.Result, error) {
	scheme, err := exactScheme(bands)
	if err != nil {
		return calc.Result{}, err
	}
	in := make([]calc.Subject, 0, len(subjects))
	for _, s := range subjects {
		m, f, p, err := exactMarks(s.Marks, s.FullMarks, s.PassMarks)
		if err != nil {
			return calc.Result{}, err
		}
		in = append(in, calc.Subject{Marks: m, Full: f, Pass: p, Optional: s.Fourth})
	}
	r, err := calc.Calculate(in, scheme, calc.Policy{})
	if err != nil {
		return calc.Result{}, ErrInvalid
	}
	return r, nil
}

func gradesOf(r calc.Result) []Grade {
	out := make([]Grade, 0, len(r.Grades))
	for _, g := range r.Grades {
		out = append(out, Grade{Name: g.Grade, Point: g.Point.Float(), Passed: g.Passed})
	}
	return out
}

func exactScheme(bands []Band) (calc.Scheme, error) {
	in := make([]calc.Band, 0, len(bands))
	for _, b := range bands {
		p, err := calc.PointsOf(b.GPA)
		if err != nil {
			return calc.Scheme{}, ErrInvalid
		}
		in = append(in, calc.Band{Grade: b.Grade, Min: b.MinMarks, Max: b.MaxMarks, Point: p})
	}
	s, err := calc.NewScheme(in)
	if err != nil {
		return calc.Scheme{}, ErrInvalid
	}
	return s, nil
}

func exactMarks(marks, full, pass float64) (calc.Marks, calc.Marks, calc.Marks, error) {
	m, err1 := calc.MarksOf(marks)
	f, err2 := calc.MarksOf(full)
	p, err3 := calc.MarksOf(pass)
	if err1 != nil || err2 != nil || err3 != nil || f <= 0 || m > f || p > f {
		return 0, 0, 0, ErrInvalid
	}
	return m, f, p, nil
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
	SaveMarkEntries(context.Context, string, string, string, []MarkEntry, bool) (MarksResult, error)
	Sheets(context.Context, string, string, string) (SheetsView, error)
	SheetAction(context.Context, string, string, string, string, string, string) (Sheet, error)
	MySheets(context.Context, string) ([]QueueItem, error)
	Workspace(context.Context, string, string, string, string, string) (Workspace, error)
	SheetHistory(context.Context, string, string) ([]HistoryEntry, error)
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
	// The pre-E3 contract: present marks, all or nothing, no version check.
	in := make([]MarkEntry, 0, len(entries))
	for _, e := range entries {
		marks := e.Marks
		in = append(in, MarkEntry{ExamSubjectID: e.ExamSubjectID, StudentID: e.StudentID, Status: StatusPresent, Marks: &marks})
	}
	_, err := s.repo.SaveMarkEntries(ctx, actor, role, examID, in, true)
	return err
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
