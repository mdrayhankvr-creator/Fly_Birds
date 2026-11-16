// Command preview-school-setup reads the current school and proposed identifier
// conflicts. It has no apply mode, never migrates, and never generates passwords.
//
//	DATABASE_URL=... go run ./cmd/preview-school-setup [--json] [--manifest identifiers.json]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/schoolsetup"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	f := flag.NewFlagSet("preview-school-setup", flag.ContinueOnError)
	f.SetOutput(stderr)
	asJSON := f.Bool("json", false, "print JSON report")
	manifestPath := f.String("manifest", "", "optional JSON file of proposed identifiers only")
	if err := f.Parse(args); err != nil {
		return 2
	}
	if f.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	var manifest *schoolsetup.Manifest
	if *manifestPath != "" {
		var err error
		manifest, err = readManifest(*manifestPath)
		if err != nil {
			fmt.Fprintln(stderr, "manifest:", err)
			return 2
		}
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(stderr, "set DATABASE_URL to a PostgreSQL connection string")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		fmt.Fprintln(stderr, "database connection failed")
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintln(stderr, "database connection failed")
		return 1
	}
	defer sqlDB.Close()
	report, err := schoolsetup.Preview(ctx, sqlDB, manifest)
	if err != nil {
		fmt.Fprintln(stderr, "school setup preview failed:", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1
		}
		return 0
	}
	printReport(stdout, report)
	return 0
}

func readManifest(path string) (*schoolsetup.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("manifest exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var m schoolsetup.Manifest
	if err := decoder.Decode(&m); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("manifest must contain exactly one JSON object")
	}
	if len(m.StudentAdmissionNumbers)+len(m.TeacherEmployeeCodes)+len(m.AccountEmails) == 0 {
		return nil, fmt.Errorf("manifest contains no proposed identifiers")
	}
	return &m, nil
}

func printReport(w io.Writer, r schoolsetup.Report) {
	fmt.Fprintf(w, "READ-ONLY school setup preview: %s, session %s\n", r.School, r.Session)
	fmt.Fprintf(w, "Students: %d existing, %d proposed additions; target %d per active section\n", r.StudentExisting, r.StudentToAdd, r.StudentsPerSection)
	for _, s := range r.Sections {
		fmt.Fprintf(w, "  %s %s: existing %d, target %d, add %d\n", s.Class, s.Name, s.Existing, s.Target, s.ToAdd)
	}
	fmt.Fprintf(w, "Teachers: %d existing, %d proposed additions; target %d total\n", r.TeacherExisting, r.TeacherToAdd, r.TeacherTarget)
	for _, t := range r.Teachers {
		fmt.Fprintf(w, "  %s %s: active=%t linked_account=%t role=%s account_active=%t photo=%t assignments=%d\n", t.EmployeeCode, t.Name, t.Active, t.LinkedAccount, t.LinkedRole, t.LinkedAccountActive, t.HasPhoto, t.Assignments)
	}
	fmt.Fprintf(w, "Subjects: %d active, %d class mappings; weekly off days (Sun=0): %v\n", r.ActiveSubjects, r.ClassSubjectMappings, r.WeeklyOffDays)
	fmt.Fprintln(w, "Fourth-subject choices:")
	for _, c := range r.FourthSubjects {
		fmt.Fprintf(w, "  %s: ", c.Class)
		if len(c.Options) == 0 {
			fmt.Fprint(w, "no optional mappings")
		} else {
			for i, s := range c.Options {
				if i > 0 {
					fmt.Fprint(w, ", ")
				}
				fmt.Fprintf(w, "%s %s", s.Code, s.Name)
			}
		}
		fmt.Fprintf(w, "; unresolved existing=%d, proposed=%d", c.ExistingWithoutChoice, c.ProposedWithoutChoice)
		if c.UsesGroups {
			fmt.Fprintf(w, "; group choices unresolved existing=%d, proposed=%d", c.ExistingWithoutGroupChoice, c.ProposedWithoutGroupChoice)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Lunch: %d minutes; start not supplied\n", r.LunchMinutes)
	fmt.Fprintln(w, "Missing routine inputs:")
	for _, s := range r.MissingRoutineInputs {
		fmt.Fprintln(w, "  -", s)
	}
	fmt.Fprintln(w, "Protected administrators (excluded from every proposed change):")
	for _, a := range r.ProtectedAdmins {
		fmt.Fprintf(w, "  %s: present=%t role=%s active=%t excluded=%t\n", a.Email, a.Present, a.Role, a.Active, a.ExcludedFromProposedChanges)
	}
	fmt.Fprintf(w, "Candidate manifest checked: %t; identifier conflicts: %d\n", r.ManifestChecked, len(r.IdentifierConflicts))
	for _, c := range r.IdentifierConflicts {
		fmt.Fprintf(w, "  %s %s: %s\n", c.Kind, c.Value, c.Reason)
	}
	for _, s := range r.Warnings {
		fmt.Fprintln(w, "WARNING:", s)
	}
	fmt.Fprintln(w, "No records changed. No credentials generated.")
}
