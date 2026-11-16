# School setup preview — 2026-11-17

Created `feature/school-setup-preview` from freshly fetched `origin/main`
after confirming a clean working tree. No fbshow, Cloudflare or Joy container,
volume, school setting, account or data was modified.

Read-only fbshow snapshot: Khulna Zilla School, active session 2026; 12 active
sections, each with zero students; 22 active subjects and 72 class-subject
mappings; Friday and Saturday off; no teacher assignments or class routines.
The one teacher profile, TEACHER-01 (Md Rayhan Chowdhury), is active, has a
photo, and has no linked account. Both protected administrator emails exist
as active admins. Existing case-insensitive admission, employee-code and
account-email duplicate checks each returned zero.

The preview command calculates 180 proposed student additions and 14 proposed
teacher profiles from this snapshot. It reports existing fourth-subject
mappings, missing choices, timetable inputs and candidate identifier conflicts
from an optional identifiers-only manifest. Class Ten has no optional mapping;
class Six includes Religion as optional. It never infers student gender,
generates credentials, or changes school identity or settings.

The implementation uses a PostgreSQL `REPEATABLE READ, READ ONLY` transaction
and has no apply flag. A disposable database test verifies section arithmetic,
teacher preservation, optional warnings, identifier conflicts, both protected
admin rows before and after preview, and PostgreSQL's rejection of an attempted
admin update in the same read-only transaction. CLI tests reject an apply flag
and a manifest containing a password field without echoing its value.

This milestone intentionally contains no student, teacher, account, routine,
optional-choice or credential creation. Future apply work must independently
enforce the protected-admin exclusions and obtain school policies first.

Verification: `go test -p 1 ./... -count=1` passed with no test database URL
(database-dependent tests skipped), followed by `go build -p 1 ./...` and
`go vet -p 1 ./...`, both passing. Focused tests passed with
`AUTH_TEST_DATABASE_URL` pointed to a fresh, local disposable
`school_setup_preview_test_8d4689f35040` database on `m15-testdb`.
The integration test checks that this database is empty before migrating it.
The command also produced the expected JSON on an isolated fixture database:
three fixture sections, 44 additions, 14 teacher additions, two protected
admins excluded, read-only true, credentials-generated false. No browser or
frontend tests were needed because this milestone adds no UI.

## CI isolation fix

PR #62 exposed a mismatch: the backend job sets `AUTH_TEST_DATABASE_URL` to
its shared `flybirds_ci` database, while the preview integration test requires
a fresh database named `school_setup_preview_test_*`. The test guard is
intentional. The backend job now creates a separate, run-specific database in
its own PostgreSQL service and passes its URL through
`SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL`. The preview test refuses the shared
URL, unsafe names or hosts, and any database with existing user relations or
functions before migration. Frontend and release-stack jobs are unchanged.

Local CI reproduction: `go test ./... -p 1 -count=1` passed with
`AUTH_TEST_DATABASE_URL` pointed at newly created disposable
`flybirds_ci_local_714edb12b3` and
`SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL` pointed at separate fresh
`school_setup_preview_test_714edb12b3` on `m15-testdb`. The focused preview
and command tests passed again on fresh
`school_setup_preview_test_8fad67512665` after tightening the name guard.
`go vet ./...` passed. The workflow parsed as YAML with all three jobs, and
the frontend and release-stack job text was byte-for-byte unchanged after
normalizing line endings. Remote PR CI has not been rerun because this branch
was not pushed.
