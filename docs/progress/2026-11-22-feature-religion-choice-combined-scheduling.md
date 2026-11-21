# PR #72 backend CI repair — 2026-11-22

- Recovered the clean `feature/religion-choice-combined-scheduling` worktree
  at `c1a6033f`; no application runtime or migration files were changed.
- GitHub run #263 marked the backend **Unit and PostgreSQL integration tests**
  step failed. Its detailed log requires authentication and was unavailable
  here. The CI workflow supplies the shared `flybirds_ci` database at
  `localhost:5432`, while `TestPostgresCombinedCohortPlanning` required a
  local `flybirds_religion_test_` database at `127.0.0.1:55415`.
- Reproduced the test's `refusing non-disposable cohort test database` failure
  with a disposable database URL using the CI-style host. The repair gives
  the cohort fixture its own freshly created CI database and dedicated URL.
  The test still refuses shared, production-looking, remote and nonempty
  databases before migrations.
- Focused cohort guard and PostgreSQL integration tests passed on a new
  disposable database. The full sequential Go suite passed on fresh isolated
  databases: 33 packages passed, 12 had no tests, zero failures. Go build,
  vet and frontend build passed; EN/BN i18n check and 24 frontend unit tests
  passed. Browser E2E was not rerun because application runtime code is
  unchanged; the milestone's prior isolated run passed 18/18 checks.
- No fbshow, protected account, Cloudflare, Joy stack or live database was
  accessed or changed. No push, merge or deployment was performed.
