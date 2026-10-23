package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/attendance"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresStudentProfileAndIDCards(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	var ledger int64
	if err := db.Table("schema_migrations").Where("version=1601").Count(&ledger).Error; err != nil || ledger != 1 {
		t.Fatalf("migration 1601 missing: %d %v", ledger, err)
	}
	// Other integration tests can edit the shared role matrix. Restore the grants under test.
	if err := db.Exec("INSERT INTO role_permissions (role_id,permission_id) VALUES ('admin','students:read'),('admin','students:manage'),('admin','guardians:manage'),('admin','attendance:students_manage'),('head_teacher','students:read'),('student','students:read_self'),('guardian','guardians:read_children'),('teacher','auth:read_self') ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:6])
	year := 6000 + int(binary.BigEndian.Uint16(random[6:])%3000)
	password := "CardSecret!" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	var userIDs []string
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-card-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		userIDs = append(userIDs, u.ID)
		return u
	}
	admin, head, teacher := makeUser("admin", "admin"), makeUser("head_teacher", "head"), makeUser("teacher", "teacher")
	studentUser, otherStudentUser := makeUser("student", "student"), makeUser("student", "other-student")
	guardianUser, otherGuardianUser := makeUser("guardian", "guardian"), makeUser("guardian", "other-guardian")

	modules := map[string]bool{}
	for _, key := range []string{"students", "attendance"} {
		var old bool
		if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key=?", school.SingleSchoolID, key).Scan(&old).Error; err != nil {
			t.Fatal(err)
		}
		modules[key] = old
		if err := db.Table("school_modules").Where("school_id=? AND module_key=?", school.SingleSchoolID, key).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	var settingsBefore people.CardSettings
	if err := db.Table("id_card_settings").Select(strings.Join([]string{"COALESCE(to_char(valid_until,'YYYY-MM-DD'),'') AS valid_until", "signatory_title_en", "signatory_title_bn", "emergency_phone", "usage_note_en", "usage_note_bn", "return_note_en", "return_note_bn", "show_qr"}, ",")).Where("id=1").Take(&settingsBefore).Error; err != nil {
		t.Fatal(err)
	}
	var sessionID, classID, sectionID string
	t.Cleanup(func() {
		_ = db.Exec("UPDATE id_card_settings SET valid_until=NULLIF(?,'')::date,signatory_title_en=?,signatory_title_bn=?,emergency_phone=?,usage_note_en=?,usage_note_bn=?,return_note_en=?,return_note_bn=?,show_qr=?,signature=NULL,signature_type=NULL WHERE id=1",
			settingsBefore.ValidUntil, settingsBefore.SignatoryTitleEN, settingsBefore.SignatoryTitleBN, settingsBefore.EmergencyPhone, settingsBefore.UsageNoteEN, settingsBefore.UsageNoteBN, settingsBefore.ReturnNoteEN, settingsBefore.ReturnNoteBN, settingsBefore.ShowQR).Error
		if sessionID != "" {
			_ = db.Exec("DELETE FROM student_attendance WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
			_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		}
		_ = db.Exec("DELETE FROM guardians WHERE name_en LIKE ?", "Card Guardian "+suffix+"%").Error
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		for key, old := range modules {
			_ = db.Table("school_modules").Where("school_id=? AND module_key=?", school.SingleSchoolID, key).Update("enabled", old).Error
		}
		_ = db.Exec("DELETE FROM users WHERE id IN ?", userIDs).Error
	})
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES (?,'2026-01-01','2026-12-31') RETURNING id", fmt.Sprintf("%04d", year)).Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", "Card Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "A", "C"+strings.ToUpper(suffix[:6])).Scan(&sectionID).Error; err != nil {
		t.Fatal(err)
	}
	peopleService := people.NewService(people.NewRepository(db))
	router := NewRouterWithStudentAttendance(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), peopleService, nil, nil, attendance.NewStudentService(attendance.NewStudentRepository(db)))
	headers := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, headH, teacherH, studentH, otherStudentH, guardianH, otherGuardianH := headers(admin), headers(head), headers(teacher), headers(studentUser), headers(otherStudentUser), headers(guardianUser), headers(otherGuardianUser)
	call := func(method, path, body string, h map[string]string, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, h)
		if r.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	decode := func(body []byte, into any) {
		t.Helper()
		if err := json.Unmarshal(body, into); err != nil {
			t.Fatal(err)
		}
	}

	// ---- Profile: an old-style student (no new fields) stays valid ----
	legacy := fmt.Sprintf(`{"admission_no":"CARD-%s-1","name_en":"Legacy Student","name_bn":"পুরনো","date_of_birth":"2014-02-03","status":"active","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":1,"user_id":%q}`, suffix, sessionID, classID, sectionID, studentUser.ID)
	var created struct{ Data people.Student }
	decode(call("POST", "/students", legacy, adminH, 201), &created)
	if created.Data.Gender != "" || created.Data.FatherNameEN != "" || created.Data.AdmissionDate != "" || created.Data.PrimaryGuardian != nil {
		t.Fatalf("legacy student has unexpected details: %+v", created.Data)
	}
	studentID := created.Data.ID
	full := strings.TrimSuffix(legacy, "}") + `,"student_code":"FB-2026-0001","gender":"female","blood_group":"B+","father_name_en":"Karim Uddin","father_name_bn":"করিম উদ্দিন","mother_name_en":"Rahima Begum","mother_name_bn":"রহিমা বেগম","present_address":"12 Lake Road, Dhaka","permanent_address":"Village Road, Cumilla","admission_date":"2026-01-05"}`
	var saved struct{ Data people.Student }
	decode(call("PATCH", "/students/"+studentID, full, adminH, 200), &saved)
	if saved.Data.StudentCode != "FB-2026-0001" || saved.Data.BloodGroup != "B+" || saved.Data.MotherNameBN != "রহিমা বেগম" || saved.Data.AdmissionDate != "2026-01-05" {
		t.Fatalf("profile not saved: %+v", saved.Data)
	}
	// A client that does not know the new fields must not erase them.
	decode(call("PATCH", "/students/"+studentID, strings.Replace(legacy, "Legacy Student", "Renamed Student", 1), adminH, 200), &saved)
	if saved.Data.NameEN != "Renamed Student" || saved.Data.FatherNameEN != "Karim Uddin" || saved.Data.AdmissionDate != "2026-01-05" || saved.Data.Gender != "female" {
		t.Fatalf("legacy update erased profile details: %+v", saved.Data)
	}
	// An explicit empty value clears a field.
	decode(call("PATCH", "/students/"+studentID, strings.TrimSuffix(full, "}")+`,"blood_group":""}`, adminH, 200), &saved)
	if saved.Data.BloodGroup != "" {
		t.Fatalf("blood group not cleared: %q", saved.Data.BloodGroup)
	}
	decode(call("PATCH", "/students/"+studentID, full, adminH, 200), &saved)
	for name, body := range map[string]string{
		"gender":                 strings.Replace(full, `"gender":"female"`, `"gender":"unknown"`, 1),
		"blood group":            strings.Replace(full, `"blood_group":"B+"`, `"blood_group":"C+"`, 1),
		"admission before birth": strings.Replace(full, `"admission_date":"2026-01-05"`, `"admission_date":"2010-01-01"`, 1),
		"admission far ahead":    strings.Replace(full, `"admission_date":"2026-01-05"`, `"admission_date":"2099-01-01"`, 1),
		"long address":           strings.Replace(full, `"12 Lake Road, Dhaka"`, `"`+strings.Repeat("a", 501)+`"`, 1),
	} {
		if r := perform(router, "PATCH", "/students/"+studentID, body, nil, adminH); r.Code != 400 {
			t.Fatalf("%s accepted: %d %s", name, r.Code, r.Body.String())
		}
	}
	other := fmt.Sprintf(`{"admission_no":"CARD-%s-2","name_en":"Other Student","name_bn":"","date_of_birth":"","status":"active","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":2,"user_id":%q,"student_code":"fb-2026-0001"}`, suffix, sessionID, classID, sectionID, otherStudentUser.ID)
	call("POST", "/students", other, adminH, 409) // student ID is unique regardless of case
	var second struct{ Data people.Student }
	decode(call("POST", "/students", strings.Replace(other, "fb-2026-0001", "FB-2026-0002", 1), adminH, 201), &second)
	call("PATCH", "/students/"+studentID, full, headH, 403)
	call("PATCH", "/students/"+studentID, full, studentH, 403)

	// Guardian shown on the profile comes from the existing primary link.
	var guardian struct{ Data people.Guardian }
	decode(call("POST", "/guardians", fmt.Sprintf(`{"name_en":"Card Guardian %s","name_bn":"অভিভাবক","phone":"01711000000","email":"","address":"","user_id":%q}`, suffix, guardianUser.ID), adminH, 201), &guardian)
	call("PUT", "/students/"+studentID+"/guardians", fmt.Sprintf(`{"guardian_id":%q,"relationship":"Mother","is_primary":true}`, guardian.Data.ID), adminH, 200)
	var guardiansBefore int64
	db.Table("guardians").Where("name_en LIKE ?", "Card Guardian "+suffix+"%").Count(&guardiansBefore)
	var profile struct{ Data people.Student }
	decode(call("GET", "/students/me", "", studentH, 200), &profile)
	if profile.Data.PrimaryGuardian == nil || profile.Data.PrimaryGuardian.Relationship != "Mother" || profile.Data.PrimaryGuardian.Phone != "01711000000" || profile.Data.FatherNameBN != "করিম উদ্দিন" {
		t.Fatalf("self profile: %+v %+v", profile.Data, profile.Data.PrimaryGuardian)
	}
	call("GET", "/students/"+second.Data.ID, "", studentH, 404)

	// ---- Card data and scope ----
	type cardBody struct {
		Data struct {
			Student    people.Student      `json:"student"`
			ValidUntil string              `json:"valid_until"`
			QRPayload  string              `json:"qr_payload"`
			QRDataURL  string              `json:"qr_data_url"`
			School     people.CardSchool   `json:"school"`
			Settings   people.CardSettings `json:"settings"`
		}
	}
	link := regexp.MustCompile(`^https://school\.example\.test/verify/([A-Za-z0-9_-]{22})$`)
	var card cardBody
	decode(call("GET", "/students/"+studentID+"/card?origin=https://school.example.test", "", adminH, 200), &card)
	match := link.FindStringSubmatch(card.Data.QRPayload)
	if match == nil || !strings.HasPrefix(card.Data.QRDataURL, "data:image/png;base64,") || card.Data.ValidUntil != "2026-12-31" || card.Data.Student.PrimaryGuardian == nil {
		t.Fatalf("card: %+v", card.Data)
	}
	token := match[1]
	if strings.Contains(card.Data.QRPayload, studentID) {
		t.Fatal("card QR exposes the student identifier")
	}
	decode(call("GET", "/students/"+studentID+"/card?origin=javascript:alert(1)", "", adminH, 200), &card)
	if strings.Contains(card.Data.QRPayload, "javascript") || !strings.HasSuffix(card.Data.QRPayload, "/verify/"+token) {
		t.Fatalf("unsafe origin used or token changed: %s", card.Data.QRPayload)
	}
	call("GET", "/students/"+studentID+"/card", "", studentH, 200)
	call("GET", "/students/"+studentID+"/card", "", guardianH, 200)
	call("GET", "/students/"+studentID+"/card", "", headH, 200)
	call("GET", "/students/"+studentID+"/card", "", otherStudentH, 404)
	call("GET", "/students/"+studentID+"/card", "", otherGuardianH, 404)
	call("GET", "/students/"+studentID+"/card", "", teacherH, 403)

	// ---- Bulk cards and settings permissions ----
	var bulk struct {
		Data struct {
			Cards []struct {
				Student   people.Student `json:"student"`
				QRPayload string         `json:"qr_payload"`
			} `json:"cards"`
		}
	}
	decode(call("GET", "/students/cards?section_id="+sectionID, "", adminH, 200), &bulk)
	if len(bulk.Data.Cards) != 2 || bulk.Data.Cards[0].Student.RollNo == nil || *bulk.Data.Cards[0].Student.RollNo != 1 || bulk.Data.Cards[0].Student.PrimaryGuardian == nil {
		t.Fatalf("bulk cards: %+v", bulk.Data.Cards)
	}
	call("GET", "/students/cards?section_id="+sectionID, "", headH, 403)
	call("GET", "/students/cards?section_id="+sectionID, "", teacherH, 403)
	call("GET", "/students/cards?section_id="+sectionID, "", studentH, 403)
	call("GET", "/students/cards?section_id=not-a-uuid", "", adminH, 400)
	call("GET", "/students/card-settings", "", headH, 200)
	call("GET", "/students/card-settings", "", studentH, 403)
	settings := `{"valid_until":"2027-03-31","signatory_title_en":"Head of School","signatory_title_bn":"প্রধান শিক্ষক","emergency_phone":"+880 1711-000000","usage_note_en":"","usage_note_bn":"","return_note_en":"","return_note_bn":"","show_qr":true}`
	call("PUT", "/students/card-settings", settings, headH, 403)
	call("PUT", "/students/card-settings", strings.Replace(settings, "+880 1711-000000", "call me", 1), adminH, 400)
	call("PUT", "/students/card-settings", settings, adminH, 200)
	decode(call("GET", "/students/"+studentID+"/card", "", studentH, 200), &card)
	if card.Data.ValidUntil != "2027-03-31" || card.Data.Settings.SignatoryTitleBN != "প্রধান শিক্ষক" {
		t.Fatalf("settings not applied: %+v", card.Data.Settings)
	}

	// ---- Signature: re-encoded, administrators only ----
	var sig bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 12))
	img.Set(3, 3, color.Black)
	if err := png.Encode(&sig, img); err != nil {
		t.Fatal(err)
	}
	sigBody := append(sig.Bytes(), []byte("<script>trailing</script>")...)
	upload := func(h map[string]string, want int) {
		t.Helper()
		req := httptest.NewRequest("PUT", "/students/card-settings/signature", bytes.NewReader(sigBody))
		req.Header.Set("Content-Type", "image/png")
		for k, v := range h {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("signature upload: %d %s", w.Code, w.Body.String())
		}
	}
	upload(headH, 403)
	upload(adminH, 200)
	got := perform(router, "GET", "/students/card-settings/signature", "", nil, adminH)
	if got.Code != 200 || got.Header().Get("Content-Type") != "image/png" || bytes.Contains(got.Body.Bytes(), []byte("<script>")) {
		t.Fatalf("signature read: %d %q", got.Code, got.Header().Get("Content-Type"))
	}
	call("GET", "/students/card-settings/signature", "", headH, 403)
	call("GET", "/students/card-settings/signature", "", studentH, 403)

	// ---- Public verification reveals no personal data ----
	verified := perform(router, "GET", "/public/id-cards/"+token, "", nil, nil)
	if verified.Code != 200 || !strings.Contains(verified.Body.String(), `"state":"valid"`) {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body.String())
	}
	for _, secret := range []string{"Renamed", "Karim", "করিম", studentID, "CARD-" + suffix, "FB-2026-0001", "01711000000", "Lake Road", "2014-02-03"} {
		if strings.Contains(verified.Body.String(), secret) {
			t.Fatalf("public verification leaks %q: %s", secret, verified.Body.String())
		}
	}
	if perform(router, "GET", "/public/id-cards/AAAAAAAAAAAAAAAAAAAAAA", "", nil, nil).Code != 404 || perform(router, "GET", "/public/id-cards/x'%20OR%201=1", "", nil, nil).Code != 404 {
		t.Fatal("unknown or malformed token not rejected")
	}

	// ---- QR attendance accepts the verification link ----
	scan := func(payload string, want int) {
		t.Helper()
		call("POST", "/attendance/students/qr", fmt.Sprintf(`{"payload":%q,"attendance_date":"2026-10-20"}`, payload), adminH, want)
	}
	scan(card.Data.QRPayload, 201)
	var source string
	db.Table("student_attendance").Select("source").Where("student_id=? AND attendance_date='2026-10-20'", studentID).Scan(&source)
	if source != "qr" {
		t.Fatalf("QR attendance not recorded: %q", source)
	}
	scan("FB-STUDENT:"+second.Data.ID, 201) // cards printed before this change keep working
	scan("https://school.example.test/verify/AAAAAAAAAAAAAAAAAAAAAA", 404)
	scan("https://school.example.test/verify/short", 400)

	// ---- Reissue invalidates the old card ----
	call("POST", "/students/"+studentID+"/card/reissue", "", headH, 403)
	call("POST", "/students/"+studentID+"/card/reissue", "", adminH, 200)
	if perform(router, "GET", "/public/id-cards/"+token, "", nil, nil).Code != 404 {
		t.Fatal("old card still verifies after reissue")
	}
	decode(call("GET", "/students/"+studentID+"/card", "", adminH, 200), &card)
	newToken := card.Data.QRPayload[strings.LastIndex(card.Data.QRPayload, "/")+1:]
	if newToken == token || perform(router, "GET", "/public/id-cards/"+newToken, "", nil, nil).Code != 200 {
		t.Fatal("reissued card does not verify")
	}
	call("POST", "/attendance/students/qr", fmt.Sprintf(`{"payload":"https://school.example.test/verify/%s","attendance_date":"2026-10-21"}`, token), adminH, 404)

	// Inactive students and a disabled module.
	if err := db.Exec("UPDATE students SET status='inactive' WHERE id=?", studentID).Error; err != nil {
		t.Fatal(err)
	}
	if body := perform(router, "GET", "/public/id-cards/"+newToken, "", nil, nil).Body.String(); !strings.Contains(body, `"state":"inactive"`) {
		t.Fatalf("inactive card: %s", body)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if perform(router, "GET", "/public/id-cards/"+newToken, "", nil, nil).Code != 404 {
		t.Fatal("verification works while the Students module is disabled")
	}

	// No duplicate guardians, and no tokens or personal data in audit events.
	var guardiansAfter int64
	db.Table("guardians").Where("name_en LIKE ?", "Card Guardian "+suffix+"%").Count(&guardiansAfter)
	if guardiansAfter != guardiansBefore || guardiansAfter != 1 {
		t.Fatalf("guardian records: before %d after %d", guardiansBefore, guardiansAfter)
	}
	var events []struct {
		Action   string
		Metadata string
	}
	if err := db.Raw("SELECT action,metadata::text AS metadata FROM audit_events WHERE actor_user_id=? AND action LIKE ANY (ARRAY['student.%','id_card.%'])", admin.ID).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, e := range events {
		actions[e.Action]++
		for _, secret := range []string{token, newToken, "Karim", "Lake Road", "01711000000"} {
			if strings.Contains(e.Metadata, secret) {
				t.Fatalf("audit %s contains %q", e.Action, secret)
			}
		}
	}
	if actions["student.card_reissued"] != 1 || actions["id_card.settings_saved"] != 1 || actions["id_card.signature_saved"] != 1 {
		t.Fatalf("audit actions: %+v", actions)
	}
	_ = http.StatusOK
}
