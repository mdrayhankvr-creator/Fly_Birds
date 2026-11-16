// Command reset-school-data gives one school a clean slate: every account,
// person and record of school life is deleted, the school's own identity and
// configuration are kept, and exactly two administrators are created to set
// it up again.
//
// What stays: the school's name, logo, colours and design style, the public
// page and its photos and text, the grading scale, the weekly off days, the
// module switches, the permission matrix and the settings rows the schema
// itself provides. What goes: users and sessions, staff and students and
// guardians, classes, sections, subjects, sessions and enrolments, routines,
// exams, marks, results and their documents, fees and payments, attendance,
// notices, the diary, notifications, photo submissions and the audit trail.
//
// Nothing is printed that could not be read out loud: no password, no hash.
//
// Usage, inside the school's backend container:
//
//	# what would happen, writing nothing
//	docker compose -p <school> --env-file <env> -f deploy/compose.prod.yaml \
//	  run --rm backend /app/reset-school-data -school "Name of the School" -dry-run
//
//	# the real thing: it asks for each administrator's password twice
//	docker compose -p <school> --env-file <env> -f deploy/compose.prod.yaml \
//	  run --rm backend /app/reset-school-data -school "Name of the School" \
//	    -admin1 "Head Teacher:head@school.edu.bd" \
//	    -admin2 "Office:office@school.edu.bd" -confirm
//
// -school must be the school's English name as the database holds it, so the
// command cannot be pointed at the wrong school by accident; -any-school
// turns that guard off deliberately.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/config"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"golang.org/x/term"
)

