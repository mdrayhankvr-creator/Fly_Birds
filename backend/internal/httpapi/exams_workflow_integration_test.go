package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresExamWorkflowRouteProtection checks who can reach the E3 sheet
// and mark-status routes: students and guardians never; teachers only their
// own sheets and marks; head teachers review but cannot enter marks.
func TestPostgresExamWorkflowRouteProtection(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Other integration tests can edit the shared role matrix. Restore the grants under test.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','exams:manage'),('admin','exams:read'),('admin','exams:enter_marks'),('admin','exams:review'),('head_teacher','exams:read'),('head_teacher','exams:review'),('teacher','exams:enter_marks'),('student','exams:read_self'),('guardian','exams:read_children') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var old bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key='examinations'").Scan(&old).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key='examinations'", old).Error
	})
	if err := db.Exec("UPDATE school_modules SET enabled=true WHERE school_id=1 AND module_key='examinations'").Error; err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x77}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("e3%d", time.Now().UnixNano())
	password := "E3TestPassword!" + suffix
	users := []auth.User{}
	login := func(role string) map[string]string {
		u, err := authService.BootstrapUser(ctx, role+suffix+"@example.test", password, role)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
		credential, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + credential.AccessToken}
	}
	admin, head, teacher, student, guardian := login("admin"), login("head_teacher"), login("teacher"), login("student"), login("guardian")
	t.Cleanup(func() {
		for _, u := range users {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id=?", u.ID).Error
			_ = db.Exec("DELETE FROM users WHERE id=?", u.ID).Error
		}
	})
	router := NewRouterWithExams(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, exams.NewService(exams.NewRepository(db)))
	check := func(method, path, body string, headers map[string]string, want int) {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
	}
	const examID = "00000000-0000-4000-8000-0000000000e3"
	const sheetID = "00000000-0000-4000-8000-0000000000e4"
	cell := `{"entries":[{"exam_subject_id":"` + sheetID + `","student_id":"` + sheetID + `","status":"absent","row_version":0}]}`
	sheets, assigned, marks := "/exams/"+examID+"/sheets", "/exams/assigned/"+examID+"/sheets", "/exams/"+examID+"/marks"
	submit, approve, ret := "/exams/"+examID+"/sheets/"+sheetID+"/submit", "/exams/"+examID+"/sheets/"+sheetID+"/approve", "/exams/"+examID+"/sheets/"+sheetID+"/return"

	check("GET", sheets, "", nil, 401)
	check("PATCH", marks, cell, nil, 401)
	check("POST", approve, "", nil, 401)
	for _, who := range []map[string]string{student, guardian} {
		check("GET", sheets, "", who, 403)
		check("GET", assigned, "", who, 403)
		check("PATCH", marks, cell, who, 403)
		check("POST", submit, "", who, 403)
		check("POST", approve, "", who, 403)
		check("POST", ret, `{"reason":"x"}`, who, 403)
	}
	// Teachers: their own sheets and marks, never review or the oversight view.
	check("GET", sheets, "", teacher, 403)
	check("GET", assigned, "", teacher, 404)
	check("PATCH", marks, cell, teacher, 404)
	check("POST", submit, "", teacher, 404)
	check("POST", approve, "", teacher, 403)
	check("POST", ret, `{"reason":"x"}`, teacher, 403)
	// Head teachers: review and oversight, but no marks entry.
	check("GET", sheets, "", head, 404)
	check("GET", assigned, "", head, 403)
	check("PATCH", marks, cell, head, 403)
	check("POST", submit, "", head, 403)
	check("POST", approve, "", head, 404)
	check("POST", ret, `{"reason":""}`, head, 400)
	check("POST", ret, `{"reason":"Check the totals"}`, head, 404)
	// E4 workspace reads: the work queue is for teachers; one sheet and its
	// history through exams:read (staff) or the teacher's own route.
	queue, ws, mineWS := "/exams/my-sheets", "/exams/"+examID+"/sheets/"+sheetID, "/exams/assigned/"+examID+"/sheets/"+sheetID
	check("GET", queue, "", nil, 401)
	for _, who := range []map[string]string{student, guardian} {
		check("GET", queue, "", who, 403)
		check("GET", ws, "", who, 403)
		check("GET", mineWS, "", who, 403)
		check("GET", ws+"/history", "", who, 403)
		check("GET", mineWS+"/history", "", who, 403)
	}
	check("GET", queue, "", teacher, 200)
	check("GET", queue, "", head, 403)
	check("GET", queue, "", admin, 403)
	check("GET", ws, "", teacher, 403)
	check("GET", ws+"/history", "", teacher, 403)
	check("GET", mineWS, "", teacher, 404)
	check("GET", mineWS+"/history", "", teacher, 404)
	check("GET", mineWS, "", head, 403)
	check("GET", ws, "", head, 404)
	check("GET", ws+"/history", "", admin, 404)
	// E5 terms, blueprints and subject choices: staff with exams:read may look,
	// only administrators with exams:manage may change; students, guardians
	// and teachers reach none of it.
	const versionID = "00000000-0000-4000-8000-0000000000e5"
	session := "?session_id=" + examID
	reads := []string{"/exams/terms" + session, "/exams/blueprints" + session, "/exams/blueprint-versions/" + versionID, "/exams/subject-choices" + session + "&class_id=" + examID}
	type write struct{ method, path, body string }
	writes := []write{
		{"POST", "/exams/terms", `{"session_id":"` + examID + `","name":"First Term","kind":"term"}`},
		{"PUT", "/exams/terms/" + versionID, `{"name":"First Term","kind":"term"}`},
		{"DELETE", "/exams/terms/" + versionID, ""},
		{"POST", "/exams/blueprints", `{"session_id":"` + examID + `","class_id":"` + examID + `"}`},
		{"POST", "/exams/blueprints/" + versionID + "/versions", `{"from_version_id":"` + versionID + `"}`},
		{"PUT", "/exams/blueprint-versions/" + versionID, `{"definition":{}}`},
		{"DELETE", "/exams/blueprint-versions/" + versionID, ""},
		{"POST", "/exams/blueprint-versions/" + versionID + "/activate", ""},
		{"PUT", "/exams/blueprint-versions/" + versionID + "/visibility", `{"key":"CT1","visibility":"hidden"}`},
		{"PUT", "/exams/subject-choices", `{"session_id":"` + examID + `","class_id":"` + examID + `","choices":[{"student_id":"` + examID + `","subject_id":""}]}`},
	}
	sample := "/exams/blueprint-versions/" + versionID + "/sample"
	for _, path := range reads {
		check("GET", path, "", nil, 401)
		for _, who := range []map[string]string{student, guardian, teacher} {
			check("GET", path, "", who, 403)
		}
	}
	for _, w := range writes {
		check(w.method, w.path, w.body, nil, 401)
		for _, who := range []map[string]string{student, guardian, teacher, head} {
			check(w.method, w.path, w.body, who, 403)
		}
	}
	check("POST", sample, `{}`, teacher, 403)
	check("GET", reads[0], "", head, 200)
	check("GET", reads[1], "", head, 200)
	check("GET", reads[2], "", head, 404)
	check("POST", sample, `{"subject_id":"x","term_id":"y","marks":{}}`, head, 404)
	check("GET", "/exams/terms?session_id=not-a-uuid", "", admin, 400)
	check("PUT", "/exams/terms/"+versionID, `{"name":"First Term","kind":"term"}`, admin, 404)
	check("POST", "/exams/blueprint-versions/"+versionID+"/activate", "", admin, 404)
	check("POST", "/exams/blueprints", `{"session_id":"`+examID+`","class_id":"`+examID+`"}`, admin, 404)
	check("PUT", "/exams/blueprint-versions/"+versionID, `{"definition":{},"extra":1}`, admin, 400)
	// Administrators: everything.
	check("GET", sheets, "", admin, 404)
	check("PATCH", marks, cell, admin, 404)
	check("POST", approve, "", admin, 404)
	check("PATCH", marks, `{"entries":[]}`, admin, 400)
}
