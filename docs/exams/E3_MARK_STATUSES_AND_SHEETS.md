# E3 — Mark statuses and the sheet workflow

E3 implements the third milestone of
[E1_IMPLEMENTATION_ROADMAP.md](E1_IMPLEMENTATION_ROADMAP.md#3-milestones):

> Absent / exempt / incomplete / withheld; sheet status (submit, return,
> approve); `exams:review` for the head teacher; optimistic concurrency
> (`row_version`); per-cell errors; set-based mark writes and per-sheet locks
> instead of the global lock. Migration 1501. QA-07 and QA-18 closed; E1-11;
> existing sheets migrate without value changes.

**Scope note.** Term and session grouping, assessment blueprints and fewer
per-section setup steps are **E5** in the approved roadmap
(terms, blueprints, setup wizard). They are not part of E3.

## 1. Concepts

- **Sheet:** one `exam_subjects` row, which is one subject of one exam, and an exam belongs to one section. No new table was needed.
- **Mark status:** each mark (`exam_marks`) is one of:
  - **present:** marks required;
  - **absent:** no marks;
  - **exempt:** no marks; administrators only, with a reason;
  - **incomplete:** no marks;
  - **withheld:** marks optional; administrators only, with a reason.
- **Sheet status:**

```text
not_started → in_progress → submitted ─┬─► approved
                    ▲                  └─► returned (reason) ─┐
                    └──────────────────────────────────────────┘
publication: every sheet → approved        unpublishing: every sheet → in_progress
```

- **Who does what:**
  - teachers write their assigned subjects' sheets while the sheet is not started, in progress or returned, and submit a complete sheet;
  - the head teacher (`exams:review`, new) approves a submitted sheet or returns it with a reason;
  - administrators can do everything, including exempt and withheld.

## 2. Migration 1501

**`exam_marks`** gains:
- `status` (default `present`);
- `status_reason` (required for exempt and withheld);
- `row_version` (≥ 1).

`marks` becomes nullable. A check constraint ties marks to the status: present needs marks; absent, exempt and incomplete have none; withheld may keep its marks.

**`exam_subjects`** gains:
- `sheet_status`;
- `submitted_by/at`, `reviewed_by/at`;
- `return_reason`.

Constraints require who and when for submitted sheets, a review time for approved and returned sheets, and a reason for returned sheets.

**Backfill, with no value changes:**
- sheets of published and locked exams become `approved`, with `reviewed_at` set to the publication time;
- sheets of draft exams become `in_progress` if any mark exists, otherwise `not_started`;
- every mark becomes `present` with version 1.

Draft sheets are deliberately not marked approved: that would lock teachers out of exams still in progress.

**Also:**
- `exams:review` is granted to admin and head_teacher;
- the audit event `exam.sheets_migrated` records the counts.

## 3. API

All new routes are documented in `docs/openapi.yaml`.

| Method | Path | Guard | Purpose |
|---|---|---|---|
| PATCH | `/exams/:id/marks` | `exams:enter_marks` | Save up to 500 cells, each with a status and its `row_version`. Valid cells are saved; invalid ones come back in `errors` with a code. |
| GET | `/exams/:id/sheets` | `exams:read` | Sheets with progress counts, review state and publication readiness (blockers) |
| GET | `/exams/assigned/:id/sheets` | `exams:enter_marks`, teacher | The teacher's own sheets |
| POST | `/exams/:id/sheets/:sheetId/submit` | `exams:enter_marks` (assigned teacher or admin) | Submit a complete sheet |
| POST | `/exams/:id/sheets/:sheetId/approve` | `exams:review`, admin or head teacher | Approve a submitted sheet |
| POST | `/exams/:id/sheets/:sheetId/return` | `exams:review`, admin or head teacher | Return with a reason (1–240 characters) |

**Cell error codes:**
- `above_full_marks`, `invalid_marks`, `marks_required`, `marks_not_allowed`;
- `invalid_status`, `status_not_allowed`, `reason_required`;
- `version_conflict` (with the current value);
- `sheet_locked`, `not_assigned`, `not_in_section`, `unknown_sheet`.

**Whole-request errors** are specific now. They are still 409 or 400, and still satisfy `ErrConflict` / `ErrInvalid` internally:
- `exam_not_draft`;
- `sheet_state`;
- `sheet_incomplete`;
- `exam_sheets_pending`;
- `exam_marks_incomplete`;
- `all_subjects_exempt`.

**Roster and report cards** gain `status`, `status_reason` and `row_version` (additive JSON). On cards, students and guardians get the status but not the staff reason or version. Cards gain `withheld`.

## 4. Publication with statuses

Publication uses `exams.ScoreLines`, which wraps the E2 engine's status calculation:
- **absent** scores 0 and fails the subject; F is stored, exactly as a typed 0 was graded before E3, but now labelled;
- **exempt** leaves the subject out of the total, the full marks and the GPA denominator, and no grade is stored;
- **incomplete** blocks publication (`exam_marks_incomplete`);
- **withheld** stores no grades and no result for that student, so the student is not ranked. The student and guardian see the exam as "Result withheld — contact the school office", with no marks.

Publication is also refused while any sheet is `submitted` (awaiting review) or `returned` (`exam_sheets_pending`). Sheets that never entered review can be published directly by the administrator, as before E3.

## 5. Concurrency and performance

- **Optimistic concurrency.** Every cell carries `row_version`. A write must send the version it read (0 for an empty cell). A stale write returns `version_conflict` with the current value and changes nothing (closes QA-18). Four simultaneous writers with the same version: exactly one is saved.
- **Locks:**
  - mark writes lock the exam row `FOR SHARE` (blocking a concurrent publication) and only their own sheet rows `FOR UPDATE`, in id order;
  - structure and state changes lock the exam row `FOR UPDATE`;
  - the school-wide advisory lock is no longer taken by mark writes (E1-11).
- **Set-based writes:** one upsert statement for all accepted cells (up to 500) and one statement for the audit events (`audit.RecordMany`), instead of about five statements per cell.

**Measured** with the same test file on the same machine and database (`TestPostgresMarksSavePerformance`, opt-in with `E3_PERF=1`; 100 students × 5 subjects):

| | Before (`main` `0b0188ca`) | E3 | Faster |
|---|---|---|---|
| One 500-mark save | 8.11–8.20 s | 0.50–0.56 s | ~16× |
| Five teachers saving a 100-mark sheet at once | 8.13–8.54 s | 0.18–0.31 s | ~35× |

## 6. Compatibility

- **`PUT /exams/:id/marks` is unchanged:** present marks only, all or nothing, no version check. It now runs on the new write path. It is refused only for a sheet that is submitted or approved (`sheet_state`).
- **Unchanged areas:** existing exam, verifier, analytics, demo and HTTP tests pass unchanged. That includes unpublish → correct → republish (unpublishing reopens the sheets).
- **Analytics:**
  - subject statistics leave out exempt and withheld marks, and count an absence as 0;
  - the student view never reads a NULL mark.
- **`exam-verify`:** grades results with statuses through the same scoring as publication.

## 7. User interface (existing exam page, no new screen)

The full marks workspace is E4. E3 adds only what makes the workflow usable:
- **Sheets panel:**
  - one row per subject with a status chip in the exam palette of the UX spec (dashed slate, sky, amber, rose, emerald);
  - progress ("6 of 6 entered") and who submitted or reviewed;
  - the return reason;
  - Submit, Approve and Return, the last with an inline reason.
- **Publish:** disabled while blockers remain, which are listed by subject.
- **Marks rows:**
  - a status selector (exempt and withheld for administrators) and a reason field when needed;
  - saving sends only the changed cells with their versions;
  - refused cells are highlighted with the reason in place, and the rest are saved.
- **Head teacher:** sees sheets and marks read-only, can review, and cannot enter marks.
- **Report card:** shows Absent and Exempt in place of marks. A withheld result shows only the message. The student results list shows a "Withheld" chip instead of a GPA.

All new text is in English and Bangla (52 keys).

## 8. Verification (2026-10-21, disposable resources only)

**Go:**
- `go vet` clean; govulncheck: no vulnerabilities;
- all 24 test packages pass on a fresh PostgreSQL 17 database.

**New tests:**
- `TestPostgresSheetWorkflowAndStatuses` covers:
  - every cell error code and version conflicts;
  - teacher, head-teacher and admin rules;
  - submit, return and approve;
  - publication blocked by pending sheets and incomplete marks;
  - absent, exempt and withheld results with exact GPAs;
  - withheld and student views;
  - `exam-verify` with statuses;
  - unpublish/republish;
  - the old PUT contract;
  - the audit count.
- `TestPostgresConcurrentMarkWrites`: same cell, same version; and parallel sheets.
- `TestScoreLinesStatuses`.
- `TestPostgresExamWorkflowRouteProtection`: students and guardians are refused everywhere; teachers can't review or use the oversight view; head teachers can't enter marks; anonymous requests get 401.

**Upgrade of a disposable school** (`e3-school`, started on `main`, seeded, with one partly marked draft and one empty draft):
- after upgrading in place, fingerprints of exams, subjects, marks, results and routines are identical;
- 18 published sheets → approved; 1 → in_progress; 3 → not_started;
- 111 marks present at version 1;
- the audit event was recorded;
- `exam-verify` found no differences.

**`e2e/exam-workflow.mjs`: 17/17** on that school (API and browser). It covers the full workflow, the withheld card at 390 px with no horizontal overflow, and the head-teacher view.

**Existing suites** on that school: browser 106/106, white-label 23/23, payment safety (off) 12/12. The frontend build passes, and the translation check passes (1,864 keys per language).

## 9. Limitations

- **Head teachers cannot exempt.** Exempt and withheld are administrator-only. The blueprint spec also allows head teachers to exempt; that waits for the E4 workspace.
- **Review is optional.** A sheet that never enters review can still be published directly. A school-wide "approval required" switch belongs to the publication centre (E6).
- **No component marks yet.** `exam_component_marks` comes with blueprints (E5).
- **Teacher assignments are still per class,** not per section.
- **Corrections leave no result history.** A correction still means unpublish, correct and republish. Result versions are E6.
- **The exam page is not yet the workspace.** It is still a list, not the spreadsheet grid. Paste, autosave and recovery come with the E4 workspace.
