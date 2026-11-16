package schoolsetup

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

// The harness must supply a newly created, disposable database with this
// prefix. The test refuses any other database before it runs migrations.
func TestPostgresPreviewSchoolSetup(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	config, err := pgconn.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "school_setup_preview_test_") || (config.Host != "127.0.0.1" && config.Host != "localhost") {
		t.Fatal("refusing test: use a fresh local school_setup_preview_test_ database")
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
	var count int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("refusing migrations: disposable test database is not empty")
	}
	if err := database.Migrate(ctx, gormDB); err != nil {
		t.Fatal(err)
	}
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
