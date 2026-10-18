package school

import (
	"context"
	"regexp"
	"strings"
)

var actorID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Service struct{ repo Repository }

func NewService(repo Repository) *Service                               { return &Service{repo: repo} }
func (s *Service) Settings(ctx context.Context) (SchoolSettings, error) { return s.repo.Settings(ctx) }
func (s *Service) Branding(ctx context.Context) (Branding, error) {
	settings, err := s.repo.Settings(ctx)
	return settings.Branding(), err
}
func (s *Service) SaveSettings(ctx context.Context, actor string, input SettingsInput) (SchoolSettings, error) {
	if !actorID.MatchString(actor) {
		return SchoolSettings{}, ErrInvalid
	}
	validated, err := ValidateSettings(input)
	if err != nil {
		return SchoolSettings{}, err
	}
	return s.repo.UpdateSettings(ctx, actor, validated)
}
func (s *Service) Grading(ctx context.Context) ([]GradeBand, error) { return s.repo.Grading(ctx) }
func (s *Service) SaveGrading(ctx context.Context, actor string, input []GradeBand) ([]GradeBand, error) {
	if !actorID.MatchString(actor) {
		return nil, ErrInvalid
	}
	bands, err := ValidateBands(input)
	if err != nil {
		return nil, err
	}
	return s.repo.ReplaceGrading(ctx, actor, bands)
}
func (s *Service) Modules(ctx context.Context, search string, page, size int) (ModulePage, error) {
	if page < 1 || page > 100000 || size < 1 || size > 100 || len(search) > 100 {
		return ModulePage{}, ErrInvalid
	}
	return s.repo.Modules(ctx, search, page, size)
}
func (s *Service) Enabled(ctx context.Context, key string) (bool, error) {
	if !ValidModuleKey(key) {
		return false, ErrInvalid
	}
	return s.repo.ModuleEnabled(ctx, key)
}
// SetModule enables or disables a module. Without cascade it is refused when
// prerequisites are off or enabled modules depend on it; with cascade those
// modules are switched too, in one audited transaction.
func (s *Service) SetModule(ctx context.Context, actor, key string, enabled, cascade bool) (Module, error) {
	if !actorID.MatchString(actor) || !ValidModuleKey(key) {
		return Module{}, ErrInvalid
	}
	return s.repo.UpdateModule(ctx, actor, key, enabled, cascade)
}

func containsFold(value, search string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(search))
}
