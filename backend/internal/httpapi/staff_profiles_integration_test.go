package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

// TestPostgresStaffProfilePrivacy checks each staff profile field against
// each role: what is always shown, what the teacher shares per audience,
// what only administrators, the head teacher and the teacher see, and that
// the NID never leaves the server except through the audited reveal.
func TestPostgresStaffProfilePrivacy(t *testing.T) {
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
	const password = "Profile-test-password-2026"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x73}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(label, role string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-prof-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	adminUser, head, owner, colleague, accountant, student, guardian := makeUser("admin", "admin"), makeUser("head", "head_teacher"), makeUser("owner", "teacher"), makeUser("colleague", "teacher"), makeUser("accounts", "accountant"), makeUser("student", "student"), makeUser("guardian", "guardian")
	const nid = "1990123456789"
	const mpo = "MPO-" + "4471"
	var employeeID, colleagueEmployee string
	code := strings.ToUpper(suffix)
	if err := db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,phone,joined_on,email,qualifications) VALUES (?,?,'teacher','Owner Teacher','শিক্ষক','Assistant Teacher','01711-555000','2020-01-05','owner.contact@example.test','[\"MSc Mathematics\",\"B.Ed\"]') RETURNING id", "P"+code, owner.ID).Scan(&employeeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Colleague','Assistant Teacher','2021-01-01') RETURNING id", "Q"+code, colleague.ID).Scan(&colleagueEmployee).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM employees WHERE id IN (?,?)", employeeID, colleagueEmployee).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-prof-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-prof-"+suffix+"@example.test").Error
	})
	var oldModule bool
	db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Scan(&oldModule)
	db.Table("school_modules").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Update("enabled", true)
	t.Cleanup(func() {
		db.Table("school_modules").Where("school_id=? AND module_key='staff'", school.SingleSchoolID).Update("enabled", oldModule)
	})
	router := NewRouterWithBatchServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, staff.NewService(staff.NewRepository(db)), nil)
	token := func(u auth.User) map[string]string {
		c, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + c.AccessToken}
	}
	as := map[string]map[string]string{"admin": token(adminUser), "head": token(head), "owner": token(owner), "colleague": token(colleague), "accountant": token(accountant), "student": token(student), "guardian": token(guardian)}
	call := func(who, method, path, body string, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, as[who])
		if r.Code != want {
			t.Fatalf("%s %s %s: got %d want %d: %s", who, method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	profileOf := func(who string) map[string]any {
		t.Helper()
		var body struct {
			Data map[string]any `json:"data"`
		}
		raw := call(who, "GET", "/staff/profiles/"+employeeID, "", 200)
		if bytes.Contains(raw, []byte(nid)) {
			t.Fatalf("%s: the profile carries the full NID", who)
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		return body.Data
	}
	has := func(p map[string]any, field string) bool { _, ok := p[field]; return ok }
	expectFields := func(who string, p map[string]any, want map[string]bool) {
		t.Helper()
		for field, visible := range want {
			if has(p, field) != visible {
				t.Errorf("%s: field %s visible=%v, want %v", who, field, has(p, field), visible)
			}
		}
	}

	// Not signed in: nothing.
	for _, path := range []string{"/staff/profiles/" + employeeID, "/staff/profiles/" + employeeID + "/photo"} {
		if r := perform(router, "GET", path, "", nil, nil); r.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", path, r.Code)
		}
	}
	// Official fields are the administrator's to set; the audit names fields only.
	call("owner", "PATCH", "/staff/employees/"+employeeID+"/official", `{"nid":"`+nid+`"}`, 403)
	call("head", "PATCH", "/staff/employees/"+employeeID+"/official", `{"nid":"`+nid+`"}`, 403)
	call("admin", "PATCH", "/staff/employees/"+employeeID+"/official", `{"nid":"12345"}`, 400)
	if body := call("admin", "PATCH", "/staff/employees/"+employeeID+"/official", `{"nid":"`+nid+`","mpo_index":"`+mpo+`"}`, 200); bytes.Contains(body, []byte(nid)) {
		t.Fatal("saving the NID echoed it back")
	}

	// Defaults: nothing shared beyond the school's leaders.
	always := []string{"name_en", "designation", "teaching", "today", "photo_version"}
	for _, who := range []string{"admin", "head", "owner", "colleague", "accountant", "student", "guardian"} {
		p := profileOf(who)
		for _, field := range always {
			if !has(p, field) {
				t.Errorf("%s: %s missing", who, field)
			}
		}
		leader := who == "admin" || who == "head" || who == "owner"
		expectFields(who, p, map[string]bool{"phone": leader, "email": leader, "qualifications": leader, "joined_on": leader, "attendance": leader, "mpo_index": leader, "nid_masked": who == "admin" || who == "owner", "visibility": who == "admin" || who == "owner"})
		if who == "admin" || who == "owner" {
			if p["nid_masked"] != "••••••••••789" || p["can_reveal_nid"] != true {
				t.Errorf("%s: nid_masked %v, can_reveal %v", who, p["nid_masked"], p["can_reveal_nid"])
			}
		}
		if (who == "admin") != (p["can_see_salary"] == true) || (who == "admin") != (p["can_edit_official"] == true) || (who == "owner") != (p["can_edit_own"] == true) {
			t.Errorf("%s: abilities %v %v %v", who, p["can_see_salary"], p["can_edit_official"], p["can_edit_own"])
		}
	}
	if q := profileOf("head")["qualifications"].([]any); len(q) != 2 || q[0] != "MSc Mathematics" {
		t.Fatalf("qualifications %v", q)
	}

	// The teacher shares per field and per audience.
	call("colleague", "PATCH", "/staff/me/profile", `{"qualifications":["Hacked"],"visibility":{}}`, 200) // edits the colleague's own record only
	if q := profileOf("head")["qualifications"].([]any); len(q) != 2 {
		t.Fatal("another teacher changed this profile")
	}
	call("owner", "PATCH", "/staff/me/profile", `{"qualifications":["  MSc   Mathematics ","B.Ed",""],"visibility":{"phone":{"staff":true,"families":false},"email":{"staff":false,"families":false},"qualifications":{"staff":false,"families":true}}}`, 200)
	for who, want := range map[string]map[string]bool{
		"colleague":  {"phone": true, "email": false, "qualifications": false},
		"accountant": {"phone": true, "email": false, "qualifications": false},
		"student":    {"phone": false, "email": false, "qualifications": true},
		"guardian":   {"phone": false, "email": false, "qualifications": true},
		"head":       {"phone": true, "email": true, "qualifications": true},
	} {
		p := profileOf(who)
		expectFields(who, p, want)
		expectFields(who, p, map[string]bool{"mpo_index": who == "head", "nid_masked": false, "joined_on": who == "head"})
	}
	if q := profileOf("student")["qualifications"].([]any); len(q) != 2 || q[0] != "MSc Mathematics" {
		t.Fatalf("qualifications were not cleaned: %v", q)
	}
	// The visibility choice cannot open the NID or the MPO/index: there is no
	// such choice, and an attempt is refused.
	call("owner", "PATCH", "/staff/me/profile", `{"qualifications":[],"visibility":{"nid":{"staff":true,"families":true}}}`, 400)
	call("owner", "PATCH", "/staff/me/profile", `{"qualifications":[],"visibility":{"mpo_index":{"families":true}}}`, 400)
	call("owner", "PATCH", "/staff/me/profile", `{"qualifications":[],"visibility":{"phone":{"staff":true,"families":true},"email":{"staff":true,"families":true},"qualifications":{"staff":true,"families":true}}}`, 200)
	expectFields("student", profileOf("student"), map[string]bool{"phone": true, "email": true, "nid_masked": false, "mpo_index": false})

	// Reveal: administrators and the teacher, audited without the number.
	for _, who := range []string{"head", "colleague", "accountant", "student", "guardian"} {
		call(who, "POST", "/staff/profiles/"+employeeID+"/nid-reveal", "", 403)
	}
	revealed := func(who string) string {
		var body struct {
			Data struct {
				NID string `json:"nid"`
			} `json:"data"`
		}
		_ = json.Unmarshal(call(who, "POST", "/staff/profiles/"+employeeID+"/nid-reveal", "", 200), &body)
		return body.Data.NID
	}
	if revealed("admin") != nid || revealed("owner") != nid {
		t.Fatal("reveal did not return the NID")
	}
	var reveals []struct {
		ActorUserID string
		Metadata    string
	}
	db.Table("audit_events").Select("actor_user_id, metadata::text AS metadata").Where("action='staff.nid_revealed' AND target_id=?", employeeID).Order("created_at").Scan(&reveals)
	if len(reveals) != 2 || reveals[0].ActorUserID != adminUser.ID || !strings.Contains(reveals[0].Metadata, `"own": false`) || reveals[1].ActorUserID != owner.ID || !strings.Contains(reveals[1].Metadata, `"own": true`) {
		t.Fatalf("reveal audit %+v", reveals)
	}
	var leaked int64
	db.Table("audit_events").Where("target_id=? AND metadata::text LIKE ?", employeeID, "%"+nid+"%").Count(&leaked)
	if leaked != 0 {
		t.Fatal("the NID reached the audit log")
	}

	// Photos: the teacher's own (or any, as an administrator), resized.
	var img bytes.Buffer
	canvas := image.NewNRGBA(image.Rect(0, 0, 1200, 800))
	// Camera-like noise, so the upload is large as real photos are.
	noise := make([]byte, 1200*800)
	if _, err := rand.Read(noise); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			n := noise[y*1200+x] / 4
			canvas.Set(x, y, color.NRGBA{uint8(x/5) + n, uint8(y/4) + n, 120 + n, 255})
		}
	}
	if err := png.Encode(&img, canvas); err != nil {
		t.Fatal(err)
	}
	upload := func(who, path string, want int) {
		t.Helper()
		headers := map[string]string{"Content-Type": "image/png"}
		for k, v := range as[who] {
			headers[k] = v
		}
		if r := perform(router, "PUT", path, img.String(), nil, headers); r.Code != want {
			t.Fatalf("%s upload %s: %d %s", who, path, r.Code, r.Body.String())
		}
	}
	upload("colleague", "/staff/employees/"+employeeID+"/photo", 403)
	upload("owner", "/staff/me/photo", 200)
	photo := call("student", "GET", "/staff/profiles/"+employeeID+"/photo", "", 200)
	decoded, format, err := image.Decode(bytes.NewReader(photo))
	if err != nil || format != "jpeg" || decoded.Bounds().Dx() != staff.PhotoSize || decoded.Bounds().Dy() != staff.PhotoSize || len(photo) >= img.Len() {
		t.Fatalf("stored photo %s %v %d bytes (upload %d)", format, err, len(photo), img.Len())
	}
	upload("admin", "/staff/employees/"+employeeID+"/photo", 200)
	call("colleague", "DELETE", "/staff/employees/"+employeeID+"/photo", "", 403)

	// The NID appears in no other endpoint or export.
	for who, paths := range map[string][]string{
		"admin": {"/staff/employees?page=1&page_size=100", "/staff/employees/" + employeeID, "/staff/profiles/" + employeeID},
		"head":  {"/staff/employees?page=1&page_size=100", "/staff/employees/" + employeeID},
		"owner": {"/staff/me", "/staff/me/profile", "/staff/me/assignments"},
	} {
		for _, path := range paths {
			if body := call(who, "GET", path, "", 200); bytes.Contains(body, []byte(nid)) {
				t.Fatalf("%s %s leaks the NID", who, path)
			}
		}
	}
	exporter := admin.NewExporter(db)
	sets, err := exporter.Datasets(ctx, "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range sets {
		var out bytes.Buffer
		if _, err := exporter.Export(ctx, &out, adminUser.ID, set.Key, "2026-01-01", "2026-12-31"); err != nil {
			t.Fatalf("export %s: %v", set.Key, err)
		}
		if bytes.Contains(out.Bytes(), []byte(nid)) || bytes.Contains(out.Bytes(), []byte(mpo)) {
			t.Fatalf("export %s leaks the NID or MPO/index", set.Key)
		}
	}

}
