// Command restore-backup loads a Fly_Birds backup archive into a database.
//
// Usage (from backend/, with DATABASE_URL pointing at the target database):
//
//	go run ./cmd/restore-backup -archive flybirds-backup-....tar.gz -uploads-dir data/uploads -confirm
//
// The target is migrated first and must be at the backup's schema version.
// It must contain no users unless -force is given.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func main() {
	archive := flag.String("archive", "", "path to a flybirds-backup-*.tar.gz archive")
	uploads := flag.String("uploads-dir", os.Getenv("FILE_STORAGE_DIR"), "directory to restore attachments into (FILE_STORAGE_DIR)")
	force := flag.Bool("force", false, "replace data in a database that already has users")
	confirm := flag.Bool("confirm", false, "required: confirms that existing data in the target will be replaced")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" || *archive == "" || !*confirm {
		fmt.Fprintln(os.Stderr, "set DATABASE_URL and pass -archive and -confirm (see the package comment)")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		fail(err)
	}
	result, err := admin.Restore(ctx, db, *archive, *uploads, *force)
	if err != nil {
		fail(err)
	}
	fmt.Printf("restored %d tables, %d rows and %d attachments\n", result.Tables, result.Rows, result.Uploads)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "restore failed:", err)
	os.Exit(1)
}
