# M4 Teachers and Staff

M4 uses `0401_staff.sql`, after the merged M2 migrations `0101` and `0102`. This version leaves `0103` and `0301` available for the other developer's Sections and Students/Guardians work. The migration creates `employees`, `teacher_assignments`, `employee_leaves`, and `employee_salary_records`; it does not create users or duplicate authentication records. No employees or school names are seeded. Migration execution is transactional and the permission inserts use `ON CONFLICT DO NOTHING`.

`employees.id` is a stable UUID. Employee codes are normalized to uppercase and unique. One existing user can link to at most one employee. A teacher employee can link only to an active teacher or head teacher account; a non-teaching employee can link only to an active administrator or accountant account. Link validation occurs in the same transaction as the profile write, and self routes resolve the authenticated user against the current role and active user state. An account is optional so a school can record employees before issuing sign-in access.

Teaching assignments reference the stable academic session, class, and subject UUIDs. A composite foreign key requires the class-subject pair to exist. The API also requires an active teacher employee, class, and subject. Assignment uniqueness is per employee, session, class, and subject. This model can later be extended with sections without redefining employees or class-subject mappings. Leave dates and salary amounts have service validation and database checks. Salary records are append-only and restricted to administrators. Mutations write safe audit metadata in their database transaction; salary amounts, leave reasons, and contact details are not copied into audit metadata.

All routes require JWT authentication and the enabled `staff` module. The default module state is disabled. Administrators may enable it through school settings. Permissions are checked from the current role grants on every request.

| Permission | Default roles | Use |
|---|---|---|
| `staff:read` | admin, head_teacher | Directory and profile details |
| `staff:manage` | admin | Profile and assignment writes |
| `staff:read_self` | teacher, head_teacher, admin | Own profile and assignments |
| `staff_leaves:read` | admin, head_teacher | Employee leave history |
| `staff_leaves:manage` | admin | Request for staff and decide leave |
| `staff_leaves:request_self` | teacher, head_teacher, admin | Own leave history and request |
| `staff_salary:read` | admin | Private salary history |
| `staff_salary:manage` | admin | Add private salary record |

The APIs are documented in [OpenAPI](openapi.yaml). The main routes are `GET/POST /staff/employees`, `GET/PATCH /staff/employees/{id}`, `GET /staff/me`, assignment routes under `/staff/employees/{id}/assignments` and `/staff/me/assignments`, leave routes under `/staff/employees/{id}/leaves`, `/staff/me/leaves`, and `/staff/leaves/{id}/decision`, and salary routes under `/staff/employees/{id}/salaries`. All GET list routes return `{items, total, page, page_size}` inside `data` and accept `search`, `page`, and `page_size`. The employee directory also accepts kind and status filters. Duplicate codes or linked accounts return 409. A salary for the same employee and effective date also returns 409.

The bilingual admin directory and editor are at `/admin/staff`; the teacher-facing view is at `/staff/me`. The pages use current school colors, responsive cards, loading/error/empty states, and the existing authentication refresh flow. Salary amounts are visible only in the admin view. A user account can be created in `/admin/users` and linked by UUID from the employee form.

Verification used a disposable PostgreSQL container on port 55434 and isolated backend/frontend ports 8082/5174. The M4 HTTP integration test covers unauthorized requests, disabled modules, role restrictions, invalid dates, account linking, uniqueness, leave decisions, salary privacy, session/class/subject relationship integrity, and assignment duplication. `go vet ./...`, `go test ./... -count=1`, the frontend production build, and the OpenAPI route/reference test passed. Headless Edge created an employee through the UI at 1280 px and checked Bangla at 390 px; both widths had no horizontal overflow. Existing application containers, database, accounts, and volumes were untouched.

Run from the project root with the existing Docker Compose instructions in [README](../README.md). With the database configured, the backend applies `0401` automatically. Enable the `staff` module in school settings. For integration tests, set `AUTH_TEST_DATABASE_URL` to a **separate disposable PostgreSQL database** and run `go test ./... -count=1` from `backend/`.
