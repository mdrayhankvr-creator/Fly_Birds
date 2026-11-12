package notify

import (
	"context"
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

type fixture struct {
	t  *testing.T
	tx *gorm.DB
}

func (f fixture) id(q string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.tx.Raw(q, args...).Row().Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f fixture) exec(q string, args ...any) {
	f.t.Helper()
	if err := f.tx.Exec(q, args...).Error; err != nil {
		f.t.Fatal(err)
	}
}

func (f fixture) count(q string, args ...any) int64 {
	f.t.Helper()
	var n int64
	if err := f.tx.Raw(q, args...).Row().Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// Publishing a term's exam routine notifies the students of the classes it
// schedules and the invigilators on duty; an amendment (a new published
// version) notifies again, marked urgent. Students of other classes hear
// nothing.
func TestPostgresRoutineNotifications(t *testing.T) {
	ctx, db := openDB(t)
	tx := db.Begin()
	defer tx.Rollback()
	f := fixture{t, tx}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())

	f.exec("UPDATE school_modules SET enabled = true WHERE school_id = 1 AND module_key IN ('notices', 'routine')")
	admin := f.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'admin', 'RN admin') RETURNING id", "rn-admin-"+stamp+"@example.test")
	session := f.id("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2101', '2101-01-01', '2101-12-31') RETURNING id")
	nine := f.id("INSERT INTO academic_classes(name,display_order) VALUES (?, 9) RETURNING id", "RN Nine "+stamp)
	seven := f.id("INSERT INTO academic_classes(name,display_order) VALUES (?, 7) RETURNING id", "RN Seven "+stamp)
	nineA := f.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, nine)
	sevenA := f.id("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, seven)
	subject := f.id("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?, ?, ?) RETURNING id", "RN Bangla "+stamp, "বাংলা", "RN-"+stamp[8:])
	f.exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", nine, subject)
	term := f.id("INSERT INTO exam_terms(session_id,name,sequence,created_by) VALUES (?, ?, 1, ?) RETURNING id", session, "RN First Term "+stamp, admin)

	// A student of class nine (scheduled), one of class seven (not scheduled).
	scheduledUser := f.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'RN pupil 9') RETURNING id", "rn-s9-"+stamp+"@example.test")
	f.exec("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status,user_id) VALUES (?,?,?,?,?,'active',?)", "RN9-"+stamp, "Pupil Nine", session, nine, nineA, scheduledUser)
	otherUser := f.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'student', 'RN pupil 7') RETURNING id", "rn-s7-"+stamp+"@example.test")
	f.exec("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status,user_id) VALUES (?,?,?,?,?,'active',?)", "RN7-"+stamp, "Pupil Seven", session, seven, sevenA, otherUser)

	// The invigilator.
	teacherUser := f.id("INSERT INTO users(email,password_hash,role,display_name) VALUES (?, 'unused', 'teacher', 'RN teacher') RETURNING id", "rn-t-"+stamp+"@example.test")
	employee := f.id("INSERT INTO employees(employee_code,name_en,kind,designation,joined_on,user_id) VALUES (?, 'RN Teacher', 'teacher', 'Assistant Teacher', '2024-01-01', ?) RETURNING id", "RN-E-"+stamp, teacherUser)
	room := f.id("INSERT INTO exam_rooms(name,kind,created_by) VALUES (?, 'classroom', ?) RETURNING id", "RN Room "+stamp, admin)

	// The content goes in while the version is a draft (it freezes after);
	// publication is a status change, as in the real flow.
	publish := func(version int, basedOn any) string {
		v := f.id(`INSERT INTO routine_versions(term_id,version,based_on,status,reason,starts_on,ends_on,created_by)
 VALUES (?,?,?,'draft',?, '2026-05-01','2026-05-10', ?) RETURNING id`,
			term, version, basedOn, map[bool]any{true: nil, false: "dates moved"}[version == 1], admin)
		sitting := f.id("INSERT INTO routine_sittings(version_id,name,start_minute,end_minute) VALUES (?, 'Morning', 600, 780) RETURNING id", v)
		f.exec("INSERT INTO routine_papers(version_id,class_id,subject_id,duration_minutes,exam_date,sitting_id) VALUES (?,?,?,180,'2026-05-02',?)", v, nine, subject, sitting)
		f.exec("INSERT INTO routine_duties(version_id,exam_date,sitting_id,room_id,employee_id) VALUES (?,'2026-05-02',?,?,?)", v, sitting, room, employee)
		f.exec("UPDATE routine_versions SET status = 'submitted', submitted_by = ?, submitted_at = now() WHERE id = ?", admin, v)
		f.exec("UPDATE routine_versions SET status = 'approved', approved_by = ?, approved_at = now() WHERE id = ?", admin, v)
		f.exec("UPDATE routine_versions SET status = 'published', content_sha256 = repeat('a', 64), published_by = ?, published_at = now() WHERE id = ?", admin, v)
		return v
	}
	v1 := publish(1, nil)

	repo := NewRepository(tx)
	if _, err := repo.Dispatch(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine' AND user_id = ? AND source_key = ?", scheduledUser, "routine:"+v1+":"+f.id("SELECT id::text FROM students WHERE user_id = ?", scheduledUser)); n != 1 {
		t.Fatalf("the scheduled student has %d routine notifications, want 1", n)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine' AND user_id = ?", otherUser); n != 0 {
		t.Fatalf("a student of an unscheduled class has %d routine notifications, want 0", n)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine' AND user_id = ? AND source_key LIKE 'routine_duty:%' AND NOT urgent", teacherUser); n != 1 {
		t.Fatalf("the invigilator has %d duty notifications, want 1", n)
	}
	// A second dispatch creates nothing new.
	if _, err := repo.Dispatch(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine'"); n != 2 {
		t.Fatalf("after a repeated dispatch there are %d routine notifications, want 2", n)
	}

	// The amendment: version 1 is superseded, version 2 published.
	f.exec("UPDATE routine_versions SET status = 'superseded', superseded_at = now() WHERE id = ?", v1)
	publish(2, v1)
	if _, err := repo.Dispatch(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine' AND user_id = ? AND urgent AND title_en LIKE 'Exam routine amended:%'", scheduledUser); n != 1 {
		t.Fatalf("the amendment notified the student %d times, want 1", n)
	}
	if n := f.count("SELECT count(*) FROM notifications WHERE category = 'routine' AND user_id = ? AND urgent", teacherUser); n != 1 {
		t.Fatalf("the amendment notified the invigilator %d times, want 1", n)
	}
	// The preference row exists for the new category.
	prefs, err := repo.Preferences(ctx, scheduledUser)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range prefs {
		if p.Category == "routine" {
			found = p.InApp
		}
	}
	if !found {
		t.Fatalf("preferences do not list the routine category: %+v", prefs)
	}
}
