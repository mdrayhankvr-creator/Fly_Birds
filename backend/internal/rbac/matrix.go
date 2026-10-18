package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// ErrStale rejects a permission save whose expected grants no longer match,
// so two administrators cannot silently overwrite each other's changes.
var ErrStale = errors.New("role permissions changed since they were loaded")

// ProtectedGrants cannot be removed: without them no administrator could
// manage permissions or even sign in to the management pages.
var ProtectedGrants = map[string][]string{"admin": {"auth:read_self", "permissions:manage"}}

type MatrixRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ActiveUsers int64  `json:"active_users"`
}
type MatrixPermission struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

// Matrix is the full roles × permissions grid in one response.
type Matrix struct {
	Roles       []MatrixRole        `json:"roles"`
	Permissions []MatrixPermission  `json:"permissions"`
	Grants      map[string][]string `json:"grants"`
	Protected   map[string][]string `json:"protected"`
}

type HistoryItem struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"created_at"`
	ActorName string   `json:"actor_name"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
}

// GroupOf returns the permission area, the part before the colon.
func GroupOf(permission string) string {
	group, _, _ := strings.Cut(permission, ":")
	return group
}

func (r *GormRepository) Matrix(ctx context.Context) (Matrix, error) {
	db := r.db.WithContext(ctx)
	out := Matrix{Roles: []MatrixRole{}, Permissions: []MatrixPermission{}, Grants: map[string][]string{}, Protected: ProtectedGrants}
	if err := db.Raw(`SELECT r.id, r.name, count(u.id) FILTER (WHERE u.is_active) AS active_users
FROM roles r LEFT JOIN users u ON u.role = r.id GROUP BY r.id, r.name
ORDER BY array_position(ARRAY['admin','head_teacher','teacher','student','guardian','accountant'], r.id), r.id`).Scan(&out.Roles).Error; err != nil {
		return out, err
	}
	if err := db.Table("permissions").Select("id, description").Order("id").Scan(&out.Permissions).Error; err != nil {
		return out, err
	}
	for i := range out.Permissions {
		out.Permissions[i].Group = GroupOf(out.Permissions[i].ID)
	}
	var grants []struct{ RoleID, PermissionID string }
	if err := db.Table("role_permissions").Select("role_id, permission_id").Order("role_id, permission_id").Scan(&grants).Error; err != nil {
		return out, err
	}
	for _, role := range out.Roles {
		out.Grants[role.ID] = []string{}
	}
	for _, g := range grants {
		out.Grants[g.RoleID] = append(out.Grants[g.RoleID], g.PermissionID)
	}
	return out, nil
}

// History reconstructs each saved change from the audit log as additions and
// removals, newest first.
func (r *GormRepository) History(ctx context.Context, role string, page, size int) (Page[HistoryItem], error) {
	db := r.db.WithContext(ctx)
	out := Page[HistoryItem]{Items: []HistoryItem{}, Page: page, PageSize: size}
	base := db.Table("audit_events e").Where("e.target_type = 'role' AND e.target_id = ? AND e.action = 'permissions.replace'", role)
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	var rows []struct {
		ID        string
		CreatedAt string
		ActorName string
		Metadata  string
	}
	if err := base.Joins("LEFT JOIN users u ON u.id = e.actor_user_id").
		Select(`e.id, to_char(e.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at, COALESCE(NULLIF(u.display_name, ''), u.email, '') AS actor_name, e.metadata::text AS metadata`).
		Order("e.created_at DESC, e.id DESC").Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return out, err
	}
	for _, row := range rows {
		var meta struct{ Before, After []string }
		_ = json.Unmarshal([]byte(row.Metadata), &meta)
		item := HistoryItem{ID: row.ID, CreatedAt: row.CreatedAt, ActorName: row.ActorName, Added: []string{}, Removed: []string{}}
		for _, p := range meta.After {
			if !slices.Contains(meta.Before, p) {
				item.Added = append(item.Added, p)
			}
		}
		for _, p := range meta.Before {
			if !slices.Contains(meta.After, p) {
				item.Removed = append(item.Removed, p)
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// sameSet reports whether two permission lists hold the same ids.
func sameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}
