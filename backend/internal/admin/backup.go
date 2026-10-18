package admin

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ManifestFormat identifies the archive layout for future restores.
const ManifestFormat = "flybirds-backup-v1"

// excludedTables are never backed up: refresh-token sessions are ephemeral
// secrets, so a restore simply asks everyone to sign in again.
var excludedTables = []string{"auth_sessions"}

var (
	tableName    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	uploadEntry  = regexp.MustCompile(`^uploads/[0-9a-f]{2}/[0-9a-f]{32}$`)
	archiveName  = regexp.MustCompile(`^flybirds-backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}\.tar\.gz$`)
	uuidPattern  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	errCorrupted = errors.New("backup archive failed verification")
)

type ManifestTable struct {
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type ManifestFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Format        string           `json:"format"`
	CreatedAt     string           `json:"created_at"`
	SchemaVersion int              `json:"schema_version"`
	Tables        []ManifestTable  `json:"tables"`
	Sequences     map[string]int64 `json:"sequences"`
	Uploads       []ManifestFile   `json:"uploads"`
	Excluded      []string         `json:"excluded_tables"`
}

type Run struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Trigger       string  `json:"trigger"`
	FileName      *string `json:"file_name"`
	SizeBytes     *int64  `json:"size_bytes"`
	SHA256        *string `json:"sha256"`
	TableCount    *int    `json:"table_count"`
	RowCount      *int64  `json:"row_count"`
	FileCount     *int    `json:"file_count"`
	SchemaVersion *int    `json:"schema_version"`
	Error         string  `json:"error"`
	StartedByName string  `json:"started_by_name"`
	StartedAt     string  `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
	Deleted       bool    `json:"deleted"`
}
type RunPage struct {
	Items     []Run `json:"items"`
	Total     int64 `json:"total"`
	Page      int   `json:"page"`
	PageSize  int   `json:"page_size"`
	Retention int   `json:"retention"`
}
type Verification struct {
	OK      bool   `json:"ok"`
	Entries int    `json:"entries"`
	Problem string `json:"problem,omitempty"`
}

// Backups writes archives to dir (never inside the database) and keeps the
// newest retention completed archives.
type Backups struct {
	db        *gorm.DB
	dir       string
	uploads   string
	retention int
	now       func() time.Time
}

func NewBackups(db *gorm.DB, dir, uploads string, retention int) (*Backups, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	if retention < 1 {
		retention = 1
	}
	return &Backups{db: db, dir: abs, uploads: uploads, retention: retention, now: time.Now}, nil
}

const runSelect = `b.id, b.status, b.trigger, b.file_name, b.size_bytes, b.sha256, b.table_count, b.row_count, b.file_count, b.schema_version, b.error,
COALESCE(NULLIF(u.display_name, ''), u.email, '') AS started_by_name,
to_char(b.started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS started_at,
to_char(b.finished_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS finished_at, b.deleted_at IS NOT NULL AS deleted`

func (b *Backups) run(db *gorm.DB, id string) (Run, error) {
	var r Run
	err := db.Table("backup_runs b").Joins("LEFT JOIN users u ON u.id = b.started_by").Select(runSelect).Where("b.id = ?", id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r, ErrNotFound
	}
	return r, err
}

func (b *Backups) List(ctx context.Context, page, size int) (RunPage, error) {
	out := RunPage{Items: []Run{}, Page: page, PageSize: size, Retention: b.retention}
	base := b.db.WithContext(ctx).Table("backup_runs b").Joins("LEFT JOIN users u ON u.id = b.started_by")
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := base.Select(runSelect).Order("b.started_at DESC, b.id").Limit(size).Offset((page - 1) * size).Scan(&out.Items).Error
	return out, err
}

// Create takes a consistent snapshot and stores it as a .tar.gz archive. Only
// one backup runs at a time; a run left "running" by a crash for over an hour
// is marked failed so it cannot block future backups.
func (b *Backups) Create(ctx context.Context, actor *string, trigger string) (Run, error) {
	db := b.db.WithContext(ctx)
	if err := db.Exec("UPDATE backup_runs SET status = 'failed', error = 'interrupted', finished_at = now() WHERE status = 'running' AND started_at < now() - interval '1 hour'").Error; err != nil {
		return Run{}, err
	}
	var id string
	if err := db.Raw("INSERT INTO backup_runs(trigger, started_by) VALUES (?, ?) RETURNING id", trigger, actor).Row().Scan(&id); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return Run{}, ErrBusy
		}
		return Run{}, err
	}
	name, size, sum, manifest, err := b.write(ctx)
	if err != nil {
		message := err.Error()
		if len(message) > 400 {
			message = message[:400]
		}
		_ = db.Exec("UPDATE backup_runs SET status = 'failed', error = ?, finished_at = now() WHERE id = ?", message, id).Error
		_ = audit.Record(db, actor, "backup.failed", "backup", id, map[string]any{"trigger": trigger})
		return Run{}, err
	}
	var rows int64
	for _, t := range manifest.Tables {
		rows += t.Rows
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE backup_runs SET status = 'completed', file_name = ?, size_bytes = ?, sha256 = ?, table_count = ?, row_count = ?, file_count = ?, schema_version = ?, finished_at = now() WHERE id = ?",
			name, size, sum, len(manifest.Tables), rows, len(manifest.Uploads), manifest.SchemaVersion, id).Error; err != nil {
			return err
		}
		return audit.Record(tx, actor, "backup.created", "backup", id, map[string]any{"trigger": trigger, "tables": len(manifest.Tables), "rows": rows, "files": len(manifest.Uploads), "size_bytes": size})
	})
	if err != nil {
		_ = os.Remove(filepath.Join(b.dir, name))
		return Run{}, err
	}
	if err := b.prune(ctx, actor); err != nil {
		return Run{}, err
	}
	return b.run(db, id)
}

// prune deletes completed archives beyond the retention count, oldest first.
func (b *Backups) prune(ctx context.Context, actor *string) error {
	db := b.db.WithContext(ctx)
	var old []struct {
		ID       string
		FileName string
	}
	if err := db.Raw("SELECT id, file_name FROM backup_runs WHERE status = 'completed' AND deleted_at IS NULL ORDER BY started_at DESC, id OFFSET ?", b.retention).Scan(&old).Error; err != nil {
		return err
	}
	for _, o := range old {
		if err := b.removeFile(o.FileName); err != nil {
			return err
		}
		if err := db.Exec("UPDATE backup_runs SET deleted_at = now() WHERE id = ?", o.ID).Error; err != nil {
			return err
		}
		if err := audit.Record(db, actor, "backup.pruned", "backup", o.ID, map[string]any{"retention": b.retention}); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backups) removeFile(name string) error {
	if !archiveName.MatchString(name) {
		return ErrInvalid
	}
	if err := os.Remove(filepath.Join(b.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Delete removes an archive file and marks its run deleted.
func (b *Backups) Delete(ctx context.Context, actor, id string) error {
	if !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	r, err := b.run(b.db.WithContext(ctx), id)
	if err != nil {
		return err
	}
	if r.Status != "completed" || r.Deleted {
		return ErrNotFound
	}
	if err := b.removeFile(*r.FileName); err != nil {
		return err
	}
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE backup_runs SET deleted_at = now() WHERE id = ?", id).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "backup.deleted", "backup", id, nil)
	})
}

// Open returns an available archive for download and audits the download.
func (b *Backups) Open(ctx context.Context, actor, id string) (*os.File, Run, error) {
	if !uuidPattern.MatchString(id) {
		return nil, Run{}, ErrNotFound
	}
	r, err := b.run(b.db.WithContext(ctx), id)
	if err != nil {
		return nil, Run{}, err
	}
	if r.Status != "completed" || r.Deleted || r.FileName == nil || !archiveName.MatchString(*r.FileName) {
		return nil, Run{}, ErrNotFound
	}
	f, err := os.Open(filepath.Join(b.dir, *r.FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Run{}, ErrNotFound
	}
	if err != nil {
		return nil, Run{}, err
	}
	if err := audit.Record(b.db.WithContext(ctx), &actor, "backup.downloaded", "backup", id, nil); err != nil {
		f.Close()
		return nil, Run{}, err
	}
	return f, r, nil
}

// Verify re-reads an archive: the whole-file checksum must match the run and
// every entry must match its manifest checksum.
func (b *Backups) Verify(ctx context.Context, actor, id string) (Verification, error) {
	if !uuidPattern.MatchString(id) {
		return Verification{}, ErrNotFound
	}
	r, err := b.run(b.db.WithContext(ctx), id)
	if err != nil {
		return Verification{}, err
	}
	if r.Status != "completed" || r.Deleted || r.FileName == nil {
		return Verification{}, ErrNotFound
	}
	path := filepath.Join(b.dir, *r.FileName)
	result := Verification{OK: true}
	if sum, err := fileSHA256(path); err != nil || sum != *r.SHA256 {
		result = Verification{OK: false, Problem: "archive checksum mismatch or file missing"}
	} else if _, n, err := ReadArchive(path, nil); err != nil {
		result = Verification{OK: false, Problem: err.Error()}
	} else {
		result.Entries = n
	}
	return result, audit.Record(b.db.WithContext(ctx), &actor, "backup.verified", "backup", id, map[string]any{"ok": result.OK})
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// write builds the archive in a temporary directory and renames it into place.
func (b *Backups) write(ctx context.Context) (string, int64, string, Manifest, error) {
	work, err := os.MkdirTemp(b.dir, ".work-*")
	if err != nil {
		return "", 0, "", Manifest{}, err
	}
	defer os.RemoveAll(work)
	manifest, err := b.snapshot(ctx, work)
	if err != nil {
		return "", 0, "", Manifest{}, err
	}
	if err := b.collectUploads(&manifest); err != nil {
		return "", 0, "", Manifest{}, err
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", 0, "", Manifest{}, err
	}
	name := fmt.Sprintf("flybirds-backup-%s-%s.tar.gz", b.now().UTC().Format("20060102T150405Z"), hex.EncodeToString(suffix))
	tmp := filepath.Join(work, "archive.tar.gz")
	if err := b.pack(tmp, work, manifest); err != nil {
		return "", 0, "", Manifest{}, err
	}
	sum, err := fileSHA256(tmp)
	if err != nil {
		return "", 0, "", Manifest{}, err
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return "", 0, "", Manifest{}, err
	}
	if err := os.Rename(tmp, filepath.Join(b.dir, name)); err != nil {
		return "", 0, "", Manifest{}, err
	}
	return name, info.Size(), sum, manifest, nil
}

// snapshot copies every table in one read-only REPEATABLE READ transaction so
// the archive is consistent even while the school keeps working.
func (b *Backups) snapshot(ctx context.Context, work string) (Manifest, error) {
	manifest := Manifest{Format: ManifestFormat, CreatedAt: b.now().UTC().Format(time.RFC3339), Sequences: map[string]int64{}, Uploads: []ManifestFile{}, Excluded: excludedTables}
	sqlDB, err := b.db.DB()
	if err != nil {
		return manifest, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return manifest, err
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn any) error {
		pc := driverConn.(*stdlib.Conn).Conn()
		tx, err := pc.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := tx.QueryRow(ctx, "SELECT COALESCE(max(version), 0) FROM schema_migrations").Scan(&manifest.SchemaVersion); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND NOT (tablename = ANY($1)) ORDER BY tablename", excludedTables)
		if err != nil {
			return err
		}
		tables := []string{}
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				return err
			}
			tables = append(tables, t)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		seqs, err := tx.Query(ctx, "SELECT sequencename, COALESCE(last_value, 0) FROM pg_sequences WHERE schemaname = 'public' ORDER BY sequencename")
		if err != nil {
			return err
		}
		for seqs.Next() {
			var name string
			var value int64
			if err := seqs.Scan(&name, &value); err != nil {
				return err
			}
			manifest.Sequences[name] = value
		}
		seqs.Close()
		for _, t := range tables {
			if !tableName.MatchString(t) {
				return fmt.Errorf("unexpected table name %q", t)
			}
			file := "tables/" + t + ".csv"
			f, err := os.Create(filepath.Join(work, t+".csv"))
			if err != nil {
				return err
			}
			h := sha256.New()
			tag, err := tx.Conn().PgConn().CopyTo(ctx, io.MultiWriter(f, h), fmt.Sprintf(`COPY public.%q TO STDOUT WITH (FORMAT csv, HEADER true)`, t))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return fmt.Errorf("copy %s: %w", t, err)
			}
			manifest.Tables = append(manifest.Tables, ManifestTable{Name: t, Rows: tag.RowsAffected(), File: file, SHA256: hex.EncodeToString(h.Sum(nil))})
		}
		return nil
	})
	return manifest, err
}

func (b *Backups) collectUploads(m *Manifest) error {
	if b.uploads == "" {
		return nil
	}
	root, err := filepath.Abs(b.uploads)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := "uploads/" + filepath.ToSlash(rel)
		if !uploadEntry.MatchString(entry) {
			return nil // skips temporary partial uploads and foreign files
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		m.Uploads = append(m.Uploads, ManifestFile{Path: entry, Size: info.Size(), SHA256: sum})
		return nil
	})
}

func (b *Backups) pack(target, work string, m Manifest) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, size int64, body io.Reader) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: size, ModTime: b.now().UTC(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := io.Copy(tw, body)
		return err
	}
	manifestJSON, _ := json.MarshalIndent(m, "", "  ")
	err = add("manifest.json", 0o600, int64(len(manifestJSON)), strings.NewReader(string(manifestJSON)))
	for _, t := range m.Tables {
		if err != nil {
			break
		}
		err = addFile(add, t.File, filepath.Join(work, t.Name+".csv"))
	}
	root, _ := filepath.Abs(b.uploads)
	for _, u := range m.Uploads {
		if err != nil {
			break
		}
		err = addFile(add, u.Path, filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(u.Path, "uploads/"))))
	}
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

func addFile(add func(string, int64, int64, io.Reader) error, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return add(name, 0o600, info.Size(), f)
}

// ReadArchive validates an archive and, when visit is non-nil, passes each
// verified entry to it. Entry names are matched against the manifest, so an
// archive cannot smuggle files outside tables/ and uploads/.
func ReadArchive(path string, visit func(name string, body []byte, m *Manifest) error) (*Manifest, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, 0, errCorrupted
	}
	tr := tar.NewReader(gz)
	var manifest *Manifest
	expected := map[string]string{}
	seen := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, seen, errCorrupted
		}
		if h.Typeflag != tar.TypeReg || h.Size > 4<<30 {
			return nil, seen, errCorrupted
		}
		body, err := io.ReadAll(io.LimitReader(tr, h.Size))
		if err != nil {
			return nil, seen, errCorrupted
		}
		if manifest == nil {
			if h.Name != "manifest.json" {
				return nil, seen, errCorrupted
			}
			manifest = &Manifest{}
			if err := json.Unmarshal(body, manifest); err != nil || manifest.Format != ManifestFormat {
				return nil, seen, errCorrupted
			}
			for _, t := range manifest.Tables {
				if !tableName.MatchString(t.Name) || t.File != "tables/"+t.Name+".csv" {
					return nil, seen, errCorrupted
				}
				expected[t.File] = t.SHA256
			}
			for _, u := range manifest.Uploads {
				if !uploadEntry.MatchString(u.Path) {
					return nil, seen, errCorrupted
				}
				expected[u.Path] = u.SHA256
			}
			continue
		}
		want, ok := expected[h.Name]
		sum := sha256.Sum256(body)
		if !ok || hex.EncodeToString(sum[:]) != want {
			return nil, seen, errCorrupted
		}
		delete(expected, h.Name)
		seen++
		if visit != nil {
			if err := visit(h.Name, body, manifest); err != nil {
				return nil, seen, err
			}
		}
	}
	if manifest == nil || len(expected) != 0 {
		return nil, seen, errCorrupted
	}
	return manifest, seen, nil
}
