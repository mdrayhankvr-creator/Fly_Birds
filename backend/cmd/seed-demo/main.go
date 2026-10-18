// Command seed-demo creates or completes the fictional demo school. It is
// additive and idempotent: rows it created are recorded in demo_records and
// reused on later runs, and rows it did not create are never changed.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/demo"
	"golang.org/x/term"
)

func main() {
	confirm := flag.Bool("confirm", false, "required: confirms that demo accounts and records may be added to this database")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" || !*confirm {
		fmt.Fprintln(os.Stderr, "set DATABASE_URL and pass -confirm; demo accounts use DEMO_PASSWORD or a password typed at the prompt")
		os.Exit(1)
	}
	password := os.Getenv("DEMO_PASSWORD")
	if password == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fail(fmt.Errorf("set DEMO_PASSWORD or run in an interactive terminal"))
		}
		fmt.Fprint(os.Stderr, "Password for new demo accounts (12–72 bytes): ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fail(err)
		}
		password = string(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	if err := database.Migrate(ctx, db); err != nil {
		fail(err)
	}
	key := make([]byte, 32) // only used to construct the service; no tokens are issued
	if _, err := rand.Read(key); err != nil {
		fail(err)
	}
	authService, err := auth.NewService(auth.NewRepository(db), key)
	if err != nil {
		fail(err)
	}
	report, err := demo.Seed(ctx, db, authService, demo.Options{Password: password})
	if err != nil {
		fail(err)
	}
	fmt.Printf("Demo school ready in session %s.\n", report.Session)
	kinds := make([]string, 0, len(report.Created))
	for k := range report.Created {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	if len(kinds) == 0 {
		fmt.Println("Nothing new was needed; existing demo records were kept.")
	}
	for _, k := range kinds {
		fmt.Printf("  created %-14s %d\n", k, report.Created[k])
	}
	fmt.Println("Demo accounts (password as supplied when each was first created):")
	for _, a := range report.Accounts {
		fmt.Println("  " + a)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "seed-demo:", err)
	os.Exit(1)
}
