# Fly_Birds

Fly_Birds is a reusable, white-label school management system for schools in Bangladesh. One codebase supports configurable branding, language, grading, permissions, and module settings. M0 and M1 are merged into main. M1 provides authentication, runtime role permissions, school configuration, user administration, persistent audit logging, and six role dashboards. The M2 classes and subjects branch contains session-independent application code; its database migration is awaiting coordination with Joy's academic sessions branch.

## M2 Task 001 — Classes and Subjects

The `feature/m2-classes-subjects` branch adds validated class definitions, bilingual subjects with canonical subject codes, and class-subject assignments with exactly one required or fourth/optional designation. The backend follows Handler → Service → Repository and guards academic routes with current authentication, `academics:read` or `academics:manage`, administrator status for writes, and the `academics` module. The frontend adds `/admin/academic/classes` and `/admin/academic/subjects`, English/Bangla text, paginated search, responsive forms, and mapping controls. Admin navigation appears only when the required grant and module are available.

**Integration dependency:** Joy's session migration number has not been confirmed. This branch has no academic SQL migration or runner registration, so the academic tables and permissions do not yet exist. Academic APIs are not usable against an M1-only database. See [the schema and integration handoff](docs/M2_CLASSES_SUBJECTS_HANDOFF.md). No academic session or section implementation is included.

The full requirements are in [docs/PROJECT_BRIEF.md](docs/PROJECT_BRIEF.md). Milestone status is in [docs/PROGRESS.md](docs/PROGRESS.md) and [docs/roadmap.md](docs/roadmap.md).

## M0 Features

- Go 1.26 API with Gin, structured environment configuration, standard JSON responses, and graceful shutdown
- Optional PostgreSQL connection through GORM; the M0 liveness endpoint works without a database
- Public `GET /healthz` endpoint returning `{"data":{"status":"ok"}}`, documented in [OpenAPI](docs/openapi.yaml)
- React, Vite, TypeScript, Tailwind CSS, React Router, TanStack Query, and i18next application shell
- English and Bangla language toggle, with the choice saved in the browser
- Frontend health display using the Vite `/api` proxy
- Docker Compose development stack for PostgreSQL, backend, and frontend
- GitHub Actions workflow for Go vet, Go tests, and the frontend production build

## M1 Checkpoint 1 — Authentication

- PostgreSQL user and revocable session tables, created by a versioned migration on backend startup
- Six role identifiers on users; role permissions and management screens are later M1 work
- bcrypt password hashes, generic login failures, and five-failure, 15-minute account lockout
- 15-minute HS256 JWT access tokens validated against active server-side sessions
- Seven-day opaque refresh tokens, stored as SHA-256 hashes and rotated on use
- HttpOnly, SameSite=Strict refresh cookies and session-bound CSRF protection for refresh and logout
- A responsive English/Bangla login page, in-memory access token, session restoration, and a protected account page
- Interactive `create-user` command for the first account; no default credentials or public registration endpoint

## M1 Checkpoint 2 — Role Permissions

- Migration 2 adds normalized `roles`, `permissions`, and `role_permissions` tables and links the existing single `users.role` value to the six supported roles with a foreign key. Existing users and sessions are preserved.
- All six roles can read their own account by default. Administrators receive the ten currently defined permissions. Role grants are read from PostgreSQL on each protected request, so changes affect active sessions immediately.
- `GET /auth/permissions` returns the signed-in user's current grants. `GET /roles`, `GET /permissions`, `GET /roles/:id/permissions`, and `GET /users/:id/roles` provide read access based on `roles:read` or `permissions:read`.
- Administrators with `permissions:manage` can atomically replace a role's grants with `PUT /roles/:id/permissions` or a user's single role with `PUT /users/:id/roles`. The admin role must retain management and self-read access; the last active administrator cannot be demoted.
- `/admin/permissions` is a responsive English/Bangla permission matrix. The backend remains the authority for every update.

Both checkpoints passed local PostgreSQL-backed tests and browser checks. The later checkpoint results are recorded in [verified progress](docs/PROGRESS.md).

## M1 Checkpoint 3 — School Configuration