func main() {
	school := flag.String("school", "", "the school's English name, as the database holds it")
	anySchool := flag.Bool("any-school", false, "skip the name check (for a test database)")
	admin1 := flag.String("admin1", "", `the first administrator as "Name:email"`)
	admin2 := flag.String("admin2", "", `the second administrator as "Name:email"`)
	dryRun := flag.Bool("dry-run", false, "print what would happen and write nothing")
	confirm := flag.Bool("confirm", false, "required for the real run: confirms the deletion")
	asJSON := flag.Bool("json", false, "print the plan as JSON (with -dry-run)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if cfg.DatabaseURL == "" {
		fail(fmt.Errorf("DATABASE_URL is not set"))
	}
	if *school == "" && !*anySchool {
		fail(fmt.Errorf("pass -school with the school's English name, or -any-school to skip the check"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		fail(err)
	}
	defer sqlDB.Close()
	// The schema must be current: a plan built against an older schema could
	// miss a table that a later migration added.
	if err := database.Migrate(ctx, db); err != nil {
		fail(err)
	}

	plan, err := admin.PlanReset(ctx, db, *school, *anySchool)
	if err != nil {
		fail(err)
	}
	if *dryRun {
		if *asJSON {
			body, err := json.MarshalIndent(plan, "", "  ")
			if err != nil {
				fail(err)
			}
			fmt.Println(string(body))
			return
		}
		printPlan(plan)
		fmt.Println("\nDry run: nothing was changed.")
		return
	}
	if !*confirm {
		printPlan(plan)
		fmt.Fprintln(os.Stderr, "\nRefusing to delete anything without -confirm. Use -dry-run to look first.")
		os.Exit(2)
	}
	people := []admin.ResetAdmin{}
	for i, raw := range []string{*admin1, *admin2} {
		person, err := parseAdmin(raw)
		if err != nil {
			fail(fmt.Errorf("-admin%d: %w", i+1, err))
		}
		people = append(people, person)
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fail(fmt.Errorf("run in an interactive terminal so the passwords are not echoed"))
	}
	printPlan(plan)
	fmt.Println()
	for i := range people {
		hash, err := readPassword(people[i].Name, people[i].Email)
		if err != nil {
			fail(err)
		}
		people[i].PasswordHash = hash
	}

	var store files.Store
	if local, err := files.NewLocalStore(cfg.FileStorageDir); err == nil {
		store = local
	} else {
		fmt.Fprintf(os.Stderr, "warning: the attachment store at %s is not usable (%v); stored files will be left behind\n", cfg.FileStorageDir, err)
	}
	result, err := admin.Reset(ctx, db, store, plan, people)
	if err != nil {
		fail(err)
	}
	fmt.Printf("\nEmptied %d tables (%d rows) and deleted %d stored file(s).\n", result.Emptied, result.Rows, result.Files)
	if result.FileErrors > 0 {
		fmt.Printf("%d stored file(s) could not be deleted; nothing points at them any more.\n", result.FileErrors)
	}
	fmt.Printf("Created %d administrators; each must choose a new password at the first sign-in:\n", result.Admins)
	for _, a := range people {
		fmt.Printf("  %s <%s>\n", a.Name, a.Email)
	}
	orphans, err := admin.ResetOrphans(ctx, db)
	if err != nil {
		fail(fmt.Errorf("check for orphan rows: %w", err))
	}
	if len(orphans) > 0 {
		fmt.Fprintln(os.Stderr, "\nORPHAN ROWS REMAIN — restore the backup and report this:")
		for _, o := range orphans {
			fmt.Fprintln(os.Stderr, "  "+o)
		}
		os.Exit(1)
	}
	fmt.Println("Every foreign key checks out: no orphan rows.")
	if cfg.DemoQuickLogin {
		fmt.Println("\nNote: DEMO_QUICK_LOGIN is on for this stack. Turn it off before showing the school:")
		fmt.Println("  the one-click demo sign-in only works for seed-demo accounts, which are now gone,")
		fmt.Println("  but the switch should be off on a school that is being set up for real.")
	}
}

// printPlan is the summary an operator reads before confirming.
func printPlan(plan admin.ResetPlan) {
	fmt.Printf("School: %s", plan.SchoolNameEN)
	if plan.SchoolNameBN != "" {
		fmt.Printf(" / %s", plan.SchoolNameBN)
	}
	fmt.Println()
	fmt.Printf("\nTo be deleted — %d rows in %d tables, %d account(s), %d stored file(s):\n",
		plan.Rows, len(plan.Counted()), plan.Users, plan.Files)
	counted := plan.Counted()
	sort.Slice(counted, func(i, j int) bool {
		if counted[i].Rows != counted[j].Rows {
			return counted[i].Rows > counted[j].Rows
		}
		return counted[i].Table < counted[j].Table
	})
	for _, t := range counted {
		fmt.Printf("  %-36s %8d\n", t.Table, t.Rows)
	}
	empty := len(plan.Empty) - len(counted)
	if empty > 0 {
		fmt.Printf("  (%d more tables are already empty)\n", empty)
	}
	fmt.Printf("\nTo be kept — %d tables:\n", len(plan.Keep))
	for _, t := range plan.Keep {
		fmt.Printf("  %-36s %8d\n", t.Table, t.Rows)
	}
	if len(plan.Nulled) > 0 {
		fmt.Println("\nKept rows whose record of who last changed them is cleared:")
		for _, n := range plan.Nulled {
			fmt.Printf("  %s.%s (pointed at %s)\n", n.Table, n.Column, n.Points)
		}
	}
}

// parseAdmin reads "Name:email@example.test".
func parseAdmin(raw string) (admin.ResetAdmin, error) {
	name, address, found := strings.Cut(raw, ":")
	if !found {
		return admin.ResetAdmin{}, fmt.Errorf(`use "Name:email@example.test"`)
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || len(name) > 160 {
		return admin.ResetAdmin{}, fmt.Errorf("the name must be 1 to 160 characters")
	}
	email, err := admin.NormalizeResetEmail(address)
	if err != nil {
		return admin.ResetAdmin{}, fmt.Errorf("%q is not an email address", strings.TrimSpace(address))
	}
	return admin.ResetAdmin{Name: name, Email: email}, nil
}

// readPassword asks twice, never echoes, and keeps the typed bytes only as
// long as it takes to hash them.
func readPassword(name, email string) (string, error) {
	fmt.Fprintf(os.Stderr, "Password for %s <%s> (12–72 UTF-8 bytes): ", name, email)
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	defer wipe(first)
	fmt.Fprint(os.Stderr, "Again: ")
	again, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	same := subtle.ConstantTimeCompare(first, again) == 1
	wipe(again)
	if !same {
		return "", fmt.Errorf("the two passwords for %s differ; nothing was changed", email)
	}
	hash, err := admin.HashResetPassword(string(first))
	if err != nil {
		return "", fmt.Errorf("%s: %w", email, err)
	}
	return hash, nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "reset failed:", err)
	os.Exit(1)
}
