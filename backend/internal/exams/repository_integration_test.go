package exams

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

func TestPostgresExamLifecycle(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var migrations int64
	if err := db.Table("schema_migrations").Where("version=701").Count(&migrations).Error; err != nil || migrations != 1 {
		t.Fatalf("migration 701: %d %v", migrations, err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	index := 0
	user := func(role string) string {
		index++
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("m7-%s-%d-%s@example.test", role, index, stamp), "unused", role, role)
	}
	admin, teacherUser, otherTeacherUser, studentUser, guardianUser, otherGuardian := user("admin"), user("teacher"), user("teacher"), user("student"), user("guardian"), user("guardian")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES ('2192','2192-01-01','2192-12-31') RETURNING id")
	classID := insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class M7 "+stamp)
	section := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, classID)
	subjectIDs := []string{}
	for i, code := range []string{"BAN", "ENG", "ICT"} {
		subjectIDs = append(subjectIDs, insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", code+stamp, "বিষয়", fmt.Sprintf("%s%s%d", code, stamp, i)))
	}
	for i, id := range subjectIDs {
		if err := tx.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", classID, id, i < 2, i == 2).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Teacher M7','Teacher','2191-01-01') RETURNING id", "T"+stamp, teacherUser)
	otherTeacher := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Other Teacher','Teacher','2191-01-01') RETURNING id", "U"+stamp, otherTeacherUser)
	for i, id := range subjectIDs {
		assigned := teacher
		if i == 2 {
			assigned = otherTeacher
		}
		if err := tx.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", assigned, session, classID, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	student1 := insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,user_id,status) VALUES (?,'Student One',?,?,?,?,'active') RETURNING id", "S"+stamp, session, classID, section, studentUser)
	student2 := insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,'Student Two',?,?,?,'active') RETURNING id", "R"+stamp, session, classID, section)
	guardian := insert("INSERT INTO guardians(name_en,user_id) VALUES ('Guardian M7',?) RETURNING id", guardianUser)
	if err := tx.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship) VALUES (?,?,'parent')", student1, guardian).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewRepository(tx))
	input := ExamInput{Name: "Annual", ExamType: "annual", SessionID: session, ClassID: classID, SectionID: section}
	exam, err := svc.SaveExam(ctx, admin, "", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveExam(ctx, admin, "", input); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate exam: %v", err)
	}
	if cards, err := svc.Cards(ctx, "student", studentUser, "", ""); err != nil || len(cards) != 0 {
		t.Fatalf("draft visibility: %d %v", len(cards), err)
	}
	for _, id := range subjectIDs {
		exam, err = svc.SaveSubject(ctx, admin, exam.ID, SubjectInput{SubjectID: id, FullMarks: 100, PassMarks: 33})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SetState(ctx, admin, exam.ID, "published"); !errors.Is(err, ErrConflict) {
		t.Fatalf("incomplete marks publication: %v", err)
	}
	subjectMap := map[string]string{}
	for _, sub := range exam.Subjects {
		subjectMap[sub.SubjectID] = sub.ID
	}
	if err := svc.SaveMarks(ctx, teacherUser, "teacher", exam.ID, []MarkInput{{ExamSubjectID: subjectMap[subjectIDs[2]], StudentID: student1, Marks: 80}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unassigned teacher: %v", err)
	}
	if err := svc.SaveMarks(ctx, teacherUser, "teacher", exam.ID, []MarkInput{{ExamSubjectID: subjectMap[subjectIDs[0]], StudentID: student1, Marks: 101}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out of range: %v", err)
	}
	if err := svc.SaveMarks(ctx, teacherUser, "teacher", exam.ID, []MarkInput{{ExamSubjectID: subjectMap[subjectIDs[0]], StudentID: student1, Marks: 80}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Transaction(func(inner *gorm.DB) error {
		return inner.Exec("INSERT INTO exam_marks(exam_id,exam_subject_id,student_id,session_id,class_id,section_id,marks,entered_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?)", exam.ID, subjectMap[subjectIDs[0]], student1, session, classID, section, 80, admin, admin).Error
	}); err == nil {
		t.Fatal("duplicate marks accepted by database")
	}
	if err := tx.Transaction(func(inner *gorm.DB) error {
		return inner.Exec("INSERT INTO exam_marks(exam_id,exam_subject_id,student_id,session_id,class_id,section_id,marks,entered_by,updated_by) VALUES (?,?,?,?,?,?,?,?,?)", exam.ID, subjectMap[subjectIDs[1]], student1, session, classID, section, 101, admin, admin).Error
	}); err == nil {
		t.Fatal("out-of-range marks accepted by database")
	}
	entries := []MarkInput{{subjectMap[subjectIDs[1]], student1, 70}, {subjectMap[subjectIDs[2]], student1, 80}, {subjectMap[subjectIDs[0]], student2, 32}, {subjectMap[subjectIDs[1]], student2, 80}, {subjectMap[subjectIDs[2]], student2, 100}}
	if err := svc.SaveMarks(ctx, admin, "admin", exam.ID, entries); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMarks(ctx, admin, "admin", exam.ID, []MarkInput{{subjectMap[subjectIDs[0]], student1, 80}, {subjectMap[subjectIDs[0]], student1, 79}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate bulk pair: %v", err)
	}
	exam, err = svc.SetState(ctx, admin, exam.ID, "published")
	if err != nil {
		t.Fatal(err)
	}
	if exam.Status != "published" {
		t.Fatal(exam.Status)
	}
	results, err := svc.Tabulation(ctx, exam.ID)
	if err != nil || len(results) != 2 {
		t.Fatalf("tabulation: %+v %v", results, err)
	}
	card, err := svc.Card(ctx, "student", studentUser, "", exam.ID)
	if err != nil || !card.Result.Passed || card.Result.GPA != 5 || len(card.Subjects) != 3 {
		t.Fatalf("student card: %+v %v", card, err)
	}
	for _, s := range card.Subjects {
		if s.SubjectNameBN != "বিষয়" {
			t.Fatalf("report card lacks Bangla subject name: %+v", s)
		}
	}
	second, err := svc.Card(ctx, "admin", admin, student2, exam.ID)
	if err != nil || second.Result.Passed || second.Result.GPA != 0 {
		t.Fatalf("failed student: %+v %v", second.Result, err)
	}
	if _, err := svc.Card(ctx, "guardian", otherGuardian, student1, exam.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked guardian: %v", err)
	}
	if _, err := svc.Card(ctx, "guardian", guardianUser, student1, exam.ID); err != nil {
		t.Fatal(err)
	}
	sectionB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, classID)
	student3 := insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,status) VALUES (?,'Student Three',?,?,?,'active') RETURNING id", "Q"+stamp, session, classID, sectionB)
	examB, err := svc.SaveExam(ctx, admin, "", ExamInput{Name: "Annual", ExamType: "annual", SessionID: session, ClassID: classID, SectionID: sectionB})
	if err != nil {
		t.Fatal(err)
	}
	otherSubjects := map[string]string{}
	for _, id := range subjectIDs {
		examB, err = svc.SaveSubject(ctx, admin, examB.ID, SubjectInput{SubjectID: id, FullMarks: 100, PassMarks: 33})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range examB.Subjects {
		otherSubjects[sub.SubjectID] = sub.ID
	}
	if err := svc.SaveMarks(ctx, admin, "admin", examB.ID, []MarkInput{{otherSubjects[subjectIDs[0]], student3, 90}, {otherSubjects[subjectIDs[1]], student3, 90}, {otherSubjects[subjectIDs[2]], student3, 90}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, admin, examB.ID, "published"); err != nil {
		t.Fatal(err)
	}
	reranked, err := svc.Card(ctx, "admin", admin, student1, exam.ID)
	if err != nil || reranked.Result.ClassPosition != 2 || reranked.Result.SectionPosition != 1 {
		t.Fatalf("class and section merit: %+v %v", reranked.Result, err)
	}
	if _, err := svc.SetState(ctx, admin, examB.ID, "draft"); err != nil {
		t.Fatal(err)
	}
	// Its results were published once: the withdrawn versions are history,
	// so the exam is kept (E6).
	if err := svc.DeleteExam(ctx, admin, examB.ID); !errors.Is(err, ErrHasVersions) {
		t.Fatalf("delete of an exam with published history: %v", err)
	}
	if again, err := svc.Card(ctx, "admin", admin, student1, exam.ID); err != nil || again.Result.ClassPosition != 1 {
		t.Fatalf("class position after the withdrawal: %+v %v", again.Result, err)
	}
	if err := svc.SaveMarks(ctx, admin, "admin", exam.ID, []MarkInput{{subjectMap[subjectIDs[0]], student1, 90}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("published marks edit: %v", err)
	}
	if _, err := svc.SetState(ctx, admin, exam.ID, "draft"); err != nil {
		t.Fatal(err)
	}
	if cards, err := svc.Cards(ctx, "student", studentUser, "", ""); err != nil || len(cards) != 0 {
		t.Fatalf("unpublished visibility: %d %v", len(cards), err)
	}
	if err := svc.SaveMarks(ctx, admin, "admin", exam.ID, []MarkInput{{subjectMap[subjectIDs[0]], student1, 90}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, admin, exam.ID, "published"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, admin, exam.ID, "locked"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetState(ctx, admin, exam.ID, "draft"); !errors.Is(err, ErrConflict) {
		t.Fatalf("locked result reverted: %v", err)
	}
}
