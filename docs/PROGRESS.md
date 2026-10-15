# Fly_Birds — Development Progress

## Measurement

Each bullet under M0–M13 in [PROJECT_BRIEF.md](PROJECT_BRIEF.md) is one provisional acceptance criterion. There are 83 roadmap criteria, including four for M0. Count a criterion when its implementation has repository evidence and applicable local verification. Recalculate both denominators if the criteria are refined.

- **Overall completion:** 6 verified criteria ÷ 83 total criteria × 100 = **7.2%** (rounded to one decimal place).
- **Current milestone completion:** 2 verified M1 criteria ÷ 12 M1 criteria × 100 = **16.7%**.

The six counted criteria are all four M0 criteria plus M1 JWT access/refresh tokens and the Bangla/English switch. JWT signing, expiry, refresh rotation, and revocation passed unit and HTTP tests; PostgreSQL persistence remains unverified. Other checkpoint-one features are implemented but not counted complete until database-backed testing or later M1 work meets their full criteria. Remote M1 GitHub Actions execution and Docker runtime remain unverified.

## Current Status

| Item | Status |
|---|---|
| Current development phase | M1 checkpoint 1 authentication foundation implemented locally; awaiting database-backed verification and review |
| Current milestone | M1 — Authentication and School Configuration |
| Completed acceptance criteria | Four M0 criteria; two M1 criteria (JWT access/refresh tokens, Bangla/English switch) |
| Remaining acceptance criteria | Ten M1 criteria and 67 criteria in M2–M13 |
| Features implemented | M0 foundation plus user/session schema, bcrypt passwords, login/refresh/logout/me API, account lockout, revocable sessions, CSRF protection, bootstrap command, bilingual login and protected account page |
| Tests passed | M1 local Go vet, Go unit/HTTP tests, frontend production build; see verification details |
| Tests failed | None in completed local checks |
| Application runnability | Health-only backend and frontend can run without PostgreSQL; authentication requires PostgreSQL and has not been exercised end-to-end here |
| Next planned milestone | Remaining M1 checkpoints only after developer approval; no M2 work |

## M0 Acceptance Criteria and Evidence

| Criterion | Status | Evidence |
|---|---|---|
| Project foundation | Complete | Go and React projects, PostgreSQL Compose service, Dockerfiles, environment examples, and local integrated HTTP smoke check |
| Health endpoint | Complete | `GET /healthz` returned `{"data":{"status":"ok"}}`; backend health and error response tests passed |
| CI | Complete (configured) | `.github/workflows/ci.yml` runs Go vet, Go tests, and frontend build; M0 is merged into main. M1 branch CI is pending a developer push |
| Initial documentation | Complete | README, roadmap, progress record, project brief, and M0 OpenAPI document |

## M1 Acceptance Criteria

| Criterion | Status | Checkpoint-one evidence or remaining work |
|---|---|---|
| Login and authentication | Partial | API and UI implemented; database-backed login has not run in this workspace |
| Users | Partial | User model, schema, and interactive first-user command; user management remains |
| Roles and permissions matrix | Not started | Six role values exist on users; runtime matrix remains |
| JWT access and refresh tokens | Complete locally | 15-minute signed JWTs, seven-day hashed refresh tokens, rotation, session revocation, and lifecycle tests; PostgreSQL persistence unverified |
| School settings and white-label configuration | Not started | Later M1 checkpoint |
| Bangladesh grading configuration | Not started | Later M1 checkpoint |
| Runtime module toggles | Not started | Later M1 checkpoint |
| Audit logging | Not started | Later M1 checkpoint |
| Login lockout | Partial | Five-failure, 15-minute logic and unit tests; PostgreSQL integration unverified |
| Themed login page | Partial | Responsive page uses the existing theme; per-school branding configuration remains |
| Bangla/English language switch | Complete locally | Existing switch covers the new login/account text; frontend TypeScript and production build pass |
| Role-based dashboards | Not started | Later M1 checkpoint |

## M1 Checkpoint-one Verification

- `go test ./...` passed, including password, JWT, login/lockout, refresh, CSRF, logout, and HTTP response tests.
- `go vet ./...` passed.
- `npm.cmd run build` passed, including TypeScript checking.
- Headless Chrome rendered the login page in English and Bangla, confirmed Inter and Noto Sans Bengali font stacks, and found no horizontal overflow at a 390-pixel mobile viewport.
- `TestPostgresAuthenticationLifecycle` skipped because `AUTH_TEST_DATABASE_URL` is unset. It covers migration, lockout, refresh rotation, and logout against a dedicated PostgreSQL database when available.
- Docker daemon and PostgreSQL are unavailable here. Container startup, SQL migration execution, account creation, and browser login end-to-end remain unverified.

## M0 Verification History

- `go vet ./...` passed in `backend/`.
- `go test ./...` passed in `backend/`; the configuration and HTTP API packages contain tests. The command reports no test files for the API entrypoint and database connection packages.
- `npm.cmd ci` (using the local offline cache) and `npm.cmd run build` passed in `frontend/`; the build includes TypeScript checking.
- `docker compose config --quiet` exited successfully. The CLI warned that it could not read the local Docker config file, but configuration validation succeeded.
- A locally running backend returned HTTP 200 from `http://localhost:8080/healthz`. A locally running Vite frontend returned HTTP 200 from `http://localhost:5173/` and proxied `http://localhost:5173/api/healthz` with the same healthy JSON response.

## Known Limitations and Blockers

- The Docker daemon is not running in this environment. Container builds, PostgreSQL connectivity, SQL migration execution, and full Compose startup remain unverified.
- GitHub Actions has not run on the M1 feature branch because no push was performed.
- No demo credentials are stored or shipped. Create the first account interactively after connecting PostgreSQL.
- Full runtime RBAC, permission editing, school settings, module toggles, audit logging, and role dashboards remain for later M1 checkpoints.
