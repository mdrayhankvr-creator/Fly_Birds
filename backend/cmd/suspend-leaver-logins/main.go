// Command suspend-leaver-logins finds students who already left the school
// (withdrawn or transferred) but whose login is still active — left over
// from before a withdrawal or transfer suspended the login (M15, QA-03).
//
// Without -confirm it only lists them, in a read-only transaction. With
// -confirm it disables those logins and revokes their sessions in one
// transaction, writing one audit event per student. It never runs by
// itself. Readmitting a student later restores their login.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/promotion"
	"gorm.io/gorm"
)

func main() {
	confirm := flag.Bool("confirm", false, "disable the listed logins and revoke their sessions")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" {
		fail(fmt.Errorf("set DATABASE_URL"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	// The schema must already include M15 stage B (migration 1702); this
	// command does not migrate, so listing never writes.
	var upgraded bool
	if err := db.Raw("SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'students' AND column_name = 'login_suspended_at')").Row().Scan(&upgraded); err != nil {
		fail(err)
	}
	if !upgraded {
		fail(fmt.Errorf("the database is not upgraded to M15 stage B; start the application once, then run this again"))
	}
	if !*confirm {
		var list []promotion.LeaverLogin
		err := db.Transaction(func(tx *gorm.DB) error {
			var e error
			list, e = promotion.ListLeaverLogins(ctx, tx)
			return e
		}, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			fail(err)
		}
		show(list)
		if len(list) > 0 {
			fmt.Println("\nNothing was changed. Run again with -confirm to disable these logins and revoke their sessions.")
		}
		return
	}
	changed, err := promotion.SuspendLeaverLogins(ctx, db, "suspend-leaver-logins")
	if err != nil {
		fail(err)
	}
	show(changed)
	fmt.Printf("\nDisabled %d login(s) and revoked their sessions; each is recorded in the audit log.\n", len(changed))
}

func show(list []promotion.LeaverLogin) {
	if len(list) == 0 {
		fmt.Println("No withdrawn or transferred student has an active login.")
		return
	}
	fmt.Printf("%d withdrawn or transferred student(s) with an active login:\n", len(list))
	for _, l := range list {
		fmt.Printf("  %-16s %-30s %-12s %s (%d open session(s))\n", l.AdmissionNo, l.StudentName, l.Status, l.Email, l.Sessions)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "suspend-leaver-logins:", err)
	os.Exit(1)
}
