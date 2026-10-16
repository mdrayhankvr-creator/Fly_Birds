# Fly_Birds — Development Progress

## Measurement

Each bullet under M0–M13 in [PROJECT_BRIEF.md](PROJECT_BRIEF.md) is one provisional acceptance criterion. There are 83 roadmap criteria, including four for M0. Count a criterion when its implementation has repository evidence and applicable local verification. Recalculate both denominators if the criteria are refined.

- **Overall completion:** 16 verified criteria / 83 total criteria x 100 = **19.3%** (rounded to one decimal place).
- **Current milestone completion:** 0 verified M2 criteria / 6 M2 criteria x 100 = **0.0%**. Application code for four criteria awaits a coordinated migration and database verification.

The sixteen counted criteria are all four M0 criteria and all twelve M1 criteria from the project brief. The final audit repeated backend and PostgreSQL tests, checked a rebuilt frontend in an isolated browser stack, and reviewed the security limitations. Remote M1 GitHub Actions execution remains unverified because this branch has not been pushed.

## Current Status — M2 Task 001

| Item | Status |
|---|---|
| Development phase | Session-independent classes and subjects code and UI prepared; migration coordination pending |
| Current milestone | M2 — Academic Structure |
| Verified criteria | M0: 4/4; M1: 12/12; M2: 0/6 |
| Remaining criteria | Six M2 criteria and 61 criteria in M3–M13 |
| Backend and frontend | Class, subject, and class-subject code and bilingual admin pages added; academic tables and permissions are not installed |
| Tests passed | Go vet, Go unit and M1 PostgreSQL suites, frontend production build in Docker, OpenAPI route/reference test, Compose validation |
| Unverified | M2 PostgreSQL integrity and HTTP lifecycle, browser responsive behavior, remote CI |
| Application runnability | Existing M1 Compose stack remains running. Academic APIs require the coordinated schema and permission seeds. |
| Next step | Confirm Joy's migration number, add this branch's subsequent migration, then run M2 integration and browser tests. No other M2 step is in this checkpoint. |

## M2 Task 001 checkpoint — 2026-10-16

- Started from clean updated `main` at merge commit `49a03c08` and created `feature/m2-classes-subjects`. Joy's branch was not published at the time of the check, so no migration version or runner registration was guessed. [The handoff](M2_CLASSES_SUBJECTS_HANDOFF.md) specifies the session-independent schema and merge sequence.
- Added class definitions (configurable name, order, active state), bilingual subjects (English/Bangla names, normalized code, active state), and class-subject mapping with exactly one required or fourth/optional designation. The service validates inputs; repositories are prepared for UUID keys, unique indexes, foreign keys, transactional authorization, and audit records once the migration exists.
- Added academic APIs guarded by authentication, `academics:read`/`academics:manage`, the `academics` module, and administrator status for writes. Class, subject, and mapping lists support search and pagination. OpenAPI covers all registered routes and references.
- Added bilingual `/admin/academic/classes` and `/admin/academic/subjects` views with responsive cards, filters, editors, and mapping controls. Navigation requires current permission and module state. The production frontend build passed, but the pages could not be exercised in a browser without the academic migration and grants; responsive behavior remains unverified.
- `go vet ./...` passed with a workspace-local Go cache. `go test ./... -count=1` passed against a dedicated PostgreSQL database; `TestPostgresAcademicLifecycle` skipped explicitly because the coordinated tables do not exist. The M1 PostgreSQL tests passed. The OpenAPI route/reference test passed. `docker compose config --quiet` passed. The host `npm ci` hit the same locked `lightningcss` binary as M1; a Docker production build passed.
- **Integration dependency:** the migration filename, runner entry, permission seeds, and database constraints must be added after Joy's version is confirmed. Subject-code uniqueness, duplicate mapping prevention, foreign-key integrity, module-enabled HTTP behavior, and responsive browser behavior are prepared in tests/UI but not yet verified end to end. No sessions, sections, or later milestone data was added.

## M1 Historical Status

