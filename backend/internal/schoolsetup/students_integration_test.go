package schoolsetup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

var disposableStudentSetupDB = regexp.MustCompile(`^student_setup_test_[0-9a-f]{10,32}$`)

func TestPostgresStudentSetup(t *testing.T) {
	dsn := os.Getenv("STUDENT_SETUP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("STUDENT_SETUP_TEST_DATABASE_URL is unset")
	}
	config, err := pgconn.ParseConfig(dsn)
	if err != nil || !disposableStudentSetupDB.MatchString(config.Database) || (config.Host != "127.0.0.1" && config.Host != "localhost") {
		t.Fatal("refusing any database except a fresh local student_setup_test_ database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	insertID := func(query string, args ...any) string {
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
	admin := insertID("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ($1,'protected-hash','admin','Protected',true) RETURNING id::text", protectedAdminEmails[0])
	insertID("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ($1,'protected-hash','admin','Protected',true) RETURNING id::text", protectedAdminEmails[1])
	teacher := insertID("INSERT INTO users(email,password_hash,role,display_name,is_active) VALUES ('teacher@student-setup.test','teacher-hash','teacher','Teacher',true) RETURNING id::text")
	session := insertID("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2026','2026-01-01','2026-12-31',true) RETURNING id::text")
	class := insertID("INSERT INTO academic_classes(name,display_order) VALUES ('SIX-06',1) RETURNING id::text")
	sectionA := insertID("INSERT INTO academic_sections(session_id,class_id,name,code,display_order) VALUES ($1,$2,'A','A',1) RETURNING id::text", session, class)
	sectionB := insertID("INSERT INTO academic_sections(session_id,class_id,name,code,display_order) VALUES ($1,$2,'B','B',2) RETURNING id::text", session, class)
	for i := 1; i <= 13; i++ {
		insertID("INSERT INTO students(admission_no,name_en,status,session_id,class_id,section_id,roll_no) VALUES ($1,'Existing Student','active',$2,$3,$4,$5) RETURNING id::text", fmt.Sprintf("EXIST-A-%02d", i), session, class, sectionA, i)
	}
	for i := 1; i <= 15; i++ {
		insertID("INSERT INTO students(admission_no,name_en,status,session_id,class_id,section_id,roll_no) VALUES ($1,'Existing Student','active',$2,$3,$4,$5) RETURNING id::text", fmt.Sprintf("EXIST-B-%02d", i), session, class, sectionB, i)
	}
	if _, err := PreviewStudents(ctx, db, teacher, session); !errors.Is(err, ErrStudentSetupForbidden) {
		t.Fatalf("teacher preview authorization: %v", err)
	}
	grantTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := grantTx.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id='admin' AND permission_id='users:create'"); err != nil {
		t.Fatal(err)
	}
	if err := requireStudentSetupAdmin(ctx, grantTx, admin); !errors.Is(err, ErrStudentSetupForbidden) {
		t.Fatalf("revoked grant accepted: %v", err)
	}
	_ = grantTx.Rollback()
	plan, err := PreviewStudents(ctx, db, admin, session)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Existing != 28 || plan.ToAdd != 2 || len(plan.Students) != 2 || plan.Sections[0].ToAdd != 2 || plan.Sections[1].ToAdd != 0 || plan.Students[0].RollNo != 14 || plan.Students[1].RollNo != 15 {
		t.Fatalf("15-minus-existing or roll allocation: %+v", plan)
	}
	for _, student := range plan.Students {
		if !strings.HasPrefix(student.NameEN, "TEST STUDENT TEST-") || student.LoginID != student.AdmissionNo || student.TestMarker != "TEST" {
			t.Fatalf("synthetic identity or login: %+v", student)
		}
	}
	collisionTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collisionTx.ExecContext(ctx, "INSERT INTO users(email,password_hash,role,login_id) VALUES ('collision@example.test','fixture-hash','student',$1)", plan.Students[0].LoginID); err != nil {
		t.Fatal(err)
	}
	if _, err := studentPlanTx(ctx, collisionTx, session); err == nil {
		t.Fatal("login-ID collision was not detected")
	} else {
		var conflict *StudentSetupConflictError
		if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].Kind != "login_id" {
			t.Fatalf("collision details: %v", err)
		}
	}
	_ = collisionTx.Rollback()
	key := "11111111-1111-4111-8111-111111111111"
	request := StudentApplyRequest{SessionID: session, Fingerprint: plan.Fingerprint, IdempotencyKey: key, Confirm: true}
	if _, err := ApplyStudents(ctx, db, teacher, request); !errors.Is(err, ErrStudentSetupForbidden) {
		t.Fatalf("teacher apply authorization: %v", err)
	}
	if _, err := ApplyStudents(ctx, db, admin, StudentApplyRequest{SessionID: session, Fingerprint: plan.Fingerprint, IdempotencyKey: key}); !errors.Is(err, ErrStudentSetupInvalid) {
		t.Fatalf("missing explicit confirmation: %v", err)
	}
	exec("UPDATE academic_sections SET name='A changed' WHERE id=$1", sectionA)
	if _, err := ApplyStudents(ctx, db, admin, request); !errors.Is(err, ErrStudentSetupStale) {
		t.Fatalf("stale approval: %v", err)
	}
	exec("UPDATE academic_sections SET name='A' WHERE id=$1", sectionA)
	refreshed, err := PreviewStudents(ctx, db, admin, session)
	if err != nil || refreshed.Fingerprint != plan.Fingerprint {
		t.Fatalf("stable preview after restoration: %v", err)
	}
	// Reject the second account after the first student and account insert.
	// The complete transaction must roll back students, users and enrolments.
	exec(`CREATE FUNCTION reject_second_setup_account() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
IF NEW.role='student' AND (SELECT count(*) FROM users WHERE role='student') > 0 THEN RAISE EXCEPTION 'fixture rollback'; END IF;
RETURN NEW; END; $$`)
	exec("CREATE TRIGGER reject_second_setup_account BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION reject_second_setup_account()")
	if _, err := ApplyStudents(ctx, db, admin, request); err == nil {
		t.Fatal("forced second-account failure did not roll back")
	}
	exec("DROP TRIGGER reject_second_setup_account ON users")
	exec("DROP FUNCTION reject_second_setup_account()")
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM students WHERE admission_no LIKE 'TEST-%'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial students remained: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='student'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan student account remained: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM student_setup_runs").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed run was recorded: %d %v", count, err)
	}
	result, err := ApplyStudents(ctx, db, admin, request)
	if err != nil || result.Admitted != 2 || result.Replayed || len(result.Logins) != 2 {
		t.Fatalf("apply: admitted=%d replayed=%t slips=%d err=%v", result.Admitted, result.Replayed, len(result.Logins), err)
	}
	seenPasswords := map[string]bool{}
	for _, login := range result.Logins {
		if seenPasswords[login.InitialPassword] || login.InitialPassword == "" || login.LoginID != login.AdmissionNo || !strings.HasPrefix(login.StudentName, "TEST STUDENT") {
			t.Fatal("duplicate or incorrect one-time credential")
		}
		seenPasswords[login.InitialPassword] = true
		var hash string
		var forced bool
		var expiry time.Time
		var enrolments int
		if err := db.QueryRowContext(ctx, "SELECT password_hash,must_change_password,temporary_password_expires_at FROM users WHERE id=$1", login.UserID).Scan(&hash, &forced, &expiry); err != nil || !forced || expiry.Before(time.Now()) || bcrypt.CompareHashAndPassword([]byte(hash), []byte(login.InitialPassword)) != nil {
			t.Fatal("temporary credential hash or forced first change is incorrect")
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM student_enrolments e JOIN students s ON s.id=e.student_id
WHERE s.id=$1 AND s.user_id=$2 AND e.session_id=$3 AND e.class_id=$4 AND e.section_id=$5 AND e.state='current' AND e.roll_no=s.roll_no`, login.StudentID, login.UserID, session, class, sectionA).Scan(&enrolments); err != nil || enrolments != 1 {
			t.Fatal("student login and current enrolment are inconsistent")
		}
		var leaked int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE metadata::text LIKE '%' || $1 || '%'", login.InitialPassword).Scan(&leaked); err != nil || leaked != 0 {
			t.Fatal("audit contains a temporary password")
		}
	}
	retry, err := ApplyStudents(ctx, db, admin, request)
	if err != nil || !retry.Replayed || retry.Admitted != 2 || len(retry.Logins) != 0 {
		t.Fatalf("idempotent replay: %+v %v", retry, err)
	}
	request.Fingerprint = strings.Repeat("0", 64)
	if _, err := ApplyStudents(ctx, db, admin, request); !errors.Is(err, ErrStudentSetupKeyReused) {
		t.Fatalf("key reuse: %v", err)
	}
	for _, protected := range protectedAdminEmails {
		var role, hash string
		if err := db.QueryRowContext(ctx, "SELECT role,password_hash FROM users WHERE email=$1", protected).Scan(&role, &hash); err != nil || role != "admin" || hash != "protected-hash" {
			t.Fatal("protected administrator changed")
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM students WHERE section_id=$1", sectionB).Scan(&count); err != nil || count != 15 {
		t.Fatal("full section was changed")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM students WHERE section_id=$1", sectionA).Scan(&count); err != nil || count != 15 {
		t.Fatal("section did not reach exactly 15")
	}
	if _, err := PreviewStudents(ctx, db, admin, session); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("post-apply preview failed:", err)
	}
}
