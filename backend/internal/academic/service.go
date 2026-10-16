package academic

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid academic input")
	ErrNotFound  = errors.New("academic record not found")
	ErrDuplicate = errors.New("academic record already exists")
	ErrForbidden = errors.New("academic change denied")
	ErrInactive  = errors.New("class or subject is inactive")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var codePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)

type Class struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}
type Subject struct {
	ID       string `json:"id"`
	NameEN   string `json:"name_en"`
	NameBN   string `json:"name_bn"`
	Code     string `json:"code"`
	IsActive bool   `json:"is_active"`
}
type Mapping struct {
	ID              string `json:"id"`
	ClassID         string `json:"class_id"`
	SubjectID       string `json:"subject_id"`
	IsRequired      bool   `json:"is_required"`
	IsFourthSubject bool   `json:"is_fourth_subject"`
	SubjectNameEN   string `json:"subject_name_en"`
	SubjectNameBN   string `json:"subject_name_bn"`
	SubjectCode     string `json:"subject_code"`
}
type ClassInput struct {
	Name         string `json:"name"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}
type SubjectInput struct {
	NameEN   string `json:"name_en"`
	NameBN   string `json:"name_bn"`
	Code     string `json:"code"`
	IsActive *bool  `json:"is_active"`
}
type MappingInput struct {
	SubjectID       string `json:"subject_id"`
	IsRequired      *bool  `json:"is_required"`
	IsFourthSubject *bool  `json:"is_fourth_subject"`
}
type Query struct {
	Search   string
	Page     int
	PageSize int
	Status   string
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
type Repository interface {
	ListClasses(context.Context, Query) (Page[Class], error)
	GetClass(context.Context, string) (Class, error)
	SaveClass(context.Context, string, string, Class) (Class, error)
	ListSubjects(context.Context, Query) (Page[Subject], error)
	GetSubject(context.Context, string) (Subject, error)
	SaveSubject(context.Context, string, string, Subject) (Subject, error)
	ListMappings(context.Context, string, Query) (Page[Mapping], error)
	CreateMapping(context.Context, string, Mapping) (Mapping, error)
	UpdateMapping(context.Context, string, Mapping) (Mapping, error)
	DeleteMapping(context.Context, string, string, string) error
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validText(value string, maxBytes int, required bool) bool {
	value = strings.TrimSpace(value)
	return utf8.ValidString(value) && len(value) <= maxBytes && (!required || value != "")
}
func validQuery(q Query) bool {
	return q.Page > 0 && q.Page <= 100000 && q.PageSize > 0 && q.PageSize <= 100 && validText(q.Search, 100, false) && (q.Status == "" || q.Status == "active" || q.Status == "inactive")
}
func (s *Service) ListClasses(ctx context.Context, q Query) (Page[Class], error) {
	if !validQuery(q) {
		return Page[Class]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListClasses(ctx, q)
}
func (s *Service) ListSubjects(ctx context.Context, q Query) (Page[Subject], error) {
	if !validQuery(q) {
		return Page[Subject]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListSubjects(ctx, q)
}
func (s *Service) GetClass(ctx context.Context, id string) (Class, error) {
	if !uuidPattern.MatchString(id) {
		return Class{}, ErrInvalid
	}
	return s.repo.GetClass(ctx, id)
}
func (s *Service) GetSubject(ctx context.Context, id string) (Subject, error) {
	if !uuidPattern.MatchString(id) {
		return Subject{}, ErrInvalid
	}
	return s.repo.GetSubject(ctx, id)
}
func (s *Service) SaveClass(ctx context.Context, actor, id string, input ClassInput) (Class, error) {
	if !uuidPattern.MatchString(actor) || (id != "" && !uuidPattern.MatchString(id)) || !validText(input.Name, 120, true) || input.DisplayOrder < 0 || input.DisplayOrder > 100000 || input.IsActive == nil {
		return Class{}, ErrInvalid
	}
	return s.repo.SaveClass(ctx, actor, id, Class{Name: strings.TrimSpace(input.Name), DisplayOrder: input.DisplayOrder, IsActive: *input.IsActive})
}
func (s *Service) SaveSubject(ctx context.Context, actor, id string, input SubjectInput) (Subject, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if !uuidPattern.MatchString(actor) || (id != "" && !uuidPattern.MatchString(id)) || !validText(input.NameEN, 160, true) || !validText(input.NameBN, 160, true) || !codePattern.MatchString(code) || input.IsActive == nil {
		return Subject{}, ErrInvalid
	}
	return s.repo.SaveSubject(ctx, actor, id, Subject{NameEN: strings.TrimSpace(input.NameEN), NameBN: strings.TrimSpace(input.NameBN), Code: code, IsActive: *input.IsActive})
}
func validSemantics(required, fourth *bool) bool {
	return required != nil && fourth != nil && *required != *fourth
}
func (s *Service) ListMappings(ctx context.Context, classID string, q Query) (Page[Mapping], error) {
	if !uuidPattern.MatchString(classID) || !validQuery(q) || q.Status != "" {
		return Page[Mapping]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListMappings(ctx, classID, q)
}
func (s *Service) CreateMapping(ctx context.Context, actor, classID string, input MappingInput) (Mapping, error) {
	if !uuidPattern.MatchString(actor) || !uuidPattern.MatchString(classID) || !uuidPattern.MatchString(input.SubjectID) || !validSemantics(input.IsRequired, input.IsFourthSubject) {
		return Mapping{}, ErrInvalid
	}
	return s.repo.CreateMapping(ctx, actor, Mapping{ClassID: classID, SubjectID: input.SubjectID, IsRequired: *input.IsRequired, IsFourthSubject: *input.IsFourthSubject})
}
func (s *Service) UpdateMapping(ctx context.Context, actor, classID, subjectID string, input MappingInput) (Mapping, error) {
	if !uuidPattern.MatchString(actor) || !uuidPattern.MatchString(classID) || !uuidPattern.MatchString(subjectID) || (input.SubjectID != "" && input.SubjectID != subjectID) || !validSemantics(input.IsRequired, input.IsFourthSubject) {
		return Mapping{}, ErrInvalid
	}
	return s.repo.UpdateMapping(ctx, actor, Mapping{ClassID: classID, SubjectID: subjectID, IsRequired: *input.IsRequired, IsFourthSubject: *input.IsFourthSubject})
}
func (s *Service) DeleteMapping(ctx context.Context, actor, classID, subjectID string) error {
	if !uuidPattern.MatchString(actor) || !uuidPattern.MatchString(classID) || !uuidPattern.MatchString(subjectID) {
		return ErrInvalid
	}
	return s.repo.DeleteMapping(ctx, actor, classID, subjectID)
}
