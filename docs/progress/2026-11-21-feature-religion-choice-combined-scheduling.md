# Religion Education choices and combined scheduling — 2026-11-21

- Resumed the existing isolated worktree without changing branches or live
  environments. Fetched origin/main; it remains `3179fad3`, the branch base.
- Reviewed migration 1817, the clean-slate schema guard, Admin and family
  authorization, result compatibility, cohort publication and attendance.
- Fixed a duplicate EN/BN translation key found by the TypeScript build.
- Kept the last-Admin HTTP regression assertion active on shared disposable
  test databases by checking it inside a rollback-only transaction.
- Moved the browser roster-editor check after the read-only preview loads its
  catalogue. The full isolated browser flow then passed 18/18 checks.
- Full sequential Go suite passed on fresh disposable PostgreSQL databases:
  33 packages passed, 12 had no tests, zero failures. `go build ./...` and
  `go vet ./...` passed. Frontend build, EN/BN i18n check (4 parser tests) and
  24 unit tests passed.
- No fbshow write, deployment, school setting change, account change or
  Cloudflare action was performed. Migration and school configuration remain
  pending a separate reviewed deployment.
