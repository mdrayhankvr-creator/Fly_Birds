# Fly_Birds

**A white-label school management system for schools in Bangladesh.**
Attendance, results, fees, notices, class diary and analytics for
administrators, head teachers, teachers, accountants, students and guardians,
in Bangla and English. Each school runs its own private installation, with its
own database, files, backups, name, logo and colours.

![Administrator dashboard](docs/screenshots/admin-dashboard.png)

## Screenshots

| | |
|---|---|
| ![Public landing page](docs/screenshots/landing.png) | ![School analytics](docs/screenshots/analytics.png) |
| Public landing page with the school's own branding | School analytics: attendance, results, alerts and money |
| ![Roll-wise attendance register](docs/screenshots/attendance.png) | ![Guardian dashboard in Bangla](docs/screenshots/guardian-bangla.png) |
| Roll-wise attendance: one tap per student, save once | Guardian dashboard in Bangla, with a child selector |
| ![Accountant dashboard](docs/screenshots/accountant-dashboard.png) | ![Dark theme](docs/screenshots/dark-theme.png) |
| Accountant dashboard: collections, dues and overdue aging | Dark theme |

<p align="center"><img src="docs/screenshots/student-mobile.png" alt="Student dashboard on a phone" width="260"></p>

All screenshots show the fictional demo school created by `seed-demo`.

## Features

