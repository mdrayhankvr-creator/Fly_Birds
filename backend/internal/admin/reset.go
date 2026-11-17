package admin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
)

// Giving a school a clean slate: every person and every record of school life
// goes, while the school's own identity and configuration stay, and two
// administrators are created to set it up again.
//
// What stays is not a hand-written list of favourites. It is, with three
// additions, exactly the set of tables the schema itself fills in: a table
// that a fresh migration leaves with rows holds reference or configuration
// data (the roles, the permission matrix, the module list, the grading scale,
// the settings rows every page expects to find), and emptying it would leave
// the application looking for rows that are not there. The three additions
// are the school's own uploads that belong to its identity: the logo, the
// landing slideshow and the public gallery.
//
// Everything else is school life and goes: accounts, people, the academic
// structure, exams, results, documents, fees, attendance, messages and the
// audit trail of all of it.

var (
	// ErrResetSchoolMismatch means the school in the database is not the one
	// the caller named.
	ErrResetSchoolMismatch = errors.New("the school in this database is not the one named")
	// ErrResetNotNullReference means a kept row points at something being
	// deleted through a column that cannot be emptied. The schema has grown a
	// link this code does not know how to keep, and a human has to decide.
	ErrResetNotNullReference = errors.New("a kept table has a NOT NULL reference to a table being emptied")
	// ErrResetAdmins means the administrators to create are not usable.
	ErrResetAdmins = errors.New("exactly two administrators with a name, an address and a password are required")
	// ErrResetSchemaUnreviewed stops a future migration from silently changing
	// which school records this command preserves or deletes.
	ErrResetSchemaUnreviewed = errors.New("school reset policy has not been reviewed for this schema")
)

// Migration 1814 adds class_fourth_subject_policies. It is empty after a
// fresh migration and belongs to the academic structure, so a school reset
// empties it with classes, mappings and student choices.
const resetReviewedSchemaVersion = 1814
const resetReviewedMigrationCount = 58
const resetReviewedTableCount = 115

// Student setup migration 1765 adds one unseeded retry table. A school reset
// must empty it with students and accounts. Keep this delta separate so the
// reviewed base can advance independently (for example migration 1814).
const studentSetupReviewedDelta = 1

// resetKeepExtra are the school's own uploads that belong to its identity.
// They are empty in a fresh schema, so they have to be named.
var resetKeepExtra = []string{"school_logo", "school_slides", "school_gallery"}

// ResetAdmin is one administrator to create. The password arrives already
// hashed, so a plain password never reaches this package.
type ResetAdmin struct {
	Name         string
	Email        string
	PasswordHash string
}

// TableRows is a table and how many rows it holds.
type TableRows struct {
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

// NulledColumn is a reference from a kept row into something being deleted;
// the reset empties it.
type NulledColumn struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	Points string `json:"points_at"`
}

// ResetPlan is what a reset would do. Building one reads but never writes, so
// it is also the dry run.
type ResetPlan struct {
	SchoolNameEN string         `json:"school_name_en"`
	SchoolNameBN string         `json:"school_name_bn"`
	Keep         []TableRows    `json:"keep"`
	Empty        []TableRows    `json:"empty"`
	Nulled       []NulledColumn `json:"nulled"`
	// Rows is how many rows the reset deletes, and Files how many stored
	// files (result PDFs and attachments) go with them.
	Rows  int64 `json:"rows"`
	Files int   `json:"files"`
	Users int64 `json:"users"`
}

// Counted lists only the tables that actually hold rows, which is what an
// operator wants to read before confirming.
func (p ResetPlan) Counted() []TableRows {
	out := make([]TableRows, 0, len(p.Empty))
	for _, t := range p.Empty {
		if t.Rows > 0 {
			out = append(out, t)
		}
	}
	return out
}

// ResetResult is what a reset did.
type ResetResult struct {
	Emptied int   `json:"tables_emptied"`
	Rows    int64 `json:"rows_deleted"`
	Files   int   `json:"files_deleted"`
	Admins  int   `json:"admins_created"`
	// FileErrors counts stored files that could not be removed. The rows are
	// gone either way, so this never fails the reset; it is reported so the
	// leftovers can be cleared by hand.
	FileErrors int `json:"file_errors"`
}

// resetSeeded is the set of tables a freshly migrated database leaves
// non-empty. TestResetKeepsWhatTheSchemaSeeds checks it against a real
// migration, so a new migration that seeds a table fails the test until
// someone decides whether a clean slate should keep it.
var resetSeeded = []string{
	"academic_groups",
	"admit_card_settings",
	"analytics_settings",
	"diary_settings",
	"grading_bands",
	"grading_schemes",
	"id_card_settings",
	"module_definitions",
	"payment_settings",
	"permissions",
	"result_comment_presets",
	"role_permissions",
	"roles",
	"schema_migrations",
	"school_modules",
	"school_public_features",
	"school_public_page",
	"school_settings",
	"school_weekly_off_days",
}

