package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
	"gorm.io/gorm"
)

func TestPostgresCheckpointFourLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL checkpoint-four test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-InitialSecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x55}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "cp4-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%"+suffix+"@example.test").Error })
	userService := users.NewService(users.NewRepository(db))
	router := NewRouterWithAll(authService, true, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), userService, audit.NewRepository(db), dashboard.NewRepository(db))
	adminCredentials, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	authorization := map[string]string{"Authorization": "Bearer " + adminCredentials.AccessToken}
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	check("GET", "/users", "", nil, 401)
	check("GET", "/audit/events", "", nil, 401)
	createdBody, _ := json.Marshal(map[string]any{"email": "cp4-teacher-" + suffix + "@example.test", "display_name": "Checkpoint Teacher", "role": "teacher", "password": password})
	created := check("POST", "/users", string(createdBody), authorization, 201)
	if bytes.Contains(created, []byte(password)) || bytes.Contains(created, []byte("password_hash")) || bytes.Contains(created, []byte("refresh")) {
		t.Fatal("user response exposed credentials")
	}
	var response struct {
		Data users.User `json:"data"`
	}
	if err := json.Unmarshal(created, &response); err != nil {
		t.Fatal(err)
	}
	user := response.Data
	if user.ID == "" || user.Role != "teacher" || !user.IsActive {
		t.Fatal("unexpected created user")
	}
	check("POST", "/users", string(createdBody), authorization, 409)
	check("POST", "/users", `{"email":"bad","role":"admin","password":"x"}`, authorization, 400)
	check("POST", "/users", `{"email":"x@example.test","display_name":"X","role":"teacher","password":"too short","is_active":false}`, authorization, 400)
	check("GET", "/users/"+user.ID, "", authorization, 200)
	filtered := check("GET", "/users?search=Checkpoint&role=teacher&status=active&page=1&page_size=1", "", authorization, 200)
	var filteredPage struct {
		Data users.Page `json:"data"`
	}
	if err := json.Unmarshal(filtered, &filteredPage); err != nil {
		t.Fatal(err)
	}
	if filteredPage.Data.Total != 1 || len(filteredPage.Data.Items) != 1 || filteredPage.Data.Items[0].ID != user.ID {
		t.Fatal("search, role, status, or pagination result incorrect")
	}
	secondPage := check("GET", "/users?search=Checkpoint&role=teacher&status=active&page=2&page_size=1", "", authorization, 200)
	if err := json.Unmarshal(secondPage, &filteredPage); err != nil {
		t.Fatal(err)
	}
	if filteredPage.Data.Total != 1 || len(filteredPage.Data.Items) != 0 {
		t.Fatal("second page should be empty")
	}
	check("GET", "/users?page=0", "", authorization, 400)
	check("GET", "/users?role=unknown", "", authorization, 400)
	check("PATCH", "/users/"+user.ID, `{"email":"cp4-teacher-`+suffix+`@example.test","display_name":"Updated Teacher"}`, authorization, 200)
	check("PUT", "/users/"+user.ID+"/roles", `{"ids":["guardian"]}`, authorization, 200)
	check("PATCH", "/users/"+admin.ID+"/status", `{"is_active":false}`, authorization, 409)
	teacherCredentials, err := authService.Login(ctx, user.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	teacherAuth := map[string]string{"Authorization": "Bearer " + teacherCredentials.AccessToken}
	check("GET", "/users", "", teacherAuth, 403)
	check("GET", "/audit/events", "", teacherAuth, 403)
	check("POST", "/users", string(createdBody), teacherAuth, 403)
	check("PATCH", "/users/"+user.ID+"/status", `{"is_active":false}`, teacherAuth, 403)
	check("PUT", "/users/"+user.ID+"/roles", `{"ids":["admin"]}`, teacherAuth, 403)
	teacherDashboard := check("GET", "/dashboard", "", teacherAuth, 200)
	if bytes.Contains(teacherDashboard, []byte("total_users")) || !bytes.Contains(teacherDashboard, []byte("guardian")) {
		t.Fatal("non-admin dashboard leaked admin summary or stale role")
	}
	adminDashboard := check("GET", "/dashboard", "", authorization, 200)
	if !bytes.Contains(adminDashboard, []byte("total_users")) || !bytes.Contains(adminDashboard, []byte("by_role")) {
		t.Fatal("admin summary absent")
	}
	var dashboardResponse struct {
		Data dashboard.Data `json:"data"`
	}
	if err := json.Unmarshal(adminDashboard, &dashboardResponse); err != nil {
		t.Fatal(err)
	}
	var actualUsers, activeUsers, enabledModules int64
	if err := db.Table("users").Count(&actualUsers).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("users").Where("is_active=true").Count(&activeUsers).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=1 AND enabled=true").Count(&enabledModules).Error; err != nil {
		t.Fatal(err)
	}
	summary := dashboardResponse.Data.Summary
	if summary == nil || summary.TotalUsers != actualUsers || summary.ActiveUsers != activeUsers || summary.InactiveUsers != actualUsers-activeUsers || summary.EnabledModules != enabledModules {
		t.Fatal("dashboard counts do not match database")
	}
	check("PATCH", "/users/"+user.ID+"/status", `{"is_active":false}`, authorization, 200)
	check("GET", "/auth/me", "", teacherAuth, 401)
	if _, err := authService.Refresh(ctx, teacherCredentials.RefreshToken, teacherCredentials.CSRFToken); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("deactivated user refreshed a session")
	}
	var revoked int64
	if err := db.Table("auth_sessions").Where("user_id=? AND revoked_at IS NOT NULL", user.ID).Count(&revoked).Error; err != nil || revoked == 0 {
		t.Fatal("deactivation did not persist revocation")
	}
	check("PATCH", "/users/"+user.ID+"/status", `{"is_active":true}`, authorization, 200)
	auditPage := check("GET", "/audit/events?action=user.status_changed&sort=oldest&page_size=2", "", authorization, 200)
	var filteredAudit struct {
		Data audit.Page `json:"data"`
	}
	if err := json.Unmarshal(auditPage, &filteredAudit); err != nil {
		t.Fatal(err)
	}
	if filteredAudit.Data.Total < 2 || len(filteredAudit.Data.Items) != 2 || filteredAudit.Data.Items[0].Action != "user.status_changed" || filteredAudit.Data.Items[0].CreatedAt.After(filteredAudit.Data.Items[1].CreatedAt) {
		t.Fatal("audit filter or order incorrect")
	}
	check("GET", "/audit/events?sort=unknown", "", authorization, 400)
	check("GET", "/audit/events?actor=bad", "", authorization, 400)

	roleGrants, err := rbac.NewRepository(db).RolePermissions(ctx, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	changedGrants := append(append([]string{}, roleGrants...), "permissions:read")
	if !strings.Contains(strings.Join(roleGrants, ","), "permissions:read") {
		payload, _ := json.Marshal(map[string]any{"ids": changedGrants})
		check("PUT", "/roles/teacher/permissions", string(payload), authorization, 200)
		originalPayload, _ := json.Marshal(map[string]any{"ids": roleGrants})
		defer func() {
			_ = perform(router, "PUT", "/roles/teacher/permissions", string(originalPayload), nil, authorization)
		}()
	}
	settings, err := school.NewRepository(db).Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	input := school.SettingsInput{NameEN: settings.NameEN, NameBN: settings.NameBN, LogoURL: settings.LogoURL, Address: "Checkpoint test address", Phone: settings.Phone, Email: settings.Email, PreferredLanguage: settings.PreferredLanguage, PrimaryColor: settings.PrimaryColor, SecondaryColor: settings.SecondaryColor, DashboardTheme: settings.DashboardTheme, GradingSchemeID: settings.GradingSchemeID}
	settingsBody, _ := json.Marshal(input)
	check("PUT", "/school/settings", string(settingsBody), authorization, 200)
	input.Address = settings.Address
	originalSettingsBody, _ := json.Marshal(input)
	defer func() {
		_ = perform(router, "PUT", "/school/settings", string(originalSettingsBody), nil, authorization)
	}()
	bands, err := school.NewRepository(db).Grading(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gradingBody, _ := json.Marshal(map[string]any{"bands": bands})
	check("PUT", "/school/grading", string(gradingBody), authorization, 200)
	attendance, err := school.NewRepository(db).ModuleEnabled(ctx, "attendance")
	if err != nil {
		t.Fatal(err)
	}
	moduleBody, _ := json.Marshal(map[string]any{"enabled": !attendance})
	check("PUT", "/school/modules/attendance", string(moduleBody), authorization, 200)
	originalModuleBody, _ := json.Marshal(map[string]any{"enabled": attendance})
	defer func() {
		_ = perform(router, "PUT", "/school/modules/attendance", string(originalModuleBody), nil, authorization)
	}()
	for _, action := range []string{"permissions.replace", "school.settings_updated", "school.grading_updated", "school.module_updated", "user.role_changed"} {
		var count int64
		if err := db.Table("audit_events").Where("actor_user_id=? AND action=?", admin.ID, action).Count(&count).Error; err != nil || count == 0 {
			t.Fatalf("missing persisted audit action %s", action)
		}
	}
	var events []audit.Event
	if err := db.Where("target_id=?", user.ID).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Metadata), password) || strings.Contains(string(event.Metadata), "password_hash") {
			t.Fatal("audit event contains credential")
		}
	}
	var actorCount int64
	if err := db.Table("audit_events").Where("actor_user_id=? AND target_id=? AND action='user.created'", admin.ID, user.ID).Count(&actorCount).Error; err != nil || actorCount != 1 {
		t.Fatal("creation audit attribution missing")
	}
	// A mandatory audit insert failure must roll back the profile update.
	rollback := errors.New("discard test constraint")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE audit_events ADD CONSTRAINT audit_reject_cp4 CHECK (false) NOT VALID").Error; err != nil {
			return err
		}
		_, changeErr := users.NewRepository(tx).Update(ctx, admin.ID, user.ID, users.UpdateInput{Email: user.Email, DisplayName: "Must Roll Back"})
		if changeErr == nil {
			t.Fatal("update committed without mandatory audit")
		}
		var name string
		if err := tx.Table("users").Select("display_name").Where("id=?", user.ID).Scan(&name).Error; err != nil {
			return err
		}
		if name != "Updated Teacher" {
			t.Fatalf("audit failure left changed user: %q", name)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback test: %v", err)
	}
}
