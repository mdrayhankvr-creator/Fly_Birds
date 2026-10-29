package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/promotion"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// Promotion routes: the administrator prepares and applies, only the head
// teacher approves, and teachers, accountants, students and guardians reach
// none of them. A generic edit cannot move a student with records to
// another session (409 use_promotion).
func TestPostgresPromotionRoutesEnforceRoles(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	var random [5]byte
	_, _ = rand.Read(random[:])
	suffix := hex.EncodeToString(random[:])
	password := "M15Secret!" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x62}, 32))
	if err != nil {
		t.Fatal(err)
	}
	var wasOn bool
	_ = db.Raw("SELECT enabled FROM school_modules WHERE school_id = 1 AND module_key = 'students'").Row().Scan(&wasOn)
	var from, to, class, section string
	userIDs := []string{}
	t.Cleanup(func() {
		_ = db.Exec("UPDATE school_modules SET enabled = ? WHERE school_id = 1 AND module_key = 'students'", wasOn).Error
		if class != "" {
			_ = db.Exec("DELETE FROM promotion_batches WHERE class_id = ?", class).Error
			_ = db.Exec("DELETE FROM students WHERE class_id = ?", class).Error
			_ = db.Exec("DELETE FROM academic_sections WHERE class_id = ?", class).Error
			_ = db.Exec("DELETE FROM academic_classes WHERE id = ?", class).Error
		}
		if len(userIDs) > 0 {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN ?", userIDs).Error
			_ = db.Exec("DELETE FROM users WHERE id IN ?", userIDs).Error
		}
		_ = db.Exec("DELETE FROM academic_sessions WHERE id IN (?, ?)", from, to).Error
	})
	if err := db.Exec("UPDATE school_modules SET enabled = true WHERE school_id = 1 AND module_key = 'students'").Error; err != nil {
		t.Fatal(err)
	}
	headers := map[string]map[string]string{}
	login := func(role string) string {
		u, err := authService.BootstrapUser(ctx, fmt.Sprintf("m15-%s-%s@example.test", role, suffix), password, role)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		c, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		headers[role] = map[string]string{"Authorization": "Bearer " + c.AccessToken}
		return u.ID
	}
	for _, role := range []string{"admin", "head_teacher", "teacher", "accountant", "student", "guardian"} {
		login(role)
	}
	scan := func(dst any, q string, args ...any) {
		t.Helper()
		if err := db.Raw(q, args...).Row().Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	scan(&from, "INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2421','2421-01-01','2421-12-31') RETURNING id")
	scan(&to, "INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2422','2422-01-01','2422-12-31') RETURNING id")
	scan(&class, "INSERT INTO academic_classes(name,display_order) VALUES (?, 5) RETURNING id", "M15 Five "+suffix)
	scan(&section, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", from, class)

	router := NewRouterWithPromotion(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil,
		academic.NewSectionService(academic.NewSectionRepository(db)), people.NewService(people.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, promotion.NewService(db))
	call := func(method, path, body, role string, want int) map[string]any {
		t.Helper()
		r := perform(router, method, path, body, nil, headers[role])
		if r.Code != want {
			t.Fatalf("%s %s as %s: got %d want %d: %s", method, path, role, r.Code, want, r.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(r.Body.Bytes(), &out)
		return out
	}
	outsiders := []string{"teacher", "accountant", "student", "guardian"}
	create := fmt.Sprintf(`{"from_session_id":%q,"to_session_id":%q,"class_id":%q}`, from, to, class)
	for _, role := range outsiders {
		call("GET", "/promotions", "", role, 403)
		call("GET", "/promotions/progression", "", role, 403)
		call("POST", "/promotions", create, role, 403)
	}
	call("GET", "/promotions", "", "head_teacher", 200)
	call("GET", "/promotions/progression", "", "head_teacher", 200)
	call("POST", "/promotions", create, "head_teacher", 403)
	call("PUT", "/promotions/progression/"+class, `{"final":true}`, "head_teacher", 403)
	if v := call("POST", "/promotions", create, "admin", 409); v["error"].(map[string]any)["code"] != "progression_unset" {
		t.Fatalf("a class with no progression: %v", v)
	}
	call("PUT", "/promotions/progression/"+class, `{"next_class_id":"`+class+`"}`, "admin", 400)
	call("PUT", "/promotions/progression/"+class, `{"final":true}`, "admin", 200)
	call("POST", "/promotions", `{"from_session_id":"x"}`, "admin", 400)
	batch := call("POST", "/promotions", create, "admin", 201)["data"].(map[string]any)
	id, hash := batch["id"].(string), batch["inputs_sha256"].(string)
	call("POST", "/promotions", create, "admin", 409)
	approve := fmt.Sprintf(`{"inputs_sha256":%q}`, hash)
	for _, role := range append(outsiders, "admin") {
		call("POST", "/promotions/"+id+"/approve", approve, role, 403)
	}
	call("POST", "/promotions/"+id+"/apply", "", "admin", 409) // not approved yet
	call("POST", "/promotions/"+id+"/approve", approve, "head_teacher", 200)
	for _, role := range append(outsiders, "head_teacher") {
		want := 403
		if role == "head_teacher" {
			want = 200
		}
		call("GET", "/promotions/"+id, "", role, want)
		call("POST", "/promotions/"+id+"/apply", "", role, 403)
		call("POST", "/promotions/"+id+"/revert", `{"reason":"test"}`, role, 403)
	}
	if v := call("POST", "/promotions/"+id+"/apply", "", "admin", 200)["data"].(map[string]any); v["status"] != "applied" {
		t.Fatalf("apply: %v", v)
	}
	call("POST", "/promotions/"+id+"/apply", "", "admin", 200) // idempotent
	call("POST", "/promotions/"+id+"/revert", `{"reason":""}`, "admin", 400)
	call("POST", "/promotions/"+id+"/revert", `{"reason":"Prepared for the wrong year"}`, "admin", 200)
	var audits int64
	scan(&audits, "SELECT count(*) FROM audit_events WHERE target_id = ? AND action IN ('promotion.batch_created','promotion.batch_approved','promotion.batch_applied','promotion.batch_reverted')", id)
	if audits != 4 {
		t.Fatalf("audited %d of 4 batch actions", audits)
	}

	// The generic edit: a student with a record this year cannot be moved to
	// another session; one with nothing recorded can be corrected.
	var otherSection, studentID string
	scan(&otherSection, "INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", to, class)
	body := func(session, sec string) string {
		return fmt.Sprintf(`{"admission_no":"M15-%s","name_en":"Moved Student","status":"active","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":1}`, suffix, session, class, sec)
	}
	studentID = call("POST", "/students", body(from, section), "admin", 201)["data"].(map[string]any)["id"].(string)
	call("PATCH", "/students/"+studentID, body(to, otherSection), "admin", 200)
	call("PATCH", "/students/"+studentID, body(from, section), "admin", 200)
	if err := db.Exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,'2421-02-01','present','manual',?,?)",
		studentID, from, class, section, userIDs[0], userIDs[0]).Error; err != nil {
		t.Fatal(err)
	}
	if v := call("PATCH", "/students/"+studentID, body(to, otherSection), "admin", 409); v["error"].(map[string]any)["code"] != "use_promotion" {
		t.Fatalf("generic move with records: %v", v)
	}
}
