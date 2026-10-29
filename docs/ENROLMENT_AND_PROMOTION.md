# M15 — Enrolment history and year-end promotion

M15 lets a school close an academic year and move its students into the
next one without losing or rewriting anything recorded for the old year.
It closes QA-04 (no promotion or enrolment history) and QA-06 (a section
transfer hid earlier attendance) from the lifecycle audit.

It is delivered in two stages:

| Stage | Scope | Status |
|---|---|---|
| **A — Enrolment history foundation** | Enrolment table and history, migration 1701 with backfill, marks and results tied to the year they belong to, history-safe rosters and reports, history API and export | This branch |
| **B — Promotion workflow and UI** | Class progression, promotion batches (review, approve, apply, revert), repeat/graduate/withdraw/transfer, login handling, per-session concession review, screens | Not started |

## 1. The problem stage A fixes

Before M15, `exam_marks` and `exam_results` referenced the student's
*current* `(session, class, section)`. Once a student had a single mark,
changing their section, class or session failed with a foreign-key error
(23503, shown as "invalid input"). A school could not transfer such a
student, let alone promote a class. Students moved by editing (only
possible without marks) disappeared from their old year's rosters and
reports, and their old class was kept only in the audit log.

`TestPostgresSectionTransferKeepsMarks` reproduces the failure on the
pre-M15 schema and passes on 1701.

## 2. Data model (migration 1701)

**`student_enrolments`**: one row per student per session.

| Column | Meaning |
|---|---|
| `session_id`, `class_id`, `section_id`, `roll_no` | Where the student was that year (the section and roll they ended the year with) |
| `state` | `current` (the `students` row points here) or `closed` (an earlier year). One current row per student. |
| `student_status` | The student's status in that enrolment (the `students.status` values); for a closed year, the status when it closed |
| `outcome` | How a closed year ended: `promoted`, `repeated`, `graduated`, `withdrawn`, `transferred`, or `moved` (moved to another session by editing the student, without a recorded decision). Empty for inferred years. |
| `source` | `admission`, `backfill` (the upgrade), `inferred` (the upgrade, from attendance), `moved`, `promotion` |
| `started_on`, `ended_on` | `ended_on` is set exactly when the year is closed |

Keys: `UNIQUE (student_id, session_id)`; one current row per student;
roll numbers unique per section (sections belong to one session).

**`student_enrolment_events`**: append-only log of what happened to an
enrolment (`enrolled`, `section_changed`, `class_changed`, `roll_changed`,
`status_changed`, `session_corrected`, `closed`) with the acting user.
Rows can never be updated.

**`student_rolls`** (view): the students on a session's roll — current
enrolments of active students and closed years the student completed
(`promoted`, `repeated`, `graduated`, `moved`). Withdrawn and transferred
years and inferred years are history only. Before any promotion this is
exactly "students placed here today who are active", so existing behaviour
is unchanged.

### `students` stays the current placement

`students.session_id/class_id/section_id/roll_no/status` are unchanged and
still read by every existing query and API. Triggers keep the current
enrolment in step, so every writer (admission, CSV import, the demo seed,
`school-provision`, tests) records history with no code change:

- **Admission** opens a current enrolment.
- **Same session:** class, section, roll and status are copied to the
  current enrolment and each change is logged. A **class** change is
  refused (409 `enrolment_history`) while the student has marks or results
  in another class that year. A **section** change is allowed and keeps
  every mark, result and attendance row in the section where it was
  recorded.
- **Another session:** if nothing is recorded for the student in the old
  year, the enrolment is corrected in place (a wrong session at admission).
  Otherwise the old year is closed with outcome `moved` and a new current
  enrolment opens. Moving back into a year the student already has is
  refused (409).
- A commit-time check refuses any change that leaves the current enrolment
  different from the `students` row.

"Recorded in a session" means attendance, marks, results, result versions,
fee dues, optional-subject choices or homework submissions.

### Marks, results and attendance

- `exam_marks` and `exam_results` reference `student_enrolments(student_id,
  session_id)`. A new mark or result must match the class of that year's
  enrolment and its section or a section the student moved from. Rows
  already stored are never re-checked, so a later move cannot invalidate
  them.
