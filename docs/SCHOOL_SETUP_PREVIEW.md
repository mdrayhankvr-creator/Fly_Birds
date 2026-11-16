# School setup preview (first milestone)

`preview-school-setup` reads the active school's current setup and reports the
work implied by a target of 15 students **total** in each active section and 15
teacher profiles **total**. It has no import, account-creation, settings-edit,
password-generation, migration, or apply mode. It opens one PostgreSQL
`REPEATABLE READ, READ ONLY` transaction. School name is used only as a label.

From `backend/`, with `DATABASE_URL` supplied securely by the operator:

```powershell
go run ./cmd/preview-school-setup
go run ./cmd/preview-school-setup --json
go run ./cmd/preview-school-setup --manifest .\identifiers.json --json
```

Do not place a database URL or credentials in the manifest or commit them.
The optional JSON file contains only proposed identifiers:

```json
{
  "student_admission_numbers": ["TEST-2026-001"],
  "teacher_employee_codes": ["TEACHER-02"],
  "account_emails": ["new-teacher@example.test"]
}
```

Unknown JSON fields, including password fields, and empty manifests are
rejected. The preview
checks case-insensitive duplicates within the candidate lists and conflicts
with existing students, employees and users. Without a manifest it still
checks existing identifiers, but reports that candidate conflicts are unknown.
The candidate list is an identifier check only; it does not promise that an
import is valid or approved.

The report explicitly excludes the existing administrators
`mdrayhankvr@gmail.com` and `2023331031@student.sust.edu` from proposed
changes and flags either account if missing, inactive or no longer an admin.
It never reads or displays their password hashes. This preview's database
transaction cannot change their user rows, role permissions, credentials,
sessions, profile links or any school data. A future apply milestone needs
its own independent admin-protection gate and tests.

The fourth-subject section lists the configured options and counts students
without a choice. All proposed students are unresolved until the school
approves class-specific policies and each student's choice. Gender is never
inferred. Class Ten currently has no optional mappings; class Six currently
marks Religion optional. The routine section reserves a 45-minute lunch
requirement but cannot schedule it without an approved start time. It also
lists other missing timetable inputs. The report never proposes a teacher to
account link.

For a disposable integration test, set `AUTH_TEST_DATABASE_URL` to a **new,
local** PostgreSQL database whose name starts with
`school_setup_preview_test_`, then run:

```powershell
go test -p 1 ./internal/schoolsetup ./cmd/preview-school-setup -count=1
```

The integration test refuses other database names or a nonempty database
before migration and leaves its fixture in that disposable database. Never point this test variable at
fbshow or any school database.