| Item | Status |
|---|---|
| Current development phase | M1 final local audit complete; ready for Pull Request review, awaiting developer push and remote CI |
| Current milestone | M1 — Authentication and School Configuration |
| Completed acceptance criteria | Four M0 criteria and all twelve M1 criteria verified locally |
| Remaining acceptance criteria | No M1 brief criteria; 67 criteria in M2-M13 remain |
| Features implemented | M0 and M1 checkpoints 1-4: authentication, six roles and runtime grants, persisted school profile, branding, grading, module toggles, user administration, audit logging, and six role dashboards |
| Tests passed | Go vet, Go unit and PostgreSQL suites, Go race suite without PostgreSQL, fresh container npm install and frontend production build, Compose/OpenAPI checks, npm audit, isolated headless Chrome workflows at desktop/mobile widths |
| Tests failed | Windows host `npm ci` was blocked by a locked native binary; the fresh isolated-container install and build passed. No application test failed in the final run. |
| Application runnability | Existing PostgreSQL, backend, and frontend continue through Compose on 5432/8080/5173. The final browser audit used and removed a separate database and services; migration 4 remains applied to the existing database. |
| Next planned milestone | No M2 work until the developer explicitly says `next`; first publish M1 and review remote CI |

## M0 Acceptance Criteria and Evidence

| Criterion | Status | Evidence |
|---|---|---|
| Project foundation | Complete | Go and React projects, PostgreSQL Compose service, Dockerfiles, environment examples, and a running local Compose stack |
| Health endpoint | Complete | `GET /healthz` returned `{"data":{"status":"ok"}}`; backend health and error response tests passed |
| CI | Complete (configured) | `.github/workflows/ci.yml` runs Go vet, Go tests, and frontend build; M0 is merged into main. M1 branch CI is pending a developer push |
| Initial documentation | Complete | README, roadmap, progress record, project brief, and M0 OpenAPI document |

## M1 Acceptance Criteria

| Criterion | Status | Verified evidence |
|---|---|---|
| Login and authentication | Complete locally | PostgreSQL-backed HTTP lifecycle and isolated browser login/logout passed; protected `/auth/me` rejects anonymous and invalid access |
| Users | Complete locally | PostgreSQL-backed create/list/search/filter/page/read/update/status APIs, single-role assignment integration, last-admin protection, transactionally revoked sessions, bilingual responsive admin page and browser flows |
| Roles and permissions matrix | Complete locally | Six seeded roles, fifteen defined permissions after migration 4, live PostgreSQL grants, guarded APIs, responsive bilingual matrix; integration and browser checks passed |
| JWT access and refresh tokens | Complete locally | 15-minute signed JWTs, seven-day hashed refresh tokens, rotation, expiration rejection, and revocation passed PostgreSQL-backed HTTP tests |
| School settings and white-label configuration | Complete locally | Migration 3, validated profile and public branding APIs, transactionally guarded updates, bilingual responsive editor, and neutral defaults; PostgreSQL and browser checks passed |
| Bangladesh grading configuration | Complete locally | Default A+–F bands stored in PostgreSQL; all specified boundaries, edits, gaps, overlaps, GPA range, and transactional replacement tested |
| Runtime module toggles | Complete locally | Twelve stable definitions, current database checks per request, active-session enforcement, core locks, permission checks, and admin controls tested |
| Audit logging | Complete locally | Migration 4, transactional audit hooks for privileged mutations and important authentication events, redacted metadata, read-only filtered API, rollback test |
| Login lockout | Complete locally | Five failures caused a persisted 15-minute lockout; the correct password was rejected while locked in PostgreSQL-backed tests |
| Themed login page | Complete locally | Login loads public school identity and CSS colors; a browser check with mocked configured branding rendered the dark theme and Bangla school name without changing live school data |
| Bangla/English language switch | Complete locally | Existing switch covers the new login/account text; frontend TypeScript and production build pass |
| Role-based dashboards | Complete locally | Server-authoritative role routing, six shared role shells, actual admin counts, allowed navigation and school branding; five nonadmin dashboards isolate admin statistics |

## M1 Final Audit and Completion Report — 2026-10-16

