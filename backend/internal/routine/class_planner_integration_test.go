package routine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

var plannerDisposableName = regexp.MustCompile(`^flybirds_routine_test_[0-9a-f]{12}$`)

func validPlannerTestDSN(dsn string) bool {
	u, err := url.Parse(dsn)
	return err == nil && u.Scheme == "postgresql" && u.Hostname() == "127.0.0.1" && u.Port() == "55415" && plannerDisposableName.MatchString(strings.TrimPrefix(u.Path, "/"))
}

func TestClassPlannerTestDatabaseGuard(t *testing.T) {
	for _, tc := range []struct {
		dsn  string
		want bool
	}{
		{"postgresql://test@127.0.0.1:55415/flybirds_routine_test_abcdef123456", true},
		{"postgresql://test@127.0.0.1:55415/fbshow", false},
		{"postgresql://test@127.0.0.1:55415/fbqa_cp1b", false},
		{"postgresql://test@127.0.0.1:55415/flybirds_routine_test_fbshow", false},
		{"postgresql://test@school.example:55415/flybirds_routine_test_abcdef123456", false},
	} {
		if got := validPlannerTestDSN(tc.dsn); got != tc.want {
			t.Fatalf("guard accepted=%t for a database that should be accepted=%t", got, tc.want)
		}
	}
}

