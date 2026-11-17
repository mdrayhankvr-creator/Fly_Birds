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
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
)

func TestPostgresStaffWithLogin(t *testing.T) {
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
	const fixturePassword = "Staff-create-test-only-2026"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(label, role string) auth.User {
		t.Helper()
		u, err := authService.BootstrapUser(ctx, label+"-newstaff-"+suffix+"@example.test", fixturePassword, role)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	adminUser, teacher, head := makeUser("admin", "admin"), makeUser("teacher", "teacher"), makeUser("head", "head_teacher")
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM teacher_assignments WHERE employee_id IN (SELECT id FROM employees WHERE employee_code LIKE ?)", "NS"+strings.ToUpper(suffix)+"%").Error
		_ = db.Exec("DELETE FROM employee_photos WHERE employee_id IN (SELECT id FROM employees WHERE employee_code LIKE ?)", "NS"+strings.ToUpper(suffix)+"%").Error
		_ = db.Exec("DELETE FROM employees WHERE employee_code LIKE ?", "NS"+strings.ToUpper(suffix)+"%").Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-newstaff-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-newstaff-"+suffix+"@example.test").Error
	})
	for _, module := range []string{"staff", "permissions"} {
		var old bool
		if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key=?", school.SingleSchoolID, module).Scan(&old).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("school_modules").Where("school_id=? AND module_key=?", school.SingleSchoolID, module).Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = db.Table("school_modules").Where("school_id=? AND module_key=?", school.SingleSchoolID, module).Update("enabled", old).Error
		})
	}
	router := NewRouterWithBatchServices(authService, true, nil, school.NewService(school.NewRepository(db)), users.NewService(users.NewRepository(db)), nil, nil, nil, nil, staff.NewService(staff.NewRepository(db)), nil)
	token := func(u auth.User) map[string]string {
		t.Helper()
		credentials, err := authService.Login(ctx, u.Email, fixturePassword)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + credentials.AccessToken}
	}
	adminHeader, teacherHeader, headHeader := token(adminUser), token(teacher), token(head)
	baseCode := "NS" + strings.ToUpper(suffix)
	newInput := func(tag, role string) staff.CreateWithLoginInput {
		return staff.CreateWithLoginInput{EmployeeCode: baseCode + tag, ConfirmEmployeeCode: baseCode + tag, Role: role, Email: "new-" + tag + "-newstaff-" + suffix + "@example.test", NameEN: "New Staff " + tag, Designation: "Teacher", JoinedOn: "2026-01-01", Qualifications: []string{"Mathematics"}}
	}
	var imageBody bytes.Buffer
	canvas := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			canvas.Set(x, y, color.NRGBA{R: 35, G: 100, B: 180, A: 255})
		}
	}
	if err := png.Encode(&imageBody, canvas); err != nil {
		t.Fatal(err)
	}
	create := func(input staff.CreateWithLoginInput, photo []byte, who map[string]string, want int) (staff.CreatedWithLogin, http.Header) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		payload, _ := json.Marshal(input)
		if err := writer.WriteField("profile", string(payload)); err != nil {
			t.Fatal(err)
		}
		if photo != nil {
			header := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="photo"; filename="staff.png"`}, "Content-Type": {"image/png"}}
			part, err := writer.CreatePart(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(photo); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		headers := map[string]string{"Content-Type": writer.FormDataContentType()}
		for key, value := range who {
			headers[key] = value
		}
		response := perform(router, "POST", "/staff/employees/with-login", body.String(), nil, headers)
		if response.Code != want {
			t.Fatalf("create staff: got %d, want %d", response.Code, want)
		}
		var wrapped struct {
			Data staff.CreatedWithLogin `json:"data"`
		}
		if want == 201 && json.Unmarshal(response.Body.Bytes(), &wrapped) != nil {
			t.Fatal("invalid creation response")
		}
		return wrapped.Data, response.Header()
	}
	count := func(table, column, value string) int64 {
		t.Helper()
		var n int64
		if err := db.Table(table).Where(column+"=?", value).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	create(newInput("DENIED", "teacher"), nil, nil, 401)
	create(newInput("DENIED", "teacher"), nil, teacherHeader, 403)
	create(newInput("DENIED", "teacher"), nil, headHeader, 403)
	badRole := newInput("BADROLE", "admin")
	create(badRole, nil, adminHeader, 400)
	for _, protected := range []string{"mdrayhankvr@gmail.com", "2023331031@student.sust.edu"} {
		before := count("users", "email", protected)
		input := newInput("PROTECTED", "teacher")
		input.Email = protected
		create(input, nil, adminHeader, 409)
		if count("users", "email", protected) != before {
			t.Fatal("protected account was created or changed")
		}
	}
	invalidPhoto := newInput("PHOTOFAIL", "teacher")
	create(invalidPhoto, []byte("not a photo"), adminHeader, 400)
	if count("users", "email", invalidPhoto.Email) != 0 {
		t.Fatal("invalid photo left an account")
	}
	invalidAssignment := newInput("ROLLBACK", "teacher")
	invalidAssignment.Assignment = &staff.AssignmentInput{SessionID: "00000000-0000-4000-8000-000000000001", ClassID: "00000000-0000-4000-8000-000000000002", SubjectID: "00000000-0000-4000-8000-000000000003"}
	create(invalidAssignment, nil, adminHeader, 409)
	if count("users", "email", invalidAssignment.Email) != 0 || count("employees", "employee_code", invalidAssignment.EmployeeCode) != 0 {
		t.Fatal("failed assignment left an orphan record")
	}

	teacherInput := newInput("TEACHER", "teacher")
	created, responseHeader := create(teacherInput, imageBody.Bytes(), adminHeader, 201)
	if responseHeader.Get("Cache-Control") != "no-store" || created.Employee.UserID == nil || created.Employee.PhotoVersion == "" || created.InitialPassword == "" {
		t.Fatal("creation response lacks no-store, linked employee, photo or one-time credential")
	}
	if created.Employee.Kind != "teacher" || created.Role != "teacher" || created.Employee.EmployeeCode != teacherInput.EmployeeCode {
		t.Fatal("teacher role or profile mismatch")
	}
	var stored struct {
		PasswordHash               string
		MustChangePassword         bool
		TemporaryPasswordExpiresAt *time.Time
	}
	if err := db.Table("users").Select("password_hash,must_change_password,temporary_password_expires_at").Where("id=?", *created.Employee.UserID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	expiryMatches := stored.TemporaryPasswordExpiresAt != nil && stored.TemporaryPasswordExpiresAt.After(created.ExpiresAt.Add(-time.Second)) && stored.TemporaryPasswordExpiresAt.Before(created.ExpiresAt.Add(time.Second))
	if stored.PasswordHash == created.InitialPassword || !auth.VerifyPassword(stored.PasswordHash, created.InitialPassword) || !stored.MustChangePassword || !expiryMatches {
		t.Fatal("temporary credential storage or forced-change state is wrong")
	}
	var auditRows []struct{ Metadata string }
	if err := db.Table("audit_events").Select("metadata::text AS metadata").Where("target_id IN ?", []string{created.Employee.ID, *created.Employee.UserID}).Scan(&auditRows).Error; err != nil {
		t.Fatal(err)
	}
	if len(auditRows) == 0 {
		t.Fatal("creation was not audited")
	}
	for _, row := range auditRows {
		if strings.Contains(row.Metadata, created.InitialPassword) {
			t.Fatal("audit contains plaintext credential")
		}
	}
	create(newInput("EMAILDUP", "teacherWithBadRole"), nil, adminHeader, 400)
	emailDuplicate := newInput("EMAILDUP", "teacher")
	emailDuplicate.Email = teacherInput.Email
	create(emailDuplicate, nil, adminHeader, 409)
	if count("employees", "employee_code", emailDuplicate.EmployeeCode) != 0 {
		t.Fatal("duplicate email left an employee")
	}
	codeDuplicate := newInput("TEACHER", "teacher")
	create(codeDuplicate, nil, adminHeader, 409)
	if count("users", "email", codeDuplicate.Email) != 0 {
		t.Fatal("duplicate employee code left an account")
	}
	credentials, err := authService.Login(ctx, teacherInput.Email, created.InitialPassword)
	if err != nil || !credentials.User.MustChangePassword {
		t.Fatal("new teacher cannot use temporary password for first sign-in")
	}
	staffURL := "/staff/me/profile"
	userHeader := map[string]string{"Authorization": "Bearer " + credentials.AccessToken}
	if response := perform(router, "GET", staffURL, "", nil, userHeader); response.Code != 403 || !strings.Contains(response.Body.String(), "password_change_required") {
		t.Fatal("forced password change did not block staff profile")
	}
	change := perform(router, "POST", "/auth/password", `{"current_password":"`+created.InitialPassword+`","new_password":"A-new-staff-password-2026"}`, nil, userHeader)
	if change.Code != 200 {
		t.Fatal("first password change failed")
	}
	if response := perform(router, "GET", staffURL, "", nil, userHeader); response.Code != 200 {
		t.Fatal("staff profile did not open after password change")
	}
	if response := perform(router, "GET", "/staff/me/avatar", "", nil, userHeader); response.Code != 200 || response.Body.Len() == 0 {
		t.Fatal("stored photo missing from linked teacher avatar")
	}
	if response := perform(router, "GET", "/users/"+*created.Employee.UserID, "", nil, adminHeader); response.Code != 200 || strings.Contains(response.Body.String(), created.InitialPassword) {
		t.Fatal("user read exposed credential")
	}
	if response := perform(router, "GET", "/staff/profiles/"+created.Employee.ID, "", nil, adminHeader); response.Code != 200 || strings.Contains(response.Body.String(), created.InitialPassword) {
		t.Fatal("staff profile exposed credential")
	}

	headInput := newInput("HEAD", "head_teacher")
	headCreated, _ := create(headInput, nil, adminHeader, 201)
	if headCreated.Role != "head_teacher" || headCreated.Employee.Kind != "teacher" || headCreated.InitialPassword == created.InitialPassword || headCreated.Employee.PhotoVersion != "" {
		t.Fatal("head-teacher credential, role or photo fallback mismatch")
	}
	headLogin, err := authService.Login(ctx, headInput.Email, headCreated.InitialPassword)
	if err != nil || !headLogin.User.MustChangePassword {
		t.Fatal("head teacher did not receive forced password change")
	}
}
