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
response offers bilingual printable slips and a user-triggered encrypted
credential file (AES-256-GCM with a separate passphrase); the Admin can open
that file locally on the setup page to obtain a readable CSV;
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

## Follow-up: bilingual TEST profiles and CI reset guard

The follow-up preserves the shortfall, fingerprints, transaction, permission
and idempotency rules. Stable curated English/Bangla name pairs are combined
into fictional TEST names, alternating male/female gender across a batch.
The first 180 generated names are distinct. Student profiles now hold both
names and gender, show a TEST badge in the directory/profile, and leave DOB,
parents, guardian, contacts, address, group and explicit fourth-subject choice
blank. Admission number remains the supported sign-in ID. A configured
fourth-subject policy may infer effective subjects from gender after PR #66 is
merged; no unconfigured default or explicit choice is written here.

The one-time export includes both names, academic session, class, section,
roll, admission number, sign-in ID, expiry and temporary password. The browser
encrypts it with a user-selected passphrase and never stores the passphrase
or plaintext in localStorage. Admins can reopen a saved file locally; if the
file or passphrase is lost, the setup page permanently links to the existing
Admin user-account password reset flow, which reissues a new one-time password
for each affected account. Retrying the admission never re-exposes passwords.

PR #67 backend CI failed in the PostgreSQL test step. GitHub exposes only the
step annotation to anonymous access; `gh` is unavailable here. A fresh local
run with the same CI environment variables reproduced the first real failure:
the school-reset guard reviewed 57 migrations/114 tables, while migration
1765 makes 58/115. The reset policy now verifies 1765 and its unseeded
`student_setup_runs` table and empties it. The adjustment is additive to the
reviewed base, so main's independently reviewed migration 1814 can advance
the base counts without a conflicting edit. A local test-role privilege error
was eliminated by rerunning with the CI service's superuser capability.

Final verification: focused reset tests passed; full `go test -p 1 -count=1
./...` passed with fresh disposable PostgreSQL databases, as did Go build
and vet. Frontend typecheck, build, EN/BN i18n and 23/23 unit tests passed.
On a fresh disposable demo database, `e2e/student-setup.mjs` passed 32/32,
covering encryption/unlock, one-time credentials, bilingual profiles, TEST
badges, the recovery link, section totals and first-login change. The
`fbqa-cp1b` app was restored to its original QA database (57 migrations,
readiness HTTP 200); all test databases were left intact. No live school
records were created.
