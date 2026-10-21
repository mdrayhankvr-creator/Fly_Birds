package blueprints

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

// Service checks input and delegates to the repository.
type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (s *Service) Terms(ctx context.Context, sessionID string) ([]TermView, error) {
	if !isUUID(sessionID) {
		return nil, ErrInvalid
	}
	return s.repo.Terms(ctx, sessionID)
}

func (s *Service) SaveTerm(ctx context.Context, actor, id string, in TermInput) (TermView, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !isUUID(actor) || (id != "" && !isUUID(id)) || (id == "" && !isUUID(in.SessionID)) || in.Name == "" || utf8.RuneCountInString(in.Name) > 120 ||
		(in.Kind != "term" && in.Kind != "annual") || !validDate(in.StartsOn) || !validDate(in.EndsOn) ||
		(in.StartsOn != "" && in.EndsOn != "" && in.EndsOn < in.StartsOn) {
		return TermView{}, ErrInvalid
	}
	return s.repo.SaveTerm(ctx, actor, id, in)
}

func (s *Service) DeleteTerm(ctx context.Context, actor, id string) error {
	if !isUUID(actor) || !isUUID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteTerm(ctx, actor, id)
}

func (s *Service) ClassBlueprints(ctx context.Context, sessionID string) ([]ClassBlueprint, error) {
	if !isUUID(sessionID) {
		return nil, ErrInvalid
	}
	return s.repo.ClassBlueprints(ctx, sessionID)
}

func (s *Service) CreateBlueprint(ctx context.Context, actor, sessionID, classID, copyFrom string) (VersionView, error) {
	if !isUUID(actor) || !isUUID(sessionID) || !isUUID(classID) || (copyFrom != "" && !isUUID(copyFrom)) {
		return VersionView{}, ErrInvalid
	}
	return s.repo.CreateBlueprint(ctx, actor, sessionID, classID, copyFrom)
}

func (s *Service) Version(ctx context.Context, id string) (VersionView, error) {
	if !isUUID(id) {
		return VersionView{}, ErrInvalid
	}
	return s.repo.Version(ctx, id)
}

func (s *Service) NewDraft(ctx context.Context, actor, blueprintID, from string) (VersionView, error) {
	if !isUUID(actor) || !isUUID(blueprintID) || !isUUID(from) {
		return VersionView{}, ErrInvalid
	}
	return s.repo.NewDraft(ctx, actor, blueprintID, from)
}

// maxDefinition bounds a definition's size (a large school: 30 subjects ×
// 12 assessments × 6 components is well under it).
const maxDefinition = 4000

func (s *Service) SaveDraft(ctx context.Context, actor, id string, d Definition) (VersionView, error) {
	if !isUUID(actor) || !isUUID(id) || len(d.Assessments) > 60 || len(d.Subjects) > 60 {
		return VersionView{}, ErrInvalid
	}
	items := 0
	for _, sub := range d.Subjects {
		items += len(sub.Sheets) + len(sub.Terms)
		for _, sh := range sub.Sheets {
			items += len(sh.Components)
		}
		for _, t := range sub.Terms {
			for _, c := range t.Contributions {
				items += len(c.From)
			}
		}
	}
	if items > maxDefinition {
		return VersionView{}, ErrInvalid
	}
	return s.repo.SaveDraft(ctx, actor, id, d)
}

func (s *Service) DeleteDraft(ctx context.Context, actor, id string) error {
	if !isUUID(actor) || !isUUID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteDraft(ctx, actor, id)
}

func (s *Service) Activate(ctx context.Context, actor, id string) (VersionView, error) {
	if !isUUID(actor) || !isUUID(id) {
		return VersionView{}, ErrInvalid
	}
	return s.repo.Activate(ctx, actor, id)
}

func (s *Service) SetVisibility(ctx context.Context, actor, id, key, value string) (VersionView, error) {
	if !isUUID(actor) || !isUUID(id) || !keyPattern.MatchString(key) || !visibility[value] {
		return VersionView{}, ErrInvalid
	}
	return s.repo.SetVisibility(ctx, actor, id, key, value)
}

func (s *Service) Choices(ctx context.Context, sessionID, classID string) (ChoicesView, error) {
	if !isUUID(sessionID) || !isUUID(classID) {
		return ChoicesView{}, ErrInvalid
	}
	return s.repo.Choices(ctx, sessionID, classID)
}

func (s *Service) SaveChoices(ctx context.Context, actor, sessionID, classID string, choices []Choice) (ChoicesView, error) {
	if !isUUID(actor) || !isUUID(sessionID) || !isUUID(classID) || len(choices) == 0 || len(choices) > 500 {
		return ChoicesView{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range choices {
		if !isUUID(c.StudentID) || (c.SubjectID != "" && !isUUID(c.SubjectID)) || seen[c.StudentID] {
			return ChoicesView{}, ErrInvalid
		}
		seen[c.StudentID] = true
	}
	return s.repo.SaveChoices(ctx, actor, sessionID, classID, choices)
}

// SampleMark is one sample mark for the worked example in the builder.
type SampleMark struct {
	Status     string             `json:"status"`
	Marks      *float64           `json:"marks"`
	Components map[string]float64 `json:"components"`
}

// SampleRequest asks how a subject's term mark comes out for sample marks.
type SampleRequest struct {
	SubjectID  string                `json:"subject_id"`
	TermID     string                `json:"term_id"`
	Definition *Definition           `json:"definition"` // the unsaved draft, if any
	Marks      map[string]SampleMark `json:"marks"`      // by assessment key
}

// Sample works one subject's term mark through the exact engine, with every
// contribution shown, so the builder can explain the rules on real numbers.
func (s *Service) Sample(ctx context.Context, versionID string, req SampleRequest) (SubjectResult, error) {
	if !isUUID(versionID) || len(req.Marks) > 60 {
		return SubjectResult{}, ErrInvalid
	}
	v, err := s.repo.Version(ctx, versionID)
	if err != nil {
		return SubjectResult{}, err
	}
	d := v.Definition
	if req.Definition != nil && v.Status == "draft" {
		d = *req.Definition
		d.Defaults()
	}
	bands := []calc.Band{}
	for _, b := range v.Bands {
		p, err := calc.PointsOf(b.GPA)
		if err != nil {
			return SubjectResult{}, err
		}
		bands = append(bands, calc.Band{Grade: b.Grade, Min: b.Min, Max: b.Max, Point: p})
	}
	scheme, err := calc.NewScheme(bands)
	if err != nil {
		return SubjectResult{}, ErrInvalid
	}
	return SampleSubject(d, req.SubjectID, req.TermID, req.Marks, scheme)
}
