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
)

func TestPostgresDemoSeedIsIdempotentAndAdditive(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
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
