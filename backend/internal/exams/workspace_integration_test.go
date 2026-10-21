package exams

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

// TestPermissionsFor checks that the workspace's permissions mirror the E3
// write rules for every role and state.
func TestPermissionsFor(t *testing.T) {
	teacher := []string{StatusPresent, StatusAbsent, StatusIncomplete}
	admin := []string{StatusPresent, StatusAbsent, StatusIncomplete, StatusExempt, StatusWithheld}
	for _, c := range []struct {
		exam, sheet, role string
		want              SheetPermissions
	}{
		{"draft", SheetInProgress, "teacher", SheetPermissions{CanEdit: true, CanSubmit: true, Statuses: teacher}},
		{"draft", SheetReturned, "teacher", SheetPermissions{CanEdit: true, CanSubmit: true, Statuses: teacher}},
		{"draft", SheetSubmitted, "teacher", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "sheet_submitted"}},
		{"draft", SheetApproved, "teacher", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "sheet_approved"}},
		{"draft", SheetNotStarted, "admin", SheetPermissions{CanEdit: true, CanSubmit: true, Statuses: admin}},
		{"draft", SheetSubmitted, "admin", SheetPermissions{CanReview: true, Statuses: []string{}, ReadOnlyReason: "sheet_submitted"}},
		{"draft", SheetSubmitted, "head_teacher", SheetPermissions{CanReview: true, Statuses: []string{}, ReadOnlyReason: "review_only"}},
		{"draft", SheetInProgress, "head_teacher", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "review_only"}},
		{"draft", SheetApproved, "head_teacher", SheetPermissions{CanReview: true, Statuses: []string{}, ReadOnlyReason: "review_only"}},
		{"published", SheetApproved, "admin", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "exam_published"}},
		{"locked", SheetApproved, "teacher", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "exam_locked"}},
		{"draft", SheetInProgress, "accountant", SheetPermissions{Statuses: []string{}, ReadOnlyReason: "review_only"}},
	} {
		if got := permissionsFor(c.exam, c.sheet, c.role); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s/%s/%s: %+v, want %+v", c.exam, c.sheet, c.role, got, c.want)
		}
	}
}

