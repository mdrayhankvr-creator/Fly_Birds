// Command reset-password lets the server operator set a new password for an
// account, for example when a user is locked out or has forgotten it. The
// account's sessions are revoked and the reset is recorded in the audit log.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"golang.org/x/term"
)

func main() {
	email := flag.String("email", "", "email address of the account to reset")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" || *email == "" {
		fail(fmt.Errorf("set DATABASE_URL and supply -email"))
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fail(fmt.Errorf("run in an interactive terminal so the password is not echoed"))
	}
	fmt.Fprint(os.Stderr, "New password (12–72 UTF-8 bytes): ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fail(err)
	}
	fmt.Fprint(os.Stderr, "Repeat the new password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fail(err)
	}
	if string(first) != string(second) {
		fail(fmt.Errorf("the passwords do not match; nothing was changed"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	user, err := auth.NewRepository(db).ResetPassword(ctx, *email, string(first), time.Now())
	if err != nil {
		fail(err)
	}
	fmt.Printf("Password reset for %s (%s); all of the account's sessions were signed out.\n", user.Email, user.Role)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "reset-password:", err)
	os.Exit(1)
}
