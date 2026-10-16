package academic

import (
	"context"
	"strings"
)

type Section struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	ClassID      string `json:"class_id"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
	SessionName  string `json:"session_name"`
	ClassName    string `json:"class_name"`
}

type SectionInput struct {
	SessionID    string `json:"session_id"`
	ClassID      string `json:"class_id"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

type SectionQuery struct {
	Query
	SessionID string
	ClassID   string
}

type SectionRepository interface {
	ListSections(context.Context, SectionQuery) (Page[Section], error)
	GetSection(context.Context, string) (Section, error)
	SaveSection(context.Context, string, string, Section) (Section, error)
}

type SectionService struct{ repo SectionRepository }

func NewSectionService(repo SectionRepository) *SectionService { return &SectionService{repo: repo} }

func validOptionalID(value string) bool { return value == "" || uuidPattern.MatchString(value) }

func (s *SectionService) List(ctx context.Context, q SectionQuery) (Page[Section], error) {
	if !validQuery(q.Query) || !validOptionalID(q.SessionID) || !validOptionalID(q.ClassID) {
		return Page[Section]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListSections(ctx, q)
}

func (s *SectionService) Get(ctx context.Context, id string) (Section, error) {
	if !uuidPattern.MatchString(id) {
		return Section{}, ErrInvalid
	}
	return s.repo.GetSection(ctx, id)
}

func (s *SectionService) Save(ctx context.Context, actor, id string, input SectionInput) (Section, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if !uuidPattern.MatchString(actor) || !validOptionalID(id) || !uuidPattern.MatchString(input.SessionID) || !uuidPattern.MatchString(input.ClassID) || !validText(input.Name, 120, true) || !codePattern.MatchString(code) || input.DisplayOrder < 0 || input.DisplayOrder > 100000 || input.IsActive == nil {
		return Section{}, ErrInvalid
	}
	return s.repo.SaveSection(ctx, actor, id, Section{
		SessionID: input.SessionID, ClassID: input.ClassID,
		Name: strings.TrimSpace(input.Name), Code: code,
		DisplayOrder: input.DisplayOrder, IsActive: *input.IsActive,
	})
}