func TestPostgresMarksWorkspace(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	tx := db.Begin()
	defer tx.Rollback()
	f := newWorkflowFixture(t, ctx, tx, 2196)
	svc := f.svc
	exam, sheet := f.exam(t, "Workspace")
	other, otherSheet := f.exam(t, "Workspace Two")

	// Work queue: the teacher's assigned sheets of both exams, nobody else's.
	queue, err := svc.MySheets(ctx, f.teacher)
	if err != nil {
		t.Fatal(err)
	}
	mine := map[string]QueueItem{}
	for _, q := range queue {
		mine[q.SheetID] = q
	}
	if len(mine) != 4 || mine[sheet["S1"]].Students != 3 || mine[sheet["S1"]].Missing != 3 || mine[sheet["S3"]].SheetID != "" || mine[otherSheet["S2"]].ExamName != "Workspace Two" {
		t.Fatalf("teacher queue %+v", queue)
	}
	if q, _ := svc.MySheets(ctx, f.teacher2); len(q) != 2 {
		t.Fatalf("teacher 2 queue: %d sheets", len(q))
	}

	// One sheet: rows of active students, permissions and read-only reasons.
	w, err := svc.Workspace(ctx, exam.ID, sheet["S1"], f.teacher, "teacher", "teacher")
	if err != nil || len(w.Rows) != 3 || w.Sheet.ID != sheet["S1"] || w.Exam.Name != "Workspace" || !w.Permissions.CanEdit || len(w.Permissions.Statuses) != 3 {
		t.Fatalf("teacher workspace %+v %v", w, err)
	}
	for _, r := range w.Rows {
		if r.ExamSubjectID != sheet["S1"] || r.RowVersion != 0 || r.Status != "" {
			t.Fatalf("row %+v", r)
		}
	}
	if _, err := svc.Workspace(ctx, exam.ID, sheet["S3"], f.teacher, "teacher", "teacher"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unassigned sheet: %v", err)
	}
	if _, err := svc.Workspace(ctx, exam.ID, otherSheet["S1"], f.teacher, "teacher", "teacher"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a sheet of another exam under this exam id: %v", err)
	}
	if w, _ := svc.Workspace(ctx, exam.ID, sheet["S3"], f.admin, "admin", "admin"); !w.Permissions.CanEdit || len(w.Permissions.Statuses) != 5 {
		t.Fatalf("admin workspace %+v", w.Permissions)
	}
	if w, _ := svc.Workspace(ctx, exam.ID, sheet["S1"], f.headTeacher, "head_teacher", "admin"); w.Permissions.CanEdit || w.Permissions.ReadOnlyReason != "review_only" {
		t.Fatalf("head teacher workspace %+v", w.Permissions)
	}

	// Writes, submission, and the history of this sheet only.
	entries := []MarkEntry{}
	for _, st := range f.students {
		entries = append(entries, MarkEntry{ExamSubjectID: sheet["S1"], StudentID: st, Status: StatusPresent, Marks: num(30)})
	}
	if res, err := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, entries); err != nil || len(res.Saved) != 3 {
		t.Fatalf("marks: %+v %v", res, err)
	}
	if res, _ := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S2"], StudentID: f.students[0], Status: StatusAbsent}}); len(res.Saved) != 1 {
		t.Fatal("other sheet")
	}
	// A teacher cannot undo an administrator's exemption, even back to present.
	if res, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S2"], StudentID: f.students[1], Status: StatusExempt, Reason: "Medical certificate"}}); len(res.Saved) != 1 {
		t.Fatalf("admin exemption: %+v", res)
	}
	undo, _ := svc.SaveMarkEntries(ctx, f.teacher, "teacher", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S2"], StudentID: f.students[1], Status: StatusPresent, Marks: num(50), RowVersion: 1}})
	if len(undo.Saved) != 0 || len(undo.Errors) != 1 || undo.Errors[0].Code != "status_not_allowed" {
		t.Fatalf("teacher changing an exempt mark: %+v", undo)
	}
	if res, _ := svc.SaveMarkEntries(ctx, f.admin, "admin", exam.ID, []MarkEntry{{ExamSubjectID: sheet["S2"], StudentID: f.students[1], Status: StatusPresent, Marks: num(50), RowVersion: 1}}); len(res.Saved) != 1 {
		t.Fatalf("admin lifts the exemption: %+v", res)
	}
	if _, err := svc.SheetAction(ctx, f.teacher, "teacher", exam.ID, sheet["S1"], ActionSubmit, ""); err != nil {
		t.Fatal(err)
	}
	w, _ = svc.Workspace(ctx, exam.ID, sheet["S1"], f.teacher, "teacher", "teacher")
	if w.Permissions.CanEdit || w.Permissions.ReadOnlyReason != "sheet_submitted" || w.Sheet.Entered != 3 || w.Rows[0].RowVersion != 1 {
		t.Fatalf("after submit %+v %+v", w.Permissions, w.Sheet)
	}
	if w, _ := svc.Workspace(ctx, exam.ID, sheet["S1"], f.headTeacher, "head_teacher", "admin"); !w.Permissions.CanReview {
		t.Fatalf("head teacher review %+v", w.Permissions)
	}
	history, err := svc.SheetHistory(ctx, exam.ID, sheet["S1"], f.teacher, "teacher", "teacher")
	if err != nil || len(history) != 4 || history[0].Action != "exam.sheet_submitted" || history[1].Action != "exam.mark_saved" || history[1].StudentName == "" || history[1].Actor != "E3 teacher" {
		t.Fatalf("history %+v %v", history, err)
	}
	if _, err := svc.SheetHistory(ctx, exam.ID, sheet["S3"], f.teacher, "teacher", "teacher"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("history of an unassigned sheet: %v", err)
	}
	queue, _ = svc.MySheets(ctx, f.teacher)
	if queue[0].Status == SheetSubmitted || queue[len(queue)-1].Status != SheetSubmitted {
		t.Fatalf("queue order: work that needs the teacher first: %+v", queue)
	}
	_ = other
}
