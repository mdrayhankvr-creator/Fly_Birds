package demo

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

// freshDatabase migrates a new, empty database for one test.
func freshDatabase(t *testing.T, ctx context.Context) (*gorm.DB, *auth.Service) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	admin, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, _ := admin.DB()
	t.Cleanup(func() { _ = adminSQL.Close() })
	name := fmt.Sprintf("flybirds_demo_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error })
	parsed, _ := url.Parse(dsn)
	parsed.Path = "/" + name
	db, err := database.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return db, authService
}

func TestPostgresDemoSeedIsIdempotentAndAdditive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	db, authService := freshDatabase(t, ctx)
	// The school's own data that the seed must leave alone.
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec("UPDATE school_settings SET name_en = 'Real School', name_bn = 'আসল স্কুল' WHERE id = 1")
	exec("INSERT INTO academic_classes (name, display_order) VALUES ('Class Six', 6)")
	exec("INSERT INTO academic_subjects (name_en, name_bn, code) VALUES ('Bangla First Paper', 'বাংলা প্রথম পত্র', 'BAN')")
	owner, err := authService.BootstrapUser(ctx, "owner@real.test", "OwnerPassword!123", "admin")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Seed(ctx, db, authService, Options{Password: "short"}); err != ErrPassword {
		t.Fatalf("short password accepted: %v", err)
	}
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC) // a Monday
	first, err := Seed(ctx, db, authService, Options{Password: "DemoPassword!2026", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if first.Created["student"] != 36 || first.Created["user"] != 8 || first.Created["exam"] != 6 || len(first.Accounts) != 8 {
		t.Fatalf("first run %+v", first)
	}
	counts := func() map[string]int64 {
		out := map[string]int64{}
		for _, table := range []string{"users", "students", "guardians", "student_guardians", "academic_classes", "academic_subjects", "academic_sections", "employees",
			"teacher_assignments", "class_routines", "exams", "exam_marks", "exam_results", "student_attendance", "teacher_attendance", "student_fees", "fee_payments",
			"fee_payment_allocations", "ledger_entries", "notices", "school_events", "class_diaries", "demo_records", "academic_sessions"} {
			var n int64
			if err := db.Raw("SELECT count(*) FROM " + table).Row().Scan(&n); err != nil {
				t.Fatal(err)
			}
			out[table] = n
		}
		return out
	}
	before := counts()
	second, err := Seed(ctx, db, authService, Options{Password: "AnotherPassword!2026", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("second run created %v", second.Created)
	}
	after := counts()
	for table, n := range before {
		if after[table] != n {
			t.Errorf("%s: %d after the second run, %d before", table, after[table], n)
		}
	}

	var schoolName, classSix, subject string
	db.Raw("SELECT name_en FROM school_settings WHERE id = 1").Scan(&schoolName)
	if schoolName != "Real School" {
		t.Fatalf("school name overwritten: %s", schoolName)
	}
	db.Raw("SELECT a.name FROM academic_classes a JOIN demo_records d ON d.record_id = a.id AND d.kind = 'class' AND d.ref = 'Class Six'").Scan(&classSix)
	db.Raw("SELECT s.code FROM academic_subjects s JOIN demo_records d ON d.record_id = s.id AND d.kind = 'subject' AND d.ref = 'BAN'").Scan(&subject)
	if classSix != "Class Six (Demo)" || subject != "DEMO-BAN" {
		t.Fatalf("demo reused the school's own rows: class %q subject %q", classSix, subject)
	}
	// A real school gets nothing on its public page from the seed.
	var eiin, about string
	var gallery, publicNotices int64
	db.Raw("SELECT eiin FROM school_settings WHERE id = 1").Scan(&eiin)
	db.Raw("SELECT about_en FROM school_public_page WHERE id = 1").Scan(&about)
	db.Raw("SELECT count(*) FROM school_gallery").Scan(&gallery)
	db.Raw("SELECT count(*) FROM notices WHERE show_public").Scan(&publicNotices)
	if eiin != "" || about != "" || gallery != 0 || publicNotices != 0 {
		t.Fatalf("public content on a real school: eiin %q about %q gallery %d public notices %d", eiin, about, gallery, publicNotices)
	}
	var ownerRole string
	db.Raw("SELECT role FROM users WHERE id = ?", owner.ID).Scan(&ownerRole)
	if ownerRole != "admin" {
		t.Fatal("existing account changed")
	}
	if _, err := authService.Login(ctx, "demo.teacher@"+Domain, "DemoPassword!2026"); err != nil {
		t.Fatalf("demo password changed by the second run: %v", err)
	}
	var results, todayMarks, children, published int64
	db.Raw("SELECT count(*) FROM exam_results").Scan(&results)
	db.Raw("SELECT count(*) FROM exams WHERE status = 'published'").Scan(&published)
	db.Raw("SELECT count(*) FROM student_attendance WHERE attendance_date = '2026-10-19'").Scan(&todayMarks)
	db.Raw("SELECT count(*) FROM student_guardians sg JOIN guardians g ON g.id = sg.guardian_id JOIN users u ON u.id = g.user_id WHERE u.email = ?", "demo.guardian@"+Domain).Scan(&children)
	if results != 36 || published != 6 || todayMarks != 0 || children != 2 {
		t.Fatalf("results %d published %d today %d children %d", results, published, todayMarks, children)
	}
	var fridays int64
	db.Raw("SELECT count(*) FROM student_attendance WHERE extract(dow FROM attendance_date) = 5").Scan(&fridays)
	if fridays != 0 {
		t.Fatalf("%d attendance marks on Fridays", fridays)
	}
	// A later run adds only the missing school days.
	later, err := Seed(ctx, db, authService, Options{Password: "DemoPassword!2026", Now: now.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	var newDay int64
	db.Raw("SELECT count(*) FROM student_attendance WHERE attendance_date = '2026-10-19'").Scan(&newDay)
	if newDay == 0 || later.Created["student"] != 0 {
		t.Fatalf("later run: marks for the previous day %d, created %v", newDay, later.Created)
	}
}

// The demo school's public page is filled once; what an administrator writes
// afterwards is kept, and a later run adds nothing twice.
func TestPostgresDemoSeedFillsTheDemoSchoolsPublicPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	db, authService := freshDatabase(t, ctx)
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	first, err := Seed(ctx, db, authService, Options{Password: "DemoPassword!2026", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if first.Created["gallery"] != 4 || first.Created["head_photo"] != 1 {
		t.Fatalf("first run %v", first.Created)
	}
	var page struct {
		Name, EIIN, Phone, Email, HeadName, About, Message, Photo string
		Founded                                                   int
		Counts                                                    bool
	}
	read := func() {
		t.Helper()
		if err := db.Raw(`SELECT s.name_en AS name, s.eiin, s.phone, s.email, s.head_teacher_name_en AS head_name, s.founded_year AS founded,
p.about_en AS about, p.head_message_en AS message, p.head_photo_version AS photo, p.show_counts AS counts
FROM school_settings s, school_public_page p WHERE s.id = 1 AND p.id = 1`).Scan(&page).Error; err != nil {
			t.Fatal(err)
		}
	}
	read()
	if page.Name != demoSchoolName || page.EIIN != "123456" || page.Founded != 1987 || page.Phone == "" || page.Email != "office@"+Domain ||
		page.HeadName != "Shahana Begum" || page.About == "" || page.Message == "" || page.Photo == "" || !page.Counts {
		t.Fatalf("demo public page %+v", page)
	}
	var captions, publicNotices int64
	db.Raw("SELECT count(*) FROM school_gallery WHERE caption_en <> '' AND caption_bn <> ''").Scan(&captions)
	db.Raw("SELECT count(*) FROM notices WHERE show_public AND status = 'published'").Scan(&publicNotices)
	if captions != 4 || publicNotices != 1 {
		t.Fatalf("captioned photos %d, public notices %d", captions, publicNotices)
	}
	// The administrator edits the page; the seed keeps their words.
	if err := db.Exec("UPDATE school_public_page SET about_en = 'Our own words' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE school_settings SET phone = '01811-111111' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	second, err := Seed(ctx, db, authService, Options{Password: "DemoPassword!2026", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("second run created %v", second.Created)
	}
	read()
	if page.About != "Our own words" || page.Phone != "01811-111111" {
		t.Fatalf("the seed overwrote the administrator's edits: %+v", page)
	}
	// Renamed, it is no longer the demo school: removed photos stay removed.
	if err := db.Exec("UPDATE school_settings SET name_en = 'Renamed School' WHERE id = 1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM school_gallery").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, db, authService, Options{Password: "DemoPassword!2026", Now: now}); err != nil {
		t.Fatal(err)
	}
	var gallery int64
	db.Raw("SELECT count(*) FROM school_gallery").Scan(&gallery)
	if gallery != 0 {
		t.Fatalf("the seed added %d photos to a school that is no longer the demo", gallery)
	}
}
