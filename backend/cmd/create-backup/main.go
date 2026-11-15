// Command create-backup writes one backup archive (the whole database and
// every uploaded file) and deletes the oldest archives beyond the retention
// count. It is what the server's nightly job runs, so the backup happens at a
// chosen hour instead of drifting with the API's uptime.
//
// Usage (inside the backend container, which already has the environment):
//
//	docker compose -p <school> --env-file deploy/<school>.env -f deploy/compose.prod.yaml \
//	  exec -T backend /app/create-backup
//
// It prints the archive's name, size and SHA-256, so a job can log them, and
// exits non-zero if the archive was not written. BACKUP_DIR, FILE_STORAGE_DIR
// and BACKUP_RETENTION come from the environment, as they do for the API, so
// a backup made here is identical to one made from Admin → Backups.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/admin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/config"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if cfg.DatabaseURL == "" {
		fail(fmt.Errorf("DATABASE_URL is not set"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fail(err)
	}
	backups, err := admin.NewBackups(db, cfg.BackupDir, cfg.FileStorageDir, cfg.BackupRetention)
	if err != nil {
		fail(err)
	}
	// nil actor: nobody signed in asked for this one, the schedule did.
	run, err := backups.Create(ctx, nil, "scheduled")
	if err != nil {
		fail(err)
	}
	if run.FileName == nil || run.SizeBytes == nil || run.SHA256 == nil {
		fail(fmt.Errorf("the backup finished with status %q and wrote no archive", run.Status))
	}
	fmt.Printf("%s %d %s\n", *run.FileName, *run.SizeBytes, *run.SHA256)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "backup failed:", err)
	os.Exit(1)
}
