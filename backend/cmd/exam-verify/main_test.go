package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

func TestRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "DATABASE_URL") {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
}

// TestRunsReadOnly runs the command against the test database over a
// connection on which PostgreSQL refuses every write.
func TestRunsReadOnly(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	// The command never migrates; prepare the schema over a normal connection.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	t.Setenv("DATABASE_URL", dsn+sep+"default_transaction_read_only=on")
	var out, errOut bytes.Buffer
	// A session with no exams: no differences, exit 0.
	if code := run([]string{"-session", "00000000-0000-4000-8000-000000000000"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "No differences") || !strings.Contains(out.String(), "read-only") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run([]string{"-json", "-session", "00000000-0000-4000-8000-000000000000"}, &out, &errOut); code != 0 {
		t.Fatalf("json exit %d: %s", code, errOut.String())
	}
	var report exams.VerifyReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Differences == nil {
		t.Fatalf("json report %q: %v", out.String(), err)
	}
	if code := run([]string{"-session", "nope"}, &out, &errOut); code != 1 {
		t.Fatalf("invalid session: exit %d", code)
	}
}

func TestReportFormatting(t *testing.T) {
	var out bytes.Buffer
	printReport(&out, exams.VerifyReport{Exams: 1, Results: 2, Marks: 6, BoundaryCorrections: 1, Differences: []exams.Difference{{Kind: exams.BoundaryCorrection, Level: "mark", Exam: "Half Yearly", Session: "2026", Class: "Class 7", Section: "A", AdmissionNo: "A-1", Student: "Student", Subject: "Physics", Field: "grade", Stored: "F 0.00 failed", Exact: "D 1.00 passed", Detail: "16.83 of 51.00 is exactly on a band edge"}}})
	for _, want := range []string{"1 differences: 1 boundary corrections, 0 inconsistent", "F 0.00 failed", "D 1.00 passed", "unpublish and republish"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}
