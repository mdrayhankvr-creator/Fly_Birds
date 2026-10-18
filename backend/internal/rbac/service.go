package rbac

import (
	"context"
	"errors"
	"regexp"
)

var (
	ErrInvalid   = errors.New("invalid authorization input")
	ErrNotFound  = errors.New("authorization resource not found")
	ErrForbidden = errors.New("authorization denied")
	ErrLastAdmin = errors.New("last authorized administrator must remain")
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*(?::[a-z][a-z0-9_]*)?$`)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Permission struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
type Change struct {
	ActorID  string
	TargetID string
	Before   []string
	After    []string
}

type Repository interface {
	ListRoles(context.Context, string, int, int) ([]Role, int64, error)
	ListPermissions(context.Context, string, int, int) ([]Permission, int64, error)
	RolePermissions(context.Context, string) ([]string, error)
	UserRoles(context.Context, string) ([]string, error)
	ReplaceRolePermissions(context.Context, string, string, []string, []string) (Change, error)
	Matrix(context.Context) (Matrix, error)
	History(context.Context, string, int, int) (Page[HistoryItem], error)
	ReplaceUserRole(context.Context, string, string, string) (Change, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validatePage(page, size int) error {
	if page < 1 || page > 100000 || size < 1 || size > 100 {
		return ErrInvalid
	}
	return nil
}
func (s *Service) Roles(ctx context.Context, search string, page, size int) (Page[Role], error) {
	if validatePage(page, size) != nil || len(search) > 100 {
		return Page[Role]{}, ErrInvalid
	}
	items, total, err := s.repo.ListRoles(ctx, search, page, size)
	return Page[Role]{Items: items, Total: total, Page: page, PageSize: size}, err
}
func (s *Service) Permissions(ctx context.Context, search string, page, size int) (Page[Permission], error) {
	if validatePage(page, size) != nil || len(search) > 100 {
		return Page[Permission]{}, ErrInvalid
	}
	items, total, err := s.repo.ListPermissions(ctx, search, page, size)
	return Page[Permission]{Items: items, Total: total, Page: page, PageSize: size}, err
}
func (s *Service) RolePermissions(ctx context.Context, role string) ([]string, error) {
	if !identifier.MatchString(role) {
		return nil, ErrInvalid
	}
	return s.repo.RolePermissions(ctx, role)
}
func (s *Service) UserRoles(ctx context.Context, user string) ([]string, error) {
	if !uuid.MatchString(user) {
		return nil, ErrInvalid
	}
	return s.repo.UserRoles(ctx, user)
}
func (s *Service) SetRolePermissions(ctx context.Context, actor, role string, ids, expected []string) (Change, error) {
	if !uuid.MatchString(actor) || !identifier.MatchString(role) || ids == nil || len(ids) > 200 || len(expected) > 200 {
		return Change{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !identifier.MatchString(id) || seen[id] {
			return Change{}, ErrInvalid
		}
		seen[id] = true
	}
	for _, id := range expected {
		if !identifier.MatchString(id) {
			return Change{}, ErrInvalid
		}
	}
	return s.repo.ReplaceRolePermissions(ctx, actor, role, ids, expected)
}

// Matrix returns every role, permission and grant for the permission editor.
func (s *Service) Matrix(ctx context.Context) (Matrix, error) { return s.repo.Matrix(ctx) }

// History lists saved permission changes for one role, newest first.
func (s *Service) History(ctx context.Context, role string, page, size int) (Page[HistoryItem], error) {
	if !identifier.MatchString(role) {
		return Page[HistoryItem]{}, ErrInvalid
	}
	if err := validatePage(page, size); err != nil {
		return Page[HistoryItem]{}, err
	}
	return s.repo.History(ctx, role, page, size)
}
func (s *Service) SetUserRoles(ctx context.Context, actor, user string, ids []string) (Change, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(user) || len(ids) != 1 || !identifier.MatchString(ids[0]) {
		return Change{}, ErrInvalid
	}
	return s.repo.ReplaceUserRole(ctx, actor, user, ids[0])
}
