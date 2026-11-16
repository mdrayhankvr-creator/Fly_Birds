package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
)

// A reset empties almost every table, so it is run inside a transaction that
// is rolled back: the shared test database is left exactly as it was, and the
// test needs no database of its own and no right to create one.
func openResetDB(t *testing.T) (context.Context, *gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset; PostgreSQL integration test not run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	return ctx, db, tx
}

// TestPostgresResetKeepsWhatTheSchemaSeeds is the guard on the keep list. A
// reset keeps the tables a fresh migration fills in, because those hold the
// reference and settings rows the application expects to find. A migration
// that starts seeding a new table fails this test until someone decides
// whether a clean slate should keep it too.
//
// "Fresh" is the point, so the check cannot run against the shared database:
// it empties a transaction's view of every table first, applies the seeding
// migrations again, and looks at what came back.
func TestPostgresResetKeepsWhatTheSchemaSeeds(t *testing.T) {
	ctx, _, tx := openResetDB(t)
	tables, err := publicTables(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	// Every table a reset would empty, emptied — which is the state a fresh
	// database would be in but for the rows the migrations put back.
	plan, err := PlanReset(ctx, tx, "", true)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashResetPassword("Seeded-check-password-26!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reset(ctx, tx, nil, plan, []ResetAdmin{
		{Name: "A", Email: "a@example.test", PasswordHash: hash},
		{Name: "B", Email: "b@example.test", PasswordHash: hash},
	}); err != nil {
		t.Fatal(err)
	}
	// What still holds rows is what the schema itself provides, plus the two
	// administrators and the one audit entry the reset writes.
	afterReset := map[string]bool{"users": true, "audit_events": true}
	seeded := []string{}
	for _, name := range tables {
		if afterReset[name] {
			continue
		}
		var rows int64
		if err := tx.Raw(`SELECT count(*) FROM "` + name + `"`).Row().Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows > 0 {
			seeded = append(seeded, name)
		}
	}
	sort.Strings(seeded)
	want := append([]string{}, resetSeeded...)
	for _, extra := range resetKeepExtra {
		// The identity uploads are empty in a fresh database; they only
		// appear here if this database happens to have one.
		var rows int64
		if err := tx.Raw(`SELECT count(*) FROM "` + extra + `"`).Row().Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows > 0 {
			want = append(want, extra)
		}
	}
	sort.Strings(want)
	if strings.Join(seeded, ",") != strings.Join(want, ",") {
		t.Fatalf("a reset leaves a different set of tables with rows in them.\n got: %v\nwant: %v\n\nIf a migration now seeds a table, decide whether a clean slate keeps it, then update resetSeeded and docs/progress.", seeded, want)
	}
}

// resetSchool is the school the reset test works on: identity, public page
// content, the uploads that carry the school's look, and people and records.
type resetSchool struct{ admin, teacher, student string }

