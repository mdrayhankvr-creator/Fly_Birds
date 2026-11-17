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

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

func TestPostgresStaffAccountLink(t *testing.T) {
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
	const testPassword = "Staff-link-test-only-2026"
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(label, role string) auth.User {
		t.Helper()
		u, err := authService.BootstrapUser(ctx, label+"-link-"+suffix+"@example.test", testPassword, role)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	adminUser, head, teacher, otherTeacher, student, accountant := makeUser("admin", "admin"), makeUser("head", "head_teacher"), makeUser("teacher", "teacher"), makeUser("other", "teacher"), makeUser("student", "student"), makeUser("accountant", "accountant")
	var employeeID, linkedID, headEmployeeID string
	code := "LINK" + strings.ToUpper(suffix)
	insert := func(target *string, employeeCode, userID string) {
		t.Helper()
		var err error
		if userID == "" {
			err = db.Raw("INSERT INTO employees (employee_code,kind,name_en,designation,joined_on) VALUES (?,'teacher','Test Teacher','Teacher','2026-01-01') RETURNING id", employeeCode).Scan(target).Error
		} else {
			err = db.Raw("INSERT INTO employees (employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Other Teacher','Teacher','2026-01-01') RETURNING id", employeeCode, userID).Scan(target).Error
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(&employeeID, code, "")
	insert(&linkedID, "USED"+strings.ToUpper(suffix), otherTeacher.ID)
	insert(&headEmployeeID, "HEAD"+strings.ToUpper(suffix), "")
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM employee_photos WHERE employee_id IN (?,?,?)", employeeID, linkedID, headEmployeeID).Error
		_ = db.Exec("DELETE FROM employees WHERE id IN (?,?,?)", employeeID, linkedID, headEmployeeID).Error
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-link-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-link-"+suffix+"@example.test").Error
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
	router := NewRouterWithBatchServices(authService, true, nil, school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, staff.NewService(staff.NewRepository(db)), nil)
	token := func(u auth.User) map[string]string {
		t.Helper()
		credentials, err := authService.Login(ctx, u.Email, testPassword)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{"Authorization": "Bearer " + credentials.AccessToken}
	}
	adminHeader, teacherHeader, headHeader := token(adminUser), token(teacher), token(head)
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		response := perform(router, method, path, body, nil, headers)
		if response.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.Bytes()
	}
	link := func(employee, userID, email, confirmCode string, headers map[string]string, want int) []byte {
		t.Helper()
		body, _ := json.Marshal(staff.AccountLinkInput{UserID: userID, ConfirmEmail: email, ConfirmEmployeeCode: confirmCode})
		return call(http.MethodPost, "/staff/employees/"+employee+"/account-link", string(body), headers, want)
	}

	// Upload before linking: the photo already belongs to the unlinked profile.
	var photo bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{R: 30, G: 100, B: 180, A: 255})
		}
	}
	if err := png.Encode(&photo, img); err != nil {
		t.Fatal(err)
	}
	uploadHeader := map[string]string{"Authorization": adminHeader["Authorization"], "Content-Type": "image/png"}
	call(http.MethodPut, "/staff/employees/"+employeeID+"/photo", photo.String(), uploadHeader, 200)
	call(http.MethodGet, "/staff/me/avatar", "", teacherHeader, 204)

	link(employeeID, teacher.ID, teacher.Email, code, nil, 401)
	link(employeeID, teacher.ID, teacher.Email, code, teacherHeader, 403)
	link(employeeID, teacher.ID, teacher.Email, code, headHeader, 403)
	link(employeeID, adminUser.ID, adminUser.Email, code, adminHeader, 409) // Protected administrator role.
	link(employeeID, student.ID, student.Email, code, adminHeader, 409)
	link(employeeID, accountant.ID, accountant.Email, code, adminHeader, 409)
	link(employeeID, otherTeacher.ID, otherTeacher.Email, code, adminHeader, 409) // Already linked elsewhere.
	link(employeeID, teacher.ID, teacher.Email, "WRONG", adminHeader, 409)
	link(employeeID, teacher.ID, "wrong@example.test", code, adminHeader, 409)
	link(employeeID, teacher.ID, teacher.Email, code, adminHeader, 200)
	link(employeeID, teacher.ID, teacher.Email, code, adminHeader, 409)
	if photo := call(http.MethodGet, "/staff/me/avatar", "", teacherHeader, 200); len(photo) == 0 {
		t.Fatal("linked teacher avatar has no photo")
	}
	var own struct {
		Data struct {
			ID           string `json:"id"`
			PhotoVersion string `json:"photo_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(call(http.MethodGet, "/staff/me/profile", "", teacherHeader, 200), &own); err != nil {
		t.Fatal(err)
	}
	if own.Data.ID != employeeID || own.Data.PhotoVersion == "" {
		t.Fatal("linked teacher profile or photo version missing")
	}
	if body := call(http.MethodGet, "/staff/profiles/"+employeeID, "", headHeader, 200); bytes.Contains(body, []byte("linked_account_email")) {
		t.Fatal("non-admin profile exposed linked account")
	}
	if body := call(http.MethodGet, "/staff/profiles/"+employeeID, "", adminHeader, 200); !bytes.Contains(body, []byte(teacher.Email)) {
		t.Fatal("administrator cannot see linked account status")
	}
	link(headEmployeeID, head.ID, head.Email, "HEAD"+strings.ToUpper(suffix), adminHeader, 200)
}