- **Scope and progress:** 12/12 M1 criteria = 100.0% locally; (4 M0 + 12 M1)/83 roadmap criteria = 19.3% overall. M2 is not started. The feature branch contains remote `main` as an ancestor and was clean before this audit. Its commits remain local.
- **Backend and frontend:** Handler → Service → Repository → PostgreSQL for authentication, RBAC, school configuration, users, audit, and dashboards. The frontend has `/`, `/login`, `/account`, `/admin/permissions`, `/admin/school-settings`, `/admin/users`, `/dashboard`, and `/dashboard/:role`. The audit viewer remains M10 scope; M1 supplies a guarded read-only audit API. API methods, permissions, and module checks are listed in [OpenAPI](openapi.yaml). The development stack and interactive bootstrap command are documented in the [README](../README.md).
- **Schema:** Migration 1 creates users and revocable sessions; 2 seeds six roles and role grants; 3 adds one configurable school profile, seven Bangladesh grade bands, and twelve module definitions; 4 adds display names, audit events, and `audit:read`. The existing application database reports versions 1–4, with no reset.
- **Automated gates:** `go vet ./...` passed. `go test ./... -count=1` passed both without a database URL and against the separate `flybirds_cp4_test` PostgreSQL database. `go test -race ./... -count=1` passed without the database URL. A fresh `npm ci` and `npm run build` passed inside the isolated frontend container; Windows host `npm ci` hit `EPERM` on a locked native binary. No frontend unit-test script exists. `docker compose config --quiet`, PostgreSQL readiness, direct/proxied health HTTP 200, and OpenAPI parse (20 paths, 26 operations, 151 resolved references) passed. All 26 registered method/path pairs match OpenAPI operations. `npm audit` reported zero vulnerabilities among current installed dependencies. A Go dependency advisory scanner was unavailable, so that check is unverified. Remote GitHub Actions is pending the developer's push.
- **Final browser and API regression:** A separate `flybirds_cp5_audit` database, backend on 8081, and rebuilt frontend on 5174 protected the existing application data. Headless Chrome passed English/Bangla login and font checks, styled page and favicon loading, invalid/valid login, protected route redirects, browser user creation, permission editor save, active-session grant change and restoration, last-admin denial, school profile edit and persistence after reload, grading overlap/gap/GPA rejection and valid replacement, module state and Coming Soon navigation, audit events, six role dashboards and role isolation, logout, and 1280 px/390 px layout checks with no horizontal overflow. API checks verified refresh rotation/reuse rejection, logout revocation, deactivation revocation, and current-user denial after revocation. The browser logged one expected HTTP 401 resource message from a deliberate denial; no unexpected JavaScript exceptions appeared. The test-only configuration was restored before the temporary stack and database were removed.
- **Module-route verification limit:** No optional academic, attendance, or financial feature route exists in M1. `RequireModule` returned 403 for a disabled optional module on an isolated PostgreSQL-backed test route; a real optional feature route must be checked when its milestone implements one. The final browser audit verified module toggle persistence and navigation, not an unavailable future route.
- **Security findings:** No confirmed Critical or High M1 defect in this audit. **Medium:** the administrator knows each initial password until a future credential-change workflow; deliver unique passwords privately. **Medium:** lockout is per account, with no independent IP/edge rate limit; add one before public exposure. **Medium, unverified deployment control:** HTTPS and Secure-cookie behavior were not exercised; require both outside local HTTP development. **Low:** the server validates the external logo URL shape but does not inspect image bytes; use a trusted image host. SQL values use bound parameters, protected routes use current database grants and module states, React renders text without unsafe HTML, and login/session tests cover CSRF and token replay. Continue dependency and deployment review when publishing.
- **Release decision:** Locally ready for Pull Request review. Merge after developer push, green GitHub Actions, and review of the documented limits. Do not push or start M2 automatically.

## M1 Checkpoint-four Verification