func TestPostgresClassPlannerPublicationSafety(t *testing.T) {
	dsn := os.Getenv("CLASS_ROUTINE_PLANNER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated disposable planner database not supplied")
	}
	if !validPlannerTestDSN(dsn) {
		t.Fatal("refusing test: use a fresh local flybirds_routine_test_ database on m15-testdb")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal("open disposable database:", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var objects int
	const nonSystemObjects = `SELECT
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
    AND c.relkind IN ('r','p','f','v','m','S'))
 +(SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%')
 +(SELECT count(*) FROM pg_namespace
  WHERE nspname NOT IN ('pg_catalog','information_schema','public') AND nspname NOT LIKE 'pg_toast%')`
	if err := db.Raw(nonSystemObjects).Row().Scan(&objects); err != nil || objects != 0 {
		t.Fatal("planner test database must be empty and disposable")
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(nonSystemObjects).Row().Scan(&objects); err != nil || objects == 0 {
		t.Fatal("nonempty migrated database was not detected")
	}
	tx := db.Begin()
	defer tx.Rollback()
	insert := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(q, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := fmt.Sprint(time.Now().UnixNano())
	admin := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,'unused','admin','Planner admin') RETURNING id", "routine-"+stamp+"@example.test")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES (?,'2280-01-01','2280-12-31',true) RETURNING id", "2280")
	class := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Planner "+stamp)
	sectionA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, class)
	sectionB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, class)
	subject := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", "Math "+stamp, "M"+stamp)
	missing := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Bangla',?) RETURNING id", "Unassigned "+stamp, "U"+stamp)
	for _, sub := range []string{subject, missing} {
		if err := tx.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", class, sub).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher := insert("INSERT INTO employees(employee_code,kind,name_en,designation,joined_on) VALUES (?,'teacher','Planner teacher','Teacher','2279-01-01') RETURNING id", "T"+stamp)
	teacher2 := insert("INSERT INTO employees(employee_code,kind,name_en,designation,joined_on) VALUES (?,'teacher','Second teacher','Teacher','2279-01-01') RETURNING id", "V"+stamp)
	for _, id := range []string{teacher, teacher2} {
		if err := tx.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", id, session, class, subject).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec("INSERT INTO exam_rooms(name,kind) VALUES ('Room A','classroom'),('Room B','classroom')").Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(tx))
	old := ClassInput{SessionID: session, ClassID: class, SectionID: sectionA, SubjectID: subject, TeacherID: teacher, DayOfWeek: 0, StartTime: "09:00", EndTime: "09:45", Room: "Room A"}
	if _, err := svc.SaveClass(ctx, admin, "", old); err != nil {
		t.Fatal(err)
	}
	plan := ClassPlan{SessionID: session, Config: ClassPlanConfig{StartTime: "09:00", EndTime: "12:00", LunchStart: "10:30", LunchMinutes: 45, PeriodMinutes: 45, Days: []int{0}, AvailabilityConfirmed: true, Frequencies: []TeachingFrequency{{ClassID: class, SubjectID: subject, PeriodsPerWeek: 1}, {ClassID: class, SubjectID: missing, PeriodsPerWeek: 1}}}, Slots: []ClassInput{{SessionID: session, ClassID: class, SectionID: sectionA, SubjectID: subject, TeacherID: teacher, DayOfWeek: 0, StartTime: "09:00", EndTime: "09:45", Room: "Room A"}, {SessionID: session, ClassID: class, SectionID: sectionB, SubjectID: subject, TeacherID: teacher2, DayOfWeek: 0, StartTime: "09:45", EndTime: "10:30", Room: "Room A"}}}
	preview, err := svc.ClassPlanner.Preview(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanPublish || !containsPlannerIssue(preview.Issues, "teacher_missing") {
		t.Fatalf("missing teacher was not reported: %+v", preview.Issues)
	}
	plan.Config.Frequencies[1].PeriodsPerWeek = 0
	plan.Config.Unavailable = []TeacherUnavailable{{TeacherID: teacher, DayOfWeek: 0, StartTime: "09:00", EndTime: "09:45"}}
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlannerIssue(preview.Issues, "teacher_unavailable") {
		t.Fatal("teacher unavailable time ignored")
	}
	plan.Config.Unavailable = nil
	plan.Slots[1].StartTime, plan.Slots[1].EndTime = "09:00", "09:45"
	plan.Slots[1].Room = " room a "
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlannerIssue(preview.Issues, "overlap") {
		t.Fatal("room overlap ignored")
	}
	plan.Slots[1].Room, plan.Slots[1].TeacherID = "Room B", teacher
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "overlap") {
		t.Fatalf("teacher overlap ignored: %+v %v", preview.Issues, err)
	}
	plan.Slots[1].SectionID, plan.Slots[1].TeacherID = sectionA, teacher2
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "overlap") {
		t.Fatalf("section overlap ignored: %+v %v", preview.Issues, err)
	}
	plan.Slots[1].SectionID, plan.Slots[1].Room = sectionB, "Room A"
	plan.Slots[1].StartTime, plan.Slots[1].EndTime = "09:45", "10:30"
	plan.Slots[1].DayOfWeek = 5
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlannerIssue(preview.Issues, "outside_period") {
		t.Fatal("weekly off day ignored")
	}
	plan.Slots[1].DayOfWeek = 0
	plan.Slots[1].StartTime, plan.Slots[1].EndTime = "10:30", "11:15"
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlannerIssue(preview.Issues, "outside_period") {
		t.Fatal("lunch lesson accepted")
	}
	plan.Slots[1].StartTime, plan.Slots[1].EndTime = "09:45", "10:30"
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil || !preview.CanPublish || len(preview.Existing) != 1 {
		t.Fatalf("valid preview: %+v %v", preview.Issues, err)
	}
	if _, err := svc.ClassPlanner.SaveDraft(ctx, admin, plan, "0000"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("save without preview digest: %v", err)
	}
	draft, err := svc.ClassPlanner.SaveDraft(ctx, admin, plan, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ClassPlanner.Publish(ctx, admin, session, draft.ID, preview.Digest, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("publish without confirmation: %v", err)
	}
	if _, err := svc.ClassPlanner.Publish(ctx, admin, session, draft.ID, strings.Repeat("0", 64), true); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale preview accepted: %v", err)
	}
	if _, err := svc.ClassPlanner.Publish(ctx, admin, session, draft.ID, preview.Digest, true); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.Raw("SELECT count(*) FROM class_routines WHERE session_id=?", session).Row().Scan(&count); err != nil || count != 2 {
		t.Fatalf("published lesson count %d %v", count, err)
	}
	if err := tx.Raw("SELECT count(*) FROM class_routine_plan_versions WHERE session_id=? AND state='legacy' AND version=0", session).Row().Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy version missing %d %v", count, err)
	}
	if _, err := svc.SaveClass(ctx, admin, "", old); err == nil {
		t.Fatal("direct mutation of published class routine accepted")
	}
	plan.Slots[0].StartTime, plan.Slots[0].EndTime = "11:15", "12:00"
	preview, err = svc.ClassPlanner.Preview(ctx, plan)
	if err != nil || !preview.CanPublish {
		t.Fatalf("replacement preview: %+v %v", preview.Issues, err)
	}
	draft2, err := svc.ClassPlanner.SaveDraft(ctx, admin, plan, preview.Digest)
	if err != nil || draft2.Version != 2 {
		t.Fatalf("replacement draft: %+v %v", draft2, err)
	}
	if _, err := svc.ClassPlanner.Publish(ctx, admin, session, draft2.ID, preview.Digest, true); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT count(*) FROM class_routine_plan_versions WHERE session_id=? AND state='published'", session).Row().Scan(&count); err != nil || count != 2 {
		t.Fatalf("published history %d %v", count, err)
	}
}

func containsPlannerIssue(issues []PlanIssue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
