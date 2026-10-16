# M2 Task 001: classes and subjects handoff

Task 001 prepared session-independent class, subject, and class-subject code while Joy owned Academic Sessions. Task 002 verified Joy's commit `0bb712d` in `origin/main`, merged that history normally, and added `0102_classes_subjects.sql` after her `0101_academic_sessions.sql`. This document records the schema contract and its local integration results. The branch remains local and has not been pushed.

## Schema contract for the coordinated migration

The implemented forward-only migration uses version 102. Its tables are independent of `academic_sessions`:

| Table | Columns | Constraints and indexes |
|---|---|---|
| `academic_classes` | `id uuid DEFAULT gen_random_uuid()`, `name text`, `display_order integer`, `is_active boolean DEFAULT true`, `created_at timestamptz`, `updated_at timestamptz` | UUID primary key; trimmed nonempty name, max 120 UTF-8 bytes; nonnegative order up to 100000; unique index on `lower(name)`; index on `(display_order, name, id)` and active status |
| `academic_subjects` | `id uuid DEFAULT gen_random_uuid()`, `name_en text`, `name_bn text`, `code text`, `is_active boolean DEFAULT true`, timestamps | UUID primary key; both names nonempty, max 160 UTF-8 bytes; code matches uppercase `[A-Z0-9][A-Z0-9_-]{0,31}`; unique index on `upper(code)`; index on active status |
| `academic_class_subjects` | `id uuid DEFAULT gen_random_uuid()`, `class_id uuid`, `subject_id uuid`, `is_required boolean`, `is_fourth_subject boolean`, timestamps | UUID primary key; foreign keys to classes and subjects with `ON DELETE RESTRICT`; unique `(class_id, subject_id)`; `CHECK (is_required <> is_fourth_subject)`; index on `(class_id, subject_id)` and on `subject_id` |

The migration idempotently seeds the `academics` module definition and disabled school module state, adds `academics:read` and `academics:manage`, grants both to administrators, and grants read to head teachers. It adds no session foreign key. In this model, “fourth/optional” is one designation: exactly one of `is_required` and `is_fourth_subject` must be true. A future session-specific assignment should reference these stable class and subject IDs rather than change their meaning.

## Task 002 integration result

1. `git merge-base --is-ancestor 0bb712d origin/main` succeeded. The merge preserved both developers' commits and left Joy's migration unchanged.
2. `backend/internal/database/migrate.go` discovers versioned SQL files by numeric prefix, so adding `0102_classes_subjects.sql` registered it without a hard-coded runner edit. Fresh and M1-upgrade disposable databases both reached versions 1–4, 101, 102.
3. Dedicated PostgreSQL-backed HTTP tests passed for validation, uniqueness, foreign keys, exclusive flags, permissions, disabled module behavior, search, pagination, and sessions. `TestPostgresAcademicLifecycle` now requires the installed schema when a test database is configured.
4. Headless Edge passed authenticated academic creation and read flows, read-only head-teacher access, English/Bangla rendering at 1280 px and 390 px, mobile mapping display, and disabled-module navigation. Go vet, the full Go suite, frontend production build, Compose validation, and OpenAPI route/reference validation passed locally. Remote CI and production migration remain unverified.

Sections, academic-session assignments, student and teacher profiles, attendance, and exams are outside this checkpoint.
