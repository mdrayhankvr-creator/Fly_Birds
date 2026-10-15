# Fly_Birds — Development Progress

## Measurement

Each bullet under M0–M13 in [PROJECT_BRIEF.md](PROJECT_BRIEF.md) is one provisional acceptance criterion. There are 83 roadmap criteria, including four for M0. Count a criterion when its implementation has repository evidence and applicable local verification. Recalculate both denominators if the criteria are refined.

- **Overall completion:** 13 verified criteria ÷ 83 total criteria × 100 = **15.7%** (rounded to one decimal place).
- **Current milestone completion:** 9 verified M1 criteria ÷ 12 M1 criteria × 100 = **75.0%**.

The thirteen counted criteria are all four M0 criteria plus M1 login/authentication, JWT access/refresh tokens, login lockout, Bangla/English switch, roles and permissions matrix, school settings and white-label configuration, Bangladesh grading configuration, runtime module toggles, and a themed login page. Checkpoint-three PostgreSQL integration tests, live migration, and browser checks verify these locally. Full user management, audit logging, and role dashboards remain. Remote M1 GitHub Actions execution remains unverified because this branch has not been pushed.

## Current Status

| Item | Status |
|---|---|
| Current development phase | M1 checkpoint 3 school configuration verified locally; awaiting approval for a later checkpoint |
| Current milestone | M1 — Authentication and School Configuration |
| Completed acceptance criteria | Four M0 criteria; nine M1 criteria (authentication, tokens, lockout, language switch, role permissions, school settings, grading, module toggles, themed login) |
| Remaining acceptance criteria | Three M1 criteria and 67 criteria in M2–M13 |
| Features implemented | M0 and checkpoints 1–3: authentication, six roles and runtime grants, persisted school profile, branding, grading bands, optional module toggles and middleware, bilingual admin configuration UI |
| Tests passed | Go vet; fresh Go suite including isolated PostgreSQL auth, RBAC, and school tests on repeat runs; frontend production build; Compose and OpenAPI validation; live migration and health; headless Chrome desktop/mobile, login, language, preview, and theme checks |
| Tests failed | None in completed local checks |
| Application runnability | PostgreSQL, backend, and frontend run through Compose. Migration 3 applied to the existing database; the original one user and one session remained after disposable browser test cleanup. |
| Next planned milestone | Remaining M1 work only after developer approval; no M2 work |

## M0 Acceptance Criteria and Evidence

| Criterion | Status | Evidence |
|---|---|---|
| Project foundation | Complete | Go and React projects, PostgreSQL Compose service, Dockerfiles, environment examples, and a running local Compose stack |
| Health endpoint | Complete | `GET /healthz` returned `{"data":{"status":"ok"}}`; backend health and error response tests passed |
| CI | Complete (configured) | `.github/workflows/ci.yml` runs Go vet, Go tests, and frontend build; M0 is merged into main. M1 branch CI is pending a developer push |
| Initial documentation | Complete | README, roadmap, progress record, project brief, and M0 OpenAPI document |

## M1 Acceptance Criteria

| Criterion | Status | Checkpoint-one evidence or remaining work |
|---|---|---|
| Login and authentication | Complete locally | PostgreSQL-backed HTTP lifecycle and isolated browser login/logout passed; protected `/auth/me` rejects anonymous and invalid access |
| Users | Partial | User model, schema, interactive first-user command, and protected role assignment API; full user CRUD remains |
| Roles and permissions matrix | Complete locally | Six seeded roles, fourteen stable permissions after migration 3, live PostgreSQL grants, guarded APIs, responsive bilingual matrix; integration and browser checks passed |
| JWT access and refresh tokens | Complete locally | 15-minute signed JWTs, seven-day hashed refresh tokens, rotation, expiration rejection, and revocation passed PostgreSQL-backed HTTP tests |
| School settings and white-label configuration | Complete locally | Migration 3, validated profile and public branding APIs, transactionally guarded updates, bilingual responsive editor, and neutral defaults; PostgreSQL and browser checks passed |
| Bangladesh grading configuration | Complete locally | Default A+–F bands stored in PostgreSQL; all specified boundaries, edits, gaps, overlaps, GPA range, and transactional replacement tested |
| Runtime module toggles | Complete locally | Twelve stable definitions, current database checks per request, active-session enforcement, core locks, permission checks, and admin controls tested |
| Audit logging | Not started | Later M1 checkpoint |
| Login lockout | Complete locally | Five failures caused a persisted 15-minute lockout; the correct password was rejected while locked in PostgreSQL-backed tests |
| Themed login page | Complete locally | Login loads public school identity and CSS colors; a browser check with mocked configured branding rendered the dark theme and Bangla school name without changing live school data |
| Bangla/English language switch | Complete locally | Existing switch covers the new login/account text; frontend TypeScript and production build pass |
| Role-based dashboards | Not started | Later M1 checkpoint |

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
- Full user CRUD, audit logging, and role dashboards remain for later M1 checkpoints. Optional module feature routes do not exist yet; `RequireModule` now guards implemented protected module routes and is ready for those future routes.
- The external logo URL is shape-validated but remote image bytes are not inspected. Uploaded files are not supported. External image hosts may observe browser requests.
- Browser submission of an edited permission matrix was not exercised against the live application database. Saving, atomicity, and immediate enforcement passed in isolated PostgreSQL-backed HTTP tests.
