// Command exam-verify recalculates every published examination result with
// the exact result engine and reports any stored grade, GPA, total or merit
// position that differs. It is read-only: it runs in one READ ONLY
// transaction, never migrates and never changes marks, results or
// publication.
//
//	exam-verify [-session <session id>] [-json]
//
// Exit status: 0 no differences, 3 differences found, 1 error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("exam-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	session := flags.String("session", "", "check only this academic session (id)")
	asJSON := flags.Bool("json", false, "print the report as JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(stderr, "set DATABASE_URL")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		fmt.Fprintln(stderr, "exam-verify:", err)
		return 1
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	report, err := exams.Verify(ctx, db, exams.VerifyOptions{SessionID: *session})
	if err != nil {
		fmt.Fprintln(stderr, "exam-verify:", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(stderr, "exam-verify:", err)
			return 1
		}
	} else {
		printReport(stdout, report)
	}
	if len(report.Differences) > 0 {
		return 3
	}
	return 0
}

func printReport(w io.Writer, r exams.VerifyReport) {
	fmt.Fprintf(w, "Checked %d published exams, %d results and %d marks (read-only; nothing was changed).\n", r.Exams, r.Results, r.Marks)
	if len(r.Differences) == 0 {
		fmt.Fprintln(w, "No differences: every stored result matches the exact engine.")
		return
	}
	fmt.Fprintf(w, "%d differences: %d boundary corrections, %d inconsistent.\n\n", len(r.Differences), r.BoundaryCorrections, r.Inconsistent)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tLEVEL\tEXAM\tCLASS\tADMISSION\tSTUDENT\tSUBJECT\tFIELD\tSTORED\tEXACT\tDETAIL")
	for _, d := range r.Differences {
		fmt.Fprintf(tw, "%s\t%s\t%s (%s)\t%s %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Kind, d.Level, d.Exam, d.Session, d.Class, d.Section, d.AdmissionNo, d.Student, dash(d.Subject), d.Field, d.Stored, d.Exact, d.Detail)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nboundary_correction: stored by the pre-E2 engine for a mark exactly on a band edge; unpublish and republish the exam to correct it.")
	fmt.Fprintln(w, "inconsistent: not reproducible from today's marks and grading scheme (the scheme or a fourth subject changed after publication, or data was edited outside the application); investigate before republishing.")
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