func seedResetSchool(t *testing.T, ctx context.Context, tx *gorm.DB) resetSchool {
	t.Helper()
	var s resetSchool
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.WithContext(ctx).Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", strings.SplitN(strings.TrimSpace(q), "\n", 2)[0], err)
		}
	}
	one := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.WithContext(ctx).Raw(q, args...).Row().Scan(&id); err != nil {
			t.Fatalf("%s: %v", strings.SplitN(strings.TrimSpace(q), "\n", 2)[0], err)
		}
		return id
	}
	stamp := strings.ReplaceAll(time.Now().Format("150405.000"), ".", "")

	// Identity and public page content, as an administrator would have set it.
	exec("UPDATE school_settings SET name_en = ?, name_bn = ?, dashboard_theme = ?",
		"Jalalabad High School", "জালালাবাদ উচ্চ বিদ্যালয়", "dark")
	exec("UPDATE school_public_page SET about_en = ?, head_message_en = ?, privacy_policy_en = ?",
		"About this school", "A word from the head teacher", "Our privacy policy")

	hash, err := auth.HashPassword("Old-demo-password-2026!")
	if err != nil {
		t.Fatal(err)
	}
	user := func(local, name, role string) string {
		return one(`INSERT INTO users (email, display_name, password_hash, role, is_active)
VALUES (?, ?, ?, ?, true) RETURNING id`, local+stamp+"@demo.test", name, hash, role)
	}
	s.admin, s.teacher, s.student = user("old.admin", "Old Admin", "admin"), user("old.teacher", "Old Teacher", "teacher"), user("old.student", "Old Student", "student")

	// The uploads that belong to the school's identity, each recording who
	// put it there: the reference a reset has to clear to keep the row.
	exec(`INSERT INTO school_logo (id, content_type, content, version, updated_by)
VALUES (1, 'image/png', '\x89504e47'::bytea, ?, ?)
ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content, version = EXCLUDED.version, updated_by = EXCLUDED.updated_by`, hexVersion(12), s.admin)
	exec(`INSERT INTO school_slides (position, content_type, content, width, height, version, created_by)
VALUES (1, 'image/jpeg', '\xffd8ffe0'::bytea, 800, 600, ?, ?)`, hexVersion(16), s.admin)
	exec(`INSERT INTO school_gallery (position, content, thumb, width, height, version, created_by)
VALUES (1, '\x89504e47'::bytea, '\x89504e47'::bytea, 800, 600, ?, ?)`, hexVersion(16), s.admin)

	// School life.
	// A four-digit year, as the schema requires, and one far enough out that
	// it does not collide with a session the shared database already holds.
	year := 2040 + int(time.Now().UnixNano()%50)
	session := one(`INSERT INTO academic_sessions (name, start_date, end_date) VALUES (?, ?, ?) RETURNING id`,
		strconv.Itoa(year), strconv.Itoa(year)+"-01-01", strconv.Itoa(year)+"-12-31")
	class := one(`INSERT INTO academic_classes (name) VALUES (?) RETURNING id`, "Class "+stamp)
	section := one(`INSERT INTO academic_sections (session_id, class_id, name, code) VALUES (?, ?, 'A', ?) RETURNING id`, session, class, "S"+stamp)
	subject := one(`INSERT INTO academic_subjects (name_en, name_bn, code) VALUES ('Bangla', 'বাংলা', ?) RETURNING id`, "C"+stamp)
	exec(`INSERT INTO academic_class_subjects (class_id, subject_id, is_required, is_fourth_subject) VALUES (?, ?, true, false)`, class, subject)
	student := one(`INSERT INTO students (admission_no, name_en, name_bn, session_id, class_id, section_id, user_id)
VALUES (?, 'Imran Rahman', 'ইমরান রহমান', ?, ?, ?, ?) RETURNING id`, "ADM"+stamp, session, class, section, s.student)
	// An enrolment row comes with the student, written by the schema itself.
	guardian := one(`INSERT INTO guardians (name_en) VALUES ('Rahman Ali') RETURNING id`)
	exec(`INSERT INTO student_guardians (student_id, guardian_id, relationship) VALUES (?, ?, 'father')`, student, guardian)
	exec(`INSERT INTO employees (employee_code, user_id, kind, name_en, designation, joined_on)
VALUES (?, ?, 'teacher', 'Old Teacher', 'Assistant Teacher', '2020-01-01')`, "EMP"+stamp, s.teacher)
	exec(`INSERT INTO notices (title_en, body_en, audience_roles, status, publish_at, created_by, updated_by)
VALUES ('Welcome', 'Term begins', ARRAY['student']::text[], 'published', now(), ?, ?)`, s.admin, s.admin)
	exec(`INSERT INTO audit_events (actor_user_id, action, target_type, target_id, outcome, metadata)
VALUES (?, 'school.settings_updated', 'school', ?, 'success', '{}'::jsonb)`, s.admin, class)
	// Seeding and resetting share one transaction here, which production
	// never does, so the deferred checks the inserts queued are run now:
	// TRUNCATE refuses a table that still has trigger events waiting.
	exec("SET CONSTRAINTS ALL IMMEDIATE")
	return s
}

