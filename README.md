# Fly_Birds

Fly_Birds is a reusable, white-label school management system for schools in Bangladesh. One codebase will support school-specific branding, language, grading, permissions, and module settings. M0 is merged. M1 checkpoint 1 adds the authentication foundation; school settings, runtime RBAC, module toggles, and role dashboards remain pending.

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

The login flow is unit-tested, but database-backed login and Docker startup have not been verified in this environment because PostgreSQL and the Docker daemon are unavailable.

## Repository Layout

```text
backend/                  Go API, auth layers, migrations, PostgreSQL connection, tests
frontend/                 React application and API client
.github/workflows/ci.yml  Continuous integration
compose.yaml              Development services
docs/                     Project brief, roadmap, and progress
```

M1 authentication follows Handler → Service → Repository → Database. M0's health endpoint remains a simple liveness handler.

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

Keep that environment variable available when restarting the backend. Use `AUTH_COOKIE_SECURE=true` behind HTTPS; the Compose sample uses `false` solely for local HTTP. Create the first user in another terminal while the stack runs (the command prompts for a password without echo):

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

Keep the same environment variables in that backend terminal. The `create-user` command asks for a password without echo. The backend checks the database connection and applies the versioned migration at startup. `GET /healthz` is a liveness check and does not test database readiness. The authentication API routes are `/auth/login`, `/auth/refresh`, `/auth/logout`, and `/auth/me`; the browser uses them through `/api/auth/*`.

## Environment Variables

See `.env.example`, `backend/.env.example`, and `frontend/.env.example` for examples.

| Variable | Purpose | Default |
|---|---|---|
| `PORT` | Backend HTTP port | `8080` |
| `DATABASE_URL` | PostgreSQL DSN; required for authentication, optional for the health-only server | Unset |
| `JWT_SIGNING_KEY_B64` | Base64 encoding of at least 32 random signing-key bytes; required when the database is configured | Unset |
| `AUTH_COOKIE_SECURE` | Set `false` only for local HTTP; keep `true` behind HTTPS | `true` in the backend, `false` in local Compose |
| `AUTH_TEST_DATABASE_URL` | Dedicated PostgreSQL database for the opt-in authentication integration test | Unset |
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

For the database lifecycle test, point `AUTH_TEST_DATABASE_URL` at a dedicated disposable PostgreSQL database, then run `go test ./internal/auth -run TestPostgresAuthenticationLifecycle -v` from `backend/`. The test creates a user and removes it afterward; the versioned schema remains. Without that variable, the integration test skips explicitly. The GitHub Actions workflow runs Go vet, Go tests, and the frontend build on pushes and pull requests. M1 branch CI remains unverified until a developer push.

## Future Work

The following features are documentation only and are excluded from M0–M13:

- Online admission
- Library management
- Stipend management
- Assignment portal
- Chatbot
- Machine-learning prediction
