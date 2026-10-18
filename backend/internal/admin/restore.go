package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/gorm"
)

var (
	ErrSchemaMismatch = errors.New("backup schema version differs from the target database")
	ErrNotEmpty       = errors.New("target database already has users; pass force to overwrite")
)

// RestoreResult reports what a restore loaded.
type RestoreResult struct {
	Tables  int   `json:"tables"`
	Rows    int64 `json:"rows"`
	Uploads int   `json:"uploads"`
}

// Restore loads a verified archive into a migrated database at the same schema
// version. All tables are replaced in one transaction with triggers and
// foreign-key checks deferred through session_replication_role, which needs a
// PostgreSQL superuser (the Compose database user is one). Uploads are written
// beneath uploadsDir only at validated paths. Without force the target must
// have no users, so a live school cannot be overwritten by accident.
func Restore(ctx context.Context, db *gorm.DB, archive, uploadsDir string, force bool) (RestoreResult, error) {
	var result RestoreResult
	manifest, _, err := ReadArchive(archive, nil)
	if err != nil {
		return result, err
	}
	var version int
	if err := db.WithContext(ctx).Raw("SELECT COALESCE(max(version), 0) FROM schema_migrations").Row().Scan(&version); err != nil {
		return result, err
	}
	if version != manifest.SchemaVersion {
		return result, fmt.Errorf("%w: backup %d, target %d", ErrSchemaMismatch, manifest.SchemaVersion, version)
	}
	var users int64
	if err := db.WithContext(ctx).Table("users").Count(&users).Error; err != nil {
		return result, err
	}
	if users > 0 && !force {
		return result, ErrNotEmpty
	}
	tables := map[string][]byte{}
	uploads := map[string][]byte{}
	if _, _, err := ReadArchive(archive, func(name string, body []byte, _ *Manifest) error {
		if strings.HasPrefix(name, "tables/") {
			tables[strings.TrimSuffix(strings.TrimPrefix(name, "tables/"), ".csv")] = body
		} else {
			uploads[name] = body
		}
		return nil
	}); err != nil {
		return result, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return result, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return result, err
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn any) error {
		pc := driverConn.(*stdlib.Conn).Conn()
		tx, err := pc.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
			return fmt.Errorf("restore needs a superuser connection: %w", err)
		}
		names := []string{}
		for _, t := range manifest.Tables {
			if t.Name != "schema_migrations" {
				names = append(names, fmt.Sprintf("public.%q", t.Name))
			}
		}
		if len(names) > 0 {
			if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")+" CASCADE"); err != nil {
				return err
			}
		}
		for _, t := range manifest.Tables {
			if t.Name == "schema_migrations" {
				continue
			}
			tag, err := tx.Conn().PgConn().CopyFrom(ctx, bytes.NewReader(tables[t.Name]), fmt.Sprintf(`COPY public.%q FROM STDIN WITH (FORMAT csv, HEADER match)`, t.Name))
			if err != nil {
				return fmt.Errorf("restore %s: %w", t.Name, err)
			}
			if tag.RowsAffected() != t.Rows {
				return fmt.Errorf("restore %s: loaded %d rows, expected %d", t.Name, tag.RowsAffected(), t.Rows)
			}
			result.Tables++
			result.Rows += t.Rows
		}
		for name, value := range manifest.Sequences {
			if !tableName.MatchString(name) {
				return errCorrupted
			}
			if value > 0 {
				if _, err := tx.Exec(ctx, "SELECT setval($1::regclass, $2)", "public."+name, value); err != nil {
					return err
				}
			}
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return result, err
	}
	if uploadsDir != "" {
		root, err := filepath.Abs(uploadsDir)
		if err != nil {
			return result, err
		}
		for name, body := range uploads {
			if !uploadEntry.MatchString(name) {
				return result, errCorrupted
			}
			target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(name, "uploads/")))
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return result, err
			}
			if err := os.WriteFile(target, body, 0o600); err != nil {
				return result, err
			}
			result.Uploads++
		}
	}
	return result, nil
}
