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
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notices"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresPublicSchoolPage: the page below the sign-in shows only what
// the administrator wrote for the public, hides empty sections, shows a
// notice only when its author chose to, and never carries student data.
func TestPostgresPublicSchoolPage(t *testing.T) {
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
	password := "Public!Secret" + suffix
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x70}, 32))
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(role, label string) auth.User {
		t.Helper()
		u, e := authService.BootstrapUser(ctx, label+"-pub-"+suffix+"@example.test", password, role)
		if e != nil {
			t.Fatal(e)
		}
		return u
	}
	admin, head := makeUser("admin", "admin"), makeUser("head_teacher", "head")
	// Start from an empty page and put back whatever was there.
	var saved map[string]any
	db.Table("school_public_page").Where("id = 1").Take(&saved)
	var savedFeatures []school.PublicFeature
	db.Table("school_public_features").Order("position").Scan(&savedFeatures)
	db.Exec("UPDATE school_public_page SET about_en='',about_bn='',show_counts=false,head_message_en='',head_message_bn='',head_photo=NULL,head_photo_version='' WHERE id=1")
	// The school details are school settings (migration 1759).
	var savedDetails map[string]any
	db.Table("school_settings").Select("eiin, founded_year, head_teacher_name_en, head_teacher_name_bn, address, address_bn, phone, email, map_url").Where("id = 1").Take(&savedDetails)
	db.Exec("UPDATE school_settings SET eiin='',founded_year=NULL,head_teacher_name_en='',head_teacher_name_bn='',address='',address_bn='',phone='',email='',map_url='' WHERE id=1")
	var galleryBefore []string
	db.Table("school_gallery").Pluck("id::text", &galleryBefore)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM school_gallery WHERE created_by IN (SELECT id FROM users WHERE email LIKE ?)", "%-pub-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM notices WHERE title_en LIKE ?", "%"+suffix+"%").Error
		_ = db.Exec("DELETE FROM students WHERE admission_no LIKE ?", "PUB-"+suffix+"%").Error
		if saved != nil {
			delete(saved, "id")
			_ = db.Table("school_public_page").Where("id = 1").Updates(saved).Error
		}
		_ = db.Exec("DELETE FROM school_public_features").Error
		for _, f := range savedFeatures {
			_ = db.Exec("INSERT INTO school_public_features(position,icon,title_en,title_bn,description_en,description_bn) VALUES (?,?,?,?,?,?)", f.Position, f.Icon, f.TitleEN, f.TitleBN, f.DescriptionEN, f.DescriptionBN).Error
		}
		if savedDetails != nil {
			_ = db.Table("school_settings").Where("id = 1").Updates(savedDetails).Error
		}
		_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN (SELECT id FROM users WHERE email LIKE ?)", "%-pub-"+suffix+"@example.test").Error
		_ = db.Exec("UPDATE school_public_page SET updated_by = NULL WHERE updated_by IN (SELECT id FROM users WHERE email LIKE ?)", "%-pub-"+suffix+"@example.test").Error
		_ = db.Exec("DELETE FROM users WHERE email LIKE ?", "%-pub-"+suffix+"@example.test").Error
	})
	var oldModule bool
	db.Table("school_modules").Select("enabled").Where("school_id=? AND module_key='school_settings'", school.SingleSchoolID).Scan(&oldModule)
	db.Table("school_modules").Where("school_id=? AND module_key='school_settings'", school.SingleSchoolID).Update("enabled", true)
	t.Cleanup(func() {
		_ = db.Table("school_modules").Where("school_id=? AND module_key='school_settings'", school.SingleSchoolID).Update("enabled", oldModule).Error
	})
	router := NewRouterWithServices(authService, true, nil, school.NewService(school.NewRepository(db)))
	bearer := func(u auth.User) map[string]string {
		v, e := authService.Login(ctx, u.Email, password)
		if e != nil {
			t.Fatal(e)
		}
		return map[string]string{"Authorization": "Bearer " + v.AccessToken}
	}
	adminH, headH := bearer(admin), bearer(head)
	call := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		res := perform(router, method, path, body, nil, headers)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	type view struct {
		Data school.PublicView `json:"data"`
	}
	publicView := func() (view, string) {
		t.Helper()
		body := call(http.MethodGet, "/school/public-page", "", nil, http.StatusOK)
		var v view
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		return v, string(body)
	}

	// Empty sections are left out.
	if v, _ := publicView(); v.Data.About != nil || v.Data.Head != nil || v.Data.Contact != nil {
		t.Fatalf("empty page has sections: %+v", v.Data)
	}
	if v, _ := publicView(); v.Data.DesignStyle != "modern_bold" {
		t.Fatalf("existing school default design = %q", v.Data.DesignStyle)
	}
	if v, _ := publicView(); len(v.Data.Features) != 4 || v.Data.Features[0].TitleBN == "" {
		t.Fatalf("modern page defaults need four bilingual features: %+v", v.Data.Features)
	}

	// Only administrators edit it.
	page := `{"about_en":"A village school since 1972.\nCo-educational.","about_bn":"১৯৭২ সাল থেকে গ্রামের বিদ্যালয়।","founded_year":1972,"eiin":"123456","show_counts":true,"head_name_en":"Ayesha Rahman","head_name_bn":"আয়েশা রহমান","head_message_en":"Welcome, families.","head_message_bn":"স্বাগতম।","contact_address_en":"Main Road, Sylhet","contact_address_bn":"প্রধান সড়ক, সিলেট","contact_phone":"+880 1711-000000","contact_email":"Office@School.example","map_url":"https://maps.example.com/?q=school"}`
	call(http.MethodPut, "/school/public-page", page, nil, http.StatusUnauthorized)
	call(http.MethodPut, "/school/public-page", page, headH, http.StatusForbidden)
	call(http.MethodPut, "/school/public-page", strings.Replace(page, `"123456"`, `"12345"`, 1), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/public-page", strings.Replace(page, "https://maps", "http://maps", 1), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/public-page", strings.Replace(page, "https://maps.example.com/?q=school", "javascript:alert(1)", 1), adminH, http.StatusBadRequest)
	stylePage := strings.Replace(page, `{"about_en"`, `{"design_style":"minimal","pass_rate":92,"about_en"`, 1)
	call(http.MethodPut, "/school/public-page", strings.Replace(stylePage, `"minimal"`, `"unknown"`, 1), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/public-page", strings.Replace(stylePage, `"pass_rate":92`, `"pass_rate":101`, 1), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/public-page", stylePage, adminH, http.StatusOK)
	if v, _ := publicView(); v.Data.DesignStyle != "minimal" || v.Data.About == nil || v.Data.About.PassRate == nil || *v.Data.About.PassRate != 92 {
		t.Fatalf("saved public design not read back: %+v", v.Data)
	}
	call(http.MethodPut, "/school/public-page", page, adminH, http.StatusOK)
	var refined map[string]any
	if err := json.Unmarshal([]byte(page), &refined); err != nil {
		t.Fatal(err)
	}
	refined["activities_en"] = "Reading\nSports"
	refined["activities_bn"] = "পাঠ\nখেলা"
	refined["footer_description_en"] = "A school for every learner."
	refined["social_facebook"] = "https://facebook.com/example.school"
	refined["privacy_policy_en"] = "We use only what is needed for learning."
	refined["features"] = []school.PublicFeature{
		{Position: 1, Icon: "book", TitleEN: "Learning", TitleBN: "শিক্ষা", DescriptionEN: "Learn together.", DescriptionBN: "একসঙ্গে শেখা।"},
		{Position: 2, Icon: "teacher", TitleEN: "Teachers", TitleBN: "শিক্ষক", DescriptionEN: "Careful guidance.", DescriptionBN: "যত্নশীল সহায়তা।"},
		{Position: 3, Icon: "shield", TitleEN: "Care", TitleBN: "যত্ন", DescriptionEN: "A caring place.", DescriptionBN: "যত্নের পরিবেশ।"},
		{Position: 4, Icon: "users", TitleEN: "Community", TitleBN: "সমাজ", DescriptionEN: "Work together.", DescriptionBN: "একসঙ্গে কাজ।"},
	}
	refinedJSON, _ := json.Marshal(refined)
	call(http.MethodPut, "/school/public-page", string(refinedJSON), adminH, http.StatusOK)
	if v, _ := publicView(); v.Data.ActivitiesBN != "পাঠ\nখেলা" || v.Data.FooterDescriptionEN != "A school for every learner." || v.Data.Features[0].Icon != "book" || v.Data.PrivacyPolicyEN == "" {
		t.Fatalf("modern editable content not published: %+v", v.Data)
	}
	refined["social_whatsapp"] = "https://wa.me/8801711000000"
	badSocial, _ := json.Marshal(refined)
	call(http.MethodPut, "/school/public-page", string(badSocial), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/public-page", page, adminH, http.StatusOK)

	v, publicJSON := publicView()
	if strings.Contains(publicJSON, "contact_phone") || strings.Contains(publicJSON, "+880 1711-000000") || v.Data.DesignStyle != "modern_bold" {
		t.Fatalf("public page leaks phone or did not restore default style: %s", publicJSON)
	}
	a, h, c := v.Data.About, v.Data.Head, v.Data.Contact
	if a == nil || h == nil || c == nil || a.EIIN != "123456" || a.FoundedYear == nil || *a.FoundedYear != 1972 || a.Students == nil || a.Teachers == nil || h.NameBN != "আয়েশা রহমান" || c.Email != "office@school.example" || c.MapURL == "" {
		t.Fatalf("public page %+v %+v %+v", a, h, c)
	}

	// The details are school settings: GET /school/settings returns them.
	type settingsBody struct {
		Data school.SchoolSettings `json:"data"`
	}
	var st settingsBody
	if err := json.Unmarshal(call(http.MethodGet, "/school/settings", "", adminH, http.StatusOK), &st); err != nil {
		t.Fatal(err)
	}
	if s := st.Data; s.EIIN != "123456" || s.FoundedYear == nil || *s.FoundedYear != 1972 || s.HeadTeacherNameEN != "Ayesha Rahman" || s.HeadTeacherNameBN != "আয়েশা রহমান" ||
		s.Address != "Main Road, Sylhet" || s.AddressBN != "প্রধান সড়ক, সিলেট" || s.Phone != "+880 1711-000000" || s.Email != "office@school.example" || s.MapURL != "https://maps.example.com/?q=school" {
		t.Fatalf("school settings %+v", s)
	}
	// PUT /school/settings changes them for the public page too; omitted
	// details keep their values and a bad EIIN is refused.
	general := map[string]any{"name_en": st.Data.NameEN, "name_bn": st.Data.NameBN, "logo_url": st.Data.LogoURL, "address": st.Data.Address, "phone": st.Data.Phone, "email": st.Data.Email,
		"preferred_language": st.Data.PreferredLanguage, "primary_color": st.Data.PrimaryColor, "secondary_color": st.Data.SecondaryColor, "dashboard_theme": st.Data.DashboardTheme, "grading_scheme_id": st.Data.GradingSchemeID}
	settingsJSON := func(extra map[string]any) string {
		m := map[string]any{}
		for k, v := range general {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	call(http.MethodPut, "/school/settings", settingsJSON(map[string]any{"eiin": "12345"}), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/settings", settingsJSON(map[string]any{"map_url": "http://maps.example.com"}), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/settings", settingsJSON(map[string]any{"founded_year": 1700}), adminH, http.StatusBadRequest)
	call(http.MethodPut, "/school/settings", settingsJSON(nil), adminH, http.StatusOK)
	if v, _ := publicView(); v.Data.About == nil || v.Data.About.EIIN != "123456" || v.Data.Head == nil || v.Data.Head.NameEN != "Ayesha Rahman" {
		t.Fatalf("omitted details were not kept: %+v", v.Data)
	}
	call(http.MethodPut, "/school/settings", settingsJSON(map[string]any{"eiin": "654321", "head_teacher_name_en": "Karim Uddin"}), adminH, http.StatusOK)
	if v, _ := publicView(); v.Data.About == nil || v.Data.About.EIIN != "654321" || v.Data.Head == nil || v.Data.Head.NameEN != "Karim Uddin" {
		t.Fatalf("settings change not on the public page: %+v", v.Data)
	}
	call(http.MethodPut, "/school/public-page", page, adminH, http.StatusOK)

	// Head teacher's photo: cropped square, public, versioned.
	var photo bytes.Buffer
	canvas := image.NewNRGBA(image.Rect(0, 0, 900, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 900; x++ {
			canvas.Set(x, y, color.NRGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	if err := png.Encode(&photo, canvas); err != nil {
		t.Fatal(err)
	}
	upload := func(method, path string, headers map[string]string, want int) []byte {
		t.Helper()
		hdr := map[string]string{"Content-Type": "image/png"}
		for k, val := range headers {
			hdr[k] = val
		}
		res := perform(router, method, path, photo.String(), nil, hdr)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	upload(http.MethodPut, "/school/public-page/head-photo", headH, http.StatusForbidden)
	upload(http.MethodPut, "/school/public-page/head-photo", adminH, http.StatusOK)
	v, _ = publicView()
	stored, err := jpeg.DecodeConfig(bytes.NewReader(call(http.MethodGet, "/school/public-page/head-photo?v="+v.Data.Head.PhotoVersion, "", nil, http.StatusOK)))
	if err != nil || stored.Width != 480 || stored.Height != 480 {
		t.Fatalf("head photo %+v %v", stored, err)
	}

	// Gallery: compressed full size and a small grid copy, public.
	var galleryPhoto struct {
		Data school.GalleryPhoto `json:"data"`
	}
	big := image.NewNRGBA(image.Rect(0, 0, 3000, 2000))
	for y := 0; y < 2000; y += 4 {
		for x := 0; x < 3000; x += 4 {
			big.Set(x, y, color.NRGBA{uint8(x / 12), uint8(y / 8), 140, 255})
		}
	}
	photo.Reset()
	if err := png.Encode(&photo, big); err != nil {
		t.Fatal(err)
	}
	upload(http.MethodPost, "/school/gallery", headH, http.StatusForbidden)
	if err := json.Unmarshal(upload(http.MethodPost, "/school/gallery", adminH, http.StatusCreated), &galleryPhoto); err != nil {
		t.Fatal(err)
	}
	g := galleryPhoto.Data
	if g.Width != 1600 || g.Height != 1066 {
		t.Fatalf("gallery size %dx%d", g.Width, g.Height)
	}
	thumb, err := jpeg.DecodeConfig(bytes.NewReader(call(http.MethodGet, "/school/gallery/"+g.ID+"/image?size=thumb&v="+g.Version, "", nil, http.StatusOK)))
	if err != nil || thumb.Width != 480 {
		t.Fatalf("thumb %+v %v", thumb, err)
	}
	call(http.MethodPatch, "/school/gallery/"+g.ID, `{"caption_en":"Annual sports day","caption_bn":"বার্ষিক ক্রীড়া দিবস"}`, adminH, http.StatusOK)
	if v, _ = publicView(); len(v.Data.Gallery) != len(galleryBefore)+1 {
		t.Fatalf("gallery has %d photos", len(v.Data.Gallery))
	}

	// Notices: only published, live notices whose author ticked "show on the
	// public page" appear, and the flag is off unless chosen.
	noticeService := notices.NewService(notices.NewRepository(db))
	draft, err := noticeService.Save(ctx, admin.ID, "", notices.NoticeInput{TitleEN: "Default notice " + suffix, BodyEN: "Staff meeting on Sunday " + suffix, Priority: "normal", AudienceRoles: []string{"teacher"}})
	if err != nil {
		t.Fatal(err)
	}
	if draft.ShowPublic {
		t.Fatal("a notice was public by default")
	}
	public, err := noticeService.Save(ctx, admin.ID, "", notices.NoticeInput{TitleEN: "Sports day " + suffix, TitleBN: "ক্রীড়া দিবস", BodyEN: "Families are welcome on the field.", Priority: "important", AudienceRoles: []string{"guardian"}, ShowPublic: true})
	if err != nil || !public.ShowPublic {
		t.Fatalf("public notice %+v %v", public, err)
	}
	unpublished, err := noticeService.Save(ctx, admin.ID, "", notices.NoticeInput{TitleEN: "Not yet " + suffix, BodyEN: "Draft text " + suffix, Priority: "normal", AudienceRoles: []string{"guardian"}, ShowPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{draft.ID, public.ID} {
		if _, err := noticeService.Publish(ctx, admin.ID, id, notices.PublishInput{}); err != nil {
			t.Fatal(err)
		}
	}
	_ = unpublished
	v, body := publicView()
	if len(v.Data.Notices) != 1 || v.Data.Notices[0].ID != public.ID || v.Data.Notices[0].TitleBN != "ক্রীড়া দিবস" {
		t.Fatalf("public notices %+v", v.Data.Notices)
	}
	if strings.Contains(body, "Staff meeting") || strings.Contains(body, "Draft text") {
		t.Fatal("a notice not chosen for the public page appeared")
	}

	// Nothing personal: a student's name, admission number and the admin's
	// identity never appear, and only the known fields are present.
	if err := db.Exec("INSERT INTO students (admission_no,name_en,status,session_id,class_id,section_id) SELECT ?, ?, 'active', s.session_id, s.class_id, s.id FROM academic_sections s LIMIT 1", "PUB-"+suffix, "Secret Pupil "+suffix).Error; err != nil {
		t.Fatal(err)
	}
	_, body = publicView()
	for _, private := range []string{"Secret Pupil", "PUB-" + suffix, admin.Email, admin.ID, "created_by", "updated_by", "user_id", "admission", "roll", "fee", "result", "gpa", "audience"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(private)) {
			t.Fatalf("the public page contains %q", private)
		}
	}
	var keys map[string]map[string]json.RawMessage
	_ = json.Unmarshal([]byte(body), &keys)
	known := map[string]bool{
		"design_style": true, "features": true, "activities_en": true, "activities_bn": true,
		"footer_description_en": true, "footer_description_bn": true,
		"social_facebook": true, "social_youtube": true, "social_whatsapp": true, "social_messenger": true,
		"privacy_policy_en": true, "privacy_policy_bn": true, "terms_en": true, "terms_bn": true,
		"about": true, "head": true, "gallery": true, "notices": true, "contact": true,
	}
	for k := range keys["data"] {
		if !known[k] {
			t.Fatalf("unexpected section %q", k)
		}
	}

	// Removing content hides sections again.
	call(http.MethodDelete, "/school/gallery/"+g.ID, "", adminH, http.StatusOK)
	call(http.MethodDelete, "/school/public-page/head-photo", "", adminH, http.StatusOK)
	call(http.MethodGet, "/school/public-page/head-photo", "", nil, http.StatusNotFound)
	call(http.MethodPut, "/school/public-page", `{}`, adminH, http.StatusOK)
	if v, _ = publicView(); v.Data.About != nil || v.Data.Head != nil || v.Data.Contact != nil {
		t.Fatalf("cleared page still has sections: %+v", v.Data)
	}
}
