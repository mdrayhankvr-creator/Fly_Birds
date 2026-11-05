package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresTermRoutineRoutes (E8): the planner's routes, who may use
// them, and their error answers. The workflow itself is covered by
// routine.TestPostgresTermRoutinePlanner.
func TestPostgresTermRoutineRoutes(t *testing.T) {
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
	var oldEnabled bool
	db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Scan(&oldEnabled)
	oldDays := []int{}
	db.Raw("SELECT day_of_week FROM school_weekly_off_days ORDER BY 1").Scan(&oldDays)
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", oldEnabled).Error
		_ = db.Exec("DELETE FROM school_weekly_off_days").Error
		for _, d := range oldDays {
			_ = db.Exec("INSERT INTO school_weekly_off_days (day_of_week) VALUES (?)", d).Error
		}
	})
	if err := db.Exec("DELETE FROM school_weekly_off_days; INSERT INTO school_weekly_off_days (day_of_week) VALUES (5)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x68}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "TermRoutinePass!" + suffix
	userIDs := []string{}
	login := func(role string) map[string]string {
		u, err := authService.BootstrapUser(ctx, "e8-"+role+suffix+"@example.test", password, role)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		credential, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + credential.AccessToken}
	}
	admin, head, teacher, student := login("admin"), login("head_teacher"), login("teacher"), login("student")
	year := 2700 + int(time.Now().UnixNano()%200)
	var sessionID, termID, classID, subjectID string
	rooms := []string{}
	t.Cleanup(func() {
		if termID != "" {
			_ = db.Exec("DELETE FROM routine_versions WHERE term_id=? AND status='draft'", termID).Error
			_ = db.Exec("DELETE FROM exam_terms WHERE id=?", termID).Error
		}
		for _, r := range rooms {
			_ = db.Exec("DELETE FROM exam_rooms WHERE id=?", r).Error
		}
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
			_ = db.Exec("DELETE FROM academic_sections WHERE class_id=?", classID).Error
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		}
		if subjectID != "" {
			_ = db.Exec("DELETE FROM academic_subjects WHERE id=?", subjectID).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		for _, id := range userIDs {
			_ = db.Exec("DELETE FROM audit_events WHERE actor_user_id=?", id).Error
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id=?", id).Error
			_ = db.Exec("DELETE FROM users WHERE id=?", id).Error
		}
	})
	must := func(query string, out *string, args ...any) {
		t.Helper()
		if err := db.Raw(query, args...).Scan(out).Error; err != nil || *out == "" {
			t.Fatalf("%s: %v", query, err)
		}
	}
	must("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", &sessionID, fmt.Sprintf("%d", year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	must("INSERT INTO exam_terms(session_id,name,sequence,kind,starts_on,ends_on) VALUES (?,'Annual exam',1,'term',?,?) RETURNING id", &termID, sessionID, fmt.Sprintf("%d-11-01", year), fmt.Sprintf("%d-11-30", year))
	must("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", &classID, "E8 Class "+suffix)
	must("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'বাংলা',?) RETURNING id", &subjectID, "E8 Bangla "+suffix, "E8B"+suffix[len(suffix)-8:])
	if err := db.Exec("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A')", sessionID, classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, subjectID).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithRoutine(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, routine.NewService(routine.NewRepository(db)))
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	errorOf := func(raw []byte) (string, []map[string]any) {
		var body struct {
			Error struct {
				Code   string           `json:"code"`
				Issues []map[string]any `json:"issues"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &body)
		return body.Error.Code, body.Error.Issues
	}
	// The routine module gates every route.
	call("GET", "/routines/terms", "", admin, 403)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='routine'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	call("GET", "/routines/terms", "", nil, 401)
	call("GET", "/routines/terms?session_id="+sessionID, "", admin, 200)
	call("GET", "/routines/terms", "", head, 200)
	call("GET", "/routines/terms", "", teacher, 403)
	call("GET", "/routines/terms?session_id=nope", "", admin, 400)
	call("GET", "/routines/rooms", "", head, 200)
	raw := call("POST", "/routines/rooms", `{"name":"E8 Room `+suffix+`","kind":"lab","capacity":30}`, admin, 201)
	var room struct {
		Data routine.Room `json:"data"`
	}
	_ = json.Unmarshal(raw, &room)
	rooms = append(rooms, room.Data.ID)
	if room.Data.Kind != "lab" || room.Data.Capacity == nil || *room.Data.Capacity != 30 || !room.Data.IsActive {
		t.Fatalf("room: %s", raw)
	}
	call("POST", "/routines/rooms", `{"name":"E8 Room `+suffix+`"}`, admin, 409)
	call("POST", "/routines/rooms", `{"name":"x","capacity":0}`, admin, 400)
	call("POST", "/routines/rooms", `{"name":"y"}`, head, 403)
	call("PATCH", "/routines/rooms/"+room.Data.ID, `{"name":"E8 Room `+suffix+`","kind":"lab","capacity":32,"is_active":true}`, admin, 200)
	// A term's routine.
	call("POST", "/routines/versions", `{"term_id":"`+termID+`"}`, head, 403)
	raw = call("POST", "/routines/versions", `{"term_id":"`+termID+`"}`, admin, 201)
	var version struct {
		Data routine.Version `json:"data"`
	}
	_ = json.Unmarshal(raw, &version)
	v := version.Data
	if v.Status != "draft" || len(v.Papers) != 1 || len(v.Sittings) != 2 || v.From != fmt.Sprintf("%d-11-01", year) {
		t.Fatalf("version: %s", raw)
	}
	call("POST", "/routines/versions", `{"term_id":"`+termID+`"}`, admin, 409)
	call("GET", "/routines/versions/"+v.ID, "", head, 200)
	call("GET", "/routines/versions/"+v.ID, "", teacher, 403)
	call("GET", "/routines/versions/not-a-uuid", "", admin, 400)
	// Manual placement on a Friday is refused with the rule.
	friday := ""
	for d := 1; d <= 7; d++ {
		day := time.Date(year, 11, d, 0, 0, 0, 0, time.UTC)
		if day.Weekday() == time.Friday {
			friday = day.Format(time.DateOnly)
		}
	}
	raw = call("PATCH", "/routines/versions/"+v.ID+"/papers/"+v.Papers[0].ID, `{"duration_minutes":180,"exam_date":"`+friday+`","sitting_id":"`+v.Sittings[0].ID+`"}`, admin, 409)
	if code, issues := errorOf(raw); code != "routine_conflict" || len(issues) != 1 || issues[0]["code"] != "weekly_off" || !strings.Contains(string(raw), "weekly off day") {
		t.Fatalf("Friday placement: %s", raw)
	}
	call("PATCH", "/routines/versions/"+v.ID+"/papers/"+v.Papers[0].ID, `{"duration_minutes":180,"exam_date":"`+friday+`"}`, admin, 400)
	call("PATCH", "/routines/versions/"+v.ID+"/papers/"+v.Papers[0].ID, `{"duration_minutes":180}`, head, 403)
	// Submission needs every paper placed; approval needs a submission.
	raw = call("POST", "/routines/versions/"+v.ID+"/submit", "", admin, 409)
	if code, issues := errorOf(raw); code != "routine_not_ready" || len(issues) == 0 || issues[0]["code"] != "unscheduled" {
		t.Fatalf("early submit: %s", raw)
	}
	raw = call("POST", "/routines/versions/"+v.ID+"/approve", "", head, 409)
	if code, _ := errorOf(raw); code != "routine_state" {
		t.Fatalf("approve a draft: %s", raw)
	}
	call("POST", "/routines/versions/"+v.ID+"/approve", "", admin, 403) // head teachers approve
	call("POST", "/routines/versions/"+v.ID+"/generate", "", head, 403)
	raw = call("POST", "/routines/versions/"+v.ID+"/generate", "", admin, 200)
	if !strings.Contains(string(raw), `"placed":1`) {
		t.Fatalf("generate: %s", raw)
	}
	call("POST", "/routines/versions/"+v.ID+"/submit", "", admin, 200)
	call("POST", "/routines/versions/"+v.ID+"/return", `{"reason":""}`, head, 400)
	call("POST", "/routines/versions/"+v.ID+"/return", `{"reason":"Check the dates"}`, head, 200)
	call("POST", "/routines/versions/"+v.ID+"/submit", "", admin, 200)
	call("POST", "/routines/versions/"+v.ID+"/publish", "", admin, 409)
	call("POST", "/routines/versions/"+v.ID+"/approve", "", head, 200)
	call("POST", "/routines/versions/"+v.ID+"/amend", `{"reason":"x"}`, admin, 409) // not published yet
	call("POST", "/routines/versions/"+v.ID+"/withdraw", `{"reason":"One more change"}`, admin, 200)
	// PDFs need the renderer, which this router does not have.
	raw = call("GET", "/routines/versions/"+v.ID+"/pdf?kind=class", "", admin, 503)
	if code, _ := errorOf(raw); code != "documents_unavailable" {
		t.Fatalf("pdf: %s", raw)
	}
	call("GET", "/routines/versions/"+v.ID+"/pdf?kind=class", "", teacher, 403)
	// Students and teachers.
	call("GET", "/routines/terms/me", "", student, 200)
	call("GET", "/routines/terms/me", "", teacher, 403)
	call("GET", "/routines/duties/me", "", teacher, 200)
	call("GET", "/routines/duties/me", "", student, 403)
	call("GET", "/routines/terms/me/"+v.ID+"/pdf", "", student, 503)
	// The per-section exam routines are unchanged.
	call("GET", "/routines/exams?page=1&page_size=5", "", admin, 200)
	call("GET", "/routines/exams/me", "", student, 200)
	call("DELETE", "/routines/versions/"+v.ID, "", head, 403)
	call("DELETE", "/routines/versions/"+v.ID, "", admin, 200)
}
