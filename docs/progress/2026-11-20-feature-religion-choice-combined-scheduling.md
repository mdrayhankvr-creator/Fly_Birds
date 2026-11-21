# Religion Education choices and combined scheduling — 2026-11-20

- Created isolated `feature/religion-choice-combined-scheduling` worktree from
  origin/main at `3179fad3dfdec50386398698a61a876e07645f32`.
- Reviewed fourth-subject, group, enrolment, result, routine, attendance,
  access control and reset paths. The read-only showcase snapshot had generic
  required code 111 in classes 6–10. No live records were changed.
- Added migration 1817 for explicit academic course options and choices,
  correction requests, versioned combined lessons, membership and attendance.
  No school course or routine is seeded.
- Reused the existing Admin choice and versioned planner flows; added family
  requests, Admin review, cohort conflict checks, assigned-teacher attendance,
  responsive EN/BN UI and reset schema coverage.
- Focused PostgreSQL tests ran against disposable local databases. The final
  report records full sequential Go, frontend and browser verification.
- fbshow, Cloudflare, protected accounts and school configuration remained
  untouched. Deployment requires a separate owner-approved preflight/backup.