// TestPostgresResetKeepsIdentityAndDeletesEverythingElse is the whole promise
// of the command, on a school with people and records in it.
func TestPostgresResetKeepsIdentityAndDeletesEverythingElse(t *testing.T) {
	ctx, _, tx := openResetDB(t)
	seedResetSchool(t, ctx, tx)

	// 1. The name is the guard against the wrong database.
	if _, err := PlanReset(ctx, tx, "Some Other School", false); !errors.Is(err, ErrResetSchoolMismatch) {
		t.Fatalf("a wrong school name was accepted: %v", err)
	}
	if _, err := PlanReset(ctx, tx, "  jalalabad high school  ", false); err != nil {
		t.Fatalf("the right name, differently spaced and cased, was refused: %v", err)
	}
	plan, err := PlanReset(ctx, tx, "Jalalabad High School", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SchoolNameBN != "জালালাবাদ উচ্চ বিদ্যালয়" {
		t.Fatalf("the plan names the school %q / %q", plan.SchoolNameEN, plan.SchoolNameBN)
	}
	if plan.Users < 3 || plan.Rows < 15 {
		t.Fatalf("the plan deletes %d users and %d rows", plan.Users, plan.Rows)
	}
	// Planning is a dry run: it writes nothing.
	usersBefore := count(t, tx, "users")
	if usersBefore != plan.Users {
		t.Fatalf("planning changed the data: %d users, plan said %d", usersBefore, plan.Users)
	}
	// Kept and emptied together are the whole schema, with nothing counted
	// twice and nothing forgotten.
	tables, err := publicTables(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Keep)+len(plan.Empty) != len(tables) {
		t.Fatalf("%d kept + %d emptied ≠ %d tables", len(plan.Keep), len(plan.Empty), len(tables))
	}
	seen := map[string]bool{}
	for _, t2 := range append(append([]TableRows{}, plan.Keep...), plan.Empty...) {
		if seen[t2.Table] {
			t.Fatalf("%s is in the plan twice", t2.Table)
		}
		seen[t2.Table] = true
	}
	// The references from kept rows into deleted ones are found.
	nulled := map[string]bool{}
	for _, n := range plan.Nulled {
		nulled[n.Table+"."+n.Column] = true
	}
	for _, want := range []string{"school_logo.updated_by", "school_slides.created_by",
		"school_gallery.created_by", "school_public_page.updated_by", "school_weekly_off_days.created_by"} {
		if !nulled[want] {
			t.Errorf("%s should be cleared by the reset; the plan has %v", want, plan.Nulled)
		}
	}

	// 2. Two administrators are required, with different addresses, and a
	//    refused reset changes nothing.
	hash, err := HashResetPassword("New-admin-password-2026!")
	if err != nil {
		t.Fatal(err)
	}
	good := []ResetAdmin{
		{Name: "Head Teacher", Email: "head@jalalabad.edu.bd", PasswordHash: hash},
		{Name: "School Office", Email: "office@jalalabad.edu.bd", PasswordHash: hash},
	}
	for name, bad := range map[string][]ResetAdmin{
		"none":         {},
		"one":          {good[0]},
		"three":        {good[0], good[1], good[0]},
		"no name":      {{Name: " ", Email: "a@b.test", PasswordHash: hash}, good[1]},
		"no password":  {{Name: "A", Email: "a@b.test"}, good[1]},
		"same address": {good[0], {Name: "Twin", Email: good[0].Email, PasswordHash: hash}},
	} {
		if _, err := Reset(ctx, tx, nil, plan, bad); !errors.Is(err, ErrResetAdmins) {
			t.Errorf("administrators %q were accepted: %v", name, err)
		}
	}
	if after := count(t, tx, "users"); after != usersBefore {
		t.Fatalf("a refused reset changed the accounts: %d then %d", usersBefore, after)
	}

	// 3. The reset itself.
	store, err := files.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reset(ctx, tx, store, plan, good)
	if err != nil {
		t.Fatal(err)
	}
	if result.Admins != 2 || result.Rows != plan.Rows || result.Emptied != len(plan.Empty) {
		t.Fatalf("result: %+v (plan: %d rows, %d tables)", result, plan.Rows, len(plan.Empty))
	}

	// Everything that should be gone is gone, whatever was there before.
	for _, name := range []string{"students", "guardians", "student_guardians", "student_enrolments",
		"employees", "academic_sessions", "academic_classes", "academic_sections", "academic_subjects",
		"academic_class_subjects", "notices", "auth_sessions", "demo_records"} {
		if rows := count(t, tx, name); rows != 0 {
			t.Errorf("%s still has %d rows", name, rows)
		}
	}
	// Two accounts, and one audit entry: the one that records the reset.
	if rows := count(t, tx, "users"); rows != 2 {
		t.Errorf("%d accounts after the reset, want 2", rows)
	}
	if rows := count(t, tx, "audit_events"); rows != 1 {
		t.Errorf("%d audit entries after the reset, want 1 (the reset itself)", rows)
	}
	var action string
	tx.Raw("SELECT action FROM audit_events").Row().Scan(&action)
	if action != "school.data_reset" {
		t.Errorf("the audit entry is %q", action)
	}

	// The school's identity and its public page are untouched.
	var nameEN, nameBN, theme, about, head, policy string
	tx.Raw("SELECT name_en, name_bn, dashboard_theme FROM school_settings").Row().Scan(&nameEN, &nameBN, &theme)
	if nameEN != "Jalalabad High School" || nameBN != "জালালাবাদ উচ্চ বিদ্যালয়" || theme != "dark" {
		t.Errorf("identity changed: %q / %q / %q", nameEN, nameBN, theme)
	}
	tx.Raw("SELECT about_en, head_message_en, privacy_policy_en FROM school_public_page").Row().Scan(&about, &head, &policy)
	if about != "About this school" || head != "A word from the head teacher" || policy != "Our privacy policy" {
		t.Errorf("public page changed: %q / %q / %q", about, head, policy)
	}
	for _, name := range []string{"school_logo", "school_slides", "school_gallery", "school_public_features",
		"grading_schemes", "grading_bands", "school_weekly_off_days", "roles", "permissions", "role_permissions",
		"module_definitions", "school_modules", "academic_groups", "result_comment_presets",
		"payment_settings", "id_card_settings", "admit_card_settings", "diary_settings", "analytics_settings"} {
		if rows := count(t, tx, name); rows == 0 {
			t.Errorf("%s was emptied; it should have been kept", name)
		}
	}
	// The kept uploads no longer point at an account that is gone.
	var logoBy, slideBy *string
	tx.Raw("SELECT updated_by FROM school_logo").Row().Scan(&logoBy)
	tx.Raw("SELECT created_by FROM school_slides LIMIT 1").Row().Scan(&slideBy)
	if logoBy != nil || slideBy != nil {
		t.Errorf("a kept upload still names its uploader: logo %v slide %v", logoBy, slideBy)
	}

	// 4. The two administrators, and only them, each having to choose a
	//    password before doing anything else.
	var rows []struct {
		Email              string
		DisplayName        string
		Role               string
		IsActive           bool
		MustChangePassword bool
	}
	if err := tx.Raw("SELECT email, display_name, role, is_active, must_change_password FROM users ORDER BY email").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%d accounts after the reset", len(rows))
	}
	for _, r := range rows {
		if r.Role != "admin" || !r.IsActive || !r.MustChangePassword {
			t.Errorf("%s: role %q active %v must change %v", r.Email, r.Role, r.IsActive, r.MustChangePassword)
		}
	}
	if rows[0].Email != "head@jalalabad.edu.bd" || rows[0].DisplayName != "Head Teacher" ||
		rows[1].Email != "office@jalalabad.edu.bd" || rows[1].DisplayName != "School Office" {
		t.Errorf("accounts: %+v", rows)
	}
	// The password that was typed signs in; the old school's does not.
	service, err := auth.NewService(auth.NewRepository(tx), []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(ctx, "head@jalalabad.edu.bd", "New-admin-password-2026!")
	if err != nil {
		t.Fatalf("the new administrator cannot sign in: %v", err)
	}
	if !credentials.User.MustChangePassword {
		t.Error("the new administrator is not asked to choose a password")
	}

	// 5. No row is left pointing at something that is gone.
	orphans, err := ResetOrphans(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) > 0 {
		t.Fatalf("orphan rows: %v", orphans)
	}

	// 6. Running it again on the clean school is harmless.
	plan2, err := PlanReset(ctx, tx, "Jalalabad High School", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.Users != 2 {
		t.Fatalf("the second plan sees %d accounts", plan2.Users)
	}
	if _, err := Reset(ctx, tx, store, plan2, good); err != nil {
		t.Fatalf("a second reset: %v", err)
	}
	if orphans, err := ResetOrphans(ctx, tx); err != nil || len(orphans) > 0 {
		t.Fatalf("orphan rows after a second reset: %v %v", orphans, err)
	}
	if rows := count(t, tx, "users"); rows != 2 {
		t.Fatalf("%d accounts after a second reset", rows)
	}
}

// TestPostgresResetAnySchoolSkipsTheNameCheck covers the deliberate override.
func TestPostgresResetAnySchoolSkipsTheNameCheck(t *testing.T) {
	ctx, _, tx := openResetDB(t)
	if _, err := PlanReset(ctx, tx, "nothing like the real name", true); err != nil {
		t.Fatalf("-any-school still checked the name: %v", err)
	}
	if _, err := PlanReset(ctx, tx, "nothing like the real name", false); !errors.Is(err, ErrResetSchoolMismatch) {
		t.Fatal("without the override the wrong name was accepted")
	}
}

// TestPostgresResetOrphansFindsOne proves the orphan check can fail, so a
// clean report from it means something.
func TestPostgresResetOrphansFindsOne(t *testing.T) {
	ctx, _, tx := openResetDB(t)
	var constraint string
	if err := tx.Raw(`SELECT conname FROM pg_constraint
WHERE contype = 'f' AND conrelid = 'school_logo'::regclass LIMIT 1`).Row().Scan(&constraint); err != nil {
		t.Fatalf("find the constraint on school_logo: %v", err)
	}
	// Written while the constraint is away, then checked with it back: a row
	// pointing at an account that does not exist.
	if err := tx.Exec("ALTER TABLE school_logo DROP CONSTRAINT " + constraint).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO school_logo (id, content_type, content, version, updated_by)
VALUES (1, 'image/png', '\x89504e47'::bytea, 'abcdef012345', gen_random_uuid())
ON CONFLICT (id) DO UPDATE SET updated_by = EXCLUDED.updated_by, version = EXCLUDED.version`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("ALTER TABLE school_logo ADD CONSTRAINT " + constraint +
		" FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	orphans, err := ResetOrphans(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || !strings.Contains(orphans[0], "school_logo.updated_by") {
		t.Fatalf("the orphan check missed a broken reference: %v", orphans)
	}
}

func count(t *testing.T, tx *gorm.DB, table string) int64 {
	t.Helper()
	var rows int64
	if err := tx.Raw(`SELECT count(*) FROM "` + table + `"`).Row().Scan(&rows); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return rows
}

// hexVersion is a version string in the form the image tables require.
func hexVersion(n int) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])[:n]
}
