# School setup identifier manifest (second milestone)

From `backend/`, supply `DATABASE_URL` securely and name the active session explicitly:

```powershell
go run ./cmd/preview-school-setup --generate-manifest .\school-setup-2026-identifiers.json --session 2026
go run ./cmd/preview-school-setup --manifest .\school-setup-2026-identifiers.json
```

The first command opens a PostgreSQL `REPEATABLE READ, READ ONLY` transaction, recounts every active section and teacher, checks existing identifiers, then writes a **new** local JSON file. It refuses an existing path, including an owner-approved manifest. The second command recounts the database in a new read-only transaction and rejects a stale or edited generated manifest. Keep the file outside Git until reviewed; it contains no credentials or real identities. Do not use a live database for tests.

The generated schema contains `schema_version: 2`, the session ID/name, a snapshot SHA-256, `student_candidates`, `teacher_candidates`, and a list of roster information still needed. Each student candidate has class/section IDs and display labels, a synthetic `TEST` admission number, and `test_marker: "TEST"`. Teacher candidates have employee codes only. IDs are deterministic: `TEST-<session>-<full section UUID>-<slot>` with the section's existing count subtracted from 15, and unused `TEACHER-01` through `TEACHER-15` codes. Existing teachers count toward 15; `TEACHER-01` is preserved if present. A conflicting generated ID stops generation for review.

The hash covers the session, active section counts, existing teacher codes, and all stored admission and employee identifiers. It is an identifier snapshot hash, not a checksum of branding or other school settings. Preview always rereads fourth-subject mappings and routine gaps independently.

Illustrative structure using invented IDs (the actual file contains one entry per proposed identifier):

```json
{
  "schema_version": 2,
  "session_id": "11111111-1111-1111-1111-111111111111",
  "session": "2026",
  "snapshot_sha256": "<64 lowercase hex characters>",
  "student_candidates": [{
    "session_id": "11111111-1111-1111-1111-111111111111",
    "session": "2026",
    "class_id": "22222222-2222-2222-2222-222222222222",
    "class": "SIX-06",
    "section_id": "33333333-3333-3333-3333-333333333333",
    "section": "A",
    "admission_no": "TEST-2026-33333333333333333333333333333333-01",
    "label": "TEST ONLY - SIX-06 A candidate 01",
    "test_marker": "TEST"
  }],
  "teacher_candidates": [{"employee_code": "TEACHER-02"}],
  "teacher_roster_required": ["teacher name", "designation", "joining date", "subject assignments", "qualifications and availability", "account-link decision"]
}
```

The manifest is **not** an import. It does not add students, teachers, accounts, passwords, guardians, personal details, subject choices, teacher assignments, or routines. It does not change school identity, branding, grading, or settings. Both protected administrator accounts remain outside all candidate lists. Student fourth-subject choices, teacher roster details, and timetable inputs remain unresolved in the preview. A 45-minute lunch duration alone does not define its start time.

For a fresh disposable PostgreSQL integration fixture, use the dedicated `SCHOOL_SETUP_PREVIEW_TEST_DATABASE_URL` documented in [SCHOOL_SETUP_PREVIEW.md](SCHOOL_SETUP_PREVIEW.md). The test refuses an unsafe name or a nonempty database before any migration. Generated manifests and test databases should not be treated as approved admissions or staff records.