// resetKeepSet is every table a reset leaves alone.
func resetKeepSet() map[string]bool {
	keep := map[string]bool{}
	for _, t := range resetSeeded {
		keep[t] = true
	}
	for _, t := range resetKeepExtra {
		keep[t] = true
	}
	return keep
}

// publicTables lists the schema's tables.
func publicTables(ctx context.Context, db *gorm.DB) ([]string, error) {
	var names []string
	err := db.WithContext(ctx).Raw(
		"SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&names).Error
	return names, err
}

// PlanReset works out what a reset would do, without writing anything.
// schoolEN must match the school's English name unless anySchool is set; it is
// the one guard that cannot be got wrong by pointing the command at the wrong
// database.
func PlanReset(ctx context.Context, db *gorm.DB, schoolEN string, anySchool bool) (ResetPlan, error) {
	var plan ResetPlan
	var version, migrations int
	if err := db.WithContext(ctx).Raw("SELECT COALESCE(max(version), 0), count(*) FROM schema_migrations").Row().Scan(&version, &migrations); err != nil {
		return plan, err
	}
	var studentSetupMigration int
	if err := db.WithContext(ctx).Raw("SELECT count(*) FROM schema_migrations WHERE version = 1765").Row().Scan(&studentSetupMigration); err != nil {
		return plan, err
	}
	if version != resetReviewedSchemaVersion || migrations != resetReviewedMigrationCount+studentSetupReviewedDelta || studentSetupMigration != 1 {
		return plan, fmt.Errorf("%w: reviewed %d migrations through %d; database has %d through %d", ErrResetSchemaUnreviewed,
			resetReviewedMigrationCount+studentSetupReviewedDelta, resetReviewedSchemaVersion, migrations, version)
	}
	row := db.WithContext(ctx).Raw("SELECT name_en, name_bn FROM school_settings ORDER BY id LIMIT 1").Row()
	if err := row.Scan(&plan.SchoolNameEN, &plan.SchoolNameBN); err != nil {
		return plan, fmt.Errorf("read the school's name: %w", err)
	}
	if !anySchool && !strings.EqualFold(strings.TrimSpace(schoolEN), strings.TrimSpace(plan.SchoolNameEN)) {
		return plan, fmt.Errorf("%w: this database holds %q", ErrResetSchoolMismatch, plan.SchoolNameEN)
	}
	tables, err := publicTables(ctx, db)
	if err != nil {
		return plan, err
	}
	studentSetupTable := false
	for _, name := range tables {
		if name == "student_setup_runs" {
			studentSetupTable = true
			break
		}
	}
	if len(tables) != resetReviewedTableCount+studentSetupReviewedDelta || !studentSetupTable {
		return plan, fmt.Errorf("%w: reviewed %d tables including student_setup_runs; database has %d", ErrResetSchemaUnreviewed, resetReviewedTableCount+studentSetupReviewedDelta, len(tables))
	}
	keep := resetKeepSet()
	for _, name := range tables {
		if !tableName.MatchString(name) {
			return plan, fmt.Errorf("unexpected table name %q; reset needs review", name)
		}
		var rows int64
		if err := db.WithContext(ctx).Raw(`SELECT count(*) FROM "` + name + `"`).Row().Scan(&rows); err != nil {
			return plan, fmt.Errorf("count %s: %w", name, err)
		}
		if keep[name] {
			plan.Keep = append(plan.Keep, TableRows{Table: name, Rows: rows})
			continue
		}
		plan.Empty = append(plan.Empty, TableRows{Table: name, Rows: rows})
		plan.Rows += rows
		if name == "users" {
			plan.Users = rows
		}
	}
	if plan.Nulled, err = keptReferences(ctx, db, keep); err != nil {
		return plan, err
	}
	keys, err := storedKeys(ctx, db)
	if err != nil {
		return plan, err
	}
	plan.Files = len(keys)
	return plan, nil
}

// keptReferences finds every column on a kept table that points into a table
// being emptied. Such a column is emptied first, so the kept row survives
// without pointing at something that is gone. A column that cannot be emptied
// is refused: the schema has grown a link this code was not written for.
func keptReferences(ctx context.Context, db *gorm.DB, keep map[string]bool) ([]NulledColumn, error) {
	var found []struct {
		Child   string
		Column  string
		Parent  string
		NotNull bool
	}
	err := db.WithContext(ctx).Raw(`
SELECT c.conrelid::regclass::text AS child, a.attname AS column, c.confrelid::regclass::text AS parent, a.attnotnull AS not_null
FROM pg_constraint c
JOIN unnest(c.conkey) WITH ORDINALITY k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace`).Scan(&found).Error
	if err != nil {
		return nil, err
	}
	out := []NulledColumn{}
	for _, f := range found {
		if !keep[f.Child] || keep[f.Parent] {
			continue
		}
		if f.NotNull {
			return nil, fmt.Errorf("%w: %s.%s → %s", ErrResetNotNullReference, f.Child, f.Column, f.Parent)
		}
		out = append(out, NulledColumn{Table: f.Child, Column: f.Column, Points: f.Parent})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].Column < out[j].Column
	})
	return out, nil
}

