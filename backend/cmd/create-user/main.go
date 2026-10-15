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
	email := flag.String("email", "", "email address for the new user")
	role := flag.String("role", "admin", "one of the six Fly_Birds roles")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" || *email == "" || !auth.ValidRole(*role) {
		fmt.Fprintln(os.Stderr, "set DATABASE_URL and supply -email and a valid -role")
		os.Exit(1)
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "run in an interactive terminal so the password is not echoed")
		os.Exit(1)
	}
	fmt.Fprint(os.Stderr, "Password (12–72 UTF-8 bytes): ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fail(err)
	}
	defer func() {
		for i := range passwordBytes {
			passwordBytes[i] = 0
		}
	}()
	hash, err := auth.HashPassword(string(passwordBytes))
	if err != nil {
		fail(err)
	}
	normalized, err := auth.NormalizeEmail(*email)
	if err != nil {
		fail(fmt.Errorf("invalid email address"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		fail(err)
	}
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		fail(err)
	}
	user := &auth.User{Email: normalized, PasswordHash: hash, Role: *role, IsActive: true}
	if err := auth.NewRepository(db).CreateUser(ctx, user); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stdout, "Created %s user %s\n", user.Role, user.Email)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
