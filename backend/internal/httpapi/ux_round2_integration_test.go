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
	"net/http"
	"net/http/httptest"
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

// TestPostgresLogoUploadAndStudentPhotoRoutes: who may upload the school
// logo and student photos, what is public, and the photo setting.
func TestPostgresLogoUploadAndStudentPhotoRoutes(t *testing.T) {
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
	password := "Ux2!Secret" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	pattern := "%-ux2-" + suffix + "@example.test"
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-ux2-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head, teacher, studentUser := makeUser("admin", "admin"), makeUser("head_teacher", "head"), makeUser("teacher", "teacher"), makeUser("student", "student")

	var before struct {
		LogoURL string
		Uploads bool
	}
	db.Raw("SELECT logo_url, student_photo_uploads AS uploads FROM school_settings WHERE id = 1").Scan(&before)
	var modules []struct {
		ModuleKey string
		Enabled   bool
	}
	db.Raw("SELECT module_key, enabled FROM school_modules WHERE school_id = 1 AND module_key IN ('school_settings', 'students')").Scan(&modules)
	db.Exec("UPDATE school_modules SET enabled = true WHERE school_id = 1 AND module_key IN ('school_settings', 'students')")
	var sessionID, classID, sectionID, studentID string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	year := fmt.Sprintf("%d", 2300+int(random[0])*2+int(random[1]%2))
	must(db.Raw("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES (?, ?::date, ?::date, false) RETURNING id", year, year+"-01-01", year+"-12-31").Row().Scan(&sessionID))
	must(db.Raw("INSERT INTO academic_classes(name,display_order) VALUES (?, 8) RETURNING id", "UX2 "+suffix).Row().Scan(&classID))
	must(db.Raw("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?, ?, 'A', 'A') RETURNING id", sessionID, classID).Row().Scan(&sectionID))
	must(db.Raw("INSERT INTO students(admission_no,name_en,roll_no,session_id,class_id,section_id,user_id,status) VALUES (?, 'Photo Pupil', 1, ?, ?, ?, ?, 'active') RETURNING id", "UX2-"+suffix, sessionID, classID, sectionID, studentUser.ID).Row().Scan(&studentID))
	if studentID == "" {
		t.Fatal("student fixture not created")
	}
	t.Cleanup(func() {
		db.Exec("UPDATE school_settings SET logo_url = ?, student_photo_uploads = ? WHERE id = 1", before.LogoURL, before.Uploads)
		db.Exec("DELETE FROM school_logo WHERE updated_by IN (SELECT id FROM users WHERE email LIKE ?)", pattern)
		for _, m := range modules {
			db.Exec("UPDATE school_modules SET enabled = ? WHERE school_id = 1 AND module_key = ?", m.Enabled, m.ModuleKey)
		}
		db.Exec("DELETE FROM students WHERE id = ?", studentID)
		db.Exec("DELETE FROM academic_sections WHERE id = ?", sectionID)
		db.Exec("DELETE FROM academic_classes WHERE id = ?", classID)
		db.Exec("DELETE FROM academic_sessions WHERE id = ?", sessionID)
		db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", pattern)
		db.Exec("DELETE FROM users WHERE email LIKE ?", pattern)
	})

	schoolService := school.NewService(school.NewRepository(db))
	schoolService.Logos = school.NewLogoStore(db)
	peopleService := people.NewService(people.NewRepository(db))
	peopleService.Photos = people.NewPhotoSubmissions(db)
	router := NewRouterWithPeople(authService, true, nil, schoolService, nil, nil, nil, nil, nil, academic.NewSectionService(academic.NewSectionRepository(db)), peopleService)
	bearer := func(u auth.User) string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return "Bearer " + v.AccessToken
	}
	adminH, headH, teacherH, studentH := bearer(admin), bearer(head), bearer(teacher), bearer(studentUser)
	call := func(method, path, token, kind string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if kind != "" {
			req.Header.Set("Content-Type", kind)
		}
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	picture := func(w, h int) []byte {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{uint8(x), 100, uint8(y), 255})
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		return b.Bytes()
	}

	// The logo: administrators upload it, everyone can read it.
	logo := picture(900, 300)
	call(http.MethodPut, "/school/logo", "", "image/png", logo, http.StatusUnauthorized)
	call(http.MethodPut, "/school/logo", headH, "image/png", logo, http.StatusForbidden)
	call(http.MethodPut, "/school/logo", adminH, "text/plain", logo, http.StatusUnsupportedMediaType)
	call(http.MethodPut, "/school/logo", adminH, "image/png", []byte("nope"), http.StatusBadRequest)
	var saved struct {
		Data struct {
			LogoURL string `json:"logo_url"`
		} `json:"data"`
	}
	_ = json.Unmarshal(call(http.MethodPut, "/school/logo", adminH, "image/png", logo, http.StatusOK).Body.Bytes(), &saved)
	if !school.IsUploadedLogo(saved.Data.LogoURL) {
		t.Fatalf("logo_url %q", saved.Data.LogoURL)
	}
	got := call(http.MethodGet, strings.TrimPrefix(saved.Data.LogoURL, "/api"), "", "", nil, http.StatusOK) // the web server strips /api
	if got.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(got.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("logo served as %q, %q", got.Header().Get("Content-Type"), got.Header().Get("Cache-Control"))
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(got.Body.Bytes())); err != nil || cfg.Width != school.LogoEdge {
		t.Fatalf("served logo %+v %v", cfg, err)
	}
	var branding struct {
		Data school.Branding `json:"data"`
	}
	_ = json.Unmarshal(call(http.MethodGet, "/school/branding", "", "", nil, http.StatusOK).Body.Bytes(), &branding)
	if branding.Data.LogoURL != saved.Data.LogoURL {
		t.Fatalf("branding logo %q", branding.Data.LogoURL)
	}
	// Saving the settings form with the uploaded logo's address keeps it.
	var settings struct {
		Data school.SchoolSettings `json:"data"`
	}
	_ = json.Unmarshal(call(http.MethodGet, "/school/settings", adminH, "", nil, http.StatusOK).Body.Bytes(), &settings)
	s := settings.Data
	form, _ := json.Marshal(map[string]any{"name_en": s.NameEN, "name_bn": s.NameBN, "logo_url": s.LogoURL, "address": s.Address, "phone": s.Phone, "email": s.Email, "preferred_language": s.PreferredLanguage,
		"primary_color": s.PrimaryColor, "secondary_color": s.SecondaryColor, "dashboard_theme": s.DashboardTheme, "grading_scheme_id": s.GradingSchemeID, "student_photo_uploads": false})
	_ = json.Unmarshal(call(http.MethodPut, "/school/settings", adminH, "application/json", form, http.StatusOK).Body.Bytes(), &settings)
	if settings.Data.LogoURL != saved.Data.LogoURL || settings.Data.StudentPhotoUploads {
		t.Fatalf("settings after save: logo %q uploads %v", settings.Data.LogoURL, settings.Data.StudentPhotoUploads)
	}
	call(http.MethodDelete, "/school/logo", headH, "", nil, http.StatusForbidden)
	call(http.MethodDelete, "/school/logo", adminH, "", nil, http.StatusOK)
	call(http.MethodGet, "/school/logo", "", "", nil, http.StatusNotFound)

	// Student photos: turned off above, so the student cannot upload.
	photo := picture(400, 400)
	res := call(http.MethodPut, "/students/me/photo-submission", studentH, "image/png", photo, http.StatusForbidden)
	if !strings.Contains(res.Body.String(), "photo_uploads_off") {
		t.Fatalf("upload while off: %s", res.Body.String())
	}
	db.Exec("UPDATE school_settings SET student_photo_uploads = true WHERE id = 1")
	call(http.MethodPut, "/students/me/photo-submission", teacherH, "image/png", photo, http.StatusForbidden)
	call(http.MethodPut, "/students/me/photo-submission", studentH, "image/png", photo, http.StatusOK)
	call(http.MethodGet, "/students/me/photo-submission/image", studentH, "", nil, http.StatusOK)
	// The pending photo is not the student's photo yet.
	call(http.MethodGet, "/students/"+studentID+"/photo", studentH, "", nil, http.StatusNotFound)
	call(http.MethodGet, "/students/photo-reviews", studentH, "", nil, http.StatusForbidden)
	// The teacher is not this section's class teacher: nothing to review.
	var reviews struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(call(http.MethodGet, "/students/photo-reviews", teacherH, "", nil, http.StatusOK).Body.Bytes(), &reviews)
	if reviews.Data.Total != 0 {
		t.Fatalf("teacher sees %d reviews", reviews.Data.Total)
	}
	call(http.MethodPost, "/students/"+studentID+"/photo-submission/approve", teacherH, "", nil, http.StatusNotFound)
	_ = json.Unmarshal(call(http.MethodGet, "/students/photo-reviews", adminH, "", nil, http.StatusOK).Body.Bytes(), &reviews)
	if reviews.Data.Total < 1 {
		t.Fatal("admin sees no reviews")
	}
	call(http.MethodGet, "/students/"+studentID+"/photo-submission/image", adminH, "", nil, http.StatusOK)
	call(http.MethodPost, "/students/"+studentID+"/photo-submission/reject", adminH, "application/json", []byte(`{"reason":""}`), http.StatusBadRequest)
	call(http.MethodPost, "/students/"+studentID+"/photo-submission/approve", adminH, "", nil, http.StatusOK)
	call(http.MethodPost, "/students/"+studentID+"/photo-submission/approve", adminH, "", nil, http.StatusConflict)
	call(http.MethodGet, "/students/"+studentID+"/photo", studentH, "", nil, http.StatusOK)
	var mine struct {
		Data people.PhotoSubmission `json:"data"`
	}
	_ = json.Unmarshal(call(http.MethodGet, "/students/me/photo-submission", studentH, "", nil, http.StatusOK).Body.Bytes(), &mine)
	if mine.Data.Status != "approved" || !mine.Data.HasPhoto || !mine.Data.Enabled {
		t.Fatalf("student state %+v", mine.Data)
	}
}