// storedKeys are the files in the attachment store that belong to rows the
// reset deletes: the result PDFs and the uploaded attachments. Photos, logos
// and signatures are held in the database itself, so they go with their rows.
func storedKeys(ctx context.Context, db *gorm.DB) ([]string, error) {
	var keys []string
	if err := db.WithContext(ctx).Raw(
		"SELECT storage_key FROM result_documents WHERE storage_key IS NOT NULL AND storage_key <> ''").Scan(&keys).Error; err != nil {
		return nil, err
	}
	var more []string
	if err := db.WithContext(ctx).Raw(
		"SELECT storage_key FROM file_objects WHERE storage_key <> ''").Scan(&more).Error; err != nil {
		return nil, err
	}
	return append(keys, more...), nil
}

// Reset empties the school and creates its new administrators, in one
// transaction: either the school comes out clean with two administrators who
// must choose their own password, or nothing changes at all.
//
// store may be nil, in which case the stored files are left behind and
// counted as errors.
func Reset(ctx context.Context, db *gorm.DB, store files.Store, plan ResetPlan, admins []ResetAdmin) (ResetResult, error) {
	var out ResetResult
	if len(admins) != 2 {
		return out, ErrResetAdmins
	}
	for _, a := range admins {
		if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Email) == "" || a.PasswordHash == "" {
			return out, ErrResetAdmins
		}
	}
	if admins[0].Email == admins[1].Email {
		return out, fmt.Errorf("%w: both administrators have the same address", ErrResetAdmins)
	}
	var keys []string
	now := time.Now().UTC()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock every table before checking the plan again. A stale dry run must
		// never be applied to a database that changed while the operator read it.
		tables, err := publicTables(ctx, tx)
		if err != nil {
			return err
		}
		locked := make([]string, 0, len(tables))
		for _, name := range tables {
			if !tableName.MatchString(name) {
				return fmt.Errorf("unexpected table name %q; reset needs review", name)
			}
			locked = append(locked, `"`+name+`"`)
		}
		if err := tx.Exec("LOCK TABLE " + strings.Join(locked, ", ") + " IN ACCESS EXCLUSIVE MODE NOWAIT").Error; err != nil {
			return fmt.Errorf("school is in use; stop writers and retry: %w", err)
		}
		current, err := PlanReset(ctx, tx, plan.SchoolNameEN, false)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, plan) {
			return errors.New("school data changed after the dry run; review a new plan before retrying")
		}
		keys, err = storedKeys(ctx, tx)
		if err != nil {
			return err
		}
		empty := make([]string, 0, len(plan.Empty))
		for _, t := range plan.Empty {
			// users is deleted last so kept attribution columns use ON DELETE SET NULL.
			if t.Table != "users" {
				empty = append(empty, `"`+t.Table+`"`)
			}
		}
		if len(empty) == 0 {
			return errors.New("nothing to empty: the plan is empty")
		}
		for _, n := range plan.Nulled {
			if err := tx.Exec(fmt.Sprintf(`UPDATE "%s" SET "%s" = NULL WHERE "%s" IS NOT NULL`, n.Table, n.Column, n.Column)).Error; err != nil {
				return fmt.Errorf("empty %s.%s: %w", n.Table, n.Column, err)
			}
		}
		// One statement, and deliberately no CASCADE: if a table outside the
		// list still points at one inside it, PostgreSQL refuses and the
		// whole reset is called off rather than quietly emptying something
		// that was meant to be kept.
		if err := tx.Exec("TRUNCATE TABLE " + strings.Join(empty, ", ") + " RESTART IDENTITY").Error; err != nil {
			return fmt.Errorf("empty the school's records: %w", err)
		}
		if err := tx.Exec("DELETE FROM users").Error; err != nil {
			return fmt.Errorf("delete the accounts: %w", err)
		}
		for _, a := range admins {
			if err := tx.Exec(`INSERT INTO users (email, display_name, password_hash, role, is_active, must_change_password)
VALUES (?, ?, ?, 'admin', true, true)`, a.Email, strings.TrimSpace(a.Name), a.PasswordHash).Error; err != nil {
				return fmt.Errorf("create the administrator %s: %w", a.Email, err)
			}
			out.Admins++
		}
		// The first entry in the school's new audit log says what happened to
		// the old one. No actor: this is the operator at the server, not a
		// signed-in user.
		var schoolID string
		if err := tx.Raw("SELECT id FROM school_settings ORDER BY id LIMIT 1").Row().Scan(&schoolID); err != nil {
			return fmt.Errorf("read the school: %w", err)
		}
		out.Emptied = len(empty) + 1
		return audit.Record(tx, nil, "school.data_reset", "school", schoolID, map[string]any{
			"source": "reset-school-data", "tables_emptied": len(empty) + 1,
			"rows_deleted": plan.Rows, "admins_created": out.Admins,
			"at": now.Format(time.RFC3339),
		})
	})
	if err != nil {
		return ResetResult{}, err
	}
	out.Rows = plan.Rows
	// The rows are gone and committed; the files they named are removed now.
	// A file that will not go is reported, never fatal: a leftover blob that
	// nothing points at is untidy, not dangerous.
	for _, key := range keys {
		if store == nil {
			out.FileErrors++
			continue
		}
		if err := store.Delete(ctx, key); err != nil {
			out.FileErrors++
			continue
		}
		out.Files++
	}
	return out, nil
}

