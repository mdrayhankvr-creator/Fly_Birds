package promotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"gorm.io/gorm"
)

func openDB(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	return ctx, db
}

// school is a top class ("Ten") of one session with section A, its
// students, a later class, and the next session.
type school struct {
	t                     *testing.T
	db                    *gorm.DB
	stamp                 string
	admin, head           string
	from, to, ten, eleven string
	sectionA              string
}

func newSchool(t *testing.T, db *gorm.DB, year int) *school {
	t.Helper()
	s := &school{t: t, db: db, stamp: fmt.Sprintf("%d", time.Now().UnixNano())}
	s.admin = s.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'M15 admin') RETURNING id", "m15b-admin-"+s.stamp+"@example.test")
	s.head = s.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'head_teacher', 'M15 head') RETURNING id", "m15b-head-"+s.stamp+"@example.test")
	s.from = s.id("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year), fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
	s.to = s.id("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", fmt.Sprint(year+1), fmt.Sprintf("%d-01-01", year+1), fmt.Sprintf("%d-12-31", year+1))
	s.ten = s.id("INSERT INTO academic_classes(name,display_order) VALUES (?, 10) RETURNING id", "M15 Ten "+s.stamp)
	s.eleven = s.id("INSERT INTO academic_classes(name,display_order) VALUES (?, 11) RETURNING id", "M15 Eleven "+s.stamp)
	s.sectionA = s.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", s.from, s.ten)
	return s
}

func (s *school) id(q string, args ...any) string {
	s.t.Helper()
	var id string
	if err := s.db.Raw(q, args...).Row().Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

func (s *school) exec(q string, args ...any) {
	s.t.Helper()
	if err := s.db.Exec(q, args...).Error; err != nil {
		s.t.Fatal(err)
	}
}

// student admits a student with a login and one attendance day, so their
// year has a record.
func (s *school) student(n int) (id, user string) {
	user = s.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'M15 pupil') RETURNING id", fmt.Sprintf("m15b-s%d-%s@example.test", n, s.stamp))
	id = s.id("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,roll_no,status,user_id) VALUES (?,?,?,?,?,?,'active',?) RETURNING id",
		fmt.Sprintf("M15B-%d-%s", n, s.stamp), fmt.Sprintf("Pupil %d", n), s.from, s.ten, s.sectionA, n, user)
	s.exec("INSERT INTO auth_sessions(id,user_id,refresh_hash,csrf_hash,expires_at) VALUES (gen_random_uuid(),?,decode(md5(random()::text), 'hex'),decode(md5(random()::text), 'hex'),now() + interval '1 day')", user)
	s.exec("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) SELECT s.id,s.session_id,s.class_id,s.section_id,se.start_date + 30,'present','manual',?,? FROM students s JOIN academic_sessions se ON se.id = s.session_id WHERE s.id = ?",
		s.admin, s.admin, id)
	return id, user
}

func (s *school) login(user string) (active bool, liveSessions int64) {
	s.t.Helper()
	if err := s.db.Raw("SELECT is_active FROM users WHERE id = ?", user).Row().Scan(&active); err != nil {
		s.t.Fatal(err)
	}
	if err := s.db.Raw("SELECT count(*) FROM auth_sessions WHERE user_id = ? AND revoked_at IS NULL", user).Row().Scan(&liveSessions); err != nil {
		s.t.Fatal(err)
	}
	return active, liveSessions
}

