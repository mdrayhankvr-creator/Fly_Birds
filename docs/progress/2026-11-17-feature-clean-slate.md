# Clean slate for the Jalalabad showcase — 2026-11-17

Branch: `feature/clean-slate`. The existing implementation arrived in commit
`ec98232c`; the working tree was clean when work resumed. The fixture image
type and version constraints named in the handoff were already corrected in
that commit. No existing changes were discarded.

## Outcome and scope

- The reset keeps the 22 reviewed configuration and identity tables, including
  the configured logo, public page, slides/gallery, grading and weekly off
  days. It deletes school-life records and creates two active administrators
  with forced first-login password changes.
- The dry run no longer applies migrations. Both dry run and real run require
  the current ledger and the reset policy is pinned to 57 reviewed migrations
  through 1813 and 114 tables. The real command requires a fresh archive
  checked against an independent SHA-256, archive contents, school name and schema version;
  rejects demo quick-login; and asks for exact school-name confirmation and
  interactive passwords.
- The transaction takes exclusive locks, recomputes the plan, and rejects
  changed counts before deleting. The FK report covers single and composite
  keys. Attachment cleanup runs after commit and a partial file-cleanup result
  makes the command exit nonzero.
- Versioned `deploy/fbshow-clean-slate.ps1`, installer and quick-login-off
  override work with fbshow's external `common.ps1`, backup and restore
  conventions. The installer backs up and patches only `common.ps1`; it never
  overwrites `fbshow.env`, `demo_password` or the old override. See
  `docs/CLEAN_SLATE.md` for exact owner commands.

## Verification

- A **copy** of `fbshow/backups/20261117-013154-update` was restored into a
  newly created `clean_slate_rehearsal` database and uploads directory inside
  the separate disposable `m15-testdb` container. The rehearsal reset ran
  inside an outer transaction and rolled back. It verified all 22 preserved
  tables, configured logo URL, school identity, public page, seven grading
  bands, two weekly off days, emptying of every deleted table, exactly two
  forced-change admins, and zero FK orphans. The final rehearsal reported
  2,893 candidate rows to delete (including its own archive run records).
  The restored backup had no `school_logo` row; its logo is configured through
  `school_settings.logo_url`.
- The rehearsal also created and verified a native application archive, and
  rejected an incorrect SHA-256 and a different school name. A CLI JSON dry
  run on that copy reported 2,890 rows to delete and left sampled database
  row counts unchanged. CLI attempts to run without a backup or while
  `DEMO_QUICK_LOGIN=true` were refused and left sampled row counts unchanged.
- Seven focused reset tests on `m15-testdb` passed, including stale-plan
  refusal, actual attachment file deletion, composite FK orphan detection,
  and refusals for an unreviewed migration or unexpected table. The separate
  restored-copy rehearsal test passed.
- `go build ./...` and `go vet ./...` passed on the final code. The full Go
  suite passed twice, sequentially, each time on a newly created database in
  `m15-testdb` (`clean_slate_suite_1117` and `clean_slate_final_1117`).
- Frontend `npm exec tsc -- --noEmit`, `npm run build`, `npm run check:i18n`
  (4 tests; 4,058 English / 4,056 Bangla keys), and `npm run test:unit`
  (22 tests) passed. Browser harness `node --test e2e/cdp.test.mjs` passed
  all 8 tests. The PowerShell scripts passed parser checks; the installer
  passed on a temporary copy of `common.ps1`, including repeat installation
  and refusal to overwrite a differing override; the Compose off override
  resolved `DEMO_QUICK_LOGIN=false`.

## Boundaries and remaining risks

No reset, restore, backup creation, container restart, volume operation,
environment edit or script installation ran on `fbshow` or Joy's
`fbqa-cp1b`. The old live showcase still has quick-login enabled until the
owner installs the versioned scripts and deploys the reviewed code. A full
browser product suite needs a fresh seeded disposable web stack; running it
against `fbshow` or Joy's stack would violate the task's safety boundary.
The real interactive PowerShell reset and rollback were intentionally not
executed. Attachment file deletion can fail after the database commits; the
operator must inspect a nonzero result and use the verified backup if needed.
