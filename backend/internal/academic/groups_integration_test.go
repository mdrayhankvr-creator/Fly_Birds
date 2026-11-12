package academic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
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

// Groups: the default list is seeded; a class opts in with uses_groups; a
// group's subjects are mapped per class (only subjects the class teaches);
// and student_takes_subject gives a grouped subject only to the students
// whose chosen group includes it, while unmapped subjects stay common.
func TestPostgresGroupsAndTakesSubject(t *testing.T) {
	ctx, db := openDB(t)
	tx := db.Begin()
	defer tx.Rollback()
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	id := func(q string, args ...any) string {
		t.Helper()
		var out string
		if err := tx.Raw(q, args...).Row().Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := tx.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	takes := func(student, session, class, subject string) bool {
		t.Helper()
		var ok bool
		if err := tx.Raw("SELECT student_takes_subject(?,?,?,?)", student, session, class, subject).Row().Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	admin := id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'G admin') RETURNING id", "g-admin-"+stamp+"@example.test")
	service := NewService(NewRepository(tx))

	// The default groups are seeded and listed in order.
	groups, err := service.ListGroups(ctx, Query{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, g := range groups.Items {
		byName[g.NameEN] = g.ID
	}
	for _, name := range []string{"Science", "Humanities", "Business Studies"} {
		if byName[name] == "" {
			t.Fatalf("the default group %q is not seeded: %+v", name, groups.Items)
		}
	}
	// A shift can be created and renamed.
	shift, err := service.SaveShift(ctx, admin, "", ShiftInput{NameEN: "G Morning " + stamp, NameBN: "প্রভাতী", DisplayOrder: 1, IsActive: boolValue(true)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveShift(ctx, admin, shift.ID, ShiftInput{NameEN: "G Day " + stamp, DisplayOrder: 2, IsActive: boolValue(true)}); err != nil {
		t.Fatal(err)
	}

	// Class Nine uses groups; physics belongs to Science, civics to
	// Humanities, bangla stays common.
	nine, err := service.SaveClass(ctx, admin, "", ClassInput{Name: "G Nine " + stamp, DisplayOrder: 9, IsActive: boolValue(true), UsesGroups: boolValue(true)})
	if err != nil || !nine.UsesGroups {
		t.Fatalf("class with groups: %+v %v", nine, err)
	}
	session := id("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2102', '2102-01-01', '2102-12-31') RETURNING id")
	section := id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, nine.ID)
	subject := func(name, code string) string {
		return id("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", name+" "+stamp, name, code)
	}
	bangla, physics, civics := subject("G Bangla", "GB-"+stamp[9:]), subject("G Physics", "GP-"+stamp[9:]), subject("G Civics", "GC-"+stamp[9:])
	outside := subject("G Outside", "GO-"+stamp[9:]) // not taught in the class
	for _, s := range []string{bangla, physics, civics} {
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", nine.ID, s)
	}
	if _, err := service.SaveGroupSubjects(ctx, admin, nine.ID, byName["Science"], GroupSubjectsInput{SubjectIDs: []string{physics}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveGroupSubjects(ctx, admin, nine.ID, byName["Humanities"], GroupSubjectsInput{SubjectIDs: []string{civics}}); err != nil {
		t.Fatal(err)
	}
	// A subject the class does not teach cannot join a group.
	if _, err := service.SaveGroupSubjects(ctx, admin, nine.ID, byName["Science"], GroupSubjectsInput{SubjectIDs: []string{physics, outside}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an outside subject in a group: %v", err)
	}
	listed, err := service.ClassGroups(ctx, nine.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range listed {
		if g.GroupID == byName["Science"] && (len(g.SubjectIDs) != 1 || g.SubjectIDs[0] != physics) {
			t.Fatalf("science subjects: %+v", g.SubjectIDs)
		}
	}

	student := func(n int, group string) string {
		s := id("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id",
			fmt.Sprintf("G-%d-%s", n, stamp), fmt.Sprintf("G pupil %d", n), session, nine.ID, section)
		if group != "" {
			exec("INSERT INTO student_group_choices(student_id,session_id,group_id) VALUES (?,?,?)", s, session, group)
		}
		return s
	}
	scientist := student(1, byName["Science"])
	humanist := student(2, byName["Humanities"])
	undecided := student(3, "")

	type row struct {
		student, subject string
		want             bool
	}
	for _, r := range []row{
		{scientist, bangla, true}, {scientist, physics, true}, {scientist, civics, false},
		{humanist, bangla, true}, {humanist, physics, false}, {humanist, civics, true},
		{undecided, bangla, true}, {undecided, physics, false}, {undecided, civics, false},
	} {
		if got := takes(r.student, session, nine.ID, r.subject); got != r.want {
			t.Fatalf("takes(student %s, subject %s) = %v, want %v", r.student, r.subject, got, r.want)
		}
	}
	// Turning uses_groups off makes every subject common again, mappings or not.
	if _, err := service.SaveClass(ctx, admin, nine.ID, ClassInput{Name: nine.Name, DisplayOrder: 9, IsActive: boolValue(true), UsesGroups: boolValue(false)}); err != nil {
		t.Fatal(err)
	}
	if !takes(humanist, session, nine.ID, physics) || !takes(undecided, session, nine.ID, civics) {
		t.Fatal("with uses_groups off, grouped subjects must be common to the class")
	}
	// An update that does not mention uses_groups keeps the saved value.
	if _, err := service.SaveClass(ctx, admin, nine.ID, ClassInput{Name: nine.Name, DisplayOrder: 9, IsActive: boolValue(true), UsesGroups: boolValue(true)}); err != nil {
		t.Fatal(err)
	}
	kept, err := service.SaveClass(ctx, admin, nine.ID, ClassInput{Name: nine.Name, DisplayOrder: 9, IsActive: boolValue(true)})
	if err != nil || !kept.UsesGroups {
		t.Fatalf("uses_groups after an update without it: %+v %v", kept, err)
	}
}
