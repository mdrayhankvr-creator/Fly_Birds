package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresStudentIDSignIn: students get logins in one click (one at a
// time or for a whole section) and sign in with their Student ID. The ID form
// answers exactly like the email form, lockout applies to it, placeholder
// addresses cannot sign in, and passwords never reach the audit log.
func TestPostgresStudentIDSignIn(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := "Login!Secret" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x6c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-sid-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head, teacher := makeUser("admin", "admin"), makeUser("head_teacher", "head"), makeUser("teacher", "teacher")
	var sessionID, classID, sectionID string
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ? OR login_id ILIKE ?)", "%-sid-"+suffix+"@example.test", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ? OR login_id ILIKE ?", "%-sid-"+suffix+"@example.test", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
	})
	var oldModule bool
	db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Scan(&oldModule)
	db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", true)
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", oldModule).Error
	})
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('2194','2194-01-01','2194-12-31') RETURNING id").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", "SID Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "SID A "+suffix, "SI"+strings.ToUpper(suffix)).Scan(&sectionID).Error; err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(suffix)
	makeStudent := func(admission, code, status string, roll int) string {
		t.Helper()
		var id string
		if err := db.Raw("INSERT INTO students (admission_no,name_en,name_bn,status,session_id,class_id,section_id,roll_no,student_code) VALUES (?,?,'শিক্ষার্থী',?,?,?,?,?,?) RETURNING id", admission, "Pupil "+admission, status, sessionID, classID, sectionID, roll, code).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	// One student uses a Student ID; one has none; one has a Student ID that
	// is another student's admission number (so it falls back to its own
	// admission number); one is inactive.
	withCode := makeStudent("ADM-"+upper+"-1", "SID-"+upper+"-1", "active", 1)
	plain := makeStudent("ADM-"+upper+"-2", "", "active", 2)
	clash := makeStudent("ADM-"+upper+"-3", "ADM-"+upper+"-2", "active", 3)
	inactive := makeStudent("ADM-"+upper+"-4", "", "inactive", 4)

	router := NewRouterWithPeople(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), people.NewService(people.NewRepository(db)))
	bearer := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, headH, teacherH := bearer(admin), bearer(head), bearer(teacher)
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		res := perform(router, method, path, body, nil, headers)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	signIn := func(body string) (int, string) {
		res := perform(router, http.MethodPost, "/auth/login", body, nil, nil)
		return res.Code, res.Body.String()
	}

	// One click on the student's profile: administrators only.
	call(http.MethodPost, "/students/"+withCode+"/login", `{}`, headH, http.StatusForbidden)
	call(http.MethodPost, "/students/"+withCode+"/login", `{}`, teacherH, http.StatusForbidden)
	var made struct {
		Data people.StudentLogin `json:"data"`
	}
	if err := json.Unmarshal(call(http.MethodPost, "/students/"+withCode+"/login", `{}`, adminH, http.StatusCreated), &made); err != nil {
		t.Fatal(err)
	}
	slip := made.Data
	// Every slip says when its temporary password stops working.
	expiresSoon := func(at time.Time) bool {
		want := time.Now().Add(auth.TemporaryPasswordLifetime)
		return at.After(want.Add(-2*time.Minute)) && at.Before(want.Add(2*time.Minute))
	}
	if !expiresSoon(slip.PasswordExpiresAt) {
		t.Fatalf("one-click slip expiry %v", slip.PasswordExpiresAt)
	}
	if slip.LoginID != "SID-"+upper+"-1" || slip.Email != "" || len(slip.InitialPassword) != 14 || slip.AdmissionNo != "ADM-"+upper+"-1" || slip.RollNo == nil || *slip.RollNo != 1 {
		t.Fatalf("slip %+v", slip)
	}
	var stored struct {
		Email              string
		MustChangePassword bool
		LoginID            string
	}
	db.Table("users").Select("email, must_change_password, login_id").Where("id=?", slip.UserID).Scan(&stored)
	if !auth.IsPlaceholderEmail(stored.Email) || !stored.MustChangePassword || stored.LoginID != slip.LoginID {
		t.Fatalf("stored login %+v", stored)
	}
	var linked string
	db.Table("students").Select("COALESCE(user_id::text,'')").Where("id=?", withCode).Scan(&linked)
	if linked != slip.UserID {
		t.Fatal("the login was not linked to the student")
	}
	if code := errCode(call(http.MethodPost, "/students/"+withCode+"/login", `{}`, adminH, http.StatusConflict)); code != "student_has_login" {
		t.Fatalf("second login: %s", code)
	}

	// Sign in with the Student ID, any case, under "identifier" or "email".
	for _, body := range []string{
		fmt.Sprintf(`{"identifier":%q,"password":%q}`, strings.ToLower(slip.LoginID), slip.InitialPassword),
		fmt.Sprintf(`{"email":%q,"password":%q}`, slip.LoginID, slip.InitialPassword),
	} {
		code, text := signIn(body)
		if code != http.StatusOK || !strings.Contains(text, `"must_change_password":true`) || !strings.Contains(text, `"login_id":"`+slip.LoginID+`"`) {
			t.Fatalf("Student ID sign-in %s: %d %s", body, code, text)
		}
	}
	// The forced first-sign-in change applies as for any reset or admission.
	creds, err := authService.Login(ctx, slip.LoginID, slip.InitialPassword)
	if err != nil {
		t.Fatal(err)
	}
	studentH := map[string]string{"Authorization": "Bearer " + creds.AccessToken}
	if code := errCode(call(http.MethodGet, "/students/me", "", studentH, http.StatusForbidden)); code != "password_change_required" {
		t.Fatalf("forced change gate: %s", code)
	}
	// The placeholder address is not a way in.
	if code, _ := signIn(fmt.Sprintf(`{"identifier":%q,"password":%q}`, stored.Email, slip.InitialPassword)); code != http.StatusUnauthorized {
		t.Fatalf("placeholder email sign-in: %d", code)
	}
	// Staff never have a sign-in ID.
	if err := db.Exec("UPDATE users SET login_id = ? WHERE id = ?", "T-"+upper, teacher.ID).Error; err == nil {
		t.Fatal("a teacher was given a sign-in ID")
	}

	// No probing: an unknown ID, a known ID with a wrong password and an
	// unknown email all get the same answer, and so does a locked account.
	_, unknownID := signIn(`{"identifier":"NOPE-` + upper + `","password":"wrong-password-1"}`)
	_, wrongPassword := signIn(fmt.Sprintf(`{"identifier":%q,"password":"wrong-password-1"}`, slip.LoginID))
	_, unknownEmail := signIn(`{"identifier":"nobody-` + suffix + `@example.test","password":"wrong-password-1"}`)
	if unknownID != wrongPassword || unknownID != unknownEmail || !strings.Contains(unknownID, "invalid_credentials") {
		t.Fatalf("answers differ:\n%s\n%s\n%s", unknownID, wrongPassword, unknownEmail)
	}
	if code, _ := signIn(`{"identifier":"has space","password":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("malformed identifier: %d", code)
	}
	// Lockout counts Student ID attempts and holds even the right password.
	for i := 1; i < auth.MaxLoginFailure; i++ {
		signIn(fmt.Sprintf(`{"identifier":%q,"password":"wrong-password-%d"}`, slip.LoginID, i))
	}
	code, locked := signIn(fmt.Sprintf(`{"identifier":%q,"password":%q}`, slip.LoginID, slip.InitialPassword))
	if code != http.StatusUnauthorized || locked != unknownID {
		t.Fatalf("locked account: %d %s", code, locked)
	}

	// The whole section at once: active students without a login only.
	call(http.MethodPost, "/students/logins?section_id="+sectionID, "", headH, http.StatusForbidden)
	var bulk struct {
		Data struct {
			Logins []people.StudentLogin `json:"logins"`
		} `json:"data"`
	}
	if err := json.Unmarshal(call(http.MethodPost, "/students/logins?section_id="+sectionID, "", adminH, http.StatusCreated), &bulk); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range bulk.Data.Logins {
		if len(l.InitialPassword) != 14 || !expiresSoon(l.PasswordExpiresAt) {
			t.Fatalf("slip without a password or expiry: %+v", l)
		}
		got[l.StudentID] = l.LoginID
	}
	if len(got) != 2 || got[plain] != "ADM-"+upper+"-2" || got[clash] != "ADM-"+upper+"-3" || got[inactive] != "" {
		t.Fatalf("section logins %v", got)
	}
	if code := errCode(call(http.MethodPost, "/students/logins?section_id="+sectionID, "", adminH, http.StatusConflict)); code != "no_students_without_login" {
		t.Fatalf("second section run: %s", code)
	}
	var sources []string
	db.Table("audit_events").Where("action='user.created' AND actor_user_id=? AND metadata->>'source'='section'", admin.ID).Pluck("metadata->>'student_id'", &sources)
	var summary int64
	db.Table("audit_events").Where("action='students.logins_created' AND target_id=? AND metadata->>'count'='2'", sectionID).Count(&summary)
	if len(sources) != 2 || summary != 1 {
		t.Fatalf("section audit: %v %d", sources, summary)
	}
	for _, l := range append(bulk.Data.Logins, slip) {
		var leaked int64
		db.Table("audit_events").Where("metadata::text LIKE ?", "%"+l.InitialPassword+"%").Count(&leaked)
		if leaked != 0 {
			t.Fatal("a temporary password reached the audit log")
		}
	}

	// Admission without an email: the login signs in with the admission number.
	var admitted struct {
		Data people.AdmissionResult `json:"data"`
	}
	body := fmt.Sprintf(`{"idempotency_key":"6f1c2a54-7d3e-4b8a-9c11-%s","student":{"admission_no":%q,"name_en":"Admitted %s","name_bn":"ভর্তি","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":9},"account":{"create":true,"email":""},"guardians":[]}`,
		suffix+"00", "ADM-"+upper+"-9", suffix, sessionID, classID, sectionID)
	if err := json.Unmarshal(call(http.MethodPost, "/students/admission", body, adminH, http.StatusCreated), &admitted); err != nil {
		t.Fatal(err)
	}
	if l := admitted.Data.Login; l == nil || l.LoginID != "ADM-"+upper+"-9" || l.Email != "" || len(l.InitialPassword) != 14 || l.PasswordExpiresAt == nil || !expiresSoon(*l.PasswordExpiresAt) {
		t.Fatalf("admission login %+v", admitted.Data.Login)
	}
	if code, _ := signIn(fmt.Sprintf(`{"identifier":"adm-%s-9","password":%q}`, suffix, admitted.Data.Login.InitialPassword)); code != http.StatusOK {
		t.Fatalf("admitted student sign-in: %d", code)
	}
}

func errCode(body []byte) string {
	var v struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &v)
	return v.Error.Code
}