- Migration 3 creates one school profile, a default Bangladesh grading scheme, and twelve stable module definitions. Defaults use the neutral name “School”; no actual school identity or credentials are seeded. Existing users and sessions are preserved.
- Public `GET /school/branding` supplies only names, logo URL, language, colors, and theme. Contact details remain behind `school_settings:read`. Administrators with explicit update permissions can edit the profile, grade bands, and optional module states from `/admin/school-settings`.
- Grade bands cover every integer mark from 0–100 without gaps or overlaps; GPA must be 0–5 with at most two decimal places. Changes are transactional. Grade calculations and examination features remain M7 work.
- The frontend applies validated CSS color tokens and a light or dark theme after loading public branding. The login page and navigation show the configured school identity. Users can preview colors and logo before saving, reset unsaved edits, and switch between English and Bangla. A saved personal language choice takes precedence over the school default.
- Core `authentication`, `permissions`, and `school_settings` modules cannot be disabled through the API. Optional module definitions start disabled; enabling one does not create its future screens or endpoints.
- Protected module routes evaluate checks in this order: active session (`RequireAuth`), current PostgreSQL grant (`RequirePermission`), current PostgreSQL module state (`RequireModule`), then the handler. A missing/invalid session returns 401; missing permission or disabled module returns 403. Changes affect active sessions on the next request. Administrator-only writes also check the account and grant in the database transaction.

**Logo handling:** This checkpoint accepts a public HTTPS URL ending in `.png`, `.jpg`, `.jpeg`, or `.webp`, with no credentials, custom port, query, or fragment. No upload endpoint or local file storage is provided. The server validates the URL shape but does not fetch or inspect remote image bytes; administrators should use a trusted image host. An external logo host can observe browser image requests. The URL may be left empty.

This is a single-school deployment configuration, not tenant-isolated multi-tenancy. The repository and service interfaces leave room for later tenant scoping.

## M1 Checkpoint 4 — User Administration, Audit, and Dashboards

- Migration 4 adds user display names and a persistent audit event table with indexes. Successful user, role-permission, role-assignment, school settings, grading, and module changes insert audit events in the same transaction. Login, failed known-account login, and logout are also audited. Events contain explicit safe metadata, not passwords, hashes, tokens, cookies, or request bodies. Earlier actions are not backfilled.
- Active administrators with the required grants can create, search, filter, page, read, and update users at `/admin/users`. The existing `PUT /users/:id/roles` endpoint assigns one of six roles. Deactivation atomically revokes all active sessions and protects the last active authorized administrator. Changes to the role permission matrix take effect on active sessions.
- New accounts use a 12–72 byte administrator-supplied initial password. The API hashes it and never returns it. The administrator must deliver it privately. **The administrator continues to know this password; self-service password change, reset, and invitation delivery are not implemented yet.** Do not reuse personal passwords for this workflow. Use HTTPS outside local development.
- `GET /audit/events` is a read-only, paginated, filtered, sorted API for administrators with `audit:read`. The advanced viewer and export belong to M10.
- `GET /dashboard` supplies current database role, school branding, granted navigation, enabled module placeholders, and actual administrator counts. The shared frontend dashboard renders `/dashboard/admin`, `/dashboard/head_teacher`, `/dashboard/teacher`, `/dashboard/student`, `/dashboard/guardian`, and `/dashboard/accountant`. A role mismatch redirects to the current server-reported role. Later milestone modules appear only as Coming Soon when enabled; no academic or financial data is invented.
- Local PostgreSQL integration tests, Go vet, the frontend production build, Compose validation, OpenAPI reference parsing, and headless Chrome checks passed. A neutral project favicon now serves successfully. The original development account remained untouched by the final isolated browser audit.

## M1 Final Audit

All twelve M1 acceptance criteria in the [project brief](docs/PROJECT_BRIEF.md) are locally verified. The final audit used a separate PostgreSQL database, backend on port 8081, and rebuilt frontend on port 5174. It exercised browser login/logout, account protection, bilingual fonts, browser user creation and permission editing, active-session grant changes, school profile editing with reload and persistent branding, grading validation, module navigation, and all six role dashboards at 1280 px and 390 px. The temporary stack and database were removed after testing; the developer's application database was not changed.

