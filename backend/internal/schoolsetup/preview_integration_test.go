package schoolsetup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

var disposablePreviewDatabase = regexp.MustCompile(`^school_setup_preview_test_([0-9]+_[0-9]+|[0-9a-f]{10,32})$`)

func validatePreviewTestDSN(dsn string) error {
	config, err := pgconn.ParseConfig(dsn)
	if err != nil || !disposablePreviewDatabase.MatchString(config.Database) || (config.Host != "127.0.0.1" && config.Host != "localhost") {
		return fmt.Errorf("use a fresh local school_setup_preview_test_ database")
	}
	return nil
}

// Check every user schema before migrations; a nonempty public or custom
// schema is not an acceptable fixture database even if its name looks safe.
func requireEmptyPreviewTestDatabase(ctx context.Context, db *sql.DB) error {
	const query = `SELECT
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
    AND c.relkind IN ('r','p','f','v','m','S'))
 +(SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%')
 +(SELECT count(*) FROM pg_namespace
  WHERE nspname NOT IN ('pg_catalog','information_schema','public') AND nspname NOT LIKE 'pg_toast%')`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("refusing migrations: disposable test database is not empty")
	}
	return nil
}

func TestPreviewDatabaseGuard(t *testing.T) {
	for _, tc := range []struct {
		name, dsn string
		wantOK    bool
	}{
		{"shared CI database", "postgres://fixture@localhost/flybirds_ci?sslmode=disable", false},
		{"fbshow database", "postgres://fixture@localhost/fbshow?sslmode=disable", false},
		{"Joy database", "postgres://fixture@localhost/fbqa_cp1b?sslmode=disable", false},
		{"remote with safe-looking name", "postgres://fixture@production.example/school_setup_preview_test_1234?sslmode=disable", false},
		{"prefix without unique suffix", "postgres://fixture@localhost/school_setup_preview_test_?sslmode=disable", false},
		{"safe-looking production suffix", "postgres://fixture@localhost/school_setup_preview_test_fbshow?sslmode=disable", false},
		{"fresh disposable name", "postgres://fixture@127.0.0.1/school_setup_preview_test_1234567890?sslmode=disable", true},
		{"CI run name", "postgres://fixture@localhost/school_setup_preview_test_1234567890_1?sslmode=disable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if gotOK := validatePreviewTestDSN(tc.dsn) == nil; gotOK != tc.wantOK {
				t.Fatalf("guard accepted=%t, want %t", gotOK, tc.wantOK)
			}
		})
	}
}

