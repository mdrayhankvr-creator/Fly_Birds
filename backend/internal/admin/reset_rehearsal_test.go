package admin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

// Run only against a newly created database restored from a COPY of the
// showcase backup. The outer transaction rolls back the rehearsal reset.
func TestPostgresResetRestoredShowcaseCopy(t *testing.T) {
	dsn := os.Getenv("RESET_REHEARSAL_DATABASE_URL")
	if dsn == "" {
		t.Skip("RESET_REHEARSAL_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var databaseName string
	if err := db.Raw("SELECT current_database()").Row().Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "clean_slate_rehearsal" {
		t.Fatalf("refusing rehearsal on %q", databaseName)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckMigrationsCurrent(ctx, db); err != nil {
		t.Fatalf("current schema was refused: %v", err)
	}
	probe := db.Begin()
	if err := probe.Exec("DELETE FROM schema_migrations WHERE version = (SELECT max(version) FROM schema_migrations)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CheckMigrationsCurrent(ctx, probe); err == nil {
		t.Fatal("missing migration was accepted by the read-only check")
	}
	probe.Rollback()
	backupDir := t.TempDir()
	backups, err := NewBackups(db, backupDir, t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	run, err := backups.Create(ctx, nil, "scheduled")
	if err != nil || run.FileName == nil || run.SHA256 == nil || run.SchemaVersion == nil {
		t.Fatalf("create rehearsal archive: %v", err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	plan, err := PlanReset(ctx, tx, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SchoolNameEN == "" || plan.Users < 2 || plan.Rows < 10 {
		t.Fatalf("restored copy lacks expected school data: school %q, users %d, rows %d", plan.SchoolNameEN, plan.Users, plan.Rows)
	}
	archive := filepath.Join(backupDir, *run.FileName)
	if err := VerifyResetBackup(archive, *run.SHA256, plan.SchoolNameEN, *run.SchemaVersion); err != nil {
		t.Fatalf("fresh backup verification failed: %v", err)
	}
	if err := VerifyResetBackup(archive, strings.Repeat("0", 64), plan.SchoolNameEN, *run.SchemaVersion); err == nil {
		t.Fatal("incorrect backup digest accepted")
	}
	if err := VerifyResetBackup(archive, *run.SHA256, "Some Other School", *run.SchemaVersion); err == nil {
		t.Fatal("backup for a different school accepted")
	}
	want := map[string]string{}
	for _, item := range plan.Keep {
		want[item.Table] = resetTableDigest(t, tx, item.Table)
	}
	for _, name := range []string{"school_settings", "school_public_page", "grading_schemes", "grading_bands", "school_weekly_off_days"} {
		if !strings.Contains(want[name], ":") || count(t, tx, name) == 0 {
			t.Fatalf("restored backup has no %s", name)
		}
	}
	var logoURL string
	if err := tx.Raw("SELECT COALESCE(logo_url, '') FROM school_settings LIMIT 1").Row().Scan(&logoURL); err != nil {
		t.Fatal(err)
	}
	if logoURL == "" && count(t, tx, "school_logo") == 0 {
		t.Fatal("restored backup has no configured logo")
	}
	hash, err := HashResetPassword("Rehearsal-password-2026!")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Reset(ctx, tx, nil, plan, []ResetAdmin{
		{Name: "Rehearsal One", Email: "rehearsal1@example.test", PasswordHash: hash},
		{Name: "Rehearsal Two", Email: "rehearsal2@example.test", PasswordHash: hash},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Admins != 2 || result.Rows != plan.Rows {
		t.Fatalf("unexpected reset result: %+v", result)
	}
	for name, before := range want {
		if after := resetTableDigest(t, tx, name); after != before {
			t.Errorf("preserved %s changed", name)
		}
	}
	var logoAfter string
	if err := tx.Raw("SELECT COALESCE(logo_url, '') FROM school_settings LIMIT 1").Row().Scan(&logoAfter); err != nil || logoAfter != logoURL {
		t.Errorf("configured logo changed: %v", err)
	}
	for _, item := range plan.Empty {
		expected := int64(0)
		if item.Table == "users" {
			expected = 2
		} else if item.Table == "audit_events" {
			expected = 1
		}
		if got := count(t, tx, item.Table); got != expected {
			t.Errorf("%s has %d rows; want %d", item.Table, got, expected)
		}
	}
	var forced int64
	if err := tx.Raw("SELECT count(*) FROM users WHERE role='admin' AND is_active AND must_change_password").Row().Scan(&forced); err != nil || forced != 2 {
		t.Errorf("expected two active administrators forced to change passwords: %d, %v", forced, err)
	}
	orphans, err := ResetOrphans(ctx, tx)
	if err != nil || len(orphans) > 0 {
		t.Fatalf("orphan check: %v, %v", orphans, err)
	}
	t.Logf("restored school %q: %d deleted rows, %d preserved tables, 2 administrators, 0 orphans", plan.SchoolNameEN, plan.Rows, len(plan.Keep))
}

func resetTableDigest(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	if !tableName.MatchString(name) {
		t.Fatal("unexpected table name")
	}
	// Attribution to deleted users is the only expected change in kept rows.
	row := fmt.Sprintf(`SELECT count(*)::text || ':' || md5(COALESCE(string_agg((to_jsonb(t) - 'updated_by' - 'created_by' - 'updated_at')::text, '' ORDER BY (to_jsonb(t) - 'updated_by' - 'created_by' - 'updated_at')::text), '')) FROM "%s" t`, name)
	var digest string
	if err := db.Raw(row).Row().Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}
