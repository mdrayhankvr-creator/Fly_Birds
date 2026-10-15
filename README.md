# Fly_Birds

Fly_Birds is a reusable, white-label school management system for schools in Bangladesh. One codebase will support school-specific branding, language, grading, permissions, and module settings. The current repository contains **M0 foundation only**. Authentication, school configuration, and role dashboards belong to M1 and are not implemented yet.

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

No accounts or demo credentials exist in M0. No database tables or migrations are created yet.

## Repository Layout

```text
backend/                  Go API, configuration, PostgreSQL connection, tests
frontend/                 React application and API client
.github/workflows/ci.yml  Continuous integration
compose.yaml              Development services
docs/                     Project brief, roadmap, and progress
```

The planned backend business architecture is Handler → Service → Repository → Database. M0 has no business entities; later milestones will use that layering.

## Prerequisites

- Docker with the Compose plugin for the complete container stack; **or** Go 1.26 and Node.js 22 with npm for local development
- PostgreSQL if you want to exercise the backend database connection locally

Docker is not required for the M0 health endpoint or for local frontend development.

## Run with Docker Compose

From the repository root in PowerShell:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
docker compose up --build
```

Use `docker compose down` to stop the stack. The default endpoints are:

| Service | URL |
|---|---|
| Frontend | http://localhost:5173 |
| Backend health | http://localhost:8080/healthz |
| Health through frontend proxy | http://localhost:5173/api/healthz |

The database is exposed on `localhost:5432` by default. Change the values in `.env` if those ports are occupied. The sample PostgreSQL password is for local development only.

Verify the services:

```powershell
docker compose ps
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:5173/api/healthz
```

## Run Locally without Docker

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

Open http://localhost:5173 and check that the API status displays as online. The frontend proxies `/api/healthz` to the backend on port 8080. On shells where `npm` is available directly, use `npm ci` and `npm run dev`.

To use PostgreSQL locally, start a PostgreSQL server, then set `DATABASE_URL` before running the backend. For example, with the sample Compose database:

```powershell
docker compose up -d db
$env:DATABASE_URL = 'postgres://flybirds:flybirds_dev@localhost:5432/flybirds?sslmode=disable'
go run ./cmd/api
```

The backend checks that configured database connection at startup. `GET /healthz` is a liveness check and does not test database readiness.

## Environment Variables

See `.env.example`, `backend/.env.example`, and `frontend/.env.example` for examples.

| Variable | Purpose | Default |
|---|---|---|
| `PORT` | Backend HTTP port | `8080` |
| `DATABASE_URL` | Optional PostgreSQL DSN for direct backend run | Unset |
| `GIN_MODE` | Gin log mode; Compose sets `release` | Gin default |
| `VITE_API_BASE_URL` | Browser API path | `/api` |
| `VITE_API_PROXY_TARGET` | Vite development proxy target | `http://localhost:8080` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Compose database credentials | Values in `compose.yaml` |
| `POSTGRES_PORT`, `BACKEND_PORT`, `FRONTEND_PORT` | Compose host ports | `5432`, `8080`, `5173` |

The backend reads shell environment variables. It does not load `.env` files automatically. Vite loads `frontend/.env` when present; Docker Compose reads the root `.env` when present. Never commit real credentials.

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

The GitHub Actions workflow runs the Go and frontend checks on pushes and pull requests. Its remote execution is pending until the branch is pushed by the developer.

## Future Work

The following features are documentation only and are excluded from M0–M13:

- Online admission
- Library management
- Stipend management
- Assignment portal
- Chatbot
- Machine-learning prediction
