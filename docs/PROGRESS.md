# Fly_Birds — Development Progress

## Measurement

Each bullet under M0–M13 in [PROJECT_BRIEF.md](PROJECT_BRIEF.md) is one provisional acceptance criterion. There are 83 roadmap criteria, including four for M0. Count a criterion when its implementation has repository evidence and applicable local verification. Recalculate both denominators if the criteria are refined.

- **Overall completion:** 66 verified criteria / 83 total criteria x 100 = **79.5%**.
- **Current milestone completion:** 5 verified M10 criteria / 5 M10 criteria = **100% locally**.
- **Remaining:** 17 criteria in M11–M13. **Next planned milestone:** M11 Analytics, only after explicit approval.

M0–M9 and the attendance UX redesign are merged into main (M9 through PR #14). M10 is complete locally on `feature/m10-admin-controls`, which is not pushed.

## Current Status — M10 Admin Control Center (2026-10-18)

M10 extends the M1 settings, RBAC, module and audit foundations instead of replacing them. No table is recreated, and the existing `/roles`, `/permissions`, `/school/modules` and `/audit/events` routes keep their contracts, with optional additions only.

| Brief criterion | Verified implementation |
|---|---|
| Audit log viewer | `/admin/audit` lists events with the actor's name, email and role. It filters by action (with counts from `GET /audit/actions`), outcome, Bangladesh date range and an escaped text search over action, target and person, with expandable metadata, newest/oldest order and paging. Signed-in users refused (401/403) by administrative routes are now recorded as `admin.access_denied` with method, route and reason only. The log stays append-only |
| Data export | `/admin/exports` streams nine CSV datasets: users, students, guardians, staff, student attendance, exam results, fee payments, ledger and audit log. Dated datasets need a range of at most 400 days. Files are UTF-8 with a BOM so Bangla opens correctly in spreadsheets, and cells starting with `= + - @`, tab or CR get a leading `'` against formula injection. Password hashes, sessions and file contents are never exported, and every export is audited as `data.exported` |
| Backup | `/admin/backups` takes a consistent backup: one read-only REPEATABLE READ snapshot of every table except `auth_sessions`, plus sequence values and all attachments, in a `.tar.gz` with a SHA-256 manifest under `BACKUP_DIR` (outside the database and web root). Only one backup runs at a time, the newest `BACKUP_RETENTION` (default 10) archives are kept, and `BACKUP_INTERVAL` can schedule them. Admins verify, download and delete archives, each audited. The `restore-backup` command restores an archive into a migrated database of the same schema version and refuses a database that already has users unless `-force` is given |
| Permission editor improvements | `/admin/permissions` groups permissions by area with search and per-group select/clear, locks the protected admin grants and shows each role's active users. Changes are reviewed as additions and removals before saving. Stale saves are refused (`expected_ids` → 409 `permissions_changed`), so two administrators cannot overwrite each other. A role comparison matrix (`GET /permissions/matrix`) and a per-role history of additions and removals from the audit log (`GET /roles/{id}/permissions/history`) were added |
| Module management | `/admin/modules` shows every module with what it needs and what uses it. Migration `1001` records the dependencies: students and staff need academics; attendance, examinations and diary need students and staff; routine needs academics and staff; fees needs students. The API refuses a change that breaks a dependency with 409 `module_dependencies`. With `cascade`, the related modules switch in one transaction and each change is audited. School settings now link to this page instead of duplicating the toggles |

**Control center.** `/admin/control` summarises accounts by role, deactivated accounts, active students, guardians and staff, enabled modules, outstanding and overdue fees, published and draft exams, the mock SMS queue, attachment storage, database size and schema version, audit activity and refused admin requests in the last 24 hours, the last backup and recent events. Each figure links to its area and comes from the database.

**Security.** Every M10 route requires an active administrator plus the route's permission: `system:read`, `data:export` or `backups:manage` (new, granted to admins only), or the existing `audit:read`, `permissions:read` or `modules:read`/`modules:manage`. Head teachers, accountants and teachers get 403 on every control-center route even when granted the permission, and the refusal is audited.

**Schema.** `1001_admin_control.sql` adds `module_definitions.depends_on`, the `backup_runs` table (a unique partial index allows one running backup), an audit target index and the three permissions. It is idempotent and changes no existing data.

**Verification (2026-10-18).**
- **Backend:** `go vet ./...` passed. In a final run on 2026-10-19 against a freshly created disposable PostgreSQL 17.11 database, `go test ./... -p 1 -count=1` passed 68 of 68 top-level tests and 17 of 17 subtests, with none failed or skipped. All 18 packages with tests passed (7 packages have no test files). This includes `TestPostgresM10AdminControls`, `TestOpenAPICoversRegisteredRoutes` and every M0–M9 integration test. Tests that enabled a module without its prerequisites were adapted.
- **Unit tests:** module dependency planning (requires, required-by and cascade), CSV formula neutralisation, export range limits and safe file names.
- **M10 PostgreSQL HTTP test:**
  - **Authorization:** 401 without a session and 403 for head teachers, accountants and teachers on all ten control-center routes, with the refusals recorded and searchable.
  - **Audit and overview:** filter validation, wildcard escaping and overview figures.
  - **Exports:** BOM, Bangla, formula neutralisation, no password hashes, range limits and auditing.
  - **Backups:** create, concurrent-run refusal, verify, download checksum, tamper detection, retention pruning and delete.
  - **Restore drill:** restore into a separate, freshly migrated database, comparing row counts, Bangla text and attachments; refusal over existing data and across schema versions.
  - **Permissions and modules:** matrix, protected grants, stale-save refusal and history; dependency refusal and cascade in both directions.
- **Frontend and Compose:** `tsc`, the production build and `docker compose config --quiet` passed. The backend Docker image (Docker Engine 29.7.2) built successfully on 2026-10-19. It contains `api`, `create-user` and `restore-backup`, runs as the non-root `app` user, and has `/app/backups` and `/app/uploads` owned by that user; `restore-backup` refuses to run without `DATABASE_URL`, `-archive` and `-confirm`. The image was built only as a check and was then removed.
- **Browser:** headless Edge on an isolated database, ports (8093/5193) and upload and backup directories.
  - **Main run:** 40 of 42 checks passed. The two failures were the headless browser not saving downloads from an isolated profile.
  - **Export bytes:** a follow-up run captured the exported file in the page and passed 4 of 4 (BOM, Bangla, neutralised formulas, no password column).
  - **Coverage:** control center; module cascade prompts in both directions and the core-module lock; protected grants, permission search, review, stale-save refusal and reload, history and matrix; denied-request filtering and details, text search and date validation; exports and range limits; backup create, verify and delete; the settings link; head-teacher denial on every admin page; and all six admin pages in Bangla at 390 px. There was no horizontal overflow and no JavaScript exception.
- **Fixes from verification:** unranged export counts passed unused date parameters and failed with 500; export file names now ignore invalid ranges; the audit sort control no longer stretches across the filter bar.
- **Isolation:** the application database, QA stacks, demo-school data, application volumes and secrets were not touched.

**Not verified / limitations.**
- **Restore** is a server-operator command, not a browser action, by design. It needs a PostgreSQL superuser (the Compose database user is one) and PostgreSQL 15 or later. Restoring into a live database was not exercised; the drill used a separate database.
- **Backups** are written to one local volume (`backups_data`). Off-site copies, encryption at rest and storage quotas are the operator's responsibility.
- **Exports** are CSV only (no Excel or PDF), and each file covers at most 400 days.
- **Denied-access logging** records signed-in users only; anonymous probes are not logged. Module dependencies are set by the migration and are not editable in the UI.
- **Not run:** remote CI, production migration and deployment.

## Current Status — M9 Notices, Notifications, Events and Class Diary (2026-10-18)

| Brief criterion | Verified implementation |
|---|---|
| Notice management | Head teachers and admins create, edit, publish now or schedule, expire, archive and delete drafts. English and Bangla content; normal, important or urgent priority; role audiences with optional class/section targets; up to 10 PDF/JPEG/PNG attachments. Role-scoped feeds show only live notices addressed to the viewer, urgent first |
| Events calendar | Holidays, examinations, meetings and other events, all-day or timed, with date/time validation, the same audience and target rules, draft/published/cancelled states, and month calendar and list views for every role |
| Mock SMS provider behind a replaceable interface | `notify.SMSProvider` with a `MockSMS` implementation that never contacts a network; statuses queued, `sent_mock`, failed (after 3 attempts) and cancelled, never "delivered"; admin log with masked numbers |
| Absence notifications | Pending absence events notify linked guardians in-app and by mock SMS; a correction withdraws the notification and cancels a still-queued SMS |
| Result notifications | Published or locked exams notify students and linked guardians; returning an exam to draft withdraws them |
| Fee-due notifications | Overdue fee balances (up to 90 days) notify students and linked guardians, with mock SMS to guardians |
| Notice notifications | Live notices notify their audience; urgent notices also queue mock SMS to guardians; archiving withdraws unread notifications |

**Daily Class Diary (approved extension).**
- **Teachers:** one diary per section, subject and day, holding lesson topics, class notes, homework, instructions and PDF/image attachments. Teachers write only for sections of classes and subjects they are assigned in the session containing the date, at most 7 days ahead. They save drafts, publish, and edit or delete only their own diaries.
- **Students and guardians:** students see published diaries of their current section, and guardians only those of linked children. Both can filter by date range, subject and text.
- **Homework:** submission is optional per diary. Students may submit text and up to five files only while it is enabled and before any due time, and may resubmit until reviewed. Teachers review against the full section roster.
- **Admins:** admins enable the separate `diary` module (disabled by default), can require daily publication with a teacher reminder time (no reminder on Fridays), and see compliance and all diaries.

**Notifications.** The in-app inbox has an unread count, read and mark-all-read actions, and per-category in-app and SMS preferences (urgent notices always arrive in-app). Dispatch is idempotent: notifications are unique per user and source fact, and a database try-lock means concurrent runs skip rather than duplicate. Dispatch and abandoned-upload cleanup run every `NOTIFY_INTERVAL` (default 1m; 0 disables), and admins can run dispatch on demand.

**Storage.** Attachments are stored on disk under `FILE_STORAGE_DIR`, not in PostgreSQL, using random 32-hex keys. Client file names never reach the filesystem, and keys are validated before any path is built.
- **Validation:**
  - Files are identified by content, never by the declared type.
  - PDFs (up to 10 MiB) must be complete and are rejected if they contain JavaScript, launch actions, embedded files or rich media. This is a best-effort check, not a full parser.
  - JPEG and PNG images (up to 5 MiB, at most 6000 px per side) are decoded and re-encoded, which strips metadata.
- **Downloads:** files are served only after the owning notice, diary or submission authorizes the viewer, with nosniff, no-store and a sandbox Content-Security-Policy.
- **Uploads:** an upload must be claimed by its own uploader for the matching purpose. Each user may hold at most 30 unattached uploads, and abandoned or removed files are deleted after 24 hours.
- **Compose:** the stack mounts a new `uploads_data` volume at `/app/uploads`. Existing volumes are unchanged.

**Schema and permissions.**
- **Migrations:** `0901_files_notices_events.sql`, `0902_notifications_sms.sql` and `0903_class_diary.sql` reference existing users, sessions, classes, sections, class subjects, students and guardians. No academic, staff, student or guardian table is duplicated.
- **Permissions:**
  - `notices:read` and `events:read` for every role.
  - `notices:manage` and `events:manage` for admins and head teachers.
  - `notifications:read_self` for every role; `notifications:manage` for admins.
  - `diary:write` for teachers and head teachers; `diary:read` for admins and head teachers.
  - `diary:read_self` for students; `diary:read_children` for guardians; `diary:manage` for admins.
- **Modules:** the `notices` module gates notices, events and notifications, and the new `diary` module gates the diary. Every change is audited.

**Verification (2026-10-18).**
- **Backend:** `go vet ./...` passed. `go test ./...` passed without PostgreSQL. On a freshly created disposable PostgreSQL 17 database it passed 62 of 62 top-level tests with none skipped, including `TestOpenAPICoversRegisteredRoutes`.
- **Unit tests:** file content detection, active-PDF and spoofed-image rejection, path-safe names and storage keys, notice/event/diary/preference validation, and mock SMS acceptance, retry and failure.
- **M9 PostgreSQL HTTP test:**
  - **Notices:** module and role enforcement; draft, scheduled and expired notices; class/section targeting for students, guardians and teachers; attachment headers.
  - **Events:** visibility and cancellation.
  - **Notifications:** three concurrent dispatches producing one notification; withdrawal on a corrected absence; preferences; read state; the masked SMS log.
  - **Diary:** teacher assignment limits, student and guardian scoping, PDF/image validation, enabled/disabled/reviewed homework, reminders and compliance.
- **Frontend and Compose:** `tsc`, the production build and `docker compose config --quiet` passed.
- **Browser:** headless Edge on an isolated database, ports (8093/5183) and upload directory passed 31 of 31 checks in two consecutive full runs. It covered head-teacher notice creation with a rejected fake PDF and an accepted PDF, the event time validation, admin dispatch and the mock SMS log, section targeting, the student notice board, bell and inbox and the guardian diary in Bangla at 390 px, teacher diary publication and homework review, student homework submission in Bangla, and admin settings and compliance. There was no horizontal overflow and no JavaScript exception.
- **Fixes from verification:**
  - The notification link CHECK used a regex repetition above PostgreSQL's 255 limit; it was fixed before release.
  - "Publish now" now uses the database clock.
  - Editor success messages were being cleared by the reopen.
  - Forms could be submitted while an upload was still running.
- **Isolation:** the application database, the QA stacks, demo-school data, application volumes and secrets were not touched.

**Not verified / limitations.**
- **SMS:** no real SMS provider is integrated (by design), and guardians without a linked user account receive no notifications or SMS.
- **Withdrawals:** fee-due notifications are not withdrawn when the balance is later paid; they describe the state when sent.
- **Attachments:** PDF screening is heuristic, and attachments are stored on one local volume (no object storage or virus scanning). In the student diary filters, date inputs use the browser's native format.
- **Diary reminders:** these skip Fridays only; there is no school holiday calendar integration yet.
- **Not run:** remote CI, production migration and deployment.

## Attendance UX Redesign (2026-10-18)

Branch `feature/attendance-premium-ux` starts at `origin/main` `8916b516`, after M8 was merged. It improves the M5 attendance workflow and adds no roadmap criteria, so the completion percentages are unchanged. It is not pushed.

| Area | Before | After |
|---|---|---|
| Marking | A dropdown and a Save button per student; 10 students per page | Full section on one screen, sorted by roll number; one-tap Present, Absent and Late per student; keyboard arrows and P/A/L hotkeys |
| Saving | One request per student | `PUT /attendance/students/roster` saves the whole class in one transaction; Mark all present; live Present/Absent/Late/Unmarked totals and progress |
| Unmarked students | Unchanged | Never sent, so never recorded as absent; the save bar states how many stay unmarked |
| Duplicates and races | A double save returned 409 | Identical marks are no-ops; the save button is guarded; a per-section-day lock serializes roster saves, QR scans and corrections |
| Corrections | Single-record PATCH | Past dates open in correction mode. Each correction carries the record version, and a change made elsewhere (for example a QR scan) is reported per student instead of overwritten |
| Unsaved work | Lost silently on date or section change | A confirmation guards date, section and session changes, in-app links and page unload |
| Reports | Plain text lists | Monthly table with attendance rate, absence-alert tab, and a student/guardian calendar view |

Absence events are still created for absent marks and cancelled by corrections. Monthly reports read the same `student_attendance` table; no table was added or duplicated. Teacher assignment, RBAC and module checks are unchanged. The teacher attendance page uses the same design with its existing APIs.

**Verification.**
- **Backend:** `go vet ./...` passed. `go test ./...` passed without PostgreSQL. On a freshly created disposable PostgreSQL database it passed 52 of 52 top-level tests with none skipped, including the existing M5 attendance test, the new roster test and the OpenAPI route test. The roster test covers membership, inactive students, future and out-of-session dates, idempotent replay, versioned corrections, stale rejection, QR coexistence, absence events and guardian visibility, five concurrent submissions and monthly totals.
- **Frontend:** `tsc`, the production build and `docker compose config --quiet` passed.
- **Browser:** headless Edge on an isolated database and ports (8092/5182) passed 35 of 35 checks. It covered roll order, one-screen roster, Mark all present, absent/late marking, keyboard control, a double-click producing one save, the guardian's Bangla calendar, the admin alert list, a correction cancelling the alert, stale detection and reload, unsaved-change prompts, QR scan and duplicate QR, section switching, the monthly report, Bangla at 390 px, tablet at 820 px, the teacher attendance page, the student report and student access denial. There was no horizontal overflow and no JavaScript exception.

The application database, the QA stacks and demo data were not touched.

**Limitations.** The browser Back button is not intercepted (`BrowserRouter` has no navigation blocker); reloads, tab closes, in-app links and context changes are guarded. Teacher attendance still saves one teacher at a time. A note-only change on an unmarked student is not saved until a status is chosen. Native camera QR scanning was not exercised in headless Edge. Remote CI and deployment are unverified.

## Current Status — M8 Fees and Accounts (2026-10-18)

| Brief criterion | Verified implementation |
|---|---|
| Fee structures | Coded monthly or one-time fee categories with English and Bangla names; one amount and due day per session, class and category; session, class and category fixed after creation; later amount changes affect only new dues |
| Student dues | Rerunnable monthly generation for every active student in a session or class, optional one-time fees, single-student assignment, standing percentage concessions, discounts, administrator-only waivers, deletion only before payment, and status and arrears filters |
| Payment receipts | Transactional full and partial payments, settling the oldest dues first or selected dues; sequenced `RCT-` receipts; idempotent replay; unique bank and mobile references; overpayment rejected in the service and by a database check; printable bilingual receipt |
| Mock bKash/Nagad payments | Replaceable `Gateway` interface with a mock provider; students and linked guardians start a 15-minute checkout and confirm with test OTP `123456`; wrong OTP declines; expired, failed and repeated confirmations are handled; the UI labels it as a test |
| Income and expense tracking | Ledger with fixed income and expense categories; every receipt posts fee income in the same transaction; manual entries can be added and removed, but receipt-linked income cannot |
| Fee collection reports | Collections by method, class, fee and day; per-student balances and arrears; receipt register; accounting summary with income, expense, net, outstanding dues and arrears |

**Implementation.** Branch `feature/m8-fees-accounts` starts at `origin/main` `32af0480`, after M7 (PR #10) and the QA fixes (PR #11). Migration `0801_fees_accounts.sql` adds `fee_categories`, `fee_structures`, `fee_concessions`, `student_fees`, `fee_adjustments`, `fee_payments`, `fee_payment_allocations`, `online_payment_intents` and `ledger_entries`. They reference the existing users, academic sessions, classes and students; no student or academic record is duplicated. `student_fees.balance` is a generated column, and `student_fees_no_overpayment` keeps `discount + paid <= amount`. Amounts are computed in integer poisha. Payments take a per-student advisory lock and lock the dues `FOR UPDATE`. New grants are `fees:read`, `fees:manage`, `fees:collect`, `accounts:read` and `accounts:manage` for administrators and accountants; `fees:read_self` for students; `fees:read_children` for guardians; and `fees:pay_online` for both. Financial writes also require the administrator or accountant role. The `fees` module still starts disabled. Every change is audited. The dashboard now links Fees and Examinations instead of listing them as Coming Soon.

**Verification (2026-10-18).**
- **Backend:** `go vet ./...` passed. `go test ./... -count=1` passed without PostgreSQL. On a freshly created disposable PostgreSQL 17 database it passed 50 of 50 top-level tests with none skipped, including `TestOpenAPICoversRegisteredRoutes`.
- **Repository test:** migration 801 is applied once and reruns cleanly. The test covers generation with concessions, partial and selected-due payments, receipt replay, reused keys, duplicate references, the database balance guard, five concurrent tellers (exactly one succeeds), discounts and waivers, deletion rules, mock checkout (declined, expired, another payer, replay), scoped receipts, reports and ledger reconciliation.
- **HTTP test:** covers disabled module, 401/403 for every other role, a mistaken teacher `fees:collect` grant that still cannot collect, 415 and unknown-field rejection, and error codes.
- **Frontend:** `tsc` and the production build passed (Vite still warns that the bundle is over 500 kB, as before M8). `docker compose config --quiet` passed.
- **Browser:** headless Edge on an isolated database and ports (backend 8091, Vite 5181) passed 26 of 26 checks. It covered English accountant setup, generation, collection, receipt, print-media isolation, overpayment message, reports and ledger at 1280 px; an administrator waiver; and Bangla guardian bKash checkout (declined then approved) and the student statement at 390 px. There was no horizontal overflow and no uncaught JavaScript exception, and teacher and student access to the staff page was denied.

The developer's application database, the QA Compose stack and their volumes were not touched. No demo-school data was modified.

**Not verified / limitations.** No real bKash or Nagad API is integrated (the brief asks for a mock). A browser-generated PDF file was not produced; print-media rendering was verified. Receipts cannot be voided or refunded; a mistaken payment needs a manual correction outside the app. There is no advance credit, so payments above the outstanding balance are rejected. Dates on Bangla screens use Latin digits. Deleting a student who has fee records returns 400 and changes nothing, the same as for students with exam marks, because the financial history is kept. Remote CI, production migration and deployment are unverified.

## Current Status — M7 Examinations and Results (2026-10-17)

| Brief criterion | Verified implementation |
|---|---|
| Exam management | Session/class/section exam drafts, types, optional matching M6 routine link, mapped subjects, configurable full and pass marks, guarded updates and deletion |
| Marks entry | Assigned teachers enter and correct only their subjects; administrators can bulk review and correct; atomic 500-entry limit, uniqueness, range checks and per-change audit |
| Bangladesh GPA calculation | Uses current school grading bands at publication, scales marks to percentage, applies subject pass thresholds, freezes grade and GPA snapshots |
| Fourth-subject GPA rule | Allows one mapped fourth subject per exam; adds only grade points above 2, divides by required subjects, caps GPA at 5, and fails required subjects consistently |
| Marksheet generation | Branded English/Bangla student report card with subject marks, grades, grade points, GPA, positions and print layout |
| Tabulation sheets | Authorized table of result totals, GPA, pass state and merit positions |
| Class and section merit positions | Ranks published matching exams across sections; re-ranks on publication and return to draft; locks prevent later changes to the group |
| Result publication | Complete-mark transactional publish, return to draft for corrections, permanent lock, and published-only self and linked-child access |

Branch `feature/m7-exams-results` starts at M6-enabled `origin/main` merge `c26f8e90`. Migration `0701_exams_results.sql` adds `exams`, `exam_subjects`, `exam_marks`, and `exam_results`, with UUIDs, academic and student context foreign keys, unique marks, a marks-range trigger, and result indexes. The existing `examinations` module starts disabled. Grants are `exams:manage`, `exams:read`, `exams:enter_marks`, `exams:read_self`, and `exams:read_children`; writes and publication require administrator or current teacher assignment as appropriate. The migration runner now preserves SQL function bodies and quoted semicolons.

Verification passed: Go vet; full Go suite on a fresh isolated PostgreSQL database; dedicated migration, grading, lifecycle, merit, permission, module and visibility tests; OpenAPI route/reference checks; TypeScript and production build; Edge desktop/mobile role checks in English and Bangla. The report card print stylesheet was visually verified in print media, with only the report card visible. This Edge instance returned “Printing is not available” for its PDF command, so a generated PDF file was not verified. Remote CI, production migration and deployment remain unverified. The test database, test accounts and browser profile were isolated from application data and volumes. Overall **48/83 = 57.8%**; M7 **8/8 = 100% locally**.

## M0–M6 Full System QA (2026-10-17)

Branch `fix/m0-m6-full-system-qa` starts from `origin/main` at `c26f8e9` (M6 merged). This QA pass adds no roadmap criteria, so it does not change the completion percentages above.

| Defect | Severity | Fix | Regression evidence |
|---|---|---|---|
| Attendance pages defaulted to an inactive session whenever two session date ranges overlapped today; the active section roster then appeared empty | High | `GET /attendance/sessions` returns `is_active` and lists the active session first; student and teacher attendance pages prefer it | `TestPostgresAttendanceSessionsListActiveFirst` failed before the fix and passes after it; the browser default-session checks failed on the previous build and pass now |
| The student attendance page was stored as double-encoded UTF-8 with a BOM, so it rendered `â€”` and `QA-001 Â· 1` | Medium | Restored `—` and `·`, removed the BOM; no other source file contained the pattern | Browser text checks and a source-wide byte scan |
| The student page reused teacher wording ("Search teachers", "already recorded for this teacher") | Low | Student-specific English and Bangla strings | Browser label checks in English and Bangla |
| An unsaved roster status survived date, section, and session changes and could be saved against another day | Medium | Drafts and roster page reset when the session, section, or date changes; monthly paging resets with its filters | Browser draft-isolation check |
| The student report heading ignored Bangla names | Low | Bangla name is used when Bangla is selected | Bangla student report browser check |

**Verification.** On a fresh isolated PostgreSQL database, `go vet ./...`, `go test ./... -count=1`, and `TestOpenAPICoversRegisteredRoutes` passed. The frontend TypeScript and production build and `docker compose config --quiet` also passed. The live API matrix on the isolated QA stack passed 167 of 167 checks. It covered M0 health and branding; login, role isolation, and module toggles for all six roles; M2 validation and duplicate rules; M3 student and guardian scoping; M4 leave and salary access; M5 roster, QR, corrections, absence-event cancellation and reopening, and reports; and M6 conflicts and exam publication. Headless Edge passed 70 of 70 checks. These covered English admin pages at 1280 px and Bangla pages at 390 px for admin, head teacher, teacher, student, guardian, and accountant, with no mojibake, no horizontal overflow, and no unexpected JavaScript errors.

**Not verified.** Native camera QR scanning, remote CI, production migration, and deployment were not run. The frontend has no unit-test runner, so its fixes are covered by browser checks only.

**Known limitation, unchanged.** Corrections keep the section where a record was first stored. If a student later moves to another section, that day's record appears on the new section roster but counts in the original section's monthly totals. This is the documented M5 snapshot behavior.

## M6 Routine handoff (2026-10-17)

| Brief criterion | Verified implementation |
|---|---|
| Class routines | Weekly session/class/section/subject/teacher periods; administrator CRUD and filters; scoped teacher, student, and linked-guardian views; bilingual printable timetable |
| Exam routines | Administrator draft and slot CRUD, publish/unpublish, date and relationship checks, published-only student and linked-guardian views, populated printable PDF |
| Teacher scheduling conflict detection | Transactional serialized writes reject overlapping teacher, room, and section class periods, including sessions with intersecting dates; exam section and room collisions are rejected across sessions |

Branch `feature/m6-routine-management` starts from M5-enabled `origin/main` at `7566209`. Migration `0601_routines.sql` adds `class_routines`, `exam_routines`, and `exam_routine_slots`, with UUID keys, academic and teaching-assignment foreign keys, time checks, unique exam names/subjects, and read indexes. The optional `routine` module starts disabled; enable it in school settings. Grants are `routine:manage` and `routine:read` for administrators, `routine:read` for head teachers, `routine:read_self` for teachers and students, and `routine:read_children` for guardians. Writes also require administrator role. Changes are audited.

Go vet, the full Go suite on a fresh isolated PostgreSQL database, dedicated concurrent conflict/visibility/access tests, OpenAPI route/reference validation, TypeScript and production build, and Edge desktop/mobile browser checks passed. Browser checks covered English admin and Bangla mobile layouts, all four user views, published exam visibility, PDF output, horizontal overflow, and JavaScript errors. The test database and browser fixtures were separate from the application database. Remote CI, production migration, and deployment remain unverified. Overall: **40/83 = 48.2%**; M6: **3/3 = 100% locally**.

## M5 Attendance handoff (2026-10-17)

| Item | Verified status |
|---|---|
| Branch | `feature/m5-attendance`, merged into `origin/main` at `7566209` |
| Schema | `0501_teacher_attendance.sql` and `0502_student_attendance.sql`; versions 1–4, 101–103, 201, 401, 501, 502 applied in a fresh disposable database |
| Daily student attendance | Complete: assigned teacher/admin entry and correction, section context, duplicate and foreign-key protection |
| Daily teacher attendance | Complete: existing M5 entry/correction, approved leave, self history, and roster preserved |
| QR attendance | Complete for M3 card payload via authenticated, assigned staff and single daily entry; input and browser camera fallback |
| Monthly reports | Complete for teacher, student, and currently linked guardian views |
| Absence alert triggers | Complete: one durable pending/cancelled event per absent student/day; delivery deferred to M9 |
| Verification | Go vet; full Go suite on fresh PostgreSQL; OpenAPI route/reference; TypeScript and production build; Compose config; mock-backed Chrome desktop/mobile UI smoke |
| Local progress | M5 5/5 = 100%; overall 37/83 = 44.6% |
| Unverified | Native camera in a physical browser/device, remote CI, production migration and deployment |

See [M5 handoff](M5_ATTENDANCE_HANDOFF.md) for permissions, schema rules, test boundaries, and run instructions.

## Current Status — Batch B, M4 (2026-10-16)

| Item | Status |
|---|---|
| Branch | `feature/m4-teachers-staff`, merged with `origin/main` at `8cc316c` |
| Schema | `0401_staff.sql`; no duplicate users, students, guardians, or sections tables |
| Backend | Staff profiles, secure existing-account links, class/subject/session assignments, leave requests and decisions, private salary records, audit logs |
| Frontend | Bilingual admin directory/editor and own profile page at `/admin/staff` and `/staff/me` |
| Verification | Original M4 slice: full PostgreSQL Go suite and isolated Edge desktop/mobile flow. Pagination update: Go vet, database-free Go suite, frontend build; new PostgreSQL assertions pending Docker daemon. |
| M4 criterion count | 4/4 locally verified |
| Pending external checks | Remote CI, review, production migration/deployment |

See [M4 handoff](M4_TEACHERS_STAFF.md) for schema, permissions, APIs, verification, and run instructions. M2, M3, and M4 are now merged into main.

## PR #7 integration verification (2026-10-17)

The normal merge of origin/main into feature/m4-teachers-staff includes M2 Sections and M3 Students/Guardians alongside M4 Teachers and Staff. The merged backend registers all three service groups, and the frontend registers their routes and English/Bangla resources. The disposable PostgreSQL test database applied migrations 1, 2, 3, 4, 101, 102, 103, 201, and 401; the corresponding academic, student, guardian, and staff tables were present.

The merged branch passed go vet ./..., go test ./... -count=1 both without PostgreSQL and with AUTH_TEST_DATABASE_URL pointing to the disposable database, the explicit OpenAPI route/reference test, frontend TypeScript and production build, and docker compose config --quiet. The non-PostgreSQL run skipped the environment-gated integration tests; the PostgreSQL run executed them. Browser interaction, remote CI, PR review, and deployment were not rerun or verified for this merge.

## Current Status — M3 Students and Guardians (2026-10-17)

M3 is complete and merged into main after M2 Sections.

| M3 brief criterion | Local evidence |
|---|---|
| Student admission | Guarded create/update/delete API, validated admission number and section/session/class context, statuses and roll numbers, admin directory and editor |
| Student profiles | Scoped student detail, read-only self view, status and account links, guarded photo and card views |
| Student photos | JPEG/PNG validation, dimension and 2 MiB limits, metadata-stripping re-encode, protected storage and no-store responses |
| Student ID cards with QR codes | Printable card and PNG QR carrying only the student UUID; browser verified for admin, student, and linked guardian |
| Bulk CSV import | Admin-only, 1,000-row/2 MiB limit, fixed header, UTF-8 BOM support, and atomic PostgreSQL import with duplicate rollback |
| Guardian-student relationships | Guardian CRUD, multiple guardians per student, at most one primary guardian, role-correct account linking, linked-child-only access, and audited link/unlink/delete |

Migration `0201_students_guardians.sql` follows 1–4 and 101–103. UUID keys, composite academic-section foreign key, case-insensitive admission uniqueness, per-section roll uniqueness, role-enforced account foreign keys, primary-guardian uniqueness, and indexes were verified in a dedicated PostgreSQL test database. Deleting a student removes its photo and guardian links; deleting a guardian removes its student links. Existing login accounts remain and can be linked again. Student and guardian data require the optional Students module, which remains disabled by default. The Academics module must be enabled for the admin editor to load session, class, and section choices.

**Verification:** `go vet ./...` and `go test ./... -count=1` passed after the final CRUD changes, both without PostgreSQL and on fresh isolated `flybirds_m3_final4_test`. The PostgreSQL lifecycle covered migration order and repeat application, relationship and role integrity, create/update/delete, search and pagination, CSV rollback, photo/QR, 401/403/404 access, unrelated guardians, and disabled-module behavior. Frontend TypeScript and Vite production build passed. `docker compose config --quiet` and the Go OpenAPI parse, route, and reference test passed. An isolated Compose project rendered the admin student/guardian directories, card, student self profile, and guardian child profile in English at 1280 px and Bangla at 390 px without horizontal overflow or JavaScript exceptions. After the final CRUD change, the rebuilt admin pages showed both delete controls; the students, guardians, and student detail pages all had 390 px scroll width with real Bangla test names. The developer's application database and volume were not modified.

**Unverified outside this workspace:** remote CI, review, production migration, and deployment. Existing user creation and password delivery use the M1 administrator workflow; M3 does not add invitations or password reset. QR codes identify a student but do not authenticate or authorize access. Academic foreign keys ensure valid placement; whether inactive academic records should forbid new admission is a school policy left configurable for a later decision.

## Current Status — M2 Sections (2026-10-16)

M2 is complete locally on `feature/m2-sections`, based on merged `origin/main` at `9c066178`. Migration `0103_sections.sql` follows versions 1–4, 101, and 102. A section belongs to one session and class; names and codes are unique within that pair, and both foreign keys restrict deletion. Editing and activation use the existing Academics permission, module, administrator, and audit controls. The section identifier and owning class/session are stable after creation. The bilingual `/admin/academic/sections` page offers search, filters, pagination, and an administrator editor; head teachers have read access.

**Verification:** `go vet ./...` and `go test ./... -count=1` passed against a dedicated PostgreSQL database, including the Sections HTTP lifecycle and the existing suite. Migration 103 applied twice without error; the test verified the ledger through 103, direct database uniqueness and foreign keys, unauthorized and head-teacher requests, disabled Academics, creation, editing, activation, search, and pagination. Docker frontend `npm run build` passed TypeScript checking and Vite production compilation. `docker compose -p flybirds_sections_smoke config --quiet` passed. An isolated Compose project with its own database and ports rendered the saved section and editor at 1280 px in English and at 390 px in Bangla, without horizontal overflow or browser exceptions. The developer's application database and volume were untouched. OpenAPI route/reference validation passed in the Go suite. Remote CI and production migration remain unverified.


## Current Status — M2 Task 002 (2026-10-16)

| Item | Status |
|---|---|
| Development phase | Joy's Academic Sessions merged from `origin/main` into `feature/m2-classes-subjects`; class and subject schema integrated locally |
| Current milestone | M2 — Academic Structure |
| Verified criteria | M0: 4/4; M1: 12/12; M2: 5/6 |
| Remaining criteria | Sections in M2 and 61 criteria in M3–M13 |
| Backend and frontend | Sessions, classes, subjects, and assignments work together with guarded APIs and bilingual admin pages |
| Tests passed | Go vet and full PostgreSQL-backed Go suite, fresh and upgrade migration runs, frontend production build, OpenAPI route/reference test, Compose validation, isolated browser flows |
| Unverified | Remote CI, deployment, production database migration; Sections are intentionally unimplemented |
| Application runnability | A dedicated PostgreSQL test database passed versioned migrations 1–4, 101, 102 and live browser flows. The developer's application database and accounts were not modified. |
| Next step | Review this integration branch; after approval, plan Sections against the merged academic session UUID. No other M2 work is included here. |

### M2 Task 002 integration evidence

| M2 criterion | Status | Local evidence |
|---|---|---|
| Academic sessions | Complete locally | Joy's `0101_academic_sessions.sql`, guarded CRUD and activation, PostgreSQL HTTP tests, browser creation and activation |
| Classes | Complete locally | `0102_classes_subjects.sql`, validation and uniqueness, guarded CRUD, search and pagination, browser create/read at desktop and mobile widths |
| Sections | Pending | Session-specific ownership and assignments require a separate approved implementation step |
| Subjects | Complete locally | English and Bangla names, unique canonical codes, guarded CRUD, browser create/read and Bangla rendering |
| Class-subject mapping | Complete locally | Stable UUID foreign keys, duplicate-pair constraint, guarded create/update/delete/list, PostgreSQL and browser assignment flow |
| Fourth/optional subject flag | Complete locally | Exclusive database check and service validation; required-to-fourth switch passed PostgreSQL and browser checks |

- **Git integration:** `0bb712d` is an ancestor of fetched `origin/main`; a normal merge preserved Joy's eight commits and this branch's six Task 001 commits. Migration `0101` was not edited. The migration runner discovers sorted numeric SQL filenames, so `0102` needs no manual registration entry.
- **Database and API:** A fresh disposable database applied versions 1, 2, 3, 4, 101, 102. A second disposable database began with versions 1–4 and upgraded through 101 and 102. PostgreSQL-backed HTTP tests checked session activation, classes, subjects, codes, assignment flags, foreign keys, duplicate prevention, grants, disabled modules, filtering, and pagination. The optional `academics` module remains disabled by default; an administrator enables it in school settings.
- **Browser:** An isolated backend on port 8082 and frontend on 5174 used a dedicated test database. Headless Edge created and activated a session, created a class and bilingual subject, assigned the subject, and switched it to fourth/optional. An authenticated head teacher read all three academic routes without edit controls. English and Bangla pages rendered at 1280 px and 390 px; the Bangla class assignment panel at 390 px had no horizontal overflow. Disabled Academics hid academic navigation and blocked page access. No JavaScript exceptions were observed.
- **Gates and limits:** `go vet ./...`, `go test ./... -count=1` with `AUTH_TEST_DATABASE_URL`, the frontend TypeScript/production build in Docker, `docker compose config --quiet`, and OpenAPI route/reference validation passed. The full suite requires a fresh disposable database: rerunning it against the browser database failed two older tests whose fixed session count and last-admin assertions assume no preexisting fixtures; it passed on a new database. Remote CI, a production migration, and deployment behavior were not run. Browser checks used disposable accounts and a separate database; existing developer accounts and Docker volumes were preserved.

## M2 Task 001 checkpoint — 2026-10-16

- Started from clean updated `main` at merge commit `49a03c08` and created `feature/m2-classes-subjects`. Joy's branch was not published at the time of the check, so no migration version or runner registration was guessed. [The handoff](M2_CLASSES_SUBJECTS_HANDOFF.md) specifies the session-independent schema and merge sequence.
- Added class definitions (configurable name, order, active state), bilingual subjects (English/Bangla names, normalized code, active state), and class-subject mapping with exactly one required or fourth/optional designation. The service validates inputs; repositories are prepared for UUID keys, unique indexes, foreign keys, transactional authorization, and audit records once the migration exists.
- Added academic APIs guarded by authentication, `academics:read`/`academics:manage`, the `academics` module, and administrator status for writes. Class, subject, and mapping lists support search and pagination. OpenAPI covers all registered routes and references.
- Added bilingual `/admin/academic/classes` and `/admin/academic/subjects` views with responsive cards, filters, editors, and mapping controls. Navigation requires current permission and module state. The production frontend build passed, but the pages could not be exercised in a browser without the academic migration and grants; responsive behavior remains unverified.
- `go vet ./...` passed with a workspace-local Go cache. `go test ./... -count=1` passed against a dedicated PostgreSQL database; `TestPostgresAcademicLifecycle` skipped explicitly because the coordinated tables do not exist. The M1 PostgreSQL tests passed. The OpenAPI route/reference test passed. `docker compose config --quiet` passed. Both backend and frontend Docker images built. An isolated backend container on port 8082 returned healthy JSON and 401 for an anonymous academic request, then was stopped. The disposable database was removed; the existing Compose services remained running. The host `npm ci` hit the same locked `lightningcss` binary as M1; a Docker production build passed.
- **Integration dependency:** the migration filename, runner entry, permission seeds, and database constraints must be added after Joy's version is confirmed. Subject-code uniqueness, duplicate mapping prevention, foreign-key integrity, module-enabled HTTP behavior, and responsive browser behavior are prepared in tests/UI but not yet verified end to end. No sessions, sections, or later milestone data was added.

## M1 Historical Status

| Item | Status |
|---|---|
| Current development phase | M2 Academic Sessions implemented and locally verified; branch not pushed |
| Current milestone | M2 — Academic Structure, Academic Sessions task only |
| Completed acceptance criteria | Four M0, twelve M1, and one M2 criterion verified locally |
| Remaining acceptance criteria | Five M2 criteria; 66 criteria remain overall |
| Features implemented | Existing M0/M1 features plus versioned academic-session schema, CRUD/read/current/activation APIs, permission and module guards, and bilingual management page |
| Tests passed | Go vet; Go unit and full PostgreSQL suites on a separate disposable container; frontend TypeScript and production build; isolated headless Chrome English desktop and Bangla mobile checks |
| Tests failed | No application test failed in the final local run. First offline `npm ci` lacked a cached font package; the subsequent normal `npm ci` succeeded. |
| Application runnability | Existing developer Compose stack was not changed. Academic routes require the new backend build, migration 101, and the Academics module enabled. A separate PostgreSQL container was used for tests. |
| Next planned milestone | Remaining M2 work belongs to separate tasks; this task stops after Academic Sessions |

## M2 Academic Sessions Task — 2026-10-16

- **Scope:** One of six M2 brief criteria is verified locally: Academic Sessions. Classes, sections, subjects, mapping, and optional-subject flags remain unstarted in this workspace.
- **Schema:** Migration `0101_academic_sessions.sql` follows M1 versions 1–4. Migration discovery uses explicit numeric prefixes, so separately numbered migrations do not overwrite one another. `academic_sessions.id` is a stable UUID; a generated normalized name key makes hyphen/en-dash variants unique. A date check and partial unique active-session index enforce core invariants in PostgreSQL. No session or school identity is seeded.
- **Backend:** Handler → Service → Repository. Read, create, update, activate/deactivate, and current-session routes use JWT authentication, current grants, and the `academics` module guard. Activation serializes on a transaction advisory lock and writes audit events. Administrator gets read/create/update/activate grants; head teacher gets read. The optional Academics module remains disabled by default and must be enabled in school settings.
- **Frontend:** `/admin/academic/sessions` supports English/Bangla, search and pagination, date and name validation, create/edit, current and active indicators, activation controls, and loading/error/empty states. It uses the school branding CSS variables and responsive cards.
- **Verification:** `go vet ./...` and `go test ./... -count=1` passed with `AUTH_TEST_DATABASE_URL` pointed at a disposable PostgreSQL 17 container on local port 55433; the test database was separate from the developer's running Compose database on 5432. The suite covered valid/invalid dates, normalized duplicate names, permission/module denial, search/pagination, idempotent migration, current-session lookup, deactivation, and two concurrent activation requests. `npm ci` and `npm run build` passed. Isolated headless Chrome rendered English at 1280 px and Bangla at 390 px, including the active indicator and form, with no horizontal overflow. The browser check used mocked API fixtures; PostgreSQL-backed HTTP tests separately verified the live API. Remote CI and deployment remain unverified.

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
