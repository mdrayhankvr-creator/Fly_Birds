package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/academic"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func TestPostgresStudentsGuardiansLifecycle(t *testing.T) {
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
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("non-idempotent migration: %v", err)
	}
	var versions int64
	if err := db.Table("schema_migrations").Where("version IN ?", []int{1, 2, 3, 4, 101, 102, 103, 201}).Count(&versions).Error; err != nil || versions != 8 {
		t.Fatalf("migration ledger %d: %v", versions, err)
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	password := "M3Secret!" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x61}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-m3-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head, studentUser, otherStudent, guardianUser, otherGuardian := makeUser("admin", "admin"), makeUser("head_teacher", "head"), makeUser("student", "student"), makeUser("student", "other-student"), makeUser("guardian", "guardian"), makeUser("guardian", "other-guardian")
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (?,?,?,?,?,?)", admin.ID, head.ID, studentUser.ID, otherStudent.ID, guardianUser.ID, otherGuardian.ID).Error
		_ = db.Exec("DELETE FROM users WHERE id IN (?,?,?,?,?,?)", admin.ID, head.ID, studentUser.ID, otherStudent.ID, guardianUser.ID, otherGuardian.ID).Error
	})
	var old bool
	if err := db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Scan(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", old).Error
	})
	var sessionID, classID, otherClass, sectionID string
	if err := db.Raw("INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('2191','2191-01-01','2191-12-31') RETURNING id").Scan(&sessionID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
		_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM guardians WHERE name_en LIKE ?", "M3 Guardian "+suffix+"%").Error
		_ = db.Exec("DELETE FROM academic_sections WHERE session_id=?", sessionID).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id IN (?,?)", classID, otherClass).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
	})
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,1) RETURNING id", "M3 Class "+suffix).Scan(&classID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_classes (name,display_order) VALUES (?,2) RETURNING id", "M3 Other "+suffix).Scan(&otherClass).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "M3 A "+suffix, "A"+strings.ToUpper(suffix)).Scan(&sectionID).Error; err != nil {
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
	adminH, headH, studentH, otherStudentH, guardianH, otherGuardianH := token(admin), token(head), token(studentUser), token(otherStudent), token(guardianUser), token(otherGuardian)
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		res := perform(router, method, path, body, nil, headers)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	input := func(admission string, roll int, user string, class string) string {
		return fmt.Sprintf(`{"admission_no":%q,"name_en":"Student %s","name_bn":"শিক্ষার্থী","date_of_birth":"2014-02-03","status":"active","session_id":%q,"class_id":%q,"section_id":%q,"roll_no":%d,"user_id":%q}`, admission, admission, sessionID, class, sectionID, roll, user)
	}
	check("GET", "/students", "", nil, 401)
	check("GET", "/students", "", studentH, 403)
	check("GET", "/students", "", headH, 200)
	check("POST", "/students", input("M3"+suffix, 1, studentUser.ID, classID), headH, 403)
	check("POST", "/students", input("M3"+suffix, 1, studentUser.ID, otherClass), adminH, 400)
	check("POST", "/students", input("M3"+suffix, 1, guardianUser.ID, classID), adminH, 400)
	created := check("POST", "/students", input("M3"+suffix, 1, studentUser.ID, classID), adminH, 201)
	var studentResult struct {
		Data people.Student `json:"data"`
	}
	if err := json.Unmarshal(created, &studentResult); err != nil || studentResult.Data.ID == "" || studentResult.Data.ClassName == "" {
		t.Fatalf("student response %+v %v", studentResult, err)
	}
	studentID := studentResult.Data.ID
	updatedStudent := strings.Replace(input("M3"+suffix, 1, studentUser.ID, classID), "\"name_en\":\"Student ", "\"name_en\":\"Updated ", 1)
	updatedBody := check("PATCH", "/students/"+studentID, updatedStudent, adminH, 200)
	if !bytes.Contains(updatedBody, []byte("Updated M3"+suffix)) {
		t.Fatalf("student update not returned: %s", updatedBody)
	}
	if err := db.Exec("INSERT INTO students (admission_no,name_en,session_id,class_id,section_id) VALUES (?,?,?,?,?)", "M3bad-context"+suffix, "Bad Context", sessionID, otherClass, sectionID).Error; err == nil {
		t.Fatal("database allowed a section from a different class")
	}
	check("POST", "/students", input("M3"+suffix, 2, "", classID), adminH, 409)
	check("POST", "/students", input("M3other"+suffix, 1, "", classID), adminH, 409)
	check("GET", "/students/me", "", studentH, 200)
	check("GET", "/students/"+studentID, "", otherStudentH, 404)
	check("GET", "/students?search="+suffix+"&status=active&session_id="+sessionID+"&page=1&page_size=1", "", headH, 200)
	guardianBody := fmt.Sprintf(`{"name_en":%q,"name_bn":"অভিভাবক","phone":"01700000000","email":"","address":"Dhaka","user_id":%q}`, "M3 Guardian "+suffix, guardianUser.ID)
	guardianCreated := check("POST", "/guardians", guardianBody, adminH, 201)
	var guardianResult struct {
		Data people.Guardian `json:"data"`
	}
	if err := json.Unmarshal(guardianCreated, &guardianResult); err != nil || guardianResult.Data.ID == "" {
		t.Fatalf("guardian %+v %v", guardianResult, err)
	}
	guardianID := guardianResult.Data.ID
	updatedGuardian := fmt.Sprintf(`{"name_en":%q,"name_bn":"অভিভাবক","phone":"01800000000","email":"","address":"Dhaka","user_id":%q}`, "M3 Guardian "+suffix, guardianUser.ID)
	if !bytes.Contains(check("PATCH", "/guardians/"+guardianID, updatedGuardian, adminH, 200), []byte("01800000000")) {
		t.Fatal("guardian update not returned")
	}
	check("POST", "/guardians", `{"name_en":"Invalid Email","email":"invalid-address"}`, adminH, 400)
	check("GET", "/guardians/me/children", "", guardianH, 200)
	check("GET", "/guardians/me/children/"+studentID, "", guardianH, 404)
	check("GET", "/guardians/me/children/"+studentID, "", otherGuardianH, 404)
	check("PUT", "/students/"+studentID+"/guardians", fmt.Sprintf(`{"guardian_id":%q,"relationship":"mother","is_primary":true}`, guardianID), adminH, 200)
	check("GET", "/guardians/me/children/"+studentID, "", guardianH, 200)
	// The student's own account lists its guardians, with phone; other roles
	// cannot use the self route.
	if own := check("GET", "/students/me/guardians", "", studentH, 200); !bytes.Contains(own, []byte(guardianID)) || !bytes.Contains(own, []byte("01800000000")) {
		t.Fatalf("own guardians missing link: %s", own)
	}
	// A student account with no student record of its own gets nothing.
	if other := check("GET", "/students/me/guardians", "", otherStudentH, 404); bytes.Contains(other, []byte(guardianID)) {
		t.Fatalf("another student saw this student's guardian: %s", other)
	}
	check("GET", "/students/me/guardians", "", guardianH, 403)
	check("GET", "/students/me/guardians", "", headH, 403)
	check("GET", "/students/"+studentID, "", guardianH, 200)
	check("GET", "/students/"+studentID+"/card", "", guardianH, 200)

	// M15: enrolment history follows the student's record scope; who made a
	// change is shown to staff only. A section change is logged.
	var sectionB string
	if err := db.Raw("INSERT INTO academic_sections (session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", sessionID, classID, "M3 B "+suffix, "B"+strings.ToUpper(suffix)).Scan(&sectionB).Error; err != nil {
		t.Fatal(err)
	}
	moved := strings.Replace(updatedStudent, `"section_id":"`+sectionID+`"`, `"section_id":"`+sectionB+`"`, 1)
	check("PATCH", "/students/"+studentID, moved, adminH, 200)
	type historyBody struct {
		Data []people.Enrolment `json:"data"`
	}
	history := func(h map[string]string) historyBody {
		t.Helper()
		var v historyBody
		if err := json.Unmarshal(check("GET", "/students/"+studentID+"/enrolments", "", h, 200), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	staffView := history(headH)
	if len(staffView.Data) != 1 || staffView.Data[0].State != "current" || staffView.Data[0].SectionID != sectionB || len(staffView.Data[0].Events) != 2 ||
		staffView.Data[0].Events[1].Kind != "section_changed" || staffView.Data[0].Events[1].ActorName == nil {
		t.Fatalf("staff history %+v", staffView)
	}
	for _, h := range []map[string]string{studentH, guardianH} {
		if v := history(h); len(v.Data) != 1 || len(v.Data[0].Events) != 2 || v.Data[0].Events[1].ActorName != nil {
			t.Fatalf("family history %+v", v)
		}
	}
	check("GET", "/students/"+studentID+"/enrolments", "", otherStudentH, 404)
	check("GET", "/students/"+studentID+"/enrolments", "", otherGuardianH, 404)
	check("GET", "/students/not-a-uuid/enrolments", "", adminH, 400)
	check("PATCH", "/students/"+studentID, updatedStudent, adminH, 200)
	cardBody := check("GET", "/students/"+studentID+"/card", "", studentH, 200)
	var cardResult struct {
		Data struct {
			QRPayload string `json:"qr_payload"`
			QRDataURL string `json:"qr_data_url"`
		} `json:"data"`
	}
	// The QR holds a verification link with an opaque token, never the student ID.
	if err := json.Unmarshal(cardBody, &cardResult); err != nil || !regexp.MustCompile(`^https?://[^/]+/verify/[A-Za-z0-9_-]{22}$`).MatchString(cardResult.Data.QRPayload) || strings.Contains(cardResult.Data.QRPayload, studentID) || !strings.HasPrefix(cardResult.Data.QRDataURL, "data:image/png;base64,") {
		t.Fatalf("invalid card QR %q: %v", cardResult.Data.QRPayload, err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	photoHeaders := map[string]string{"Authorization": adminH["Authorization"], "Content-Type": "image/png"}
	check("PUT", "/students/"+studentID+"/photo", string(encoded.Bytes()), photoHeaders, 200)
	check("GET", "/students/"+studentID+"/photo", "", guardianH, 200)
	check("GET", "/students/"+studentID+"/photo", "", otherGuardianH, 404)
	check("PUT", "/students/"+studentID+"/photo", "not an image", photoHeaders, 400)
	secondGuardianBody := fmt.Sprintf(`{"name_en":%q,"name_bn":"","phone":"","email":"","address":"","user_id":%q}`, "M3 Guardian "+suffix+" B", otherGuardian.ID)
	secondCreated := check("POST", "/guardians", secondGuardianBody, adminH, 201)
	if err := json.Unmarshal(secondCreated, &guardianResult); err != nil || guardianResult.Data.ID == "" {
		t.Fatalf("second guardian: %v", err)
	}
	secondID := guardianResult.Data.ID
	check("PUT", "/students/"+studentID+"/guardians", fmt.Sprintf(`{"guardian_id":%q,"relationship":"father","is_primary":false}`, secondID), adminH, 200)
	check("GET", "/guardians/me/children/"+studentID, "", otherGuardianH, 200)
	check("PUT", "/students/"+studentID+"/guardians", fmt.Sprintf(`{"guardian_id":%q,"relationship":"father","is_primary":true}`, secondID), adminH, 409)
	check("GET", "/guardians/"+secondID+"/students", "", headH, 200)
	check("DELETE", "/students/"+studentID+"/guardians/"+secondID, "", adminH, 200)
	check("GET", "/guardians/me/children/"+studentID, "", otherGuardianH, 404)
	if err := db.Exec("UPDATE users SET role='teacher' WHERE id=?", guardianUser.ID).Error; err == nil {
		t.Fatal("database allowed linked guardian role change")
	}
	csv := fmt.Sprintf("admission_no,name_en,name_bn,date_of_birth,roll_no,status\nM3csv%s,CSV Student,,2013-01-01,2,active\n", suffix)
	csvHeaders := map[string]string{"Authorization": adminH["Authorization"], "Content-Type": "text/csv"}
	check("POST", "/students/import?session_id="+sessionID+"&class_id="+classID+"&section_id="+sectionID, csv, csvHeaders, 201)
	listBody := check("GET", "/students?search="+suffix+"&session_id="+sessionID+"&page=2&page_size=1", "", headH, 200)
	var listed struct {
		Data people.Page[people.Student] `json:"data"`
	}
	if err := json.Unmarshal(listBody, &listed); err != nil || listed.Data.Total != 2 || len(listed.Data.Items) != 1 || listed.Data.Page != 2 {
		t.Fatalf("student pagination/filter: %+v %v", listed, err)
	}
	check("POST", "/students/import?session_id="+sessionID+"&class_id="+classID+"&section_id="+sectionID, csv, adminH, 400)
	duplicateCSV := fmt.Sprintf("admission_no,name_en,name_bn,date_of_birth,roll_no,status\nM3new%s,New Student,,,3,active\nM3csv%s,Duplicate,,,4,active\n", suffix, suffix)
	check("POST", "/students/import?session_id="+sessionID+"&class_id="+classID+"&section_id="+sectionID, duplicateCSV, csvHeaders, 409)
	var n int64
	if err := db.Table("students").Where("admission_no=?", "M3new"+suffix).Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("CSV was not atomic: %d %v", n, err)
	}
	check("DELETE", "/guardians/"+secondID, "", headH, 403)
	check("DELETE", "/guardians/"+secondID, "", adminH, 200)
	check("GET", "/guardians/"+secondID, "", headH, 404)
	check("DELETE", "/students/"+studentID, "", studentH, 403)
	check("DELETE", "/students/"+studentID, "", adminH, 200)
	check("GET", "/students/me", "", studentH, 404)
	check("GET", "/students/me/guardians", "", studentH, 404)
	check("GET", "/guardians/me/children/"+studentID, "", guardianH, 404)
	check("DELETE", "/guardians/"+guardianID, "", adminH, 200)
	check("GET", "/guardians/me", "", guardianH, 404)
	if err := db.Table("school_modules").Where("school_id=? AND module_key='students'", school.SingleSchoolID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	check("GET", "/students/me", "", studentH, 403)
	check("GET", "/students/me/guardians", "", studentH, 403)
	check("GET", "/guardians/me/children", "", guardianH, 403)
}
