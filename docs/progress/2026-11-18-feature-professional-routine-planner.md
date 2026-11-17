# 2026-11-18 — Professional class routine planner

- Fetched `origin/main` and started `feature/professional-routine-planner` from `ee2e38a0`; the fourth-subject branch was still pending and was not copied or modified.
- Reviewed class routine persistence, teacher assignments, class subjects, room registry, weekly off days, route permissions and the existing routine UI. Class routines had direct edits but no publication history. No recurring teacher availability model existed.
- Added an opt-in class routine draft and version history, a read-only preview, exact 45-minute lunch grid, manual placement, conflict and requirement validation, administrator-only save/publish routes, and an EN/BN responsive planner page.
- Added migration 1815 after checking origin/main's migrations and the pending branch's 1814. Existing routine rows are untouched until explicit publication. Recorded the combined reset-schema guard review needed when both branches merge.
- Added isolated PostgreSQL publication and conflict tests, unit period tests, route authorization checks, OpenAPI documentation and the operator guide. Final review also normalized room names for overlap checks and strengthened the disposable-database guard.
- Verification: focused planner PostgreSQL tests passed; full Go suite passed (33 packages, 12 without tests); `go build ./...` and `go vet ./...` passed after the final backend change; TypeScript, production build, EN/BN i18n parity (4 checks), and frontend unit tests (22) passed; isolated browser checks passed 14/14, including preview, manual conflict, explicit publish confirmation and Bangla mobile view. An initial full Go run failed only because the four new OpenAPI operations omitted 401 responses; the responses were added and the full suite passed on a fresh disposable database.
- Did not access or modify fbshow, Joy's worktree or stacks, Cloudflare, protected administrator accounts, school settings or live data. No timetable was published outside the disposable test transaction.

Commit hash is reported in the final task response.