- Docker Compose PostgreSQL remained healthy. The live backend applied migration 4, adding user display names and audit events without resetting the existing database. Direct and proxied `/healthz` returned HTTP 200. The original user count was one before and after browser testing; fourteen uniquely named disposable accounts, their sessions, and their test audit events were removed. The optional attendance module was restored to its previous disabled state.
- `go vet ./...` and `go test ./... -count=1` passed in `backend/` with `AUTH_TEST_DATABASE_URL` pointing to the separate `flybirds_cp4_test` PostgreSQL database. The checkpoint-four HTTP test covered create/list/detail/update/status, pagination and filters, invalid and duplicate input, 401/403 denial, role assignment, last-admin protection, session revocation, refresh rejection after deactivation, audit attribution/redaction/filtering/order, audit events for permission/settings/grading/module changes, and rollback when mandatory audit insertion fails. Existing authentication, RBAC, and school tests also passed.
- `npm.cmd run build` passed, including TypeScript checking. `docker compose config --quiet` passed. OpenAPI YAML parsed with 20 paths and 151 resolved references. M1 branch GitHub Actions remains unverified until a developer push.
- Headless Chrome at 1280 px and 390 px rendered the admin users page and dashboard with loaded Tailwind styles and no document overflow. It created, searched, viewed, and deactivated a disposable user, then verified the admin and five other role dashboards, permission-aware user navigation, enabled-module Coming Soon navigation, English/Bangla switching, logout, and protected-route redirection. The browser showed Inter for English and Noto Sans Bengali for Bangla. There were no CSS asset failures or JavaScript exceptions. Chrome reported a 404 for the pre-existing missing `/favicon.ico` request.
- Provisioning currently relies on an administrator entering an initial password and delivering it privately. No password reset or invitation delivery exists; this is a documented security limitation. The audit API is read-only; the advanced viewer/export belongs to M10. Dashboard future modules show placeholders only and no fabricated operational data.

## M1 Checkpoint-three Verification

- Docker Desktop and PostgreSQL were healthy. Migration 3 applied to the live application database. Before and after the service rebuild and disposable browser-account cleanup, the original one user and one active session remained. The migration ledger has three versions, seven default grading bands, and twelve seeded module states. Direct and proxied health/branding endpoints returned HTTP 200.
- `go vet ./...` and `go test ./... -count=1` passed in `backend/` with `AUTH_TEST_DATABASE_URL` set to a separate disposable PostgreSQL database. The full suite passed on consecutive runs. The school HTTP test ran migrations twice, verified anonymous and read-only access, validated persisted profile and public-field filtering, tested all requested mark boundaries and transactional grade edits, rejected gaps/overlaps and unsafe settings, protected core modules, and confirmed optional module changes affected active sessions. The test restores original school settings, grading, module state, and its temporary users.
- `npm.cmd run build`, `docker compose config --quiet`, and OpenAPI YAML parsing with 116 resolved references passed. No new frontend unit test runner exists. M1 branch GitHub Actions remains unverified until a developer push.
- Headless Chrome at 1280 px and 390 px logged in with a disposable administrator, loaded the settings editor with seven grade rows and twelve module toggles, previewed and reset an unsaved color, switched English/Bangla, and redirected the protected route to login after logout. The 390 px viewport had no document overflow. A network-intercepted public branding response rendered a different school name, primary color, preferred Bangla language, and dark login theme without writing to the application database. Fonts were Inter and Noto Sans Bengali respectively. Stylesheets and API requests returned 200/304; browser navigation caused expected `ERR_ABORTED` fetch cancellations, with no console exceptions. The temporary administrator and its sessions were removed.
- This is one school's deployment configuration. The logo uses a constrained external URL, whose image contents are not fetched or verified by the API. Optional modules have definitions and enforcement middleware but their feature pages/APIs are later milestones. The browser did not save settings against the live database; PostgreSQL HTTP tests verified writes and persistence in isolation.

## M1 Checkpoint-two Verification

