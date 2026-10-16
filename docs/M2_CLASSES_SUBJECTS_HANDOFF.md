# M2 Task 001: classes and subjects handoff

This branch contains session-independent class, subject, and class-subject code. Joy owns academic sessions on `feature/m2-academic-sessions-joy`. Her migration number and filename were not available when this checkpoint was prepared, so this branch does **not** add an academic migration file or edit the migration runner's ordered list. No academic table is created by this branch. The API service stays unavailable until its schema exists, and the admin links remain hidden without the new permissions.

## Schema contract for the coordinated migration

After Joy's migration number is confirmed, reserve the next unique version for this branch and add one forward-only SQL migration. The proposed tables are independent of `academic_sessions`:

| Table | Columns | Constraints and indexes |
|---|---|---|
| `academic_classes` | `id uuid DEFAULT gen_random_uuid()`, `name text`, `display_order integer`, `is_active boolean DEFAULT true`, `created_at timestamptz`, `updated_at timestamptz` | UUID primary key; trimmed nonempty name, max 120 UTF-8 bytes; nonnegative order up to 100000; unique index on `lower(name)`; index on `(display_order, name, id)` and active status |
| `academic_subjects` | `id uuid DEFAULT gen_random_uuid()`, `name_en text`, `name_bn text`, `code text`, `is_active boolean DEFAULT true`, timestamps | UUID primary key; both names nonempty, max 160 UTF-8 bytes; code matches uppercase `[A-Z0-9][A-Z0-9_-]{0,31}`; unique index on `upper(code)`; index on active status |
| `academic_class_subjects` | `id uuid DEFAULT gen_random_uuid()`, `class_id uuid`, `subject_id uuid`, `is_required boolean`, `is_fourth_subject boolean`, timestamps | UUID primary key; foreign keys to classes and subjects with `ON DELETE RESTRICT`; unique `(class_id, subject_id)`; `CHECK (is_required <> is_fourth_subject)`; index on `(class_id, subject_id)` and on `subject_id` |

The migration should add `academics:read` and `academics:manage` to `permissions` and grant both to the administrator role. Existing `module_definitions` already contains the `academics` key and its default is disabled. Do not add a session foreign key. In this model, “fourth/optional” is one designation: exactly one of `is_required` and `is_fourth_subject` must be true. A future session-specific assignment must reference these stable class and subject IDs rather than change their meaning.

## Integration steps

1. Confirm Joy's migration version and merge order. Assign a unique subsequent filename to this schema, then add it to `backend/internal/database/migrate.go` after her filename. Resolve the hard-coded migration count assertions in existing M1 integration tests to the new count.
2. Apply both migrations to a fresh database and an upgraded M1 database. Check unique indexes, foreign keys, the flag check, permission grants, and the unchanged M1 data.
3. Run `go vet ./...` and `go test ./... -count=1` with `AUTH_TEST_DATABASE_URL` pointing to a dedicated PostgreSQL database. `TestPostgresAcademicLifecycle` currently skips when its schema is absent; it must pass after the migration is registered.
4. Enable the academics module in school settings and verify the admin pages at desktop and mobile widths, the module-disabled 403 path, and the complete OpenAPI route list.

Sections, academic-session assignments, student and teacher profiles, attendance, and exams are outside this checkpoint.
