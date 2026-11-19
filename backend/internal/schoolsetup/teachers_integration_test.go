package schoolsetup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

var disposableTeacherDB = regexp.MustCompile(`^teacher_setup_test_[0-9a-f]{10,32}$`)

func validateTeacherTestDSN(dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil || !disposableTeacherDB.MatchString(cfg.Database) || (cfg.Host != "127.0.0.1" && cfg.Host != "localhost") {
		return fmt.Errorf("use a fresh local teacher_setup_test_ database")
	}
	return nil
}

func TestTeacherSetupDatabaseGuard(t *testing.T) {
	for _, dsn := range []string{
		"postgres://fixture@localhost/fbshow?sslmode=disable",
		"postgres://fixture@localhost/fbqa_cp1b?sslmode=disable",
		"postgres://fixture@localhost/flybirds_ci?sslmode=disable",
		"postgres://fixture@production.example/teacher_setup_test_1234567890?sslmode=disable",
		"postgres://fixture@localhost/teacher_setup_test_fbshow?sslmode=disable",
	} {
		if validateTeacherTestDSN(dsn) == nil {
			t.Fatalf("unsafe DSN accepted: %s", dsn)
		}
	}
	if err := validateTeacherTestDSN("postgres://fixture@127.0.0.1/teacher_setup_test_1234567890?sslmode=disable"); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTeacherSetup(t *testing.T) {
	dsn := os.Getenv("TEACHER_SETUP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEACHER_SETUP_TEST_DATABASE_URL is unset")
	}
	if err := validateTeacherTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	gormDB, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := requireEmptyPreviewTestDatabase(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, gormDB); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyPreviewTestDatabase(ctx, db); err == nil {
		t.Fatal("nonempty migrated database accepted as a fresh fixture")
	}
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var value int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	exec("UPDATE school_modules SET enabled=true WHERE module_key='staff'")
	admin := insert("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ($1,'protected-hash','admin','Protected',true) RETURNING id::text", protectedAdminEmails[0])
	insert("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ($1,'protected-hash','admin','Protected',true) RETURNING id::text", protectedAdminEmails[1])
	teacher := insert("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ('existing.teacher@example.test','teacher-hash','teacher','Teacher',true) RETURNING id::text")
	other := insert("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ('existing.other@example.test','teacher-hash','teacher','Teacher',true) RETURNING id::text")
	exec("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES ('TEACHER-01',$1,'teacher','Existing One','Teacher','2020-01-01'),('TEACHER-02',$2,'teacher','Existing Two','Teacher','2020-01-01')", teacher, other)
	if _, err := PreviewTeachers(ctx, db, teacher); !errors.Is(err, ErrTeacherSetupForbidden) {
		t.Fatalf("teacher preview allowed: %v", err)
	}
	plan, err := PreviewTeachers(ctx, db, admin)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Existing != 2 || plan.ToAdd != 13 || len(plan.Candidates) != 13 || count("SELECT count(*) FROM users") != 4 {
		t.Fatalf("bad read-only plan: %+v", plan)
	}
	exec("UPDATE users SET role='teacher' WHERE email=$1", protectedAdminEmails[1])
	if _, err := PreviewTeachers(ctx, db, admin); !errors.Is(err, ErrTeacherSetupConflict) {
		t.Fatalf("changed protected Admin accepted: %v", err)
	}
	exec("UPDATE users SET role='admin' WHERE email=$1", protectedAdminEmails[1])
	plan, err = PreviewTeachers(ctx, db, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyTeachers(ctx, db, teacher, TeacherApplyRequest{plan.Fingerprint, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", true}); !errors.Is(err, ErrTeacherSetupForbidden) {
		t.Fatalf("teacher apply allowed: %v", err)
	}
	if _, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{plan.Fingerprint, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", false}); !errors.Is(err, ErrTeacherSetupInvalid) {
		t.Fatalf("unconfirmed apply allowed: %v", err)
	}
	if _, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{strings.Repeat("0", 64), "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", true}); !errors.Is(err, ErrTeacherSetupStale) {
		t.Fatalf("stale plan accepted: %v", err)
	}
	// A colliding email and employee code must stop preview without writes.
	exec("INSERT INTO users(email,password_hash,role,is_active) VALUES ('test.teacher-03@flybirds.test','hash','student',true)")
	if _, err := PreviewTeachers(ctx, db, admin); !errors.Is(err, ErrTeacherSetupConflict) {
		t.Fatalf("email collision accepted: %v", err)
	}
	exec("DELETE FROM users WHERE email='test.teacher-03@flybirds.test'")
	exec("INSERT INTO employees(employee_code,kind,name_en,designation,joined_on) VALUES ('TEACHER-03','non_teaching','Collision','Staff','2020-01-01')")
	if _, err := PreviewTeachers(ctx, db, admin); !errors.Is(err, ErrTeacherSetupConflict) {
		t.Fatalf("employee code collision accepted: %v", err)
	}
	exec("DELETE FROM employees WHERE employee_code='TEACHER-03'")
	// A failure after two inserts rolls every account, staff row and audit out.
	exec(`CREATE FUNCTION fail_teacher_setup_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.employee_code='TEACHER-05' THEN RAISE EXCEPTION 'fixture failure'; END IF; RETURN NEW; END $$`)
	exec("CREATE TRIGGER fail_teacher_setup_test BEFORE INSERT ON employees FOR EACH ROW EXECUTE FUNCTION fail_teacher_setup_test()")
	key := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{plan.Fingerprint, key, true}); err == nil {
		t.Fatal("partial failure was accepted")
	}
	if count("SELECT count(*) FROM employees WHERE kind='teacher'") != 2 || count("SELECT count(*) FROM users WHERE role='teacher'") != 2 || count("SELECT count(*) FROM synthetic_teacher_setup_runs") != 0 {
		t.Fatal("partial apply was not rolled back")
	}
	exec("DROP TRIGGER fail_teacher_setup_test ON employees")
	exec("DROP FUNCTION fail_teacher_setup_test()")
	// A stale preview is rejected after a concurrent staff change.
	exec("INSERT INTO employees(employee_code,kind,name_en,designation,joined_on) VALUES ('EXTRA-TEMP','non_teaching','Fixture','Staff','2020-01-01')")
	if _, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{plan.Fingerprint, key, true}); !errors.Is(err, ErrTeacherSetupStale) {
		t.Fatalf("changed staff accepted: %v", err)
	}
	exec("DELETE FROM employees WHERE employee_code='EXTRA-TEMP'")
	result, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{plan.Fingerprint, key, true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 13 || result.Replayed || len(result.Credentials) != 13 || count("SELECT count(*) FROM employees WHERE kind='teacher' AND is_active") != 15 {
		t.Fatalf("bad creation counts: created=%d replayed=%t credentials=%d", result.Created, result.Replayed, len(result.Credentials))
	}
	if count("SELECT count(*) FROM employees WHERE synthetic_test AND joined_on IS NULL AND email='' AND phone='' AND qualifications='[]'::jsonb") != 13 {
		t.Fatal("TEST profiles have invented details")
	}
	if count("SELECT count(*) FROM employees e JOIN users u ON u.id=e.user_id WHERE e.employee_code IN ('TEACHER-01','TEACHER-02') AND e.kind='teacher' AND e.is_active AND NOT e.synthetic_test AND u.role='teacher' AND u.is_active AND u.password_hash='teacher-hash'") != 2 {
		t.Fatal("existing teacher profiles or linked accounts changed")
	}
	if count("SELECT count(*) FROM teacher_assignments") != 0 {
		t.Fatal("subjects were assigned without approval")
	}
	seen := map[string]bool{}
	for _, credential := range result.Credentials {
		if seen[credential.InitialPassword] || !strings.HasPrefix(credential.NameEN, "TEST ") || credential.Role != "teacher" {
			t.Fatal("duplicate or incorrect credential")
		}
		seen[credential.InitialPassword] = true
		var hash string
		var change bool
		var expiry time.Time
		if err := db.QueryRowContext(ctx, "SELECT password_hash,must_change_password,temporary_password_expires_at FROM users WHERE email=$1", credential.LoginID).Scan(&hash, &change, &expiry); err != nil {
			t.Fatal(err)
		}
		if hash == credential.InitialPassword || !auth.VerifyPassword(hash, credential.InitialPassword) || !change || expiry.Before(time.Now()) {
			t.Fatal("temporary password safety failed")
		}
	}
	var adminCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE email IN ($1,$2) AND role='admin' AND is_active AND password_hash='protected-hash'", protectedAdminEmails[0], protectedAdminEmails[1]).Scan(&adminCount); err != nil || adminCount != 2 {
		t.Fatal("protected Admin account changed", err)
	}
	retry, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{plan.Fingerprint, key, true})
	if err != nil || !retry.Replayed || retry.Created != 13 || len(retry.Credentials) != 0 || count("SELECT count(*) FROM employees WHERE kind='teacher'") != 15 {
		t.Fatalf("retry safety failed: %+v %v", retry, err)
	}
	if _, err := ApplyTeachers(ctx, db, admin, TeacherApplyRequest{strings.Repeat("1", 64), key, true}); !errors.Is(err, ErrTeacherSetupKeyReused) {
		t.Fatalf("key reused for other preview: %v", err)
	}
}
