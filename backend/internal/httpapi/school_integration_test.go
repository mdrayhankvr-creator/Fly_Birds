package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresSchoolConfigurationHTTP(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL school test not run")
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
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var migrationCount int64
	if err := db.Table("schema_migrations").Count(&migrationCount).Error; err != nil || migrationCount != 5 {
		t.Fatalf("migration count %d, error %v", migrationCount, err)
	}
	var suffix [10]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	unique := hex.EncodeToString(suffix[:])
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	schoolService := school.NewService(school.NewRepository(db))
	originalSettings, err := schoolService.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalGrades, err := schoolService.Grading(ctx)
	if err != nil {
		t.Fatal(err)
	}
	originalAttendance, err := schoolService.Enabled(ctx, "attendance")
	if err != nil {
		t.Fatal(err)
	}
	const password = "IsolatedSchoolTest123!"
	admin, err := service.BootstrapUser(ctx, "admin-school-"+unique+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := service.BootstrapUser(ctx, "teacher-school-"+unique+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id IN (?, ?)", admin.ID, teacher.ID).Error })
	t.Cleanup(func() {
		restore := school.SettingsInput{NameEN: originalSettings.NameEN, NameBN: originalSettings.NameBN, LogoURL: originalSettings.LogoURL, Address: originalSettings.Address, Phone: originalSettings.Phone, Email: originalSettings.Email, PreferredLanguage: originalSettings.PreferredLanguage, PrimaryColor: originalSettings.PrimaryColor, SecondaryColor: originalSettings.SecondaryColor, DashboardTheme: originalSettings.DashboardTheme, GradingSchemeID: originalSettings.GradingSchemeID}
		if _, err := schoolService.SaveSettings(context.Background(), admin.ID, restore); err != nil {
			t.Errorf("restore school settings: %v", err)
		}
		if _, err := schoolService.SaveGrading(context.Background(), admin.ID, originalGrades); err != nil {
			t.Errorf("restore grading: %v", err)
		}
		if _, err := schoolService.SetModule(context.Background(), admin.ID, "attendance", originalAttendance); err != nil {
			t.Errorf("restore attendance module: %v", err)
		}
	})
	router := NewRouterWithServices(service, true, rbac.NewService(rbac.NewRepository(db)), schoolService)
	router.GET("/test/attendance", RequireAuth(service), RequirePermission(service, "auth:read_self"), RequireModule(schoolService, "attendance"), func(c *gin.Context) { c.Status(204) })
	router.GET("/test/attendance-management", RequireAuth(service), RequirePermission(service, "modules:manage"), RequireModule(schoolService, "attendance"), func(c *gin.Context) { c.Status(204) })
	login := func(email string) string {
		body, _ := json.Marshal(map[string]string{"email": email, "password": password})
		response := perform(router, "POST", "/auth/login", string(body), nil, nil)
		if response.Code != 200 {
			t.Fatalf("login status=%d", response.Code)
		}
		return credentialsFromResponse(t, response)
	}
	adminHeader := map[string]string{"Authorization": "Bearer " + login(admin.Email)}
	teacherHeader := map[string]string{"Authorization": "Bearer " + login(teacher.Email)}
	check := func(method, path, body string, headers map[string]string, want int) {
		t.Helper()
		got := perform(router, method, path, body, nil, headers)
		if got.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, got.Code, want, got.Body.String())
		}
	}
	check("GET", "/school/settings", "", nil, 401)
	check("PUT", "/school/settings", `{}`, nil, 401)
	check("GET", "/school/settings", "", teacherHeader, 403)
	check("PUT", "/school/settings", `{}`, teacherHeader, 403)
	check("GET", "/school/grading", "", teacherHeader, 403)
	check("GET", "/school/modules", "", teacherHeader, 403)
	if err := db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING", "teacher", "school_settings:read").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM role_permissions WHERE role_id = ? AND permission_id = ?", "teacher", "school_settings:read").Error
	})
	check("GET", "/school/settings", "", teacherHeader, 200)
	check("PUT", "/school/settings", `{}`, teacherHeader, 403)
	if err := db.Exec("DELETE FROM role_permissions WHERE role_id = ? AND permission_id = ?", "teacher", "school_settings:read").Error; err != nil {
		t.Fatal(err)
	}
	branding := perform(router, "GET", "/school/branding", "", nil, nil)
	if branding.Code != 200 || strings.Contains(branding.Body.String(), "address") || strings.Contains(branding.Body.String(), "phone") || strings.Contains(branding.Body.String(), "email") {
		t.Fatalf("public branding leaked private fields or failed: %d", branding.Code)
	}
	settings, err := schoolService.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.NameEN != "School" || settings.GradingSchemeID != "bd_default" {
		t.Fatal("neutral default settings missing")
	}
	input := school.SettingsInput{NameEN: "Test Academy", NameBN: "পরীক্ষা বিদ্যালয়", LogoURL: "https://assets.example.org/logo.png", Address: "Test Street", Phone: "+8801712345678", Email: "contact@example.test", PreferredLanguage: "bn", PrimaryColor: "#0F766E", SecondaryColor: "#334155", DashboardTheme: "dark", GradingSchemeID: "bd_default"}
	body, _ := json.Marshal(input)
	check("PUT", "/school/settings", string(body), adminHeader, 200)
	settings, err = schoolService.Settings(ctx)
	if err != nil || settings.NameEN != "Test Academy" || settings.DashboardTheme != "dark" {
		t.Fatalf("settings did not persist: %v", err)
	}
	public := perform(router, "GET", "/school/branding", "", nil, nil)
	if public.Code != 200 || !strings.Contains(public.Body.String(), "Test Academy") || strings.Contains(public.Body.String(), "Test Street") {
		t.Fatal("public branding did not update safely")
	}
	input.LogoURL = "javascript:alert(1)"
	bad, _ := json.Marshal(input)
	check("PUT", "/school/settings", string(bad), adminHeader, 400)
	input.LogoURL = "https://assets.example.org/logo.png"
	input.PrimaryColor = "#FFFFFF"
	bad, _ = json.Marshal(input)
	check("PUT", "/school/settings", string(bad), adminHeader, 400)
	check("PUT", "/school/settings", `{"name_en":"x","unknown":true}`, adminHeader, 400)
	grades := perform(router, "GET", "/school/grading", "", nil, adminHeader)
	if grades.Code != 200 {
		t.Fatalf("grading status=%d", grades.Code)
	}
	var gradeBody struct {
		Data []school.GradeBand `json:"data"`
	}
	if err := json.Unmarshal(grades.Body.Bytes(), &gradeBody); err != nil || len(gradeBody.Data) != 7 {
		t.Fatalf("default grading: %v", err)
	}
	boundaries := map[int]string{0: "F", 32: "F", 33: "D", 39: "D", 40: "C", 49: "C", 50: "B", 59: "B", 60: "A-", 69: "A-", 70: "A", 79: "A", 80: "A+", 100: "A+"}
	for mark, want := range boundaries {
		actual := ""
		for _, band := range gradeBody.Data {
			if mark >= band.MinMarks && mark <= band.MaxMarks {
				actual = band.Grade
				break
			}
		}
		if actual != want {
			t.Fatalf("mark %d grade %s, want %s", mark, actual, want)
		}
	}
	valid := append([]school.GradeBand(nil), gradeBody.Data...)
	for i := range valid {
		if valid[i].Grade == "A+" {
			valid[i].MinMarks = 81
		}
		if valid[i].Grade == "A" {
			valid[i].MaxMarks = 80
		}
	}
	validBody, _ := json.Marshal(map[string]any{"bands": valid})
	check("PUT", "/school/grading", string(validBody), adminHeader, 200)
	current, err := schoolService.Grading(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, band := range current {
		if band.Grade == "A+" && band.MinMarks != 81 {
			t.Fatal("grading edit did not persist")
		}
	}
	invalid := append([]school.GradeBand(nil), valid...)
	for i := range invalid {
		if invalid[i].Grade == "A" {
			invalid[i].MaxMarks = 79
		}
	}
	invalidBody, _ := json.Marshal(map[string]any{"bands": invalid})
	check("PUT", "/school/grading", string(invalidBody), adminHeader, 400)
	check("PUT", "/school/grading", `{"bands":[]}`, adminHeader, 400)
	check("PUT", "/school/modules/authentication", `{"enabled":false}`, adminHeader, 409)
	check("PUT", "/school/modules/unknown", `{"enabled":true}`, adminHeader, 404)
	check("PUT", "/school/modules/attendance", `{"enabled":true}`, teacherHeader, 403)
	check("GET", "/test/attendance", "", adminHeader, 403)
	check("PUT", "/school/modules/attendance", `{"enabled":true}`, adminHeader, 200)
	check("GET", "/test/attendance", "", adminHeader, 204)
	check("GET", "/test/attendance", "", teacherHeader, 204)
	check("GET", "/test/attendance-management", "", teacherHeader, 403)
	check("PUT", "/school/modules/attendance", `{"enabled":false}`, adminHeader, 200)
	check("GET", "/test/attendance", "", adminHeader, 403)
	check("GET", "/test/attendance", "", teacherHeader, 403)
	check("GET", "/school/settings", "", adminHeader, 200) // core control remains available
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	settings, err = schoolService.Settings(ctx)
	if err != nil || settings.NameEN != "Test Academy" {
		t.Fatal("migration rerun overwrote configuration")
	}
	if got, err := schoolService.Enabled(ctx, "attendance"); err != nil || got {
		t.Fatal("migration rerun overwrote module toggle")
	}
}
