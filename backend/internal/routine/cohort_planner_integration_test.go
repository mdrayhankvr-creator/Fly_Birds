package routine

import (
	"context"
	"database/sql"
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

var cohortDisposableName = regexp.MustCompile(`^flybirds_cohort_test_[0-9a-f]{12}$`)

func validCohortTestDSN(dsn string) bool {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !cohortDisposableName.MatchString(strings.TrimPrefix(u.Path, "/")) {
		return false
	}
	return (u.Hostname() == "127.0.0.1" && u.Port() == "55415") ||
		(u.Hostname() == "localhost" && u.Port() == "5432" && os.Getenv("GITHUB_ACTIONS") == "true")
}

func TestCohortTestDatabaseGuard(t *testing.T) {
	for _, tc := range []struct {
		dsn  string
		want bool
	}{
		{"postgres://test@127.0.0.1:55415/flybirds_cohort_test_abcdef123456", true},
		{"postgres://test@127.0.0.1:55415/fbshow", false},
		{"postgres://test@127.0.0.1:55415/fbqa_cp1b", false},
		{"postgres://test@127.0.0.1:55415/flybirds_ci", false},
		{"postgres://test@school.example:55415/flybirds_cohort_test_abcdef123456", false},
		{"postgres://test@127.0.0.1:5432/flybirds_cohort_test_abcdef123456", false},
	} {
		if got := validCohortTestDSN(tc.dsn); got != tc.want {
			t.Fatalf("cohort test guard accepted=%t, want %t", got, tc.want)
		}
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	if !validCohortTestDSN("postgres://test@localhost:5432/flybirds_cohort_test_abcdef123456") {
		t.Fatal("CI service rejected for dedicated disposable cohort database")
	}
	if validCohortTestDSN("postgres://test@localhost:5432/flybirds_ci") {
		t.Fatal("CI shared database accepted as a cohort fixture")
	}
}

const cohortNonSystemObjects = `SELECT
 (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
    AND c.relkind IN ('r','p','f','v','m','S'))
 +(SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%')
 +(SELECT count(*) FROM pg_namespace
  WHERE nspname NOT IN ('pg_catalog','information_schema','public') AND nspname NOT LIKE 'pg_toast%')`

func requireEmptyCohortTestDatabase(ctx context.Context, db *sql.DB) error {
	var objects int
	if err := db.QueryRowContext(ctx, cohortNonSystemObjects).Scan(&objects); err != nil {
		return err
	}
	if objects != 0 {
		return errors.New("refusing migrations: disposable cohort test database is not empty")
	}
	return nil
}

func TestPostgresCombinedCohortPlanning(t *testing.T) {
	dsn := os.Getenv("CLASS_COHORT_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("AUTH_TEST_DATABASE_URL") != "" {
			t.Fatal("CLASS_COHORT_TEST_DATABASE_URL is required; shared AUTH_TEST_DATABASE_URL is unsafe for this fixture")
		}
		t.Skip("CLASS_COHORT_TEST_DATABASE_URL is unset")
	}
	if !validCohortTestDSN(dsn) {
		t.Fatal("refusing non-disposable cohort test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := requireEmptyCohortTestDatabase(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyCohortTestDatabase(ctx, sqlDB); err == nil {
		t.Fatal("nonempty cohort test database accepted after migrations")
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
	admin := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,'unused','admin','Cohort admin') RETURNING id", "cohort-"+stamp+"@example.test")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2473','2473-01-01','2473-12-31',true) RETURNING id")
	class := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Cohort "+stamp)
	sections := []string{}
	for _, code := range []string{"A", "B", "C"} {
		sections = append(sections, insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,?,?) RETURNING id", session, class, code, code))
	}
	subjects, teachers, teacherUsers, rooms := []string{}, []string{}, []string{}, []string{}
	for i, code := range []string{"ISL", "HIN", "BUD"} {
		sub := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'Course',?) RETURNING id", code+" Education "+stamp, code+stamp)
		if err := tx.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", class, sub).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("INSERT INTO class_religion_subjects(class_id,subject_id,created_by) VALUES (?,?,?)", class, sub, admin).Error; err != nil {
			t.Fatal(err)
		}
		teacherUser := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,'unused','teacher',?) RETURNING id", code+stamp+"@example.test", "Teacher "+code)
		teacher := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher',?,'Teacher','2472-01-01') RETURNING id", code+stamp, teacherUser, "Teacher "+code)
		if err := tx.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", teacher, session, class, sub).Error; err != nil {
			t.Fatal(err)
		}
		room := fmt.Sprintf("Cohort %s %d", stamp, i)
		if err := tx.Exec("INSERT INTO exam_rooms(name,capacity) VALUES (?,3)", room).Error; err != nil {
			t.Fatal(err)
		}
		subjects = append(subjects, sub)
		teachers = append(teachers, teacher)
		teacherUsers = append(teacherUsers, teacherUser)
		rooms = append(rooms, room)
	}
	groups := [][]string{{}, {}, {}}
	for _, section := range sections {
		for i := 0; i < 3; i++ {
			student := insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", fmt.Sprintf("C-%s-%s-%d", stamp, section, i), "Synthetic", session, class, section)
			// Section C has no student choosing Hindu Education. A class-wide
			// frequency must not demand a lesson there for this course.
			if section == sections[2] && i == 1 {
				continue
			}
			if err := tx.Exec("INSERT INTO student_subject_choices(student_id,session_id,role,subject_id,updated_by) VALUES (?,?,'religion',?,?)", student, session, subjects[i], admin).Error; err != nil {
				t.Fatal(err)
			}
			groups[i] = append(groups[i], student)
		}
	}
	plan := ClassPlan{SessionID: session, Config: ClassPlanConfig{StartTime: "09:00", EndTime: "12:00", LunchStart: "10:30", LunchMinutes: 45, PeriodMinutes: 45, Days: []int{0}, AvailabilityConfirmed: true}, Slots: []ClassInput{}, CombinedSlots: []CombinedInput{}}
	for i := range subjects {
		plan.Config.Frequencies = append(plan.Config.Frequencies, TeachingFrequency{ClassID: class, SubjectID: subjects[i], PeriodsPerWeek: 1})
		plan.CombinedSlots = append(plan.CombinedSlots, CombinedInput{ClassID: class, SubjectID: subjects[i], TeacherID: teachers[i], DayOfWeek: 0, StartTime: "09:00", EndTime: "09:45", Room: rooms[i], StudentIDs: groups[i]})
	}
	svc := NewService(NewRepository(tx)).ClassPlanner
	preview, err := svc.Preview(ctx, plan)
	if err != nil || !preview.CanPublish {
		t.Fatalf("three parallel cohorts rejected: %+v %v", preview.Issues, err)
	}
	plan.CombinedSlots[1].TeacherID = teachers[0]
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "overlap_cohort") {
		t.Fatalf("teacher conflict missed: %+v %v", preview.Issues, err)
	}
	plan.CombinedSlots[1].TeacherID = teachers[1]
	plan.CombinedSlots[1].Room = rooms[0]
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "overlap_cohort") {
		t.Fatalf("room conflict missed: %+v %v", preview.Issues, err)
	}
	plan.CombinedSlots[1].Room = rooms[1]
	originalMember := plan.CombinedSlots[1].StudentIDs[0]
	plan.CombinedSlots[1].StudentIDs[0] = groups[0][0]
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "overlap_cohort") {
		t.Fatalf("student conflict missed: %+v %v", preview.Issues, err)
	}
	plan.CombinedSlots[1].StudentIDs[0] = originalMember
	plan.CombinedSlots[1].StartTime, plan.CombinedSlots[1].EndTime = "10:30", "11:15"
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "outside_period") {
		t.Fatalf("lunch conflict missed: %+v %v", preview.Issues, err)
	}
	plan.CombinedSlots[1].StartTime, plan.CombinedSlots[1].EndTime = "09:00", "09:45"
	if err := tx.Exec("UPDATE exam_rooms SET capacity=1 WHERE name=?", rooms[1]).Error; err != nil {
		t.Fatal(err)
	}
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !containsPlannerIssue(preview.Issues, "room_capacity") {
		t.Fatalf("room capacity missed: %+v %v", preview.Issues, err)
	}
	if err := tx.Exec("UPDATE exam_rooms SET capacity=3 WHERE name=?", rooms[1]).Error; err != nil {
		t.Fatal(err)
	}
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !preview.CanPublish {
		t.Fatalf("valid preview after corrections: %+v %v", preview.Issues, err)
	}
	draft, err := svc.SaveDraft(ctx, admin, plan, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, admin, session, draft.ID, preview.Digest, true); err != nil {
		t.Fatal(err)
	}
	var lessonCount, memberCount int64
	if err := tx.Raw("SELECT count(*) FROM class_cohort_lessons WHERE session_id=? AND is_current", session).Row().Scan(&lessonCount); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT count(*) FROM class_cohort_members m JOIN class_cohort_lessons l ON l.id=m.lesson_id WHERE l.session_id=?", session).Row().Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if lessonCount != 3 || memberCount != 8 {
		t.Fatalf("published rows: %d lessons, %d members", lessonCount, memberCount)
	}
	lessons, err := svc.CohortLessons(ctx, teacherUsers[0], "teacher", session)
	if err != nil || len(lessons) != 1 || lessons[0].MemberCount != 3 {
		t.Fatalf("teacher sees wrong lessons: %+v %v", lessons, err)
	}
	activeLessons, err := svc.CohortLessons(ctx, teacherUsers[0], "teacher", "")
	if err != nil || len(activeLessons) != 1 || activeLessons[0].ID != lessons[0].ID {
		t.Fatalf("teacher active-session lessons: %+v %v", activeLessons, err)
	}
	date := time.Date(2473, 1, 1, 0, 0, 0, 0, time.UTC)
	for date.Weekday() != time.Sunday {
		date = date.AddDate(0, 0, 1)
	}
	day := date.Format("2006-01-02")
	if _, err := svc.CohortRoster(ctx, teacherUsers[1], "teacher", lessons[0].ID, day); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unassigned teacher saw roster: %v", err)
	}
	roster, err := svc.CohortRoster(ctx, teacherUsers[0], "teacher", lessons[0].ID, day)
	if err != nil || len(roster.Members) != 3 {
		t.Fatalf("assigned roster: %+v %v", roster, err)
	}
	entries := []CohortAttendanceEntry{}
	for _, member := range roster.Members {
		entries = append(entries, CohortAttendanceEntry{StudentID: member.StudentID, Status: "present"})
	}
	if _, err := svc.SaveCohortAttendance(ctx, teacherUsers[1], "teacher", lessons[0].ID, day, entries); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unassigned teacher recorded attendance: %v", err)
	}
	if _, err := svc.SaveCohortAttendance(ctx, teacherUsers[0], "teacher", lessons[0].ID, day, entries); err != nil {
		t.Fatal(err)
	}
	var originals int64
	if err := tx.Raw(`SELECT count(DISTINCT m.section_id) FROM class_cohort_attendance a JOIN class_cohort_members m
ON m.lesson_id=a.lesson_id AND m.student_id=a.student_id WHERE a.lesson_id=?`, lessons[0].ID).Row().Scan(&originals); err != nil || originals != 3 {
		t.Fatalf("attendance lost original sections: %d %v", originals, err)
	}
	if err := tx.SavePoint("immutable_roster_probe").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE class_cohort_members SET section_id=? WHERE student_id=?", sections[0], groups[0][0]).Error; err == nil {
		t.Fatal("published roster mutated")
	}
	if err := tx.RollbackTo("immutable_roster_probe").Error; err != nil {
		t.Fatal(err)
	}
	for _, probe := range []struct {
		name, query string
		args        []any
	}{
		{"choice", "UPDATE student_subject_choices SET subject_id=? WHERE student_id=? AND session_id=? AND role='religion'", []any{subjects[1], groups[0][0], session}},
		{"enrolment", "UPDATE student_enrolments SET section_id=? WHERE student_id=? AND session_id=?", []any{sections[1], groups[0][0], session}},
		{"room", "UPDATE exam_rooms SET capacity=2 WHERE name=?", []any{rooms[0]}},
	} {
		point := "cohort_" + probe.name
		if err := tx.SavePoint(point).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(probe.query, probe.args...).Error; err == nil {
			t.Fatalf("published cohort %s changed without replacement", probe.name)
		}
		if err := tx.RollbackTo(point).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A later published plan must keep the old lesson and its attendance.
	preview, err = svc.Preview(ctx, plan)
	if err != nil || !preview.CanPublish {
		t.Fatalf("replacement preview: %+v %v", preview.Issues, err)
	}
	draft, err = svc.SaveDraft(ctx, admin, plan, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(ctx, admin, session, draft.ID, preview.Digest, true); err != nil {
		t.Fatal(err)
	}
	var historical, current, attendance int64
	if err := tx.Raw("SELECT count(*),count(*) FILTER (WHERE is_current) FROM class_cohort_lessons WHERE session_id=?", session).Row().Scan(&historical, &current); err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT count(*) FROM class_cohort_attendance WHERE lesson_id=?", lessons[0].ID).Row().Scan(&attendance); err != nil {
		t.Fatal(err)
	}
	if historical != 6 || current != 3 || attendance != 3 {
		t.Fatalf("replacement lost cohort history: lessons=%d current=%d attendance=%d", historical, current, attendance)
	}

	// The legacy section editor must refuse a teacher conflict with a
	// current cohort in a concurrent academic session.
	otherSession := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2472-2473','2473-06-01','2473-12-31',false) RETURNING id")
	otherSection := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'D','D') RETURNING id", otherSession, class)
	if err := tx.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", teachers[0], otherSession, class, subjects[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, err = NewRepository(tx).SaveClass(ctx, admin, "", ClassInput{SessionID: otherSession, ClassID: class, SectionID: otherSection, SubjectID: subjects[0], TeacherID: teachers[0], DayOfWeek: 0, StartTime: "09:00", EndTime: "09:45", Room: rooms[0]}, 540, 585)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("legacy section editor overlapped cohort: %v", err)
	}
}
