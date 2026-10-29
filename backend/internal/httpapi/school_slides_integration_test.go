package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

func testJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{R: uint8(x), G: 90, B: 160, A: 255})
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, nil); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestPostgresSchoolSlidesHTTP(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL slideshow test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	unique := hex.EncodeToString(suffix[:])
	service, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	schoolService := school.NewService(school.NewRepository(db))
	original, err := schoolService.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const password = "IsolatedSlidesTest123!"
	admin, err := service.BootstrapUser(ctx, "admin-slides-"+unique+"@example.test", password, "admin")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := service.BootstrapUser(ctx, "teacher-slides-"+unique+"@example.test", password, "teacher")
	if err != nil {
		t.Fatal(err)
	}
	var added []string
	t.Cleanup(func() {
		for _, id := range added {
			_ = db.Exec("DELETE FROM school_slides WHERE id = ?", id).Error
		}
		_ = db.Exec("UPDATE school_settings SET tagline_en = ?, tagline_bn = ? WHERE id = 1", original.TaglineEN, original.TaglineBN).Error
		_ = db.Exec("DELETE FROM users WHERE id IN (?, ?)", admin.ID, teacher.ID).Error
	})
	router := NewRouterWithServices(service, true, rbac.NewService(rbac.NewRepository(db)), schoolService)
	login := func(email string) map[string]string {
		body, _ := json.Marshal(map[string]string{"email": email, "password": password})
		response := perform(router, "POST", "/auth/login", string(body), nil, nil)
		if response.Code != 200 {
			t.Fatalf("login status=%d", response.Code)
		}
		return map[string]string{"Authorization": "Bearer " + credentialsFromResponse(t, response)}
	}
	adminH, teacherH := login(admin.Email), login(teacher.Email)
	with := func(h map[string]string, kind string) map[string]string {
		out := map[string]string{"Content-Type": kind}
		for k, v := range h {
			out[k] = v
		}
		return out
	}
	check := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		got := perform(router, method, path, body, nil, headers)
		if got.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, got.Code, want, got.Body.String())
		}
		return got.Body.Bytes()
	}
	list := func() []school.Slide {
		var out struct{ Data []school.Slide }
		if err := json.Unmarshal(check("GET", "/school/slides", "", nil, 200), &out); err != nil {
			t.Fatal(err)
		}
		return out.Data
	}
	upload := func(body, kind string, headers map[string]string, want int) school.Slide {
		t.Helper()
		var out struct{ Data school.Slide }
		raw := check("POST", "/school/slides", body, with(headers, kind), want)
		if want == 201 {
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			added = append(added, out.Data.ID)
		}
		return out.Data
	}
	existing := len(list())
	photo := testJPEG(t, 800, 450)

	// Writes need an administrator; reads are public.
	check("POST", "/school/slides", photo, map[string]string{"Content-Type": "image/jpeg"}, 401)
	upload(photo, "image/jpeg", teacherH, 403)
	upload("plain text", "text/plain", adminH, 415)
	upload(photo, "image/png", adminH, 400)                   // declared type does not match the bytes
	upload(testJPEG(t, 100, 100), "image/jpeg", adminH, 400)  // too small
	upload(testJPEG(t, 2600, 600), "image/jpeg", adminH, 400) // too wide
	upload(strings.Repeat("x", school.MaxSlideUpload+10), "image/jpeg", adminH, 413)
	first := upload(photo, "image/jpeg", adminH, 201)
	if first.Width != 800 || first.Height != 450 || len(first.Version) != 16 || first.Position != existing {
		t.Fatalf("first slide %+v", first)
	}
	// A PNG with transparency is stored as JPEG.
	clear := image.NewNRGBA(image.Rect(0, 0, 640, 400))
	var pngBody bytes.Buffer
	_ = png.Encode(&pngBody, clear)
	second := upload(pngBody.String(), "image/png", adminH, 201)
	img := perform(router, "GET", "/school/slides/"+second.ID+"/image?v="+second.Version, "", nil, nil)
	if img.Code != 200 || img.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(img.Header().Get("Cache-Control"), "immutable") || img.Header().Get("ETag") != `"`+second.Version+`"` {
		t.Fatalf("image response %d %v", img.Code, img.Header())
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(img.Body.Bytes())); err != nil || format != "jpeg" {
		t.Fatalf("stored format %s %v", format, err)
	}
	if stale := perform(router, "GET", "/school/slides/"+second.ID+"/image", "", nil, nil); stale.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("unversioned cache %v", stale.Header())
	}
	check("GET", "/school/slides/"+second.ID+"/image", "", map[string]string{"If-None-Match": `"` + second.Version + `"`}, 304)
	check("GET", "/school/slides/00000000-0000-0000-0000-000000000000/image", "", nil, 404)

	// Captions.
	check("PATCH", "/school/slides/"+first.ID, `{"caption_en":"Prize day","caption_bn":"পুরস্কার দিবস"}`, teacherH, 403)
	check("PATCH", "/school/slides/"+first.ID, "{\"caption_en\":\"bad\\u0007\",\"caption_bn\":\"\"}", adminH, 400)
	check("PATCH", "/school/slides/"+first.ID, `{"caption_en":"`+strings.Repeat("a", 141)+`","caption_bn":""}`, adminH, 400)
	if body := check("PATCH", "/school/slides/"+first.ID, `{"caption_en":"Prize day","caption_bn":"পুরস্কার দিবস"}`, adminH, 200); !strings.Contains(string(body), "পুরস্কার দিবস") {
		t.Fatalf("caption not returned: %s", body)
	}

	// Reorder needs every slide exactly once.
	ids := []string{}
	for _, s := range list() {
		ids = append(ids, s.ID)
	}
	reversed := make([]string, len(ids))
	for i, id := range ids {
		reversed[len(ids)-1-i] = id
	}
	order := func(v []string) string { b, _ := json.Marshal(map[string][]string{"ids": v}); return string(b) }
	check("PUT", "/school/slides/order", order(reversed[1:]), adminH, 400)
	check("PUT", "/school/slides/order", order(append([]string{reversed[0]}, reversed...)), adminH, 400)
	check("PUT", "/school/slides/order", order(reversed), teacherH, 403)
	check("PUT", "/school/slides/order", order(reversed), adminH, 200)
	if after := list(); after[0].ID != reversed[0] || after[len(after)-1].ID != reversed[len(reversed)-1] {
		t.Fatalf("order not applied")
	}

	// At most eight photos.
	for len(list()) < school.MaxSlides {
		upload(photo, "image/jpeg", adminH, 201)
	}
	if body := check("POST", "/school/slides", photo, with(adminH, "image/jpeg"), 409); !strings.Contains(string(body), "slide_limit") {
		t.Fatalf("limit response %s", body)
	}

	// Deleting keeps positions 0..n-1.
	check("DELETE", "/school/slides/"+first.ID, "", teacherH, 403)
	check("DELETE", "/school/slides/"+first.ID, "", adminH, 200)
	check("DELETE", "/school/slides/"+first.ID, "", adminH, 404)
	for i, s := range list() {
		if s.Position != i {
			t.Fatalf("positions not compacted: %+v", list())
		}
	}
	var audits int64
	if err := db.Table("audit_events").Where("actor_user_id = ? AND action LIKE 'school.slide%'", admin.ID).Count(&audits).Error; err != nil || audits < 4 {
		t.Fatalf("slide audit entries %d %v", audits, err)
	}

	// Taglines: saved when sent, kept when a client leaves them out.
	base := map[string]any{"name_en": original.NameEN, "name_bn": original.NameBN, "logo_url": original.LogoURL, "address": original.Address, "phone": original.Phone, "email": original.Email,
		"preferred_language": original.PreferredLanguage, "primary_color": original.PrimaryColor, "secondary_color": original.SecondaryColor, "dashboard_theme": original.DashboardTheme, "grading_scheme_id": original.GradingSchemeID}
	withTagline := map[string]any{"tagline_en": "Learning together since 1972", "tagline_bn": "১৯৭২ সাল থেকে একসঙ্গে শেখা"}
	for k, v := range base {
		withTagline[k] = v
	}
	put := func(v map[string]any, want int) {
		b, _ := json.Marshal(v)
		check("PUT", "/school/settings", string(b), adminH, want)
	}
	put(withTagline, 200)
	put(base, 200)
	if branding := check("GET", "/school/branding", "", nil, 200); !strings.Contains(string(branding), "Learning together since 1972") {
		t.Fatalf("tagline lost: %s", branding)
	}
	withTagline["tagline_en"] = "line\nbreak"
	put(withTagline, 400)
}
