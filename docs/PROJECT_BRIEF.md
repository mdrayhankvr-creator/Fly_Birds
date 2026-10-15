# Fly_Birds — Project Brief

Fly_Birds is a reusable, white-label school management system for Bangladesh. The user is the only developer; the AI agent handles architecture, implementation, tests, documentation, and Git workflow.

## 1. Product Goal

Fly_Birds is a reusable, white-label school management system for typical schools in Bangladesh.

One codebase must support different schools. School-specific information must be configurable rather than hard-coded.

Configurable settings include:

- School name and logo
- Branding colors and dashboard themes
- Language
- Grading scheme
- Enabled or disabled modules

The administrator controls school configuration, permissions, and enabled modules.

## 2. Technology Stack

Backend:

- Go
- Gin
- GORM
- PostgreSQL
- JWT authentication
- Access token lifetime: 15 minutes
- Refresh token lifetime: 7 days
- RBAC
- Swagger/OpenAPI

Frontend:

- React
- Vite
- TypeScript
- Tailwind CSS
- TanStack Query
- React Router
- i18next for Bangla and English

Development:

- Docker Compose
- Git and GitHub
- Conventional Commits
- Automated tests and CI

M0 foundation, health endpoint, CI, and basic documentation are considered complete according to the initial brief, but repository verification is required.

## 3. User Roles

The system supports six roles:

1. admin
2. head_teacher
3. teacher
4. student
5. guardian
6. accountant

Every protected API route must enforce `RequirePermission`.

Every module-specific route must enforce `RequireModule`.

Administrators must be able to edit role permissions and enable or disable modules at runtime.

## 4. Bangladesh Grading System

| Grade | Marks | GPA |
|---|---|---|
| A+ | 80–100 | 5.00 |
| A | 70–79 | 4.00 |
| A- | 60–69 | 3.50 |
| B | 50–59 | 3.00 |
| C | 40–49 | 2.00 |
| D | 33–39 | 1.00 |
| F | 0–32 | 0.00 |

The grading scheme must be configurable by school settings.

## 5. Complete Development Roadmap

### M0 — Foundation

- Project foundation
- Health endpoint
- CI
- Initial documentation

### M1 — Authentication and School Configuration

- Login and authentication
- Users
- Roles and permissions matrix
- JWT access and refresh tokens
- School settings and white-label configuration
- Bangladesh grading configuration
- Runtime module toggles
- Audit logging
- Login lockout
- Themed login page
- Bangla/English language switch
- Role-based dashboards

### M2 — Academic Structure

- Academic sessions
- Classes
- Sections
- Subjects
- Class-subject mapping
- Fourth/optional subject flag

### M3 — Students and Guardians

- Student admission
- Student profiles
- Student photos
- Student ID cards with QR codes
- Bulk CSV import
- Guardian-student relationships

### M4 — Teachers and Staff

- Teacher and staff profiles
- Subject assignment
- Leave management
- Salary records

### M5 — Attendance

- Daily student attendance
- Daily teacher attendance
- QR attendance scanning
- Monthly attendance reports
- Absence alert triggers

### M6 — Routine

- Class routines
- Exam routines
- Teacher scheduling conflict detection

### M7 — Examinations and Results

- Exam management
- Marks entry
- Bangladesh GPA calculation
- Fourth-subject GPA rule
- Marksheet generation
- Tabulation sheets
- Class and section merit positions
- Result publication

### M8 — Fees and Accounts

- Fee structures
- Student dues
- Payment receipts
- Mock bKash/Nagad payments
- Income and expense tracking
- Fee collection reports

### M9 — Notices and Notifications

- Notice management
- Events calendar
- Mock SMS provider behind a replaceable interface
- Absence notifications
- Result notifications
- Fee-due notifications
- Notice notifications

### M10 — Admin Control Center

- Audit log viewer
- Data export
- Backup
- Permission editor improvements
- Module management

### M11 — Analytics

- Attendance analytics
- Pass-rate charts
- Fee collection analytics
- At-risk student alerts
- Dropout alerts

### M12 — UI and Product Polish

- Responsive user interface
- Complete Bangla/English translations
- White-label theme verification
- Empty and error states
- Idempotent demo data seeding

### M13 — Final Release

- Automated testing
- Complete Swagger documentation
- Professional README with screenshots
- ER diagram
- Data Flow Diagram
- Use case diagram
- Final project report

## 6. Future Features — Documentation Only

List these features in README under Future Work. Do not implement them during M0–M13:

- Online admission
- Library management
- Stipend management
- Assignment portal
- Chatbot
- Machine-learning prediction

## 7. Architecture Requirements

Backend architecture:

Handler → Service → Repository → Database

Requirements:

- Clear separation of responsibilities
- Standardized JSON errors
- Input validation
- Pagination and search for list endpoints
- Swagger annotations
- Idempotent data seeding
- Secure authentication
- Permission enforcement
- Module enforcement
- Reusable school configuration
- Automated tests for new business logic

## 8. Git and GitHub Workflow

For every milestone:

1. Start from an up-to-date `main` branch.
2. Create `feature/<milestone-name>`.
3. Implement backend and frontend together.
4. Create frequent, meaningful Conventional Commits.
5. Separate commits for models, migrations, repositories, services, handlers, tests, frontend, and documentation when appropriate.
6. Never create artificial or empty commits.
7. Never push automatically.
8. Update README and `docs/roadmap.md` on milestone completion.
9. Stop after completing the current milestone.
10. Never start the next milestone without my explicit instruction: `next`.

Maintain professional GitHub documentation, branch naming, and commit history.

Do not invent completion evidence or test results.

## 9. Progress Tracking

Maintain `docs/PROGRESS.md`.

Track:

- Overall project completion percentage
- Current milestone completion percentage
- Completed acceptance criteria
- Remaining acceptance criteria
- Current development phase
- Features implemented
- Tests passed and failed
- Known issues and blockers
- Application runnability
- Next planned milestone

Use a transparent acceptance-criteria-based progress formula.

Do not fabricate percentages.

Provide progress updates after meaningful implementation stages.

## 10. Application Run Instructions

At every appropriate runnable checkpoint, report:

- Prerequisites
- Docker Compose commands
- Backend startup command
- Frontend startup command
- Required environment variables
- Backend URL
- Frontend URL
- Demo credentials, if available
- Verification steps

Keep the application runnable throughout development whenever practical.

## 11. Quality Gates

Before reporting a milestone complete:

- Run `go vet` on the applicable Go module.
- Run `go test` on the applicable Go module.
- Run the frontend production build.
- Run relevant additional tests.
- Fix failures caused by the milestone.
- Update relevant documentation.
- Report any unverified functionality honestly.

## 12. Milestone Completion Report

After each milestone report:

1. Overall project completion percentage
2. Milestone completion percentage
3. New and changed files, depth 4
4. Branch Git log using `git log --oneline`
5. Completed backend and frontend features
6. API endpoints: method, path, permission
7. Frontend pages
8. How to run the application
9. Login credentials, if available
10. Test and build results
11. Known issues, assumptions, incomplete work
12. Push and Pull Request commands
13. Next milestone scope

## 13. Development Schedule

The original target is an ambitious seven-day plan covering M1–M13. Treat it as a planning target, not permission to skip testing, security, or acceptance criteria.

Quality, correctness, and milestone approval take priority over the target duration.
