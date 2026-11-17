# Staff account linking and avatar verification

An employee photo belongs to the employee record. `OwnAvatar` resolves a
teacher's employee through `employees.user_id`; an unlinked employee therefore
falls back to initials even when `employee_photos` already contains a photo.

Added an administrator-only link action to an existing teacher profile. The
administrator searches an active teacher or head-teacher login by exact email,
confirms the employee ID, and reviews both records before linking. The backend
checks both permissions, the active roles, exact email and employee ID, and
unique availability in one transaction; it writes an audit event. It creates
no account and changes no password. The linked account email and role are
visible only to administrators on the profile. The teacher staff form points
administrators to this flow instead of asking for a raw user UUID.

The existing versioned staff photo endpoint and 20-second own-avatar metadata
refresh supply the header and dashboard photo after linking and after a later
photo replacement. Initials remain the fallback when no photo exists.

Verification:

- Fresh `fbqa-cp1b-testdb` database: `TestPostgresStaffAccountLink` passed
  authorization, role mismatch (including an administrator account), identity
  mismatch, duplicate link, head-teacher link, profile privacy, and existing
  photo becoming the own avatar. The existing `TestPostgresStaffLifecycle` and
  `TestPostgresStaffProfilePrivacy` also passed on the same fresh database.
- `go test -p 1 ./internal/staff -count=1` and `go vet ./internal/staff
  ./internal/httpapi` passed.
- Frontend `tsc --noEmit`, `npm run build`, `npm run
  check:i18n`, and `npm run test:unit` passed (22 unit tests).
- Isolated browser run on a freshly seeded disposable `fbqa-cp1b` database:
  `e2e/staff-account-link.mjs` passed 12/12 checks, including 360px layout,
  linked profile, header and dashboard avatar, and replacement photo appearing
  on the open dashboard without reload.
- The QA stack was restored to its original database connection. Only the
  disposable databases created for these tests were dropped. No migration was
  needed.
