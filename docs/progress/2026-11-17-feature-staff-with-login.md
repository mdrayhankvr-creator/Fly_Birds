# Staff profile and login creation

Before this change, an administrator created a login in Users and a teacher
profile in Staff as separate actions, then linked the records. A failure or
interruption between those actions could leave an orphan account or profile.

Added an administrator-only **Create staff with login** form for teachers and
head teachers. It accepts existing employee fields, qualifications, one
optional teaching assignment from an existing class-subject mapping, and an
optional validated photo. The administrator confirms the employee ID, login
email, role and name before submitting. The server rejects either protected
owner address and does not alter existing employees or accounts.

The endpoint creates the login, linked employee, optional photo and optional
assignment in one PostgreSQL transaction. It reuses the user creation helper
for grants, audit, temporary-password expiry and forced first-login change.
The password is generated only for a confirmed create request, stored only as
a bcrypt hash, excluded from audit and logs, and returned once in a `no-store`
response. The UI keeps it only in component state, offers the existing print
slip, and removes it when the administrator acknowledges saving it. Existing
versioned staff-photo queries supply the dashboard and profile avatars.

Verification:

- Fresh `fbqa-cp1b-testdb` database: new `TestPostgresStaffWithLogin` passed
  anonymous/teacher/head denials, role and protected-address rejection,
  duplicate email/code, photo validation, transaction rollback after an
  incompatible assignment, credential hashing and audit privacy, forced
  password change, and linked photo access. Existing staff link, lifecycle
  and profile-privacy integration tests also passed (4/4).
- Existing `TestPostgresCheckpointFourLifecycle` passed on the same fresh
  database, covering the ordinary Users account-creation path.
- `go vet` on staff, users and httpapi; staff and auth unit tests; frontend
  typecheck, build, i18n and 22 unit tests all passed.
- `e2e/staff-with-login.mjs` passed 26/26 checks on a fresh disposable
  `fbqa-cp1b` demo database: teacher/head creation, one-time credential,
  forced change, photo and initials fallback, initial assignment, 375px EN/BN
  and dark layout, and no browser JavaScript errors.

The first integration run used an exact timestamp comparison that ignored
PostgreSQL's timestamp precision; that test assertion was corrected. The
first browser run targeted an explicit submit-button type absent from the
existing forced-change form; its selector was corrected. Final runs passed.

No migration was needed. The `fbqa-cp1b` backend was restored to its original
database connection. Disposable test databases were left intact because this
task prohibits deleting Docker data. No owner account, live school, subject
choice, admission or result record was changed.
