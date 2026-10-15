package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
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
	for version, name := range []string{"0001_auth.sql", "0002_rbac.sql"} {
		migrationVersion := version + 1
		var recorded int
		err = tx.QueryRowContext(ctx, "SELECT version FROM schema_migrations WHERE version = $1", migrationVersion).Scan(&recorded)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		if err == nil {
			continue
		}
		body, readErr := migrationFiles.ReadFile("migrations/" + name)
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