- `student_attendance` references the enrolment of its session (rows are
  deleted with the student, as before).
- Deleting a student who has marks is still refused, as before.

`student_takes_subject()` now decides whether a class uses per-student
optional subjects from that session's enrolments, not from where students
are today, so old results are not recalculated with the wrong subjects.

## 3. History-safe reads

Each query that answers "who was in this section/class that year" now reads
enrolments (`student_rolls`) instead of `students`:

| Area | Queries |
|---|---|
| Exams | Marks roster, workspace grid, sheet counts and readiness, which students may receive marks, publish guard, assessment/term/annual result batches, optional-subject rules, blueprint choices and setup counts |
| Roll numbers | Tabulation, corrections, result snapshots and grids show the roll of that year |
| Attendance | Daily roster and monthly report of a section (QA-06: a student moved to another section keeps their earlier days in the old section's report) |
| Fees | Dues generation for a session bills the students on that session's roll; fee reports label the section of that session |
| Analytics | Attendance report per student, session comparison and class counts per session |
| Diary | A teacher's homework roster of an old diary |

Deliberately unchanged (they are about *today*): the student directory,
routines, notices, absence SMS, ID cards, at-risk alerts, and the
student's and guardian's diary feed (documented rule: the current section).

## 4. Correctness after a move or promotion

| Record | Why it stays correct |
|---|---|
| Marks | Keep their own session, class and section and point at the old year's enrolment; never updated by a move |
| Published results (E6) | Result versions are immutable and snapshot the student, rules, inputs and outputs. The student's identity (roll) is not part of the hashed inputs. Report cards read versions by student. |
| Attendance | Rows keep their section; section reports read enrolments and the rows themselves; the student's own report is by date across all years |
| Fees | Dues keep their session and class; old dues stay owed on the old session |
| Guardians | Linked by student, not by year |
| Student login | The account is linked to the student, not the year; history is readable by the student and linked guardians |

## 5. API

`GET /students/{id}/enrolments` — the student's years, oldest first, each
with its changes. Same record scope as `GET /students/{id}`: the
administrator and head teacher (`students:history_read`), the linked
student, linked guardians. Who made a change is shown to staff only.

`PATCH /students/{id}` returns `409 enrolment_history` for refused moves.

The admin export adds the `student_enrolments` dataset.

## 6. Upgrade

Migration 1701 is additive. It records each student's current placement as
their current enrolment, and each earlier session proven by attendance (a
student moved by editing before the upgrade) as a closed `inferred`
enrolment with no outcome. It writes one `student.enrolments_migrated`
audit event with the counts. No existing row changes. Rollback is the
pre-upgrade backup (DEPLOYMENT.md §8).

Verified on a disposable demo school built with `main` (9b68bf7), plus a
student moved from 2025 by editing and a 2026 section transfer, both with
attendance:

- of the 75 pre-existing tables, 72 are identical row for row; the other
  three only gained the expected rows: `audit_events` (+1), `permissions`
  (+1) and `role_permissions` (+2);
- 38 of 38 current enrolments match their students; one inferred 2025
  enrolment (November attendance, section "Old A");
- the 2026 roll equals the 38 active students, as before;
- `exam-verify`: 6 published exams, 36 results, 108 marks, no differences;
- migration time about 1 s.

## 7. Stage B (planned, not implemented)

Decisions recorded on 2026-10-29:

1. Automatic recommendations require a published annual result. Missing or
   withheld results block automatic promotion; an authorized person decides
   manually with a reason.
2. Failed students are recommended to repeat. An authorized override needs
   a recorded reason; there is no invented pass/fail threshold.
3. Withdrawal or transfer disables the student's login and keeps all
   history. Graduates keep read-only access to their academic records.
4. Fee concessions do not carry into a new session automatically; they are
   reviewed for the new session.

Shared frontend files (`App.tsx`, `StudentProfilePage.tsx`,
`StudentsPage.tsx`, `i18n.ts`, `styles.css`) are coordinated with Joy
before stage B touches them.
