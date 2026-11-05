package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"gorm.io/gorm"
)

// withoutWeeklyOffDays clears the weekly off days for one test and restores
// them afterwards. The attendance tests written before off days (QA-12)
// record attendance on Fridays; TestPostgresAttendanceOffDays covers the rule.
func withoutWeeklyOffDays(t *testing.T, db *gorm.DB) {
	t.Helper()
	saved := []int{}
	if err := db.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY 1").Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM school_weekly_off_days").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM school_weekly_off_days").Error
		for _, d := range saved {
			_ = db.Exec("INSERT INTO school_weekly_off_days (day_of_week) VALUES (?)", d).Error
		}
	})
}

// TestPostgresAttendanceOffDays (lifecycle audit QA-12): weekly off days
// (Friday by default, set in school settings) and published holidays refuse
// new attendance with a clear message; attendance already recorded stays,
// can be read and can be corrected.
func TestPostgresAttendanceOffDays(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','attendance:teachers_read'),('admin','attendance:teachers_manage'),('admin','school_settings:update'),('admin','school_settings:read') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var oldEnabled bool
	db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Scan(&oldEnabled)
	oldDays := []int{}
	db.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY 1").Scan(&oldDays)
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
		_ = db.Exec("DELETE FROM school_weekly_off_days").Error
		for _, d := range oldDays {
			_ = db.Exec("INSERT INTO school_weekly_off_days (day_of_week) VALUES (?)", d).Error
		}
	})
	if err := db.Table("school_modules").Where("school_id=? AND module_key='attendance'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// Start from the default: Friday only.
	if err := db.Exec("DELETE FROM school_weekly_off_days; INSERT INTO school_weekly_off_days (day_of_week) VALUES (5)").Error; err != nil {
		t.Fatal(err)
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := suffix + "-OffDaySecret!"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := authService.BootstrapUser(ctx, "offday-admin-"+suffix+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := authService.BootstrapUser(ctx, "offday-teacher-"+suffix+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	// A year far ahead, so no other test's holidays fall on these dates.
	year := 2400 + int(random[0])%300
	date := func(month, day int) string { return fmt.Sprintf("%04d-%02d-%02d", year, month, day) }
	var sessionID, employeeID string
	events := []string{}
	t.Cleanup(func() {
		for _, id := range events {
			_ = db.Exec("DELETE FROM school_events WHERE id=?", id).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM teacher_attendance WHERE session_id=?", sessionID).Error
		}
		if employeeID != "" {
			_ = db.Exec("DELETE FROM employees WHERE id=?", employeeID).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		_ = db.Exec("DELETE FROM users WHERE id IN (?,?)", admin.ID, teacher.ID).Error
	})
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprintf("%04d", year), date(1, 1), date(12, 31)).Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,joined_on) VALUES (?,?,'teacher','Off Day Teacher','শিক্ষক','Teacher',?) RETURNING id", "OD"+strings.ToUpper(suffix), teacher.ID, date(1, 1)).Scan(&employeeID).Error; err != nil {
		t.Fatal(err)
	}
	schoolService := school.NewService(school.NewRepository(db))
	router := NewRouterWithBatchServices(authService, true, nil, schoolService, nil, nil, nil, nil, nil, nil, attendance.NewService(attendance.NewRepository(db)))
	adminLogin, err := authService.Login(ctx, admin.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	teacherLogin, err := authService.Login(ctx, teacher.Email, password)
	if err != nil {
		t.Fatal(err)
	}
	adminH := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken}
	teacherH := map[string]string{"Authorization": "Bearer " + teacherLogin.AccessToken}
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	weekday := func(month, day int) time.Weekday {
		d, _ := time.Parse(time.DateOnly, date(month, day))
		return d.Weekday()
	}
	// The first Friday, Saturday and two working days of March that year.
	var friday, saturday, monday, thursday string
	for d := 1; d <= 14; d++ {
		switch weekday(3, d) {
		case time.Friday:
			if friday == "" {
				friday = date(3, d)
			}
		case time.Saturday:
			if saturday == "" {
				saturday = date(3, d)
			}
		case time.Monday:
			if monday == "" {
				monday = date(3, d)
			}
		case time.Thursday:
			if thursday == "" {
				thursday = date(3, d)
			}
		}
	}
	record := func(day string) string {
		return `{"session_id":"` + sessionID + `","employee_id":"` + employeeID + `","attendance_date":"` + day + `","status":"present"}`
	}
	type dayBody struct {
		Data attendance.DayStatus `json:"data"`
	}
	type errBody struct {
		Error struct {
			Code    string               `json:"code"`
			Message string               `json:"message"`
			Day     attendance.DayStatus `json:"day"`
		} `json:"error"`
	}

	// Anyone signed in may ask whether a day is open.
	call("GET", "/attendance/day?date="+friday, "", nil, 401)
	var day dayBody
	if err := json.Unmarshal(call("GET", "/attendance/day?date="+friday, "", teacherH, 200), &day); err != nil || day.Data.Open || day.Data.Reason != "weekly_off" || day.Data.Weekday != 5 || len(day.Data.WeeklyOffDays) != 1 {
		t.Fatalf("friday: %+v %v", day, err)
	}
	call("GET", "/attendance/day?date=soon", "", teacherH, 400)

	// A Friday is refused with a clear message; a Thursday is not.
	var refused errBody
	if err := json.Unmarshal(call("POST", "/attendance/teachers", record(friday), adminH, 409), &refused); err != nil || refused.Error.Code != "attendance_day_off" ||
		!strings.Contains(refused.Error.Message, "weekly off day") || refused.Error.Day.Reason != "weekly_off" || refused.Error.Day.Date != friday {
		t.Fatalf("friday refused: %+v %v", refused, err)
	}
	call("POST", "/attendance/teachers", record(thursday), adminH, 201)

	// Attendance recorded on a Friday before the rule stays, is listed and
	// can be corrected.
	var existing string
	if err := db.Raw("INSERT INTO teacher_attendance (session_id,employee_id,attendance_date,status,note,recorded_by,updated_by) VALUES (?,?,?,'present','',?,?) RETURNING id", sessionID, employeeID, friday, admin.ID, admin.ID).Scan(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if body := call("GET", "/attendance/teachers/roster?session_id="+sessionID+"&date="+friday+"&page=1&page_size=50&search="+suffix, "", adminH, 200); !strings.Contains(string(body), existing) {
		t.Fatalf("the existing record is missing from the roster: %s", body)
	}
	call("PATCH", "/attendance/teachers/"+existing, `{"status":"absent","note":"corrected"}`, adminH, 200)

	// A published holiday is refused with its name; a draft one is not.
	holiday := func(title, from, to, status string) {
		var id string
		if err := db.Raw(`INSERT INTO school_events (title_en,title_bn,kind,starts_on,ends_on,audience_roles,status,created_by,updated_by)
VALUES (?,?,'holiday',?,?,'{teacher}',?,?,?) RETURNING id`, title, "ছুটি", from, to, status, admin.ID, admin.ID).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		events = append(events, id)
	}
	holiday("Spring holiday "+suffix, monday, monday, "published")
	holiday("Draft holiday "+suffix, thursday, thursday, "draft")
	refused = errBody{}
	if err := json.Unmarshal(call("POST", "/attendance/teachers", record(monday), adminH, 409), &refused); err != nil || refused.Error.Day.Reason != "holiday" ||
		refused.Error.Day.HolidayEN != "Spring holiday "+suffix || refused.Error.Day.HolidayBN != "ছুটি" || !strings.Contains(refused.Error.Message, "Spring holiday") {
		t.Fatalf("holiday refused: %+v %v", refused, err)
	}

	// Weekly off days are school settings: administrators change them, the
	// change is audited and applies at once.
	settings, err := schoolService.Settings(ctx)
	if err != nil || len(settings.WeeklyOffDays) != 1 || settings.WeeklyOffDays[0] != 5 {
		t.Fatalf("settings: %+v %v", settings.WeeklyOffDays, err)
	}
	put := func(days string, want int) {
		t.Helper()
		body := fmt.Sprintf(`{"name_en":%q,"name_bn":%q,"logo_url":%q,"address":%q,"phone":%q,"email":%q,"preferred_language":%q,"primary_color":%q,"secondary_color":%q,"dashboard_theme":%q,"grading_scheme_id":%q,"weekly_off_days":%s}`,
			settings.NameEN, settings.NameBN, settings.LogoURL, settings.Address, settings.Phone, settings.Email, settings.PreferredLanguage, settings.PrimaryColor, settings.SecondaryColor, settings.DashboardTheme, settings.GradingSchemeID, days)
		call("PUT", "/school/settings", body, adminH, want)
	}
	put("[7]", 400)
	put("[0,1,2,3,4,5,6]", 400)
	put("[5,5]", 400)
	call("PUT", "/school/settings", `{"weekly_off_days":[6]}`, teacherH, 403)
	put("[6,5]", 200)
	if s, _ := schoolService.Settings(ctx); fmt.Sprint(s.WeeklyOffDays) != "[5 6]" {
		t.Fatalf("saved days: %v", s.WeeklyOffDays)
	}
	var audited int64
	db.Raw("SELECT count(*) FROM audit_events WHERE action='school.settings_updated' AND actor_user_id=? AND metadata->'weekly_off_days' = '[5,6]'::jsonb", admin.ID).Scan(&audited)
	if audited != 1 {
		t.Fatalf("audit: %d", audited)
	}
	call("POST", "/attendance/teachers", record(saturday), adminH, 409)
	// No off days at all is allowed.
	put("[]", 200)
	call("POST", "/attendance/teachers", record(saturday), adminH, 201)
	// Omitting the field keeps the days.
	body := fmt.Sprintf(`{"name_en":%q,"name_bn":%q,"logo_url":%q,"address":%q,"phone":%q,"email":%q,"preferred_language":%q,"primary_color":%q,"secondary_color":%q,"dashboard_theme":%q,"grading_scheme_id":%q}`,
		settings.NameEN, settings.NameBN, settings.LogoURL, settings.Address, settings.Phone, settings.Email, settings.PreferredLanguage, settings.PrimaryColor, settings.SecondaryColor, settings.DashboardTheme, settings.GradingSchemeID)
	put("[5]", 200)
	call("PUT", "/school/settings", body, adminH, 200)
	if s, _ := schoolService.Settings(ctx); fmt.Sprint(s.WeeklyOffDays) != "[5]" {
		t.Fatalf("days after a save without them: %v", s.WeeklyOffDays)
	}
}
