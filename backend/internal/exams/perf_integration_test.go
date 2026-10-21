package exams

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

// TestPostgresMarksSavePerformance measures mark saving on a class of 100
// students and 5 subjects (500 cells): one 500-cell save, and five teachers
// saving a 100-cell sheet each at the same moment. It uses only APIs that
// exist before and after E3 (SaveExam, SaveSubject, SaveMarks), so the same
// file times both versions. Opt-in: E3_PERF=1.
func TestPostgresMarksSavePerformance(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("E3_PERF") != "1" {
		t.Skip("set AUTH_TEST_DATABASE_URL and E3_PERF=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := db.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	year := ""
	for attempt := 0; attempt < 50 && year == ""; attempt++ {
		y := fmt.Sprint(3000 + rand.Intn(6000))
		var used int64
		db.Table("academic_sessions").Where("name=?", y).Count(&used)
		if used == 0 {
			year = y
		}
	}
	admin := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,'admin','perf') RETURNING id", "perf-admin-"+stamp+"@example.test", "unused")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,?,?) RETURNING id", year, year+"-01-01", year+"-12-31")
	classID := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Perf "+stamp)
	section := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, classID)
	subjects := []string{}
	for i := 0; i < 5; i++ {
		id := insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", fmt.Sprintf("Perf %d %s", i, stamp), "বিষয়", fmt.Sprintf("PF%d%s", i, stamp))
		if err := db.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, id).Error; err != nil {
			t.Fatal(err)
		}
		subjects = append(subjects, id)
	}
	students := []string{}
	for i := 0; i < 100; i++ {
		students = append(students, insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,?,?,?,?,'active') RETURNING id", fmt.Sprintf("PF%03d-%s", i, stamp), fmt.Sprintf("Perf %d", i), session, classID, section))
	}
	// Remove the committed fixture afterwards; other tests count sessions
	// and administrators. A defer runs before the deferred sqlDB.Close.
	defer func() {
		for _, step := range []struct {
			query string
			arg   any
		}{
			{"DELETE FROM exams WHERE session_id=?", session},
			{"DELETE FROM students WHERE session_id=?", session},
			{"DELETE FROM academic_class_subjects WHERE class_id=?", classID},
			{"DELETE FROM academic_subjects WHERE id IN ?", subjects},
			{"DELETE FROM academic_sections WHERE session_id=?", session},
			{"DELETE FROM academic_classes WHERE id=?", classID},
			{"DELETE FROM academic_sessions WHERE id=?", session},
			{"DELETE FROM users WHERE id=?", admin},
		} {
			if err := db.Exec(step.query, step.arg).Error; err != nil {
				t.Errorf("cleanup %q: %v", step.query, err)
			}
		}
	}()
	svc := NewService(NewRepository(db))
	newExam := func(name string) (string, []string) {
		e, err := svc.SaveExam(ctx, admin, "", ExamInput{Name: name, ExamType: "class_test", SessionID: session, ClassID: classID, SectionID: section})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range subjects {
			if e, err = svc.SaveSubject(ctx, admin, e.ID, SubjectInput{SubjectID: s, FullMarks: 100, PassMarks: 33}); err != nil {
				t.Fatal(err)
			}
		}
		ids := []string{}
		for _, s := range e.Subjects {
			ids = append(ids, s.ID)
		}
		return e.ID, ids
	}
	sheetEntries := func(sheet string, offset int) []MarkInput {
		out := make([]MarkInput, 0, len(students))
		for i, st := range students {
			out = append(out, MarkInput{ExamSubjectID: sheet, StudentID: st, Marks: float64((i + offset) % 101)})
		}
		return out
	}

	// One 500-cell save, three times.
	single := []time.Duration{}
	for run := 0; run < 3; run++ {
		examID, sheets := newExam(fmt.Sprintf("Perf single %d", run))
		all := []MarkInput{}
		for i, s := range sheets {
			all = append(all, sheetEntries(s, i)...)
		}
		start := time.Now()
		if err := svc.SaveMarks(ctx, admin, "admin", examID, all); err != nil {
			t.Fatal(err)
		}
		single = append(single, time.Since(start))
	}
	// Five sheets of one exam saved at the same moment.
	parallel := []time.Duration{}
	for run := 0; run < 3; run++ {
		examID, sheets := newExam(fmt.Sprintf("Perf parallel %d", run))
		var wg sync.WaitGroup
		errs := make([]error, len(sheets))
		start := time.Now()
		for i, s := range sheets {
			wg.Add(1)
			go func(i int, s string) {
				defer wg.Done()
				errs[i] = svc.SaveMarks(ctx, admin, "admin", examID, sheetEntries(s, i))
			}(i, s)
		}
		wg.Wait()
		parallel = append(parallel, time.Since(start))
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("PERF one 500-cell save: %v %v %v", single[0], single[1], single[2])
	t.Logf("PERF five 100-cell sheets at once: %v %v %v", parallel[0], parallel[1], parallel[2])
}