The backend uses Handler → Service → Repository → Database for M1 domains. Four forward-only migrations add authentication and sessions, normalized role grants, school configuration and grading, and user display names and audit events. Privileged writes record safe audit metadata in their transaction. The 15-minute access token is held in browser memory; a rotated seven-day refresh token is held in an HttpOnly SameSite=Strict cookie and protected by a session-bound CSRF token. Runtime authorization reads current grants and module state from PostgreSQL.

Final local gates passed: `go vet ./...`, `go test ./... -count=1` with and without the dedicated PostgreSQL test URL, `go test -race ./... -count=1` without PostgreSQL, fresh `npm ci` and production build inside the isolated frontend container, `docker compose config --quiet`, npm advisory audit (zero reported vulnerabilities at audit time), and OpenAPI parse with 20 paths, 26 operations, and 151 resolved references. No frontend unit-test command is configured. The Windows host `npm ci` could not replace a locked native binary, so the fresh container install was used. Go dependency advisory scanning and M1 GitHub Actions remain unverified.

The security review found no confirmed critical or high M1 defect. Administrator-assisted initial password delivery remains a **medium** operational risk; use a private channel and a unique temporary password. Account lockout is per account, with no independent IP or edge rate limit (**medium**); deploy a rate limit before public exposure. Production HTTPS and Secure-cookie behavior have not been exercised (**medium**, deployment verification). External HTTPS logo bytes are not inspected and the browser contacts the chosen host (**low**); use a trusted host. There are no implemented optional feature routes yet: module enablement and the `RequireModule` middleware passed PostgreSQL tests on an isolated test route, while rejection by a real optional feature route must be checked when that route is introduced. Do not infer any academic or financial feature from a Coming Soon module link.

M1 is ready for Pull Request review. Merge only after the branch is pushed by the developer, GitHub Actions passes, and the limitations above are accepted. M2 has not started.

## Repository Layout

```text
backend/                  Go API, auth layers, migrations, PostgreSQL connection, tests
frontend/                 React application and API client
.github/workflows/ci.yml  Continuous integration
compose.yaml              Development services
docs/                     Project brief, roadmap, and progress
```

M1 authentication, RBAC, school configuration, user management, audit retrieval, and dashboards follow Handler → Service → Repository → Database. M0's health endpoint remains a simple liveness handler.

## Prerequisites

- Docker with the Compose plugin for the complete container stack; **or** Go 1.26, Node.js 22 with npm, and PostgreSQL for local authentication development
- A unique random JWT signing key of at least 32 bytes, base64 encoded, whenever `DATABASE_URL` is configured

Docker is not required for the health endpoint or for local frontend development. Authentication requires PostgreSQL.

## Run with Docker Compose

