# Jalalabad showcase clean slate

This procedure is for the owner **after review, merge, and deployment**. No
reset or restore is part of development verification on `fbshow` (port 8094).
The English school name in the 2026-11-17 backup is exactly
`Jalalabad Cantonment Public School & Collage`.

## What the command does

`reset-school-data` prints counts for every table before it can delete. It
preserves the migration defaults and school identity: school settings and
configured logo, public page, slideshow and gallery, grading, weekly off
days, module switches, permissions and other default settings. It empties
people, users and sessions, academics, exams and results, finance, attendance,
notices, attachments and the old audit trail. It creates exactly two active
administrators with interactive passwords and `must_change_password=true`.
The first new audit event records the reset. It checks foreign keys, including
composite keys, afterwards. Attachment files are removed after the database
transaction commits; any deletion errors are reported for manual cleanup.

The real command requires a matching school name, a newly made application
archive with an independently supplied SHA-256, a current schema, disabled
demo quick-login, an interactive terminal, `-confirm`, and typing the exact
school name. It verifies the archive's checksum, contents, school name, schema
version and age. It locks all tables and rejects a changed row-count plan.
The dry run uses read-only schema checking and does not apply migrations.
The reset policy is pinned to the 57 reviewed migrations through version
1813 and the 114 reviewed tables; a later migration or unexpected table
requires a deliberate policy review before reset.

## Install after review

The source of truth is `deploy/fbshow-clean-slate.ps1`,
`deploy/fbshow-quick-login-off.override.yaml`, and
`deploy/install-fbshow-clean-slate.ps1` in this branch. The installer copies
the first two beside fbshow's external `common.ps1` and changes only the
`$Override` assignment in `common.ps1`, backing up that script first. It does
not change `fbshow.env`, `demo_password`, the old override, or any Docker
volume. This makes future `update.ps1` and `restore.ps1` runs use the off
override too. Review the installer diff before running it.

```powershell
$repo = 'C:\Users\Laptop Click\Fly_Birds'
$show = 'C:\Users\Laptop Click\Fly_Birds_QA_env\fbshow'
powershell -NoProfile -ExecutionPolicy Bypass -File "$repo\deploy\install-fbshow-clean-slate.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -File "$show\update.ps1"
```

`update.ps1` should run only after the reviewed change reaches `origin/main`;
it builds the new backend and recreates it with quick-login disabled. Check
the current deployment and its existing backups before proceeding.

## Owner dry run and reset

```powershell
$show = 'C:\Users\Laptop Click\Fly_Birds_QA_env\fbshow'
$school = 'Jalalabad Cantonment Public School & Collage'
powershell -NoProfile -ExecutionPolicy Bypass -File "$show\clean-slate.ps1" -School $school
powershell -NoProfile -ExecutionPolicy Bypass -File "$show\clean-slate.ps1" -Execute -School $school
```

Read the table counts. For the real run, the wrapper prompts for two distinct
`Name:email` identities. It makes the existing `pg_dump` plus uploads backup
in `backups\<time>-pre-clean-slate`, verifies both archives, makes a second
application archive, copies that archive outside the Docker volume and checks
its SHA-256. Only then does it stop the web and backend, invoke the command,
and restart them with demo quick-login off. The Go command asks for the exact
school name and both passwords twice without echoing them. The wrapper prints
the rollback folder name. Keep that folder and its `sha256.txt` receipt.

## Owner rollback

Rollback replaces the current showcase database, uploads and code with the
chosen pre-reset backup. Stop if important new records have been entered
since the reset; decide how to preserve them first. `restore.ps1` first saves
the current state as a separate `pre-restore` backup.

```powershell
$show = 'C:\Users\Laptop Click\Fly_Birds_QA_env\fbshow'
$backup = Get-ChildItem "$show\backups" -Directory | Where-Object Name -Like '*-pre-clean-slate' | Sort-Object Name -Descending | Select-Object -First 1
if (-not $backup) { throw 'No pre-clean-slate backup found' }
powershell -NoProfile -ExecutionPolicy Bypass -File "$show\restore.ps1" -Backup $backup.Name
```

Do not use `down -v`, `-force` on a different database, or restore a rehearsal
archive into the showcase. The rehearsal used a new database in `m15-testdb`
and copied an existing fbshow backup into that disposable container.