// The harness must supply a newly created, disposable database with this
// prefix. The test refuses any other database before it runs migrations.
func TestPostgresPreviewSchoolSetup(t *testing.T) {
	dsn := os.Getenv("SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("AUTH_TEST_DATABASE_URL") != "" {
			t.Fatal("SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL is required; shared AUTH_TEST_DATABASE_URL is unsafe for this fixture")
		}
		t.Skip("SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL is unset")
	}
	if err := validatePreviewTestDSN(dsn); err != nil {
		t.Fatal("refusing test:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	gormDB, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := requireEmptyPreviewTestDatabase(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, gormDB); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyPreviewTestDatabase(ctx, sqlDB); err == nil {
		t.Fatal("nonempty migrated database passed the disposable database guard")
	}
	var count int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM academic_sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("test database is not fresh")
	}
	fixture := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := sqlDB.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := sqlDB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE school_settings SET name_en='Fixture School' WHERE id=1")
	for _, email := range protectedAdminEmails {
		fixture("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ($1,'fixture-hash','admin','Protected',true) RETURNING id::text", email)
	}
	session := fixture("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2026','2026-01-01','2026-12-31',true) RETURNING id::text")
	six := fixture("INSERT INTO academic_classes(name,display_order) VALUES ('SIX-06',1) RETURNING id::text")
	ten := fixture("INSERT INTO academic_classes(name,display_order,uses_groups) VALUES ('TEN-10',2,true) RETURNING id::text")
	sixA := fixture("INSERT INTO academic_sections(session_id,class_id,name,code,display_order) VALUES ($1,$2,'A','A',1) RETURNING id::text", session, six)
	fixture("INSERT INTO academic_sections(session_id,class_id,name,code,display_order) VALUES ($1,$2,'B','B',2) RETURNING id::text", session, six)
	fixture("INSERT INTO academic_sections(session_id,class_id,name,code,display_order) VALUES ($1,$2,'A','A',1) RETURNING id::text", session, ten)
	fixture("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES ('TEST-001','Test Student',$1,$2,$3,'active') RETURNING id::text", session, six, sixA)
	fixture("INSERT INTO employees(employee_code,kind,name_en,designation,joined_on,is_active) VALUES ('TEACHER-01','teacher','Existing Teacher','Teacher','2020-01-01',true) RETURNING id::text")
	for _, subject := range []struct{ code, name string }{{"134", "Agriculture Studies"}, {"151", "Home Science"}} {
		id := fixture("INSERT INTO academic_subjects(code,name_en,name_bn) VALUES ($1,$2,'Bangla') RETURNING id::text", subject.code, subject.name)
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES ($1,$2,false,true)", six, id)
	}
	exec("INSERT INTO school_weekly_off_days(day_of_week) VALUES (6) ON CONFLICT DO NOTHING")

	fingerprint := func() string {
		t.Helper()
		var hash string
		if err := sqlDB.QueryRowContext(ctx, `SELECT md5(string_agg(row_to_json(u)::text,'|' ORDER BY lower(u.email)))
FROM users u WHERE lower(email) IN ('mdrayhankvr@gmail.com','2023331031@student.sust.edu')`).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := fingerprint()
	manifest := &Manifest{StudentAdmissionNumbers: []string{"TEST-001", "NEW-1", "new-1"}, TeacherEmployeeCodes: []string{"teacher-01"}, AccountEmails: []string{protectedAdminEmails[0], "new@example.test"}}
	r, err := Preview(ctx, sqlDB, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if got := fingerprint(); got != before {
		t.Fatal("protected administrator rows changed")
	}
	if !r.ReadOnly || r.CredentialsGenerated || !r.ManifestChecked || r.School != "Fixture School" || r.Session != "2026" {
		t.Fatalf("unsafe or incomplete report: %+v", r)
	}
	if len(r.Sections) != 3 || r.StudentExisting != 1 || r.StudentToAdd != 44 || r.Sections[0].ToAdd != 14 || r.TeacherExisting != 1 || r.TeacherToAdd != 14 {
		t.Fatalf("counts: %+v", r)
	}
	if len(r.Teachers) != 1 || r.Teachers[0].LinkedAccount || len(r.FourthSubjects) != 2 || len(r.FourthSubjects[1].Options) != 0 {
		t.Fatalf("staff/choices: %+v", r)
	}
	if len(r.ProtectedAdmins) != 2 {
		t.Fatalf("protected admins: %+v", r.ProtectedAdmins)
	}
	for _, a := range r.ProtectedAdmins {
		if !a.Present || a.Role != "admin" || !a.Active || !a.ExcludedFromProposedChanges {
			t.Fatalf("unprotected admin: %+v", a)
		}
	}
	want := map[string]bool{"admission_number/already_exists": false, "admission_number/duplicate_in_manifest": false, "employee_code/already_exists": false, "email/already_exists": false, "email/protected_admin": false}
	for _, c := range r.IdentifierConflicts {
		key := c.Kind + "/" + c.Reason
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing conflict %s: %+v", key, r.IdentifierConflicts)
		}
	}
	joinedWarnings := strings.Join(r.Warnings, "\n")
	if !strings.Contains(joinedWarnings, "TEN-10 lacks") || !strings.Contains(joinedWarnings, "TEACHER-01 has no linked account") {
		t.Fatalf("missing setup warnings: %s", joinedWarnings)
	}
	generated, generatedReport, err := GenerateManifest(ctx, sqlDB, "2026")
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := GenerateManifest(ctx, sqlDB, "2026")
	if err != nil || !reflect.DeepEqual(generated, again) {
		t.Fatalf("manifest changed on repeat: %v", err)
	}
	if len(generated.StudentCandidates) != 44 || len(generated.TeacherCandidates) != 14 || !generatedReport.ManifestChecked {
		t.Fatalf("generated counts: %d students, %d teachers", len(generated.StudentCandidates), len(generated.TeacherCandidates))
	}
	checked, err := Preview(ctx, sqlDB, &generated)
	if err != nil || !checked.ManifestChecked || checked.ManifestStudents != 44 || checked.ManifestTeachers != 14 || len(checked.IdentifierConflicts) != 0 {
		t.Fatalf("generated manifest preview: %+v, %v", checked, err)
	}
	if _, _, err := GenerateManifest(ctx, sqlDB, "2027"); err == nil {
		t.Fatal("wrong session accepted")
	}
	altered := generated
	altered.StudentCandidates = append([]StudentCandidate(nil), generated.StudentCandidates...)
	altered.StudentCandidates[0].AdmissionNo = "TEST-TAMPERED"
	if _, err := Preview(ctx, sqlDB, &altered); err == nil {
		t.Fatal("modified manifest accepted")
	}
	if got := fingerprint(); got != before {
		t.Fatal("protected administrator rows changed during manifest generation or validation")
	}

	// Independently verify that PostgreSQL itself rejects a write through the
	// exact transaction options used by Preview.
	tx, err := beginReadOnly(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	var readOnly string
	if err := tx.QueryRowContext(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil {
		t.Fatal(err)
	}
	if readOnly != "on" {
		t.Fatalf("transaction_read_only=%s", readOnly)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET display_name='modified' WHERE lower(email)=$1", protectedAdminEmails[0]); err == nil {
		t.Fatal("read-only transaction accepted administrator update")
	}
	_ = tx.Rollback()
	if got := fingerprint(); got != before {
		t.Fatal("protected administrator changed after rejected write")
	}
	var n int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM students WHERE admission_no='TEST-001'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("student changed: %d %v", n, err)
	}
}