- Docker Compose PostgreSQL remained healthy. The live database migrated from version 1 to version 2 with six roles, ten permission IDs, and the existing one user and one active session preserved. The ignored `.env` signing key matched the running backend key before the necessary image rebuild.
- `go vet ./...` and `go test ./... -count=1` passed in `backend/` with `AUTH_TEST_DATABASE_URL` pointing to a separate PostgreSQL database. The RBAC integration test ran the migration twice and covered default grants, anonymous 401, insufficient-grant 403, authorized updates, live grants and revocations, invalid and duplicate IDs, user role assignment, last-admin protection, and concurrent role permission replacements. The checkpoint-one authentication integration tests also passed.
- `npm.cmd run build` and `docker compose config --quiet` passed. The OpenAPI YAML parsed successfully. Direct and proxied `/healthz` returned `ok` after the image rebuild.
- Headless Chrome logged in with disposable administrator accounts and rendered the permission editor at 1280 px and 390 px without horizontal overflow or runtime errors. It showed six roles and ten permissions, switched to Teacher with one checked grant, switched back to Administrator, displayed Bangla text after language change, and redirected the protected editor to login after logout. These browser checks did not submit a permission change; the PostgreSQL HTTP integration test verified saving and revocation. The test accounts and their sessions were removed.
- This branch remains local; remote CI has not run for checkpoint 2. The test database is disposable and separate from the application database.

## M1 Checkpoint-one Verification

- Docker Desktop 29.7.2 and Compose 5.4.0 ran the PostgreSQL, backend, and frontend services. PostgreSQL reported healthy; the application database migration ledger contained version 1 and the `users`, `auth_sessions`, and `schema_migrations` tables.
- `go vet ./...` and `go test ./... -count=1` passed with `AUTH_TEST_DATABASE_URL` pointing to an isolated database. `TestPostgresAuthenticationLifecycle` and `TestPostgresAuthHTTPLifecycle` passed against PostgreSQL. The HTTP test covered invalid passwords, persisted lockout, authorized and unauthorized `/auth/me`, expired access, CSRF, refresh rotation/reuse, logout, and persisted revocation.
- An isolated teacher account passed login and logout in headless Chrome against the running frontend and backend. The browser reached `/account`, kept the refresh cookie HttpOnly and the access token out of localStorage, redirected to `/login` after logout, and left no active database session. The temporary account was removed. The developer also reported a successful login with their development administrator account.
- Headless Chrome rendered English and Bangla login pages using Inter and Noto Sans Bengali. Desktop and 390-pixel mobile layouts had no horizontal overflow or JavaScript runtime errors.
- The live backend returned 401 for anonymous and invalid-token `/auth/me` requests and for an unknown-account login. Direct and proxied `/healthz` returned HTTP 200 with healthy JSON.
- `npm.cmd run build` passed, including TypeScript checking, and `docker compose config --quiet` passed. The database query logger now suppresses expected missing-record events and SQL parameter values.

## M0 Verification History

- `go vet ./...` passed in `backend/`.
- `go test ./...` passed in `backend/`; the configuration and HTTP API packages contain tests. The command reports no test files for the API entrypoint and database connection packages.
- `npm.cmd ci` (using the local offline cache) and `npm.cmd run build` passed in `frontend/`; the build includes TypeScript checking.
- `docker compose config --quiet` exited successfully. The CLI warned that it could not read the local Docker config file, but configuration validation succeeded.
- A locally running backend returned HTTP 200 from `http://localhost:8080/healthz`. A locally running Vite frontend returned HTTP 200 from `http://localhost:5173/` and proxied `http://localhost:5173/api/healthz` with the same healthy JSON response.

## Known Limitations and Blockers

- GitHub Actions has not run on the M1 feature branch because no push was performed.
- No demo credentials are stored or shipped. Create the first account interactively after connecting PostgreSQL.
- The local HTTP stack uses `AUTH_COOKIE_SECURE=false`; HTTPS deployment with secure cookies remains unverified. Login lockout is per account; an independent IP or edge rate limit is not yet configured.
- Each user currently has exactly one role; the `GET/PUT /users/:id/roles` API uses a one-item array to preserve the existing authentication model. Multi-role users are not part of this checkpoint.
- No user deletion endpoint or self-service password change is available. Administrator-assisted initial passwords must be delivered privately; the administrator continues to know the password until a supported reset workflow is implemented. Optional future module feature routes do not exist yet.
- The external logo URL is shape-validated but remote image bytes are not inspected. Uploaded files are not supported. External image hosts may observe browser requests.
- Browser submission of an edited permission matrix was not exercised against the live application database. Saving, atomicity, and immediate enforcement passed in isolated PostgreSQL-backed HTTP tests.
