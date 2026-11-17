# Preview-first synthetic student admission

Created `feature/student-bulk-admission` from `origin/main` in an isolated Joy
worktree. The original `feature/qa-sweep` checkout and other worktrees were
preserved. No owner-managed school or `fbshow` record was accessed.

The existing school-setup manifest's TEST identifier and 15-minus-existing
logic now has an Admin-only student apply path. A read-only preview lists
current section counts, proposed admission numbers, available rolls and login
IDs, an approval fingerprint, and incomplete personal/guardian/subject data.
Specific section and identifier conflicts are returned for review. Apply
requires explicit confirmation and a UUID retry key. It locks relevant tables,
recounts and compares the fingerprint immediately before writing, then creates
synthetic student rows, linked student accounts and enrolments atomically.
Existing students, protected Admin accounts, school settings, guardians and
fourth-subject choices are not changed. Migration 1765 stores only the retry
key, fingerprint and count; it stores no credentials.

Each new account receives a unique random temporary password and a reserved
`.invalid` email, signs in with its admission number, and must change the
password at first login. Only a hash is stored. The authenticated first
response offers bilingual printable slips and a user-triggered CSV download;
retries return counts only. A lost/expired credential requires the existing
authorized password-reset flow. No password is written to audit metadata,
application logs, browser localStorage, test output or CI artifacts.

Verification:

- Fresh disposable PostgreSQL integration test: 15-minus-existing and exactly
  15 per section, role and grant enforcement, collision details, stale preview,
  forced rollback after the second account insert, enrolment links, protected
  Admin rows, hash/expiry/forced change, audit privacy and idempotent replay.
- `go vet ./...` and `go test -p 1 ./...` passed. OpenAPI coverage initially
  identified missing route definitions and 401 responses; both were added,
  and the final full run passed.
- Frontend typecheck, production build, EN/BN i18n check and 22 unit tests
  passed.
- Final `e2e/student-setup.mjs` passed 23/23 checks on a fresh disposable
  `fbqa-cp1b` demo database, including UI confirmation, every section at 15,
  retry without credentials, first-login password change, and 375px EN/BN
  dark layout without browser errors.

After browser testing, `fbqa-cp1b` was rebuilt from the unchanged
main-equivalent worktree and reconnected to its original QA database; readiness
returned HTTP 200. Disposable test databases were left intact because Docker
data deletion is prohibited. A future live setup requires a fresh preview and
the owner's explicit approval; the prior 180-student estimate was not assumed.