| Area | What it does |
|---|---|
| **Accounts and security** | Email sign-in, 15-minute access tokens with rotating refresh cookies and CSRF protection, account lockout after repeated failures, per-request role permissions, an editable permission matrix and a full audit log. |
| **School configuration** | School name in Bangla and English, logo, colours (contrast-checked), preferred language, grading scheme and 13 modules that can be switched on or off. |
| **Academic structure** | Academic sessions, classes, sections, subjects (with fourth-subject support) and class-subject mapping. |
| **Students and guardians** | Admission records, photos, roll numbers, guardian links and CSV import. |
| **Teachers and staff** | Staff profiles, subject assignments, leave requests and private salary records. |
| **Attendance** | Roll-wise daily student register, conflict-safe saving, QR-assisted check-in, teacher attendance, monthly reports and consecutive-absence alerts. |
| **Routines** | Class timetables with clash detection and exam routines. |
| **Exams and results** | Exams per section, marks entry by assigned teachers, Bangladesh GPA grading, positions, mark sheets and published results. |
| **Fees and accounts** | Fee categories and structures, monthly dues, concessions, collections with receipts, overdue tracking and an income and expense ledger. Online bKash and Nagad payment is **simulated** (see [Limitations](#limitations)). |
| **Communication** | Notices and events targeted by role, class or section; a daily class diary with homework and submissions; in-app notifications with per-user preferences. SMS is **simulated**. |
| **Analytics and reports** | Role-specific dashboards, attendance and result trends, dropout and at-risk alerts, finance analytics, and CSV and printable reports. |
| **Administration** | Control center, system health, data exports, verified backups with retention, and restore into an empty database. |
| **Interface** | Responsive layout from phone to desktop, full Bangla and English (with Bangla digits and dates), light and dark themes, keyboard support and reduced-motion support. |

## Architecture

```mermaid
flowchart LR
    browser["Browser<br/>React 19 + TypeScript"] -->|HTTPS| edge["Caddy<br/>TLS, HSTS"]
    edge --> web["nginx<br/>static app, CSP,<br/>login rate limit"]
    web -->|"/api/"| api["Go API<br/>Gin, JWT, RBAC"]
    api --> db[("PostgreSQL 17")]
    api --> files[("Uploads volume")]
    api --> backups[("Backups volume")]
```

- **Backend:** Go 1.26, Gin, PostgreSQL 17 with versioned SQL migrations
  applied at startup, and a Handler → Service → Repository structure.
- **Frontend:** React 19, Vite, TypeScript, Tailwind CSS, TanStack Query and
  i18next.
- **Deployment:** Docker Compose, one project per school (see
  [Deployment](docs/DEPLOYMENT.md)).

## Documentation

| Document | Contents |
|---|---|
| [Deployment guide](docs/DEPLOYMENT.md) | Installing a school on a server, HTTPS, environment variables, upgrades and rollback |
| [Operations guide](docs/OPERATIONS.md) | First administrator, school setup, password resets, backups and recovery, troubleshooting, release validation |
| [API reference (OpenAPI 3)](docs/openapi.yaml) | Every endpoint, with permissions, request and response schemas |
| [ER diagram](docs/diagrams/er-diagram.md) | Database schema by domain |
| [Data flow diagram](docs/diagrams/data-flow-diagram.md) | Context and level 1 data flows |
| [Use case diagram](docs/diagrams/use-case-diagram.md) | What each role can do |
| [Final project report](docs/FINAL_REPORT.md) | Scope, design, testing, security and status |
| [Progress](docs/PROGRESS.md) and [roadmap](docs/roadmap.md) | Milestone-by-milestone evidence |
| [Browser suites](e2e/README.md) | End-to-end checks against a running stack |

To browse the API reference, open `docs/openapi.yaml` in any OpenAPI viewer,
for example by pasting it into https://editor.swagger.io.

## Quick start (development)

Requirements: Docker with the Compose plugin. From the repository root in
PowerShell:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
$keyBytes = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($keyBytes)
$env:JWT_SIGNING_KEY_B64 = [Convert]::ToBase64String($keyBytes)
docker compose up -d --build
docker compose exec backend /app/create-user -email you@example.com -role admin
```

Open http://localhost:5173 and sign in with the account you just created
(`create-user` asks for the password without echoing it). Keep the same
signing key across restarts, or everyone is signed out. The development stack
uses plain HTTP and publishes PostgreSQL on port 5432; never use it for a real
school. For a production installation follow the
[deployment guide](docs/DEPLOYMENT.md).

To explore with sample data instead, create a fictional demo school (8 role
accounts, 36 students, three months of activity). It never changes records it
did not create, but do not run it on a real school's database:

```powershell
$env:DEMO_PASSWORD = '<a password of at least 12 characters>'
docker compose exec -e DEMO_PASSWORD backend /app/seed-demo -confirm
```

The demo accounts are `demo.admin`, `demo.head`, `demo.teacher`,
`demo.accounts`, `demo.student` and `demo.guardian`, each
`@demo.flybirds.test`.

Stop the stack with `docker compose down`. **Never add `-v`**: that deletes
the database, uploads and backups volumes.

### Running without Docker

Start PostgreSQL (for example `docker compose up -d db`), then:

```powershell
cd backend
$env:DATABASE_URL = 'postgres://flybirds:flybirds_dev@localhost:5432/flybirds?sslmode=disable'
$env:JWT_SIGNING_KEY_B64 = '<base64 of 32 random bytes>'
$env:AUTH_COOKIE_SECURE = 'false'
go run ./cmd/api
```

In a second terminal: `cd frontend; npm ci; npm run dev`. Vite serves
http://localhost:5173 and proxies `/api` to the backend on port 8080.

## Configuration

The backend reads environment variables only; it does not load `.env` files.
Docker Compose reads the root `.env` (development) or the school's file under
`deploy/` (production). Never commit real credentials.

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | unset (health endpoint only) |
| `JWT_SIGNING_KEY_B64` | Base64 of at least 32 random bytes; required with a database | unset |
| `AUTH_COOKIE_SECURE` | `true` behind HTTPS; `false` only for local HTTP | `true` |
| `PORT` | Backend HTTP port | `8080` |
| `FILE_STORAGE_DIR` | Uploaded attachments | `data/uploads` |
| `BACKUP_DIR` | Backup archives (keep outside the web root) | `data/backups` |
| `BACKUP_RETENTION` | Completed archives to keep (1–365) | `10` |
| `BACKUP_INTERVAL` | Automatic backup interval, `0` for manual only, else at least `1h` | `0` |
| `NOTIFY_INTERVAL` | Notification dispatch interval, `0` disables, else at least `10s` | `1m` |
| `GIN_MODE` | `release` in containers | Gin default |
| `AUTH_TEST_DATABASE_URL` | Disposable database for integration tests only | unset |
| `DEMO_PASSWORD` | Password for `seed-demo` accounts | unset |
| `VITE_API_BASE_URL`, `VITE_API_PROXY_TARGET` | Frontend API path and development proxy target | `/api`, `http://localhost:8080` |

The production variables (`COMPOSE_PROJECT_NAME`, `SCHOOL_DOMAIN`,
`ACME_EMAIL`, `WEB_PORT`, `RELEASE_TAG`, database credentials) are described
in [deploy/school.env.example](deploy/school.env.example) and the
[deployment guide](docs/DEPLOYMENT.md).

## Testing

```powershell
cd backend
go vet ./...
$env:AUTH_TEST_DATABASE_URL = 'postgres://user:pass@localhost:5433/fresh_test_db?sslmode=disable'
go test ./... -p 1 -count=1
cd ../frontend
npm ci
npm run check:i18n
npm run build
```

- **Integration tests** need a **fresh, dedicated, disposable** PostgreSQL
  database; without `AUTH_TEST_DATABASE_URL` they are skipped. Never point it
  at a school's database.
- **Browser suites** in [e2e/](e2e/README.md) drive a real headless Chrome or
  Edge against a running stack with a freshly seeded demo school.
- **CI** ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs
  formatting, vet, unit and PostgreSQL integration tests, govulncheck, the
  translation check, the frontend build and npm audit. It then builds the
  production stack and runs the browser suites against it.

## Repository layout

```text
backend/        Go API, migrations (internal/database/migrations), operator commands (cmd/)
frontend/       React application and production nginx configuration
deploy/         Per-school production Compose stack, Caddy TLS edge, environment template
e2e/            Browser suites
docs/           Brief, progress, OpenAPI, diagrams, guides, screenshots and final report
compose.yaml    Development stack
```

## Limitations

- **Payments:** online bKash and Nagad payment uses a **mock** provider. No
  money moves and no real merchant account is involved. Cash and manual
  receipts are real records.
- **SMS:** messages use a **mock** provider. They are recorded and marked sent
  but never reach a phone.
- **Passwords:** there is no self-service password reset or in-app password
  change yet. The school office asks the server operator to run
  `reset-password` (see the [operations guide](docs/OPERATIONS.md)).
- One installation serves one school. Several schools on one server need one
  Compose project each.

## Future Work

The following features are documentation only and are excluded from M0–M13:

- Online admission
- Library management
- Stipend management
- Assignment portal
- Chatbot
- Machine-learning prediction
