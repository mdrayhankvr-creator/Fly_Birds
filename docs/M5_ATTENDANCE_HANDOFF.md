# M5 Attendance — teacher slice and shared-schema handoff

## Verified implementation

Migration `0501_teacher_attendance.sql` follows M4's `0401_staff.sql`. It creates only `teacher_attendance` and attendance permissions. It references the existing employee, academic session, and user UUIDs. The unique key `(session_id, employee_id, attendance_date)` prevents duplicate daily entries, including concurrent requests. The date and teacher-employment relationships are checked in the service transaction; `leave` requires an approved M4 employee leave covering the date. Corrections lock the record, update its status and note, and audit the old and new status. The attendance module remains disabled by default; no attendance rows are seeded.

The authenticated APIs are documented in [OpenAPI](openapi.yaml):

| Method | Route | Permission | Purpose |
|---|---|---|---|
| GET | `/attendance/sessions` | `attendance:teachers_read_self` | Search/page academic sessions for attendance users |
| GET | `/attendance/teachers/roster` | `attendance:teachers_read` | Search/page the daily teacher roster with recorded status |
| POST | `/attendance/teachers` | `attendance:teachers_manage` | Record one teacher on one session/date |
| PATCH | `/attendance/teachers/{id}` | `attendance:teachers_manage` | Correct status and note |
| GET | `/attendance/teachers/me` | `attendance:teachers_read_self` | Page through own history |
| GET | `/attendance/teachers/monthly` | `attendance:teachers_reports` | Search/page monthly teacher totals |
| GET | `/attendance/teachers/me/monthly` | `attendance:teachers_read_self` | Own monthly totals |

Administrators and head teachers can read/manage the roster and monthly teacher report. A head teacher cannot mark or correct their own record. Teachers can read only their own history and totals through the authenticated user-to-employee link. All routes enforce `RequirePermission` and `RequireModule("attendance")`. The frontend provides `/admin/attendance/teachers` and `/attendance/me` in English and Bangla with responsive daily entry, corrections, history, monthly totals, loading/error/empty states, and school colors.

The teacher attendance slice passed `go vet ./...`, `go test ./... -count=1` with a fresh disposable PostgreSQL database, the frontend production build, and the OpenAPI route/reference test. The full migration order was `1, 2, 3, 4, 101, 102, 401, 501`. The dedicated PostgreSQL HTTP test covers unauthenticated and disabled-module requests, admin/head-teacher/teacher permissions, invalid and out-of-session dates, approved leave, self-marking denial, unique and concurrent duplicate prevention, correction, history, pagination, search, and monthly counts. Isolated headless Edge flows at 1280 px and 390 px recorded attendance as an administrator, viewed history and totals as a linked teacher, denied teacher access to the admin page, switched English/Bangla, and found no horizontal overflow. The existing developer Docker stack, application database, users, and volumes were not modified.

On 2026-10-17, the M5 branch was rebased onto the M4 pagination fix and fresh teacher entries were restricted to active employees. Historical corrections remain allowed. Go vet, the database-free Go suite, frontend build, and OpenAPI test passed on the rebased branch. New PostgreSQL assertions for nested staff pagination and inactive teacher entry are pending because Docker Desktop's daemon was unavailable. Browser checks above predate the pagination update; no new browser run is claimed.

## M2/M3 contract required to finish M5

The fetched `origin/main` on 2026-10-17 contains sessions, classes, subjects, and class-subject mappings, but no published Sections or Students/Guardians schema or remote branches for them. Student attendance and QR scanning are **not** wired to invented tables or production fixtures. Before integration, the shared M2/M3 implementation must provide and document:

1. Stable UUID identifiers for sections and students, with a verified section-to-class/session relationship. The enrollment or placement record must establish each student's class, section, and academic session on a given date, including transfers and inactive dates.
2. The existing-user link for student self-view and an authoritative guardian-to-student link through guardian accounts. Access checks must query current links from the database, so a guardian can view only linked children and a student only their own records.
3. A teacher-to-class/section authorization rule. M4 assignments identify session, class, and subject, but do not yet identify a section. The M2 Sections owner must define whether a class assignment authorizes every section or whether explicit section assignment is required.
4. The student ID-card QR payload and verification rules. A scanned raw student UUID is insufficient as an attendance authorization token. The payload format, issue/revoke semantics, and replay handling must be agreed with the M3 owner.
5. Guardian contact/notification preferences and an approved alert-delivery boundary. An absence event should be emitted once per student/session/date and later consumed by the M9 notification provider without duplicate alerts.

After those contracts are merged, add a new migration referencing the **actual** shared student/enrollment/section tables. It should store session, student, class and section context for the date, status, actor, correction history, and a unique student/session/date key. Then implement teacher-authorized entry/correction, student and guardian read endpoints, QR scanning, student monthly reports, and absence alert triggers. Run the full PostgreSQL and browser suites against the integrated schema before marking those criteria complete. Do not create a competing `students`, `guardians`, or `sections` table in this branch.

## Progress and integration order

M5 has **1/5 fully verified roadmap criteria**: daily teacher attendance. Daily student attendance, QR scanning, and absence alert triggers are blocked on the shared M2/M3 contract. Monthly reports are implemented and verified for teachers but remain **partial** until student reports are integrated. Overall roadmap progress is **26/83 = 31.3%** locally.

Review and merge the shared M2 Sections and M3 Students/Guardians changes before completing student attendance. Review M4 before M5 because `0501` references `0401`. Then rebase or merge the shared M2/M3 changes into M5, implement the real student integration, rerun migrations and all gates, and only then consider M5 ready for its own PR. No branch was pushed or merged by this batch.

For local use, follow [README](../README.md), enable the **Attendance** module in school settings, and visit the teacher attendance pages. PostgreSQL integration tests require `AUTH_TEST_DATABASE_URL` pointing to a separate disposable database. Do not point it at the application database.