From the repository root in PowerShell, generate a key for this environment and start the stack:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
$keyBytes = New-Object byte[] 32
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($keyBytes)
$rng.Dispose()
$env:JWT_SIGNING_KEY_B64 = [Convert]::ToBase64String($keyBytes)
docker compose up --build
```

Reuse the same signing key when restarting the backend so existing access tokens remain valid. Keep it in a secure local environment source or the ignored root `.env`; do not commit it. Use `AUTH_COOKIE_SECURE=true` behind HTTPS; the Compose sample uses `false` solely for local HTTP. Create the first user in another terminal while the stack runs (the command prompts for a password without echo):

```powershell
docker compose exec backend /app/create-user -email you@example.com -role admin
```

Replace `you@example.com` with your own address. No demo account or password is supplied.

Use `docker compose down` to stop the stack. The default endpoints are:

| Service | URL |
|---|---|
| Frontend | http://localhost:5173 |
| Backend health | http://localhost:8080/healthz |
| Health through frontend proxy | http://localhost:5173/api/healthz |
| Login page | http://localhost:5173/login |
| Protected account page | http://localhost:5173/account |
| Permission editor | http://localhost:5173/admin/permissions |
| School settings | http://localhost:5173/admin/school-settings |
| User administration | http://localhost:5173/admin/users |
| Role dashboard | http://localhost:5173/dashboard |

The database is exposed on `localhost:5432` by default. Change the values in `.env` if those ports are occupied. The sample PostgreSQL password is for local development only.

Verify the services:

```powershell
docker compose ps
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:5173/api/healthz
```

## Run Backend and Frontend Locally

In one PowerShell terminal, start the backend without PostgreSQL:

```powershell
cd backend
$env:PORT = '8080'
Remove-Item Env:DATABASE_URL -ErrorAction SilentlyContinue
go run ./cmd/api
```

In a second terminal, start the frontend:

```powershell
cd frontend
npm.cmd ci
npm.cmd run dev
```

Open http://localhost:5173 and check that the API status displays as online. The frontend proxies `/api/healthz` to the backend on port 8080. Without PostgreSQL, the login page can render but the authentication API returns `503 auth_unavailable`. On shells where `npm` is available directly, use `npm ci` and `npm run dev`.

To use PostgreSQL locally, start a PostgreSQL server, then set `DATABASE_URL` before running the backend. For example, from the repository root with the sample Compose database:

```powershell
docker compose up -d db
cd backend
$env:DATABASE_URL = 'postgres://flybirds:flybirds_dev@localhost:5432/flybirds?sslmode=disable'
$keyBytes = New-Object byte[] 32
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($keyBytes)
$rng.Dispose()
$env:JWT_SIGNING_KEY_B64 = [Convert]::ToBase64String($keyBytes)
$env:AUTH_COOKIE_SECURE = 'false'
go run ./cmd/create-user -email you@example.com -role admin
go run ./cmd/api
```

Keep the same environment variables in that backend terminal. The `create-user` command asks for a password without echo. The backend checks the database connection and applies versioned migrations at startup. `GET /healthz` is a liveness check and does not test database readiness. The browser uses the API through `/api/*`. Authentication and authorization routes are documented in [OpenAPI](docs/openapi.yaml).

## Environment Variables

See `.env.example`, `backend/.env.example`, and `frontend/.env.example` for examples.

| Variable | Purpose | Default |
|---|---|---|
| `PORT` | Backend HTTP port | `8080` |
| `DATABASE_URL` | PostgreSQL DSN; required for authentication, optional for the health-only server | Unset |
| `JWT_SIGNING_KEY_B64` | Base64 encoding of at least 32 random signing-key bytes; required when the database is configured | Unset |
| `AUTH_COOKIE_SECURE` | Set `false` only for local HTTP; keep `true` behind HTTPS | `true` in the backend, `false` in local Compose |
| `AUTH_TEST_DATABASE_URL` | Dedicated PostgreSQL database for the opt-in authentication, RBAC, school configuration, and user/audit/dashboard integration tests | Unset |
| `GIN_MODE` | Gin log mode; Compose sets `release` | Gin default |
| `VITE_API_BASE_URL` | Browser API path | `/api` |
| `VITE_API_PROXY_TARGET` | Vite development proxy target | `http://localhost:8080` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Compose database credentials | Values in `compose.yaml` |
| `POSTGRES_PORT`, `BACKEND_PORT`, `FRONTEND_PORT` | Compose host ports | `5432`, `8080`, `5173` |

The backend reads shell environment variables. It does not load `.env` files automatically. Vite loads `frontend/.env` when present; Docker Compose reads the root `.env` when present. Never commit real credentials. Keep the browser API on the same origin through the configured `/api` proxy so refresh cookies and CSRF protection work.

## Verification

```powershell
cd backend
go vet ./...
go test ./...
cd ../frontend
npm.cmd ci
npm.cmd run build
cd ..
docker compose config --quiet
```

For database integration tests, point `AUTH_TEST_DATABASE_URL` at a dedicated disposable PostgreSQL database, then run `go test ./... -count=1` from `backend/`. The HTTP lifecycle tests create isolated users and remove them afterward; school configuration tests also restore the prior settings, grading, and module state. The audit rollback test temporarily adds a rejecting constraint inside a transaction and rolls it back. The versioned schema remains. These tests skip explicitly without that variable. The GitHub Actions workflow runs Go vet, Go tests, and the frontend build on pushes and pull requests. M1 branch CI remains unverified until a developer push.

## Future Work

The following features are documentation only and are excluded from M0–M13:

- Online admission
- Library management
- Stipend management
- Assignment portal
- Chatbot
- Machine-learning prediction
