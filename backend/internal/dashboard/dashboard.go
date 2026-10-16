package dashboard

import (
	"context"
	"errors"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"gorm.io/gorm"
)

var ErrForbidden = errors.New("dashboard access denied")

type RoleCount struct {
	Role  string `json:"role"`
	Count int64  `json:"count"`
}
type AdminSummary struct {
	TotalUsers           int64       `json:"total_users"`
	ActiveUsers          int64       `json:"active_users"`
	InactiveUsers        int64       `json:"inactive_users"`
	ByRole               []RoleCount `json:"by_role"`
	EnabledModules       int64       `json:"enabled_modules"`
	SchoolFieldsComplete int         `json:"school_fields_complete"`
	SchoolFieldsTotal    int         `json:"school_fields_total"`
}
type NavigationItem struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	ComingSoon bool   `json:"coming_soon"`
}
type Data struct {
	User       auth.User        `json:"user"`
	Branding   school.Branding  `json:"branding"`
	Navigation []NavigationItem `json:"navigation"`
	Summary    *AdminSummary    `json:"summary,omitempty"`
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, user auth.User) (Data, error) {
	result := Data{User: user, Navigation: []NavigationItem{{Key: "account", Name: "Account", Path: "/account"}}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settings school.SchoolSettings
		if err := tx.First(&settings, "id=?", school.SingleSchoolID).Error; err != nil {
			return err
		}
		result.Branding = settings.Branding()
		grants := []string{}
		if err := tx.Table("role_permissions").Where("role_id=?", user.Role).Pluck("permission_id", &grants).Error; err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, id := range grants {
			allowed[id] = true
		}
		var modules []struct {
			Key     string
			Name    string
			IsCore  bool
			Enabled bool
		}
		if err := tx.Table("module_definitions AS d").Joins("JOIN school_modules AS m ON m.module_key=d.key AND m.school_id=?", school.SingleSchoolID).Select("d.key,d.name,d.is_core,m.enabled").Order("d.key").Scan(&modules).Error; err != nil {
			return err
		}
		enabled := map[string]bool{}
		for _, m := range modules {
			enabled[m.Key] = m.Enabled
		}
		if allowed["users:read"] && enabled["permissions"] && user.Role == "admin" {
			result.Navigation = append(result.Navigation, NavigationItem{Key: "users", Name: "Users", Path: "/admin/users"})
		}
		if allowed["permissions:read"] && enabled["permissions"] {
			result.Navigation = append(result.Navigation, NavigationItem{Key: "permissions", Name: "Permissions", Path: "/admin/permissions"})
		}
		if allowed["school_settings:read"] && enabled["school_settings"] {
			result.Navigation = append(result.Navigation, NavigationItem{Key: "school_settings", Name: "School settings", Path: "/admin/school-settings"})
		}
		if allowed["academics:read"] && enabled["academics"] {
			var academicSchemaReady bool
			if err := tx.Raw("SELECT to_regclass('public.academic_class_subjects') IS NOT NULL").Scan(&academicSchemaReady).Error; err != nil {
				return err
			}
			if academicSchemaReady {
				result.Navigation = append(result.Navigation, NavigationItem{Key: "academic_classes", Name: "Classes", Path: "/admin/academic/classes"}, NavigationItem{Key: "academic_subjects", Name: "Subjects", Path: "/admin/academic/subjects"})
				var sectionSchemaReady bool
				if err := tx.Raw("SELECT to_regclass('public.academic_sections') IS NOT NULL").Scan(&sectionSchemaReady).Error; err != nil {
					return err
				}
				if sectionSchemaReady {
					result.Navigation = append(result.Navigation, NavigationItem{Key: "academic_sections", Name: "Sections", Path: "/admin/academic/sections"})
				}
			}
		}
		if allowed["academic_sessions:read"] && enabled["academics"] {
			result.Navigation = append(result.Navigation, NavigationItem{Key: "academic_sessions", Name: "Academic sessions", Path: "/admin/academic/sessions"})
		}
		for _, m := range modules {
			if m.Enabled && !m.IsCore && m.Key != "academics" {
				result.Navigation = append(result.Navigation, NavigationItem{Key: m.Key, Name: m.Name, ComingSoon: true})
			}
		}
		if user.Role != "admin" || !allowed["users:read"] {
			return nil
		}
		s := &AdminSummary{ByRole: []RoleCount{}, SchoolFieldsTotal: 6}
		if err := tx.Table("users").Count(&s.TotalUsers).Error; err != nil {
			return err
		}
		if err := tx.Table("users").Where("is_active=true").Count(&s.ActiveUsers).Error; err != nil {
			return err
		}
		s.InactiveUsers = s.TotalUsers - s.ActiveUsers
		if err := tx.Table("users").Select("role,count(*) AS count").Group("role").Order("role").Scan(&s.ByRole).Error; err != nil {
			return err
		}
		if err := tx.Table("school_modules").Where("school_id=? AND enabled=true", school.SingleSchoolID).Count(&s.EnabledModules).Error; err != nil {
			return err
		}
		for _, field := range []string{settings.NameEN, settings.NameBN, settings.Address, settings.Phone, settings.Email, settings.LogoURL} {
			if field != "" {
				s.SchoolFieldsComplete++
			}
		}
		result.Summary = s
		return nil
	})
	return result, err
}