// The top class: with no published annual result every student is
// blocked until someone decides with a reason. Graduates keep their login
// and read their history; withdrawal and transfer suspend the login and keep
// the history; readmission restores a login suspended that way, but not one
// an administrator disabled.
func TestPostgresPromotionGraduatesAndLeavers(t *testing.T) {
	ctx, db := openDB(t)
	tx := db.Begin()
	defer tx.Rollback()
	s := newSchool(t, tx, 2401)
	grad, gradUser := s.student(1)
	left, leftUser := s.student(2)
	moved, movedUser := s.student(3)
	disabled, disabledUser := s.student(4)
	ps := NewService(tx)
	admin, head := Actor{s.admin, "admin"}, Actor{s.head, "head_teacher"}

	b, err := ps.Create(ctx, admin, CreateInput{FromSessionID: s.from, ToSessionID: s.to, ClassID: s.ten})
	if err != nil {
		t.Fatal(err)
	}
	topWarning := false
	for _, w := range b.Warnings {
		topWarning = topWarning || w.Code == "top_class"
	}
	if len(b.Items) != 4 || b.Counts["undecided"] != 4 || !topWarning {
		t.Fatalf("batch: %d items %+v warnings %+v", len(b.Items), b.Counts, b.Warnings)
	}
	for _, it := range b.Items {
		if it.Recommendation != Blocked || it.Decision != nil || it.AnnualOutcome != nil {
			t.Fatalf("no annual result must block: %+v", it)
		}
	}
	if _, err := ps.Approve(ctx, head, b.ID, HashInput{InputsSHA256: b.InputsSHA256}); !errors.Is(err, ErrBlocked) {
		t.Fatalf("approving undecided students: %v", err)
	}
	decide := func(student, decision, reason string) {
		t.Helper()
		d, r := decision, reason
		if b, err = ps.UpdateItem(ctx, admin, b.ID, student, ItemInput{Decision: &d, Reason: &r}); err != nil {
			t.Fatalf("%s %s: %v", student, decision, err)
		}
	}
	graduate := Graduate
	if _, err := ps.UpdateItem(ctx, admin, b.ID, grad, ItemInput{Decision: &graduate}); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("a manual decision without a reason: %v", err)
	}
	decide(grad, Graduate, "Board exam passed; annual result not entered")
	decide(left, Withdraw, "Left the school")
	decide(moved, Transfer, "Transfer certificate issued")
	decide(disabled, Withdraw, "Left the school")
	s.exec("UPDATE users SET is_active = false WHERE id = ?", disabledUser) // disabled by the administrator beforehand
	if b, err = ps.Approve(ctx, head, b.ID, HashInput{InputsSHA256: b.InputsSHA256}); err != nil {
		t.Fatal(err)
	}
	if b, err = ps.Apply(ctx, admin, b.ID); err != nil {
		t.Fatal(err)
	}

	type year struct {
		State, Outcome, Status string
	}
	years := func(st string) []year {
		var out []year
		if err := tx.Raw("SELECT state, coalesce(outcome, '') AS outcome, student_status AS status FROM student_enrolments WHERE student_id = ? ORDER BY started_on", st).Scan(&out).Error; err != nil {
			t.Fatal(err)
		}
		return out
	}
	for st, want := range map[string]year{grad: {"closed", "graduated", "graduated"}, left: {"closed", "withdrawn", "inactive"}, moved: {"closed", "transferred", "transferred"}} {
		if got := years(st); len(got) != 1 || got[0] != want {
			t.Fatalf("%s: %+v, want one %+v", st, got, want)
		}
	}
	if active, live := s.login(gradUser); !active || live != 1 {
		t.Fatalf("graduate's login active=%v sessions=%d", active, live)
	}
	for _, u := range []string{leftUser, movedUser} {
		if active, live := s.login(u); active || live != 0 {
			t.Fatalf("leaver's login active=%v sessions=%d", active, live)
		}
	}
	// The graduate still reads their history (read-only: homework needs an
	// active student).
	hist, err := people.NewRepository(tx).Enrolments(ctx, people.Actor{ID: gradUser, Role: "student"}, grad)
	if err != nil || len(hist) != 1 || hist[0].Outcome == nil || *hist[0].Outcome != "graduated" {
		t.Fatalf("graduate's history: %+v %v", hist, err)
	}
	var attended int64
	if err := tx.Raw("SELECT count(*) FROM student_attendance WHERE student_id IN (?, ?, ?)", grad, left, moved).Row().Scan(&attended); err != nil || attended != 3 {
		t.Fatalf("history kept: %d %v", attended, err)
	}
	var audits int64
	if err := tx.Raw("SELECT count(*) FROM audit_events WHERE action = 'student.login_suspended' AND target_id IN (?, ?)", left, moved).Row().Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("suspension audited %d: %v", audits, err)
	}

	// Readmission into the same year restores a login the withdrawal
	// suspended, and reopens the year.
	s.exec("UPDATE students SET status = 'active' WHERE id = ?", left)
	if active, _ := s.login(leftUser); !active {
		t.Fatal("readmission did not restore the suspended login")
	}
	if got := years(left); len(got) != 1 || got[0] != (year{"current", "", "active"}) {
		t.Fatalf("readmitted year: %+v", got)
	}
	// A login the administrator disabled stays disabled.
	s.exec("UPDATE students SET status = 'active' WHERE id = ?", disabled)
	if active, _ := s.login(disabledUser); active {
		t.Fatal("readmission enabled a login the administrator had disabled")
	}
	// A transferred student readmitted into the next year gets a new year.
	sectionNext := s.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", s.to, s.ten)
	s.exec("UPDATE students SET session_id = ?, section_id = ?, status = 'active' WHERE id = ?", s.to, sectionNext, moved)
	if got := years(moved); len(got) != 2 || got[0].Outcome != "transferred" || got[1] != (year{"current", "", "active"}) {
		t.Fatalf("readmitted into the next year: %+v", got)
	}
	if active, _ := s.login(movedUser); !active {
		t.Fatal("readmission into the next year did not restore the login")
	}
	s.exec("SET CONSTRAINTS ALL IMMEDIATE")
}

