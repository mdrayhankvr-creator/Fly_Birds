# E4 — Teacher Marks Workspace

Status: implemented and verified locally on `feature/e4-premium-marks-workspace`
(2026-10-21). It builds on E3 (mark statuses, sheet workflow, versioned
per-cell writes) and adds no migration.

The design follows [E1_PREMIUM_UX_SPEC.md §5](E1_PREMIUM_UX_SPEC.md) and the
E4 row of [E1_IMPLEMENTATION_ROADMAP.md](E1_IMPLEMENTATION_ROADMAP.md). §2
lists where the implementation differs and why.

## 1. What a teacher gets

- **My exam sheets** (`/teacher/exams`): the work queue.
  - Lists every sheet assigned to the teacher in draft exams and in the
    active session's published exams.
  - Returned sheets come first, then in progress, not started, submitted
    and approved.
  - Tabs: To do · Submitted · Approved · All. Each row shows progress and,
    for a returned sheet, the reason it came back.
- **The sheet** (`/teacher/exams/:examId/sheets/:sheetId`):
  - **Context header:** subject, sheet status, exam, session, class and
    section, full and pass marks, the save state, a progress meter with
    absent, incomplete, exempt and withheld counts, and the permitted
    action.
  - **Grid** (640 px and wider):
    - columns Roll · Student · Marks/full · % · Status;
    - the header and the first two columns are pinned;
    - the focused row is highlighted.
  - **List** (below 640 px): one card per student, or **One at a time**, a
    single large input with Previous / Next.
  - **Validation per cell:** the message shows under the cell and is
    announced. One bad cell never blocks the others.
  - **Autosave:** see §4. Nothing has a Save button.
  - **Paste and CSV import:** a preview first, then a batch save (see §5).
  - **History:** a side panel with this sheet's audit events (who, when,
    old → new).
  - **Submit for review:** enabled only when the sheet is complete and fully
    saved. When disabled, the reason is written under the button
    ("42 students still need marks or a status", "Waiting for changes to
    save", "Resolve the conflicts first").
- **Head teacher and administrator**
  (`/admin/exams/:examId/sheets/:sheetId`, also linked as "Open sheet" from
  the exam page's sheets panel):
  - the same view;
  - Approve / Return (with a reason) on a submitted sheet;
  - read-only marks for the head teacher;
  - the administrator edits draft sheets and alone sets Exempt and Withheld.

### Keyboard map (also shown in the page with `?`)

| Key | Action |
|---|---|
| Enter / Shift+Enter, ↓ / ↑ | Next / previous student |
| Tab / Shift+Tab | Next / previous field (marks → status → next row) |
| Esc | Undo this cell's unsaved change |
| A · I · P (in a marks cell) | Absent · Incomplete · Present |
| Ctrl+V (in a marks cell) | Paste a column or a block from a spreadsheet |
| ? | Show or hide the shortcuts |

Typing accepts Bangla digits (৪৫ → 45), a comma as the decimal mark (`32,5`)
and status words (`ab`, `absent`, `অনুপস্থিত`, `inc`).

## 2. Differences from the E1 specification

| E1 spec | E4 | Why |
|---|---|---|
| Routes `/exams/marks` and `/exams/marks/:sheetId` | `/teacher/exams/:examId/sheets/:sheetId` and `/admin/exams/:examId/sheets/:sheetId` | The app's routes are role-prefixed, and E3 sheets belong to an exam. |
| APIs `GET /exams/sheets?mine=1`, `GET/PATCH /exams/sheets/:id…` | `GET /exams/my-sheets`, `GET /exams[/assigned]/:id/sheets/:sheetId[/history]`; saving uses E3's `PATCH /exams/:id/marks` | They reuse E3's tested write path and its teacher/admin scoping. |
| Component columns (CQ, MCQ, Total) and a Preview grade column | One marks column and a percentage | Components, blueprints and weighting are E5 and excluded from E4. |
| Unsaved recovery in `localStorage` for up to 7 days | `sessionStorage`, this tab only (§6) | The E4 brief forbids insecure persistent storage of student records without a security review. |
| Autosave after 800 ms idle | 600 ms idle, or at once on paste | A typed column is saved within about a second of the last key, still in 1–2 requests. |
| Filters All · Missing · Invalid · Absent · Changed since submission | All · Missing · Needs attention (errors, conflicts, incomplete) | Fewer, clearer choices. "Changed since submission" needs result versions (E8). |
| "Mark selected: Absent" bulk action | Not built; A/I/P keys per row | Single-key status entry covers the measured flows. |
| Ctrl+Z, Delete, Ctrl+F | Native browser behaviour (field undo, clear, find in page) | No custom handling needed. |
| Rows virtualise above 60 students | Not virtualised; rows are memoised and only one layout renders | At 56 rows, keystroke handling stays at p95 18–24 ms. Virtualisation would break the browser's find and screen-reader table navigation. |
| Typing latency under 16 ms per keystroke | Handler p95 18–24 ms, max ≤ 38 ms; 95 % of keystrokes painted within 88–112 ms | Measured on one laptop (§7). The 16 ms target is not met on this machine. |
| Sticky bottom bar on mobile one-student mode | Previous / Next directly under the input | The input and the buttons stay within one screen at 390 px. |
| Mobile: paste unavailable | Paste dialog available everywhere (a text box), and CSV import | Teachers on tablets paste from Sheets. |
| Submit notifies the head teacher | Not built | E3 added no notification. It is a small follow-up. |

## 3. Architecture

**Backend** (`internal/exams/workspace.go`, no migration):

- `MySheets(actor)`: one query for the queue, with per-sheet progress counts,
  limited to 300 rows.
- `Workspace(examID, sheetID, actor, role, scope)`:
  - returns the exam, the sheet, its rows (active students, with mark,
    status, reason and `row_version`) and the **permissions**;
  - a teacher who is not assigned gets 403; staff asking for an unknown
    sheet get 404.
- `permissionsFor(examStatus, sheetStatus, role)`, computed on the server:
  - `can_edit`, `can_submit`, `can_review`;
  - the statuses this role may set: the teacher gets present, absent and
    incomplete; the administrator also gets exempt and withheld;
  - a `read_only_reason` code: `exam_published`, `exam_locked`,
    `sheet_submitted`, `sheet_approved` or `review_only`.

  The client only displays these. The write path enforces them again.
- `SheetHistory`: up to 200 audit events for the sheet (`exam.mark_saved`
  for its subject, plus its submit, return and approve events), newest
  first. It checks access through `Workspace` first.
- **One write-path change:** `SaveMarkEntries` refuses with
  `status_not_allowed` when anyone but the administrator changes a mark the
  administrator made exempt or withheld. Before, a teacher could overwrite it
  with a number.
- **Routes:**
  - `GET /exams/my-sheets` needs `exams:enter_marks` and the teacher role;
  - `GET /exams/assigned/:id/sheets/:sheetId[/history]` needs
    `exams:enter_marks`, scoped to the teacher's assignments;
  - `GET /exams/:id/sheets/:sheetId[/history]` needs `exams:read`;
  - all are documented in `docs/openapi.yaml`.

**Frontend** (`frontend/src/pages/marks/`):

| File | Role |
|---|---|
| `marksInput.ts` | Pure parsing: `parseCell`, `parseTable` (TSV or CSV), `planPaste`. Unit-tested with `npm run test:unit`. |
| `useSheetAutosave.ts` | Drafts, batching, retries, conflicts, recovery (§4). |
| `MarksWorkspace.tsx` | The sheet: header, toolbar, grid and list layouts, paste dialog, history panel. |
| `MarksQueue.tsx` | My exam sheets. |
| `examUi.tsx` | Shared `SheetChip` and `Progress`. |
| `../../marksTranslations.ts` | 133 keys in English and Bangla, with plural forms. |

**Rendering:**

- Rows are memoised on a per-row snapshot of their values. The handlers come
  from one stable context.
- A media query renders only the grid or only the list, so a keystroke or a
  save re-renders only the rows that changed.

## 4. Autosave

- **Drafts:** each edit becomes a draft that carries the `row_version` it was
  based on.
- **Batching:**
  - after 600 ms of quiet (or at once after a paste), every pending draft
    goes in **one** `PATCH /exams/:id/marks`;
  - only one request is in flight at a time;
  - edits made meanwhile wait for the next batch.
- **Per-cell results:**
  - a saved cell takes the new version;
  - a cell error stays on the cell;
  - a `version_conflict` becomes a **conflict**: the cell shows the server's
    value next to the teacher's, with **Use theirs** and **Keep mine**. Keep
    mine re-sends against the new version.

  Nothing is ever written blindly over a newer value.
- **A retried request that had already landed** (the response was lost) is
  recognised when the server's value equals the draft. It is not reported as
  a conflict.
- **Transient failures** (network, 5xx, 408, 429):
  - state **Offline** or **Error**, with the number of kept changes;
  - retries after 2, 4, 8, 15, then every 30 s, and at once when the browser
    reports it is back online.
- **Permanent failures** (403, `exam_not_draft`, sheet locked, or assignment
  removed):
  - saving stops;
  - the sheet reloads and shows why it is read-only.
- **Leaving the page** with unsent changes asks for confirmation
  (`beforeunload`).
- **Submit** is disabled while anything is pending, failing or in conflict.
  Every action button is disabled while its request runs, so a double click
  cannot submit twice.
- **The indicator** (`data-save-state`, announced politely) reads one of:
  Changes save automatically · Saving… · All changes saved · time · Offline ·
  N changes not saved · N conflicts to resolve.

## 5. Paste and import

- **Where it starts:** pasting several cells into a marks cell, or using
  **Paste marks** or **Import CSV**, opens a preview. TSV from Excel or
  Sheets and CSV (quoted, `,` or `;`) are accepted.
- **Mapping:**
  - **By admission number** (the stable student ID), when any line's first
    cell matches one. Line order then does not matter, header lines are
    skipped, and an unknown admission number is rejected.
  - Otherwise **by position**, starting at the focused row, which the dialog
    names ("starting with Ayesha Akter"). Position mode is refused while a
    filter hides rows, because a hidden row would shift every mark.
- **Every line is listed** with its student, value and verdict: apply, skip
  or reject.
  - Rejected lines show the exact reason: not a number, above the full
    marks, negative, too many decimals, unknown admission number, beyond the
    last student, or needs a reason (exempt and withheld).
  - "Set by the administrator" marks a teacher's line for an exempt or
    withheld student.
  - If a student appears twice, **all** their lines are rejected, so an
    ambiguous paste never picks one silently.
- **Applying:**
  - saves the accepted cells in one batch and reports "39 marks saved";
  - lists any failures, which stay highlighted in the sheet.
- **No spreadsheet dependency was added.** The parsing is about 150 lines
  and is unit-tested.

## 6. Security and privacy

- **Recovery storage:**
  - kept in `sessionStorage` under `flybirds.marks.v1.<userId>.<sheetId>`;
  - holds student IDs, statuses, values and base versions only. It holds no
    names, admission numbers or reasons;
  - lasts for that browser tab only;
  - cleared when everything is saved, when the teacher discards the
    recovery, and at sign-out (all keys).

  A shared school computer therefore keeps no marks after the tab closes or
  the teacher signs out. The E2E checks the stored shape.
- **Access:**
  - a teacher reaches only assigned sheets (403 otherwise);
  - students and guardians get 403 on every new route and a denied page in
    the UI;
  - the head teacher's view is read-only;
  - submitted and approved sheets are read-only for teachers;
  - only the administrator can set, change or clear exempt and withheld (now
    also enforced on numeric overwrites).
- **No leaks:** the workspace returns only the one sheet's students. The
  history is filtered to that sheet. Responses and logs carry no reasons or
  marks beyond what the role may see; the client logs nothing.
- **Cache:** the queue, sheet and history queries use `gcTime: 0`, so their
  student data leaves the in-memory cache as soon as the page closes.

## 7. Measurements (one laptop, disposable school, not production)

**Setup:**

- Windows 11 laptop running Docker Desktop and headless Chrome on the same
  machine;
- `compose.prod.yaml` stack of this branch;
- demo seed plus `e2e/marks-workspace-setup.mjs`: 6 sheets for the demo
  teacher, one of 56 students and five of 42, 266 students in all.

These numbers describe this machine only. They are not a production claim.

| Flow | Result |
|---|---|
| Queue → sheet | 1 click; 2 clicks including focusing the first cell |
| 56 marks by keyboard | 158 keystrokes, 3.3–4.9 s of typing, all saved 0.9 s after the last key, **1–2 save requests** |
| Save latency (56 cells in one PATCH) | 109–144 ms |
| Keystroke handling | handler p95 18–24 ms, max 25–38 ms; event p50 40 ms, p95 88–112 ms; single outliers up to 712 ms are paint delay in headless Chrome |
| Paste of 42 lines (shuffled, with 5 bad lines) | preview → saved 0.5 s, **1 request**, 39 applied, 5 rejected with reasons |
| Rapid repeated edits of one cell | 1 request, last value saved |
| Conflict (admin changes a cell under the teacher) | visible conflict with server value; no overwrite |
| Network loss | edits kept, Offline shown, saved on reconnect |
| Reload with an unsaved edit | offered back, restored, saved |
| Request count and error rate | typing: 1 API request, 0 failed; paste: 1 API request, 0 failed |

**Effort compared with the old exam page** (baseline from E1 UX spec §5; E3
already added per-cell errors and version checks to that page):

| | Before (old exam page, E1 audit baseline) | E4 |
|---|---|---|
| Reaching the sheet | 3–4 clicks, subjects interleaved | 1 click from My exam sheets |
| Entering 56 marks | 56 field entries, then Save | 56 entries down one column, no Save |
| From a spreadsheet | retype every mark | 1 paste, with preview |
| Fixing one bad value | find it after the batch error | flagged on its cell, others already saved |
| Losing the tab | edits lost | offered back on reload |
| Two editors | last save won silently (QA-18) | conflict on the cell with both values and a choice |

Concurrency at the database level (four writers on one cell, five teachers at
once) was measured in E3 and is covered by its tests. E4 uses the same write
path.

## 8. Verification (2026-10-21, disposable resources only)

- **Go:**
  - `go vet` clean;
  - `go test ./... -p 1 -count=1` passes on a fresh database, including
    `TestPermissionsFor` (12 cases), `TestPostgresMarksWorkspace` (queue,
    access per role, the exempt guard, submit, review and scoped history)
    and route protection for the 5 new routes;
  - govulncheck reports no reachable vulnerabilities.
- **Frontend:**
  - `npm run build` passes;
  - i18n check passes with 1,997 keys per language;
  - `npm run test:unit` passes (5 tests).
- **Browser, on a freshly provisioned and seeded school:**
  - `e2e/marks-workspace.mjs`: **46/46**;
  - existing `suite.mjs` 106/106, `whitelabel.mjs` 23/23,
    `payment-safety.mjs` (off) 12/12 and E3 `exam-workflow.mjs` 17/17.
- **Layouts:**
  - 1440 × 900 English light, 768 × 1024 English light, 390 × 844 Bangla
    light, 1440 × 900 Bangla dark and 390 × 844 English dark;
  - two school colours (blue `#1d4ed8` and teal `#0f766e`) and amber in
    dark;
  - each combination: no horizontal overflow, measured WCAG AA text
    contrast, and **axe-core with no WCAG 2.1 AA violations**.

  axe-core 4.10.3 is a pinned devDependency (MPL-2.0, test-only). The
  screenshots were reviewed by hand.
- **Screen readers:**
  - the grid is a real `<table>` with column headers and a row header per
    student;
  - checked by the E2E: 6 column headers and one row header per student;
  - every input has a label ("Marks for Ayesha Akter, out of 50");
  - errors are tied to their cell with `aria-describedby`;
  - the save state and paste results are live regions;
  - progress is a `progressbar`.

To repeat the browser run on a disposable school:

```sh
E2E_BASE_URL=http://localhost:8104 DEMO_PASSWORD=... EXAM_NAME="E4 Marks Test" node e2e/marks-workspace-setup.mjs sheets.json
E2E_BASE_URL=http://localhost:8104 DEMO_PASSWORD=... SHEETS=sheets.json node e2e/marks-workspace.mjs
```

## 9. Limitations

- One marks column per subject. Components, CT + final weighting and annual
  averages come with blueprints (E5).
- No virtualisation. Classes much larger than 60 have not been measured.
- Signing out does not clear other pages' cached data; that is app-wide
  behaviour, outside E4 (the marks queries are not cached, see §6).
- The 16 ms per-keystroke target from the E1 spec is not met on the test
  laptop (handler p95 about 18–24 ms).
- Recovery lasts for one tab only. Closing the tab with unsaved changes (after
  the browser's warning) loses them, by design.
- No notification to the head teacher on submit, and no bulk "mark selected"
  action.
- History shows mark and sheet events only. Corrections after approval
  arrive with the correction workflow (E8).
- The head teacher cannot exempt or withhold (unchanged from E3).