// ResetOrphans reports any row left pointing at a row that is not there. A
// reset that respected every foreign key leaves none, and the command says so
// rather than asking anyone to take it on trust.
func ResetOrphans(ctx context.Context, db *gorm.DB) ([]string, error) {
	var fks []struct {
		ConstraintID string
		Child        string
		Column       string
		Parent       string
		Target       string
		MatchType    string
	}
	err := db.WithContext(ctx).Raw(`
SELECT c.oid::text AS constraint_id, child.relname AS child, a.attname AS column,
       parent.relname AS parent, f.attname AS target, c.confmatchtype::text AS match_type
FROM pg_constraint c
JOIN pg_class child ON child.oid = c.conrelid
JOIN pg_class parent ON parent.oid = c.confrelid
JOIN unnest(c.conkey) WITH ORDINALITY k(attnum, ord) ON true
JOIN unnest(c.confkey) WITH ORDINALITY fk(attnum, ord) ON fk.ord = k.ord
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
JOIN pg_attribute f ON f.attrelid = c.confrelid AND f.attnum = fk.attnum
WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace
ORDER BY c.oid, k.ord`).Scan(&fks).Error
	if err != nil {
		return nil, err
	}
	out := []string{}
	for i := 0; i < len(fks); {
		j := i + 1
		for j < len(fks) && fks[j].ConstraintID == fks[i].ConstraintID {
			j++
		}
		group := fks[i:j]
		fk := group[0]
		columns := make([]string, 0, len(group))
		present := make([]string, 0, len(group))
		joins := make([]string, 0, len(group))
		for _, part := range group {
			columns = append(columns, part.Column)
			present = append(present, fmt.Sprintf(`c.%s IS NOT NULL`, quoteResetIdent(part.Column)))
			joins = append(joins, fmt.Sprintf(`p.%s = c.%s`, quoteResetIdent(part.Target), quoteResetIdent(part.Column)))
		}
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM %s c WHERE %s
 AND NOT EXISTS (SELECT 1 FROM %s p WHERE %s)`, quoteResetIdent(fk.Child), strings.Join(present, " AND "), quoteResetIdent(fk.Parent), strings.Join(joins, " AND "))
		if err := db.WithContext(ctx).Raw(q).Row().Scan(&n); err != nil {
			return nil, fmt.Errorf("check %s.%s: %w", fk.Child, strings.Join(columns, ","), err)
		}
		if fk.MatchType == "f" && len(group) > 1 {
			partial := fmt.Sprintf(`SELECT count(*) FROM %s c WHERE (%s) AND NOT (%s)`, quoteResetIdent(fk.Child), strings.Join(present, " OR "), strings.Join(present, " AND "))
			var partialCount int64
			if err := db.WithContext(ctx).Raw(partial).Row().Scan(&partialCount); err != nil {
				return nil, err
			}
			n += partialCount
		}
		if n > 0 {
			out = append(out, fmt.Sprintf("%s.%s → %s: %d orphan row(s)", fk.Child, strings.Join(columns, ","), fk.Parent, n))
		}
		i = j
	}
	sort.Strings(out)
	return out, nil
}

func quoteResetIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// HashResetPassword turns a typed password into a stored hash. It is here so
// the command does not need the auth package's rules of its own.
func HashResetPassword(password string) (string, error) { return auth.HashPassword(password) }

// NormalizeResetEmail checks and lowercases an administrator's address.
func NormalizeResetEmail(raw string) (string, error) { return auth.NormalizeEmail(raw) }
