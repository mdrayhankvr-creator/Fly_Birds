package rbac

import (
	"context"
	"errors"
	"testing"
)

type stubRepository struct{ called bool }

func (r *stubRepository) ListRoles(context.Context, string, int, int) ([]Role, int64, error) {
	r.called = true
	return []Role{}, 0, nil
}
func (r *stubRepository) ListPermissions(context.Context, string, int, int) ([]Permission, int64, error) {
	r.called = true
	return []Permission{}, 0, nil
}
func (r *stubRepository) RolePermissions(context.Context, string) ([]string, error) {
	r.called = true
	return []string{}, nil
}
func (r *stubRepository) UserRoles(context.Context, string) ([]string, error) {
	r.called = true
	return []string{}, nil
}
func (r *stubRepository) Matrix(context.Context) (Matrix, error) { return Matrix{}, nil }
func (r *stubRepository) History(context.Context, string, int, int) (Page[HistoryItem], error) {
	return Page[HistoryItem]{}, nil
}
func (r *stubRepository) ReplaceRolePermissions(context.Context, string, string, []string, []string) (Change, error) {
	r.called = true
	return Change{}, nil
}
func (r *stubRepository) ReplaceUserRole(context.Context, string, string, string) (Change, error) {
	r.called = true
	return Change{}, nil
}

func TestServiceRejectsInvalidInputBeforeRepository(t *testing.T) {
	const actor = "b006414c-38e0-4ba9-83d4-e47a2e9b203c"
	repo := &stubRepository{}
	svc := NewService(repo)
	cases := []func() error{
		func() error { _, err := svc.Roles(context.Background(), "", 0, 50); return err },
		func() error { _, err := svc.Permissions(context.Background(), "", 1, 101); return err },
		func() error { _, err := svc.RolePermissions(context.Background(), "bad/role"); return err },
		func() error { _, err := svc.UserRoles(context.Background(), "not-uuid"); return err },
		func() error {
			_, err := svc.SetRolePermissions(context.Background(), actor, "teacher", []string{"roles:read", "roles:read"}, nil)
			return err
		},
		func() error {
			_, err := svc.SetRolePermissions(context.Background(), actor, "teacher", nil, nil)
			return err
		},
		func() error {
			_, err := svc.SetUserRoles(context.Background(), actor, actor, []string{"teacher", "admin"})
			return err
		},
	}
	for _, run := range cases {
		if err := run(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	}
	if repo.called {
		t.Fatal("repository called for invalid input")
	}
}
