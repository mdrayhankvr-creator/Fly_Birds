package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies the versioned, forward-only PostgreSQL schema in a transaction.
func Migrate(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get database handle: %w", err)
	}
	tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(517001)"); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	seen := map[int]bool{}
	for _, name := range names {
		prefix, _, ok := strings.Cut(name, "_")
		migrationVersion, parseErr := strconv.Atoi(prefix)
		if !ok || parseErr != nil || migrationVersion < 1 || seen[migrationVersion] {
			return fmt.Errorf("invalid or duplicate migration version in %q", name)
		}
		seen[migrationVersion] = true
		var recorded int
		err = tx.QueryRowContext(ctx, "SELECT version FROM schema_migrations WHERE version = $1", migrationVersion).Scan(&recorded)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		if err == nil {
			continue
		}
		body, readErr := migrationFiles.ReadFile(path.Join("migrations", name))
		if readErr != nil {
			return fmt.Errorf("read migration %d: %w", migrationVersion, readErr)
		}
		for _, statement := range strings.Split(string(body), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, execErr := tx.ExecContext(ctx, statement); execErr != nil {
				return fmt.Errorf("apply migration %d: %w", migrationVersion, execErr)
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migrationVersion); err != nil {
			return fmt.Errorf("record migration %d: %w", migrationVersion, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