// Two administrators applying the same batch at the same moment: one
// applies, the other waits and gets the applied batch; the class moves once.
func TestPostgresPromotionConcurrentApply(t *testing.T) {
	ctx, db := openDB(t)
	s := newSchool(t, db, 2411)
	// Registered first, so a failure part-way still removes what was made.
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM promotion_batches WHERE class_id IN (?, ?)",
			"DELETE FROM students WHERE class_id IN (?, ?)",
			"DELETE FROM academic_sections WHERE class_id IN (?, ?)",
			"DELETE FROM academic_classes WHERE id IN (?, ?)",
		} {
			if err := db.Exec(q, s.ten, s.eleven).Error; err != nil {
				t.Errorf("cleanup %s: %v", q, err)
			}
		}
		if err := db.Exec("DELETE FROM users WHERE email LIKE ?", "m15b-%-"+s.stamp+"@example.test").Error; err != nil {
			t.Errorf("cleanup users: %v", err)
		}
		if err := db.Exec("DELETE FROM academic_sessions WHERE id IN (?, ?)", s.from, s.to).Error; err != nil {
			t.Errorf("cleanup sessions: %v", err)
		}
	})
	s.exec("UPDATE academic_classes SET next_class_id = ? WHERE id = ?", s.eleven, s.ten)
	elevenA := s.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", s.to, s.eleven)
	ids := []string{}
	for n := 1; n <= 3; n++ {
		id, _ := s.student(n)
		ids = append(ids, id)
	}
	other := s.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'M15 admin 2') RETURNING id", "m15b-admin2-"+s.stamp+"@example.test")
	ps := NewService(db)
	admin := Actor{s.admin, "admin"}
	b, err := ps.Create(ctx, admin, CreateInput{FromSessionID: s.from, ToSessionID: s.to, ClassID: s.ten})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range ids {
		d, r := Promote, "Annual result kept on paper"
		if b, err = ps.UpdateItem(ctx, admin, b.ID, st, ItemInput{Decision: &d, Reason: &r}); err != nil {
			t.Fatal(err)
		}
	}
	if b, err = ps.Approve(ctx, Actor{s.head, "head_teacher"}, b.ID, HashInput{InputsSHA256: b.InputsSHA256}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]Batch, 2)
	errs := make([]error, 2)
	for i, actor := range []Actor{admin, {other, "admin"}} {
		wg.Add(1)
		go func(i int, a Actor) {
			defer wg.Done()
			results[i], errs[i] = ps.Apply(ctx, a, b.ID)
		}(i, actor)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil || results[i].Status != "applied" {
			t.Fatalf("apply %d: %s %v", i, results[i].Status, errs[i])
		}
	}
	var n int64
	if err := db.Raw("SELECT count(*) FROM audit_events WHERE action = 'promotion.batch_applied' AND target_id = ?", b.ID).Row().Scan(&n); err != nil || n != 1 {
		t.Fatalf("applied %d times: %v", n, err)
	}
	if err := db.Raw("SELECT count(*) FROM student_enrolments WHERE student_id IN ? AND session_id = ? AND section_id = ? AND state = 'current'", ids, s.to, elevenA).Row().Scan(&n); err != nil || n != 3 {
		t.Fatalf("students in the new year: %d %v", n, err)
	}
	if err := db.Raw("SELECT count(*) FROM student_enrolments WHERE student_id IN ?", ids).Row().Scan(&n); err != nil || n != 6 {
		t.Fatalf("enrolments: %d %v (two years each)", n, err)
	}
}
