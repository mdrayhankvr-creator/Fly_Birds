# M5 Attendance — completion handoff

M5 is locally complete on `feature/m5-attendance` after a normal merge of `origin/main` at `69e3298`. M2 Sections, M3 Students/Guardians, and M4 Staff retain their original commits. The merge commit is `0769b80`. No branch was pushed or PR opened.

## Data model and rules

`0501_teacher_attendance.sql` preserves daily teacher attendance. `0502_student_attendance.sql` adds student daily records and durable absence trigger events after versions 1–4, 101–103, 201, 401, and 501. UUID foreign keys connect existing students, sessions, classes, sections, and users. The composite section foreign key protects the stored class/session context. A unique student/session/date constraint prevents duplicate entries, including QR replay on the same day. Deleting a student cascades their attendance and absence events, consistent with M3 deletion behavior. No attendance rows are seeded.

An active teacher or head teacher with an M4 assignment to a session and class may manage attendance for all sections in that class. Administrators may manage any class. This uses the M4 model, whose assignments have no section identifier. A new record requires an active student, class, and section and a date inside the academic session. Corrections retain the original section snapshot. Monthly section totals count records stored in that section. Student and guardian reports verify current account links on every read.

The M3 ID-card QR contains `FB-STUDENT:<student UUID>`. It identifies a student and is **not** an authorization token. QR entry requires login, current `attendance:students_manage` permission, the enabled Attendance module, an assigned class for teachers, and an unrecorded day. Scanning marks only `present`. The UI supports keyboard QR scanners and native browser `BarcodeDetector` camera scanning when available. A copied printed card cannot prove physical presence; staff must supervise the scan.

An `absent` record creates or reopens one `attendance_absence_events` row for that student/session/date. Correcting the record away from `absent` cancels the event. Administrators can review pending triggers. Sending SMS or other notifications belongs to M9 and is not implemented here.

## Routes and permissions

All routes require authentication, current RBAC permission, and `RequireModule("attendance")`.

| Method | Route | Permission |
|---|---|---|
| GET | `/attendance/sessions` | `attendance:teachers_read_self` |
| GET | `/attendance/teachers/roster` | `attendance:teachers_read` |
| POST | `/attendance/teachers` | `attendance:teachers_manage` |
| PATCH | `/attendance/teachers/{id}` | `attendance:teachers_manage` |
| GET | `/attendance/teachers/me` | `attendance:teachers_read_self` |
| GET | `/attendance/teachers/monthly` | `attendance:teachers_reports` |
| GET | `/attendance/teachers/me/monthly` | `attendance:teachers_read_self` |
| GET | `/attendance/students/sections` | `attendance:students_read` |
| GET | `/attendance/students/roster` | `attendance:students_read` |
| POST | `/attendance/students` | `attendance:students_manage` |
| POST | `/attendance/students/qr` | `attendance:students_manage` |
| PATCH | `/attendance/students/{id}` | `attendance:students_manage` |
| GET | `/attendance/students/monthly` | `attendance:students_read` |
| GET | `/attendance/students/me/report` | `attendance:students_read_self` |
| GET | `/attendance/guardians/children/{id}/report` | `attendance:guardians_read_children` |
| GET | `/attendance/alerts` | `attendance:alerts_read` (administrator only) |

Administrators and head teachers retain teacher attendance access. Teachers retain their own history and receive student management only for assigned classes. Students see only their linked profile's report. Guardians see only currently linked children. List endpoints use search and pagination where relevant. The first M5 teacher attendance slice remains intact.

## UI

`/admin/attendance/teachers` and `/attendance/me` retain the bilingual teacher flow. `/admin/attendance/students` provides section roster, daily entry/correction, QR scanning, monthly totals, and administrator alert review. `/student/attendance` shows the linked student's monthly report. `/guardian/children/{id}/attendance` shows a currently linked child's report; its link appears on the M3 children screen. The pages use school colors and responsive layouts.

## Verification

A fresh disposable PostgreSQL database applied versions `1, 2, 3, 4, 101, 102, 103, 201, 401, 501, 502`. Applying the migration runner twice succeeded. The full `go test ./... -count=1` suite passed with `AUTH_TEST_DATABASE_URL` set to that database. The new HTTP integration test covers unauthenticated and module-disabled requests, role restrictions, teacher assignment, duplicate manual and QR entry, invalid QR payloads, corrections, event cancellation/reopening, student and guardian report scoping, search/pagination, and academic foreign-key integrity. `go vet ./...`, the explicit OpenAPI route/reference test, frontend TypeScript/production build, and `docker compose config --quiet` passed.

Headless Chrome browser checks used an isolated mock API with the production frontend bundle. At 1280px English and 390px Bangla, the admin roster and QR input rendered without horizontal overflow or JavaScript exceptions; QR input updated the screen. Student and guardian report routes rendered. These checks validate UI behavior only; PostgreSQL HTTP tests validate server access and persistence. Native camera scanning was not exercised in headless Chrome. Remote CI, review, and production migration/deployment remain unverified.

## Run

Use the root README Docker Compose instructions, then enable the optional Attendance module in school settings. For guardians to discover children in the existing M3 screen, enable the Students module too. No demo credentials are shipped. For isolated PostgreSQL tests, set `AUTH_TEST_DATABASE_URL` to a disposable database and run `go test ./... -count=1` from `backend/`. Never use the application database for integration tests.
