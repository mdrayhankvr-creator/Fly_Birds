package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
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

// TestPostgresOnePageAdmission covers the atomic admission: everything is
// created or nothing is, duplicate guardians are offered instead of created,
// the initial password works once and is never stored or audited in plain
// text, and the permissions of each part are enforced.
func TestPostgresOnePageAdmission(t *testing.T) {
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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := "Admit!Secret" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x62}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-adm-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head, teacher, student := makeUser("admin", "admin"), makeUser("head_teacher", "head"), makeUser("teacher", "teacher"), makeUser("student", "student")
	mail := func(label string) string { return label + "-adm-" + suffix + "@example.test" }
	var sessionID, classID, sectionID string
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
		_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM guardians WHERE name_en LIKE ?", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-adm-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-adm-"+suffix+"@example.test").Error
	})
	var oldModule bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Scan(&oldModule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", oldModule).Error
	})
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('2193','2193-01-01','2193-12-31') RETURNING id").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", "Adm Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "Adm A "+suffix, "AD"+strings.ToUpper(suffix)).Scan(&sectionID).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouterWithPeople(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), people.NewService(people.NewRepository(db)))
	token := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, headH, teacherH, studentH := token(admin), token(head), token(teacher), token(student)
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		res := perform(router, method, path, body, nil, headers)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	errorCode := func(body []byte) string {
		var v struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &v)
		return v.Error.Code
	}
	studentJSON := func(admission string, roll int) string {
		return fmt.Sprintf(`{"admission_no":%q,"name_en":"Admitted %s","name_bn":"ভর্তি","date_of_birth":"2015-04-05","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":%d}`, admission, admission, sessionID, classID, sectionID, roll)
	}
	newGuardian := func(name, phone, email, relationship string, primary, confirm bool) string {
		return fmt.Sprintf(`{"new":{"name_en":%q,"name_bn":"","phone":%q,"email":%q,"address":""},"relationship":%q,"is_primary":%t,"confirm_new":%t}`, name+" "+suffix, phone, email, relationship, primary, confirm)
	}
	newKey := func() string {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		b[6], b[8] = (b[6]&0x0f)|0x40, (b[8]&0x3f)|0x80
		h := hex.EncodeToString(b[:])
		return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	}
	keyed := func(key, admissionNo string, roll int, loginEmail string, guardians ...string) string {
		account := `{"create":false}`
		if loginEmail != "" {
			account = fmt.Sprintf(`{"create":true,"email":%q}`, loginEmail)
		}
		return fmt.Sprintf(`{"idempotency_key":%q,"student":%s,"account":%s,"guardians":[%s]}`, key, studentJSON(admissionNo, roll), account, strings.Join(guardians, ","))
	}
	// admission is one attempt with a fresh key.
	admission := func(admissionNo string, roll int, loginEmail string, guardians ...string) string {
		return keyed(newKey(), admissionNo, roll, loginEmail, guardians...)
	}
	// leftovers counts what an admission attempt could have written.
	leftovers := func(admissionNo, loginEmail, guardianName string) (students, users, guardians int64) {
		t.Helper()
		db.Table("students").Where("admission_no=?", admissionNo).Count(&students)
		db.Table("users").Where("email=?", loginEmail).Count(&users)
		db.Table("guardians").Where("name_en=?", guardianName+" "+suffix).Count(&guardians)
		return
	}
	phone := fmt.Sprintf("0171%07d", time.Now().UnixNano()%10000000)

	// Permissions: only an administrator with students:manage may admit.
	firstKey := newKey()
	body := keyed(firstKey, "ADM-"+suffix+"-0001", 1, mail("kid1"), newGuardian("Father One", phone, "", "Father", true, false))
	call("POST", "/students/admission", body, nil, 401)
	call("POST", "/students/admission", body, headH, 403)
	call("POST", "/students/admission", body, teacherH, 403)
	call("POST", "/students/admission", body, studentH, 403)
	call("GET", "/students/admission/defaults?section_id="+sectionID, "", headH, 403)
	call("GET", "/guardians/matches?phone="+phone, "", studentH, 403)
	call("POST", "/students/admission", `{"idempotency_key":"`+newKey()+`","student":`+studentJSON("ADM-"+suffix+"-bad", 1)+`,"account":{"create":true,"email":"not-an-email"}}`, adminH, 400)
	call("POST", "/students/admission", `{"student":`+studentJSON("ADM-"+suffix+"-bad", 1)+`,"account":{"create":false}}`, adminH, 400)
	call("POST", "/students/admission", admission("ADM-"+suffix+"-bad", 1, "", newGuardian("G1", "", "", "Father", true, false), newGuardian("G2", "", "", "Mother", true, false)), adminH, 400)
	if n, _, _ := leftovers("ADM-"+suffix+"-0001", mail("kid1"), "Father One"); n != 0 {
		t.Fatal("a refused admission wrote a student")
	}

	// A full admission: student, login and the father as primary guardian.
	var admitted struct {
		Data people.AdmissionResult `json:"data"`
	}
	if err := json.Unmarshal(call("POST", "/students/admission", body, adminH, 201), &admitted); err != nil {
		t.Fatal(err)
	}
	first := admitted.Data
	if first.Student.ID == "" || first.Student.UserID == nil || first.Login == nil || first.Login.Email != mail("kid1") || len(first.Login.InitialPassword) < 12 {
		t.Fatalf("admission result %+v", first)
	}
	if len(first.Guardians) != 1 || !first.Guardians[0].IsPrimary || first.Guardians[0].Relationship != "Father" {
		t.Fatalf("guardians %+v", first.Guardians)
	}
	// The initial password signs in as the new student, and only it does.
	if creds, err := authService.Login(ctx, first.Login.Email, first.Login.InitialPassword); err != nil || creds.User.Role != "student" || creds.User.ID != *first.Student.UserID {
		t.Fatalf("initial password login: %+v %v", creds.User, err)
	}
	var hash string
	db.Table("users").Where("id=?", first.Login.UserID).Pluck("password_hash", &hash)
	if hash == "" || strings.Contains(hash, first.Login.InitialPassword) {
		t.Fatal("the initial password is stored in plain text")
	}
	var leaked int64
	db.Table("audit_events").Where("metadata::text LIKE ?", "%"+first.Login.InitialPassword+"%").Count(&leaked)
	if leaked != 0 {
		t.Fatal("the initial password reached the audit log")
	}
	var admittedEvents int64
	db.Table("audit_events").Where("action='student.admitted' AND target_id=?", first.Student.ID).Count(&admittedEvents)
	if admittedEvents != 1 {
		t.Fatalf("admission audit events %d", admittedEvents)
	}
	// Enrolment history starts with the admission.
	var enrolments int64
	db.Table("student_enrolments").Where("student_id=? AND state='current' AND section_id=?", first.Student.ID, sectionID).Count(&enrolments)
	if enrolments != 1 {
		t.Fatalf("current enrolments %d", enrolments)
	}
	fatherID := first.Guardians[0].GuardianID

	// Retry safety: the same key again returns the original admission (200,
	// replayed, the login's email but never its password) and creates nothing.
	var retried struct {
		Data people.AdmissionResult `json:"data"`
	}
	if err := json.Unmarshal(call("POST", "/students/admission", body, adminH, 200), &retried); err != nil {
		t.Fatal(err)
	}
	again := retried.Data
	if !again.Replayed || first.Replayed || again.Student.ID != first.Student.ID || again.Login == nil || again.Login.Email != first.Login.Email || again.Login.UserID != first.Login.UserID || again.Login.InitialPassword != "" || len(again.Guardians) != 1 || again.Guardians[0].GuardianID != fatherID {
		t.Fatalf("retried admission %+v", again)
	}
	if s, u, g := leftovers("ADM-"+suffix+"-0001", mail("kid1"), "Father One"); s != 1 || u != 1 || g != 1 {
		t.Fatalf("a retry created records: %d students, %d logins, %d guardians", s, u, g)
	}
	// The same key with another admission number is not a retry.
	if code := errorCode(call("POST", "/students/admission", keyed(firstKey, "ADM-"+suffix+"-other", 9, ""), adminH, 409)); code != "idempotency_key_reused" {
		t.Fatalf("reused key: %s", code)
	}
	// A new key with the same admission number is a second admission: refused.
	if code := errorCode(call("POST", "/students/admission", admission("ADM-"+suffix+"-0001", 9, mail("kid1b")), adminH, 409)); code != "admission_no_taken" {
		t.Fatalf("second admission with the same number: %s", code)
	}
	if s, u, _ := leftovers("ADM-"+suffix+"-0001", mail("kid1b"), ""); s != 1 || u != 0 {
		t.Fatalf("a refused second admission left %d students, %d logins", s, u)
	}
	var requests int64
	db.Table("admission_requests").Where("idempotency_key=?", firstKey).Count(&requests)
	if requests != 1 {
		t.Fatalf("admission requests for the key: %d", requests)
	}

	// Duplicate guardian: the same phone written another way is offered
	// instead of created, and nothing else is written.
	samePhone := "+880 " + phone[1:5] + "-" + phone[5:]
	dupBody := admission("ADM-"+suffix+"-0002", 2, mail("kid2"), newGuardian("Mother Two", "", "", "Mother", false, false), newGuardian("Father Again", samePhone, "", "Father", true, false))
	res := call("POST", "/students/admission", dupBody, adminH, 409)
	var conflict struct {
		Error struct {
			Code    string                 `json:"code"`
			Index   int                    `json:"index"`
			Matches []people.GuardianMatch `json:"matches"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res, &conflict); err != nil || conflict.Error.Code != "guardian_exists" || conflict.Error.Index != 1 || len(conflict.Error.Matches) != 1 || conflict.Error.Matches[0].ID != fatherID || conflict.Error.Matches[0].MatchedOn != "phone" || len(conflict.Error.Matches[0].Children) != 1 {
		t.Fatalf("duplicate guardian response %s", res)
	}
	if s, u, g := leftovers("ADM-"+suffix+"-0002", mail("kid2"), "Mother Two"); s+u+g != 0 {
		t.Fatalf("a refused duplicate left %d students, %d logins, %d guardians", s, u, g)
	}
	// The lookup used before submitting finds the same guardian, by phone or email.
	if found := call("GET", "/guardians/matches?phone="+url.QueryEscape(samePhone), "", headH, 200); !bytes.Contains(found, []byte(fatherID)) {
		t.Fatalf("matches by phone: %s", found)
	}
	call("GET", "/guardians/matches?phone=12", "", adminH, 200)
	// Linking the existing guardian instead works (siblings share a guardian).
	linked := admission("ADM-"+suffix+"-0002", 2, "", newGuardian("Mother Two", "", mail("mother2"), "Mother", false, false), fmt.Sprintf(`{"guardian_id":%q,"relationship":"Father","is_primary":true}`, fatherID))
	call("POST", "/students/admission", linked, adminH, 201)
	if found := call("GET", "/guardians/matches?email="+url.QueryEscape(strings.ToUpper(mail("mother2"))), "", adminH, 200); !bytes.Contains(found, []byte(`"matched_on":"email"`)) {
		t.Fatalf("matches by email: %s", found)
	}
	// Staff can still confirm a different family on the same phone.
	call("POST", "/students/admission", admission("ADM-"+suffix+"-0003", 3, "", newGuardian("Uncle Three", samePhone, "", "Uncle", true, true)), adminH, 201)

	// Forced failures part-way through: each leaves nothing behind.
	// (a) The roll number is taken: the login was already inserted.
	res = call("POST", "/students/admission", admission("ADM-"+suffix+"-0004", 1, mail("kid4"), newGuardian("Aunt Four", "", "", "Aunt", true, true)), adminH, 409)
	if errorCode(res) != "roll_taken" {
		t.Fatalf("roll conflict %s", res)
	}
	if s, u, g := leftovers("ADM-"+suffix+"-0004", mail("kid4"), "Aunt Four"); s+u+g != 0 {
		t.Fatalf("failed admission (roll) left %d students, %d logins, %d guardians", s, u, g)
	}
	// (b) The second guardian does not exist: the login, the student and the
	// first new guardian were already inserted when the link fails.
	res = call("POST", "/students/admission", admission("ADM-"+suffix+"-0005", 5, mail("kid5"), newGuardian("Aunt Five", "", "", "Aunt", true, true), `{"guardian_id":"00000000-0000-4000-8000-000000000000","relationship":"Father","is_primary":false}`), adminH, 400)
	if s, u, g := leftovers("ADM-"+suffix+"-0005", mail("kid5"), "Aunt Five"); s+u+g != 0 {
		t.Fatalf("failed admission (guardian link) left %d students, %d logins, %d guardians: %s", s, u, g, res)
	}
	// (c) The login email is taken, and (d) the admission number is taken.
	res = call("POST", "/students/admission", admission("ADM-"+suffix+"-0006", 6, mail("kid1"), newGuardian("Aunt Six", "", "", "Aunt", true, true)), adminH, 409)
	if errorCode(res) != "login_email_taken" {
		t.Fatalf("login conflict %s", res)
	}
	if s, _, g := leftovers("ADM-"+suffix+"-0006", "", "Aunt Six"); s+g != 0 {
		t.Fatal("failed admission (login email) left records")
	}
	res = call("POST", "/students/admission", admission(strings.ToLower("ADM-"+suffix+"-0001"), 7, mail("kid7"), newGuardian("Aunt Seven", "", "", "Aunt", true, true)), adminH, 409)
	if errorCode(res) != "admission_no_taken" {
		t.Fatalf("admission number conflict %s", res)
	}
	if _, u, g := leftovers("", mail("kid7"), "Aunt Seven"); u+g != 0 {
		t.Fatal("failed admission (admission number) left records")
	}

	// Defaults: the next free roll in the section and the next admission number.
	var defaults struct {
		Data people.AdmissionDefaults `json:"data"`
	}
	if err := json.Unmarshal(call("GET", "/students/admission/defaults?section_id="+sectionID, "", adminH, 200), &defaults); err != nil {
		t.Fatal(err)
	}
	if defaults.Data.NextRoll != 4 || defaults.Data.AdmissionNo != "ADM-"+suffix+"-0004" || defaults.Data.LoginDomain != "example.test" {
		t.Fatalf("defaults %+v", defaults.Data)
	}
	call("GET", "/students/admission/defaults?section_id=nope", "", adminH, 400)

	// Two requests with the same key at the same moment admit once.
	raceBody := admission("ADM-"+suffix+"-0010", 10, "")
	codes := make(chan int, 2)
	for range 2 {
		go func() { codes <- perform(router, "POST", "/students/admission", raceBody, nil, adminH).Code }()
	}
	got := map[int]int{}
	for range 2 {
		got[<-codes]++
	}
	var raced int64
	db.Table("students").Where("admission_no=?", "ADM-"+suffix+"-0010").Count(&raced)
	if raced != 1 || got[201] != 1 || got[200] != 1 {
		t.Fatalf("concurrent same-key admissions: %v, %d students", got, raced)
	}

	// Each part keeps its own permission: without users:create an
	// administrator can admit, but not create a login.
	if err := db.Exec("DELETE FROM role_permissions WHERE role_id='admin' AND permission_id='users:create'").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','users:create') ON CONFLICT DO NOTHING").Error
	})
	call("POST", "/students/admission", admission("ADM-"+suffix+"-0008", 8, mail("kid8")), adminH, 403)
	if s, u, _ := leftovers("ADM-"+suffix+"-0008", mail("kid8"), ""); s+u != 0 {
		t.Fatal("a refused login left records")
	}
	call("POST", "/students/admission", admission("ADM-"+suffix+"-0008", 8, ""), adminH, 201)
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','users:create') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
}
