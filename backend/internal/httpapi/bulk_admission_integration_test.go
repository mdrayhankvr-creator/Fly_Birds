package httpapi

import (
	"archive/zip"
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

// inlineXLSX writes a one-sheet workbook with inline strings.
func inlineXLSX(t *testing.T, rows [][]string) string {
	t.Helper()
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, r+1)
		for c, v := range row {
			var esc bytes.Buffer
			_ = xmlEscape(&esc, v)
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>%s</t></is></c>`, 'A'+c, r+1, esc.String())
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"xl/workbook.xml":          `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="S" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/worksheets/sheet1.xml": sheet.String(),
	} {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func xmlEscape(b *bytes.Buffer, s string) error {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	_, err := b.WriteString(r.Replace(s))
	return err
}

// TestPostgresBulkAdmission: a spreadsheet is previewed row by row with
// clear problems, admitted all-or-nothing, linked to guardians on record or
// shared between siblings, safe to upload again and to retry, and Bangla
// names survive both CSV and .xlsx.
func TestPostgresBulkAdmission(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
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
	up := strings.ToUpper(suffix)
	password := "Bulk!Secret" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x6b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-bulk-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head := makeUser("admin", "admin"), makeUser("head_teacher", "head")
	var sessionID, classID, sectionA, sectionB string
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM bulk_admissions WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
		_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM guardians WHERE name_en LIKE ?", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ? OR login_id ILIKE ?)", "%-bulk-"+suffix+"@example.test", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ? OR login_id ILIKE ?", "%-bulk-"+suffix+"@example.test", "%"+suffix+"%").Error
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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('2195','2195-01-01','2195-12-31') RETURNING id").Scan(&sessionID).Error)
	className := "Bulk Class " + suffix
	must(db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", className).Scan(&classID).Error)
	must(db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,'A',?) RETURNING id", sessionID, classID, "BA"+up).Scan(&sectionA).Error)
	must(db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,'B',?) RETURNING id", sessionID, classID, "BB"+up).Scan(&sectionB).Error)
	// Already on record: a student holding roll 5 in A, and a guardian.
	must(db.Exec("INSERT INTO students (admission_no,name_en,status,session_id,class_id,section_id,roll_no) VALUES (?,?,'active',?,?,?,5)", "OLD-"+up, "Old Pupil", sessionID, classID, sectionA).Error)
	phone := fmt.Sprintf("0171%07d", time.Now().UnixNano()%10000000)
	var existingGuardian string
	must(db.Raw("INSERT INTO guardians (name_en,phone) VALUES (?,?) RETURNING id", "Known Parent "+suffix, phone).Scan(&existingGuardian).Error)
	siblingPhone := fmt.Sprintf("0181%07d", time.Now().UnixNano()%10000000)

	router := NewRouterWithPeople(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), people.NewService(people.NewRepository(db)))
	bearer := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		must(e)
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, headH := bearer(admin), bearer(head)
	upload := func(path, kind, body string, headers map[string]string, want int) []byte {
		t.Helper()
		h := map[string]string{"Content-Type": kind}
		for k, v := range headers {
			h[k] = v
		}
		res := perform(router, http.MethodPost, path, body, nil, h)
		if res.Code != want {
			t.Fatalf("%s got %d want %d: %s", path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	csvOf := func(lines ...string) string {
		return "\xef\xbb\xbfadmission_no,name_en,name_bn,class,section,roll,date_of_birth,guardian_name,guardian_relationship,guardian_phone,login_email\n" + strings.Join(lines, "\n") + "\n"
	}
	adm := func(n int) string { return fmt.Sprintf("B-%s-%d", up, n) }
	good := []string{
		adm(1) + ",Rahim Uddin,রহিম উদ্দিন," + className + ",A,1,21/03/2014,Mother One " + suffix + ",Mother," + siblingPhone + ",",
		adm(2) + ",Karima Uddin,করিমা উদ্দিন," + className + ",a,2,2014-04-02,Mother One " + suffix + ",Mother," + siblingPhone + ",",
		adm(3) + ",Selim Hossain,সেলিম," + className + ",B,1,,Known Parent " + suffix + ",Father," + phone + ",selim-" + suffix + "@example.test",
		"OLD-" + up + ",Old Pupil,," + className + ",A,,,,,,",
	}
	bad := []string{
		adm(4) + ",No Class,,,,,,,,,",
		adm(5) + ",Wrong Section,," + className + ",Z,,,,,,",
		adm(6) + ",Bad Date,," + className + ",A,,31/02/2014,,,,",
		adm(7) + ",Roll Taken,," + className + ",A,5,,,,,",
		adm(1) + ",Duplicate Number,," + className + ",B,,,,,,",
		"OLD-" + up + ",Different Name,," + className + ",A,,,,,,",
	}
	students := func() int64 {
		var n int64
		db.Table("students").Where("session_id=?", sessionID).Count(&n)
		return n
	}
	before := students()
	path := "/students/bulk-admission"
	q := "?session_id=" + sessionID

	// Administrators only.
	upload(path+"/preview"+q, "text/csv", csvOf(good...), headH, http.StatusForbidden)

	// Preview: every row with its problems, nothing written.
	var preview struct {
		Data people.BulkPreview `json:"data"`
	}
	must(json.Unmarshal(upload(path+"/preview"+q, "text/csv", csvOf(append(append([]string{}, good...), bad...)...), adminH, http.StatusOK), &preview))
	p := preview.Data
	if p.Ready != 3 || p.Skipped != 1 || p.Errors != 6 || len(p.Rows) != 10 || len(p.FileSHA256) != 64 {
		t.Fatalf("preview counts %d ready %d skipped %d errors %d rows", p.Ready, p.Skipped, p.Errors, len(p.Rows))
	}
	wantErrors := map[int]string{6: "missing_class", 7: "section_not_found", 8: "bad_date_of_birth", 9: "roll_taken", 10: "duplicate_admission_no_in_file", 11: "admission_no_taken"}
	for _, row := range p.Rows {
		if code, ok := wantErrors[row.Line]; ok && (row.Status != "error" || !strings.Contains(strings.Join(row.Errors, ","), code)) {
			t.Errorf("line %d: %s %v, want %s", row.Line, row.Status, row.Errors, code)
		}
	}
	if r := p.Rows[0]; r.Status != "ready" || r.NameBN != "রহিম উদ্দিন" || r.Guardian == nil || r.Guardian.Action != "new" {
		t.Fatalf("first row %+v", r)
	}
	if r := p.Rows[1]; r.Guardian == nil || r.Guardian.Action != "same_as_row" || r.Guardian.SameAsLine != 2 {
		t.Fatalf("sibling row %+v", r.Guardian)
	}
	if r := p.Rows[2]; r.Guardian == nil || r.Guardian.Action != "existing" || r.Guardian.ExistingName != "Known Parent "+suffix {
		t.Fatalf("existing guardian row %+v", r.Guardian)
	}
	if p.Rows[3].Status != "skip" {
		t.Fatalf("already admitted row %+v", p.Rows[3])
	}
	if students() != before {
		t.Fatal("the preview wrote students")
	}

	// Admitting a file with problems admits nobody.
	if body := upload(path+q+"&idempotency_key=11111111-2222-4333-8444-"+suffix+"01", "text/csv", csvOf(append(append([]string{}, good...), bad...)...), adminH, http.StatusUnprocessableEntity); !strings.Contains(string(body), "rows_have_errors") || !strings.Contains(string(body), "roll_taken") {
		t.Fatalf("admit with errors: %s", body)
	}
	if students() != before {
		t.Fatal("a file with problems admitted students")
	}

	// All or nothing, even when the database fails part-way through.
	must(db.Exec(fmt.Sprintf(`CREATE FUNCTION bulk_fail_%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name_en = 'Selim Hossain' THEN RAISE EXCEPTION 'forced failure'; END IF; RETURN NEW; END $$`, suffix)).Error)
	must(db.Exec(fmt.Sprintf(`CREATE TRIGGER bulk_fail_%s BEFORE INSERT ON students FOR EACH ROW EXECUTE FUNCTION bulk_fail_%s()`, suffix, suffix)).Error)
	upload(path+q+"&idempotency_key=11111111-2222-4333-8444-"+suffix+"02", "text/csv", csvOf(good...), adminH, http.StatusInternalServerError)
	must(db.Exec(fmt.Sprintf(`DROP TRIGGER bulk_fail_%s ON students`, suffix)).Error)
	must(db.Exec(fmt.Sprintf(`DROP FUNCTION bulk_fail_%s()`, suffix)).Error)
	var strayGuardians int64
	db.Table("guardians").Where("name_en = ?", "Mother One "+suffix).Count(&strayGuardians)
	if students() != before || strayGuardians != 0 {
		t.Fatal("a failure part-way left students or guardians behind")
	}

	// The good rows, with logins.
	key := "11111111-2222-4333-8444-" + suffix + "03"
	var result struct {
		Data people.BulkResult `json:"data"`
	}
	must(json.Unmarshal(upload(path+q+"&create_logins=true&idempotency_key="+key, "text/csv", csvOf(good...), adminH, http.StatusCreated), &result))
	if result.Data.Admitted != 3 || result.Data.Skipped != 1 || len(result.Data.Logins) != 3 {
		t.Fatalf("result %+v", result.Data)
	}
	ids := map[string]string{}
	for _, l := range result.Data.Logins {
		if len(l.InitialPassword) != 14 {
			t.Fatal("a slip has no password")
		}
		if d := time.Until(l.PasswordExpiresAt) - auth.TemporaryPasswordLifetime; d < -2*time.Minute || d > 2*time.Minute {
			t.Fatalf("slip expiry %v", l.PasswordExpiresAt)
		}
		ids[l.AdmissionNo] = l.LoginID
		if l.AdmissionNo == adm(3) && l.Email != "selim-"+suffix+"@example.test" {
			t.Fatalf("login email %q", l.Email)
		}
	}
	if ids[adm(1)] != adm(1) {
		t.Fatalf("sign-in IDs %v", ids)
	}
	var nameBN string
	db.Table("students").Where("admission_no=?", adm(1)).Pluck("name_bn", &nameBN)
	if nameBN != "রহিম উদ্দিন" {
		t.Fatalf("Bangla name stored as %q", nameBN)
	}
	// Siblings share one new guardian; the known parent was linked, not copied.
	var mothers, links, knownLinks int64
	db.Table("guardians").Where("name_en = ?", "Mother One "+suffix).Count(&mothers)
	db.Table("student_guardians AS sg").Joins("JOIN guardians g ON g.id = sg.guardian_id").Where("g.name_en = ?", "Mother One "+suffix).Count(&links)
	db.Table("student_guardians").Where("guardian_id = ?", existingGuardian).Count(&knownLinks)
	var knownCopies int64
	db.Table("guardians").Where("phone = ?", phone).Count(&knownCopies)
	if mothers != 1 || links != 2 || knownLinks != 1 || knownCopies != 1 {
		t.Fatalf("guardians: mothers %d links %d known links %d copies %d", mothers, links, knownLinks, knownCopies)
	}
	for _, l := range result.Data.Logins {
		var leaked int64
		db.Table("audit_events").Where("metadata::text LIKE ?", "%"+l.InitialPassword+"%").Count(&leaked)
		if leaked != 0 {
			t.Fatal("a password reached the audit log")
		}
	}
	afterAdmit := students()

	// A retry with the same key answers with the first result.
	var retry struct {
		Data people.BulkResult `json:"data"`
	}
	must(json.Unmarshal(upload(path+q+"&create_logins=true&idempotency_key="+key, "text/csv", csvOf(good...), adminH, http.StatusOK), &retry))
	if !retry.Data.Replayed || retry.Data.Admitted != 3 || len(retry.Data.Logins) != 0 {
		t.Fatalf("retry %+v", retry.Data)
	}
	// The same key with another file is refused.
	if code := errCode(upload(path+q+"&idempotency_key="+key, "text/csv", csvOf(good[0]), adminH, http.StatusConflict)); code != "idempotency_key_reused" {
		t.Fatalf("key reuse: %s", code)
	}
	// Uploading the same file again (a new key) skips everyone already admitted.
	var again struct {
		Data people.BulkResult `json:"data"`
	}
	must(json.Unmarshal(upload(path+q+"&idempotency_key=11111111-2222-4333-8444-"+suffix+"04", "text/csv", csvOf(good...), adminH, http.StatusCreated), &again))
	if again.Data.Admitted != 0 || again.Data.Skipped != 4 || students() != afterAdmit {
		t.Fatalf("re-upload %+v, students %d -> %d", again.Data, afterAdmit, students())
	}

	// .xlsx works the same, Bangla included.
	xlsx := inlineXLSX(t, [][]string{{"Admission No", "Name EN", "Name BN", "Class", "Section", "Roll"}, {adm(8), "Nusrat Jahan", "নুসরাত জাহান", className, "B", "2"}})
	must(json.Unmarshal(upload(path+q+"&idempotency_key=11111111-2222-4333-8444-"+suffix+"05", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsx, adminH, http.StatusCreated), &again))
	db.Table("students").Where("admission_no=?", adm(8)).Pluck("name_bn", &nameBN)
	if again.Data.Admitted != 1 || nameBN != "নুসরাত জাহান" {
		t.Fatalf("xlsx admit %+v, name %q", again.Data, nameBN)
	}

	// Files that cannot be read, and the row limit.
	if code := errCode(upload(path+"/preview"+q, "text/csv", "admission_no,name_bn\nX,\xe0\x41\n", adminH, http.StatusBadRequest)); code != "sheet_not_utf8" {
		t.Fatalf("non-UTF-8: %s", code)
	}
	if code := errCode(upload(path+"/preview"+q, "text/csv", "name_en,class\nA,B\n", adminH, http.StatusBadRequest)); code != "missing_columns" {
		t.Fatalf("missing columns: %s", code)
	}
	var many strings.Builder
	for i := 0; i <= people.MaxBulkRows; i++ {
		fmt.Fprintf(&many, "M-%s-%d,Pupil %d,,%s,A,,,,,,\n", up, i, i, className)
	}
	if code := errCode(upload(path+"/preview"+q, "text/csv", csvOf(many.String()), adminH, http.StatusBadRequest)); code != "too_many_rows" {
		t.Fatalf("row limit: %s", code)
	}

	// The template downloads with its columns and Bangla guidance.
	res := perform(router, http.MethodGet, path+"/template?format=csv", "", nil, adminH)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "admission_no,name_en") || !strings.Contains(res.Body.String(), "ভর্তি") {
		t.Fatalf("CSV template: %d", res.Code)
	}
	if res := perform(router, http.MethodGet, path+"/template", "", nil, adminH); res.Code != http.StatusOK || !bytes.HasPrefix(res.Body.Bytes(), []byte("PK")) {
		t.Fatalf("xlsx template: %d", res.Code)
	}
}
